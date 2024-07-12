/*
Copyright 2024 The RequeueIP Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/big"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/iiiceoo/iprange"
	str2duration "github.com/xhit/go-str2duration/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

func NewIPPoolClaimReconciler(c client.Client, reader client.Reader, recorder record.EventRecorder) *claimReconciler {
	return &claimReconciler{
		client:   c,
		reader:   reader,
		recorder: recorder,
	}
}

type claimReconciler struct {
	client   client.Client
	reader   client.Reader
	recorder record.EventRecorder
}

func (r *claimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.IPPoolClaim{}).
		Complete(r)
}

var _ reconcile.Reconciler = &claimReconciler{}

func (r *claimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var claim requeueipv1.IPPoolClaim
	if err := r.client.Get(ctx, req.NamespacedName, &claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Remove the auto-created IPPoolClaim when the owner workload does not exist or
	// is terminating.
	if !claim.DeletionTimestamp.IsZero() {
		metadata, err := r.getOwnerMetadata(ctx, &claim)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get owner workload: %v", err)
		}

		if metadata == nil || !metadata.DeletionTimestamp.IsZero() {
			if controllerutil.RemoveFinalizer(&claim, consts.RFinalizer) {
				return ctrl.Result{}, r.client.Update(ctx, &claim)
			}
			return ctrl.Result{}, nil
		}
	}

	// To ensure that IPBlocks are always correctly recycled, it is necessary to
	// create an empty IPPool before creating IPBlocks.
	subnet, pool, err := r.getOrMarkIPPool(ctx, &claim)
	if err != nil {
		return ignoreRequeue(fmt.Errorf("failed to get or mark IPPool: %w", err))
	}

	alloc, err := getIPBlockAllocation(subnet, pool)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get IPBlock allocation: %v", err)
	}

	if err := r.scale(ctx, alloc, &claim); err != nil {
		return ignoreRequeue(err)
	}

	// Update IPPoolClaim status if its current status has changed.
	status, err := getIPPoolClaimStatus(alloc, &claim)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get IPPoolClaim status: %v", err)
	}

	if reflect.DeepEqual(status, &claim.Status) {
		return ctrl.Result{}, nil
	}
	old := claim.DeepCopy()
	claim.Status = *status

	return ctrl.Result{}, r.client.Status().Patch(ctx, &claim, client.MergeFrom(old))
}

// getOwnerMetadata gets the metadata of owner resource. It is commonly used to
// determine whether the owner is alive.
func (r *claimReconciler) getOwnerMetadata(ctx context.Context, object client.Object) (*metav1.ObjectMeta, error) {
	ref := metav1.GetControllerOf(object)
	if ref == nil {
		return nil, nil
	}

	metadata := &metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
		},
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: object.GetNamespace(),
		Name:      ref.Name,
	}, metadata); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	return &metadata.ObjectMeta, nil
}

// getOrMarkIPPool gets the corresponding IPPool based on IPPoolClaim. If IPPool
// does not exist, an empty one will be created.
func (r *claimReconciler) getOrMarkIPPool(
	ctx context.Context,
	claim *requeueipv1.IPPoolClaim,
) (*requeueipv1.Subnet, *requeueipv1.IPPool, error) {
	// Do not use IPPool in the cache as it may cause meaningless conflicts when
	// updating IPPool later. In fact, when IPPool is not cached, it can be patched
	// instead of updated later.
	exist := true
	var rp requeueipv1.IPPool
	if err := r.reader.Get(ctx, types.NamespacedName{
		Namespace: claim.Namespace,
		Name:      claim.Name,
	}, &rp); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, nil, err
		}
		exist = false
	}

	// TODO(iiiceoo): If the subnets of IPPoolClaim changes.
	if exist {
		var rn requeueipv1.Subnet
		if err := r.client.Get(ctx, types.NamespacedName{Name: rp.Spec.Subnet}, &rn); err != nil {
			return nil, nil, err
		}
		return &rn, &rp, nil
	}

	subnet, err := r.selectSubnet(ctx, claim)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to select Subnet from candidates: %w", err)
	}

	labels := map[string]string{
		consts.LabelIPVersion: claim.Spec.Version,
		consts.LabelRefSubnet: subnet.Name,
	}

	// Only auto-created IPPoolClaim requires setting the workload UID label, it
	// will be used in the subsequent retrieval of auto-created IPPool.
	workload := metav1.GetControllerOf(claim)
	if workload != nil {
		labels[consts.LabelRefWorkloadUID] = string(workload.UID)
	}

	newRP := &requeueipv1.IPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
			Labels:    labels,
		},
		Spec: requeueipv1.IPPoolSpec{
			Version: claim.Spec.Version,
			Subnet:  subnet.Name,
			Ranges:  []string{},
		},
	}
	controllerutil.AddFinalizer(newRP, consts.RFinalizer)
	if err := controllerutil.SetControllerReference(claim, newRP, r.client.Scheme()); err != nil {
		return nil, nil, err
	}
	if err := r.client.Create(ctx, newRP); err != nil {
		return nil, nil, err
	}

	return subnet, newRP, nil
}

// selectSubnet returns the first Subnet with sufficient IPBlocks.
func (r *claimReconciler) selectSubnet(ctx context.Context, claim *requeueipv1.IPPoolClaim) (*requeueipv1.Subnet, error) {
	for _, s := range claim.Spec.Subnets {
		var rn requeueipv1.Subnet
		if err := r.client.Get(ctx, types.NamespacedName{Name: s}, &rn); err != nil {
			if apierrors.IsNotFound(err) {
				msg := fmt.Sprintf("Candidate Subnet %s does not exist", s)
				r.recorder.Eventf(claim, corev1.EventTypeWarning, "SubnetNotFound", msg)
				continue
			}
			return nil, err
		}

		if *rn.Spec.Version != claim.Spec.Version {
			msg := fmt.Sprintf("Candidate Subnet %s is %s but claimed as %s", rn.Name, *rn.Spec.Version, claim.Spec.Version)
			r.recorder.Eventf(claim, corev1.EventTypeWarning, "SubnetVersionMismatch", msg)
			continue
		}

		// Do not skip the Subnet that is not ready, but try again later, respecting the
		// order of candidate Subnets as much as possible.
		if rn.Status.BlockCount == nil {
			return nil, newErrorRequeue()
		}

		step, err := net.CountFromMaskSize(*rn.Spec.Version, int(*rn.Spec.BlockSize))
		if err != nil {
			return nil, err
		}

		count := new(big.Int)
		count.SetString(rn.Status.BlockCount.Free, 10)
		count.Mul(count, step)
		if count.Cmp(big.NewInt(int64(claim.Spec.Replicas))) >= 0 {
			return &rn, nil
		}
	}

	return nil, fmt.Errorf("no Subnets are available in %s: invalid Subnet or %w", claim.Spec.Subnets, errInsufficientIPBlocks)
}

// scale scales the size of the IPPool up or down to workload replicas.
func (r *claimReconciler) scale(ctx context.Context, alloc *ipBlockAllocation, claim *requeueipv1.IPPoolClaim) error {
	replicas := int(claim.Spec.Replicas)
	if replicas == alloc.poolSize {
		return nil
	}

	if replicas > alloc.poolSize {
		if err := r.scaleUp(ctx, alloc, replicas); err != nil {
			return fmt.Errorf(
				"failed to scale up the size of IPPool to %d from Subnet %s: %w",
				replicas,
				alloc.subnet.Name,
				err,
			)
		}
		return nil
	}

	if *claim.Spec.ScaleDownDelay != "0" {
		// No wasted IPBlocks.
		next := claim.Status.NextScaleDownTime
		if next == nil {
			return nil
		}

		// The workload was rescaled up so that No wasted IPBlocks.
		if alloc.poolSize-replicas < alloc.step {
			return nil
		}

		now := time.Now()
		if now.Before(next.Time) {
			return newErrorRequeueAfter(next.DeepCopy().Sub(now))
		}
	}

	if err := r.scaleDown(ctx, alloc, replicas); err != nil {
		return fmt.Errorf(
			"failed to scale down the size of IPPool to %d from Subnet %s: %w",
			replicas,
			alloc.subnet.Name,
			err,
		)
	}

	return nil
}

// A set of parameters commonly required in a scale process.
type ipBlockAllocation struct {
	// IPPool to be scaled up/down.
	pool *requeueipv1.IPPool

	// The total IP ranges of the IPPool.
	poolRanges *iprange.IPRanges

	// The current size of the IPPool.
	poolSize int

	// Subnet to which IPPool belongs.
	subnet *requeueipv1.Subnet

	// The currently available IP ranges of the Subnet.
	freeRanges *iprange.IPRanges

	// The count of currently available IPBlocks in the Subnet.
	freeBlockCount *big.Int

	// The size(int) of a single IPBlock in the Subnet.
	step int

	// The size(*big.Int) of a single IPBlock in the Subnet.
	bStep *big.Int
}

// getIPBlockAllocation gets allocaion of IPBlock for IPPool scaling up/down.
func getIPBlockAllocation(subnet *requeueipv1.Subnet, pool *requeueipv1.IPPool) (*ipBlockAllocation, error) {
	// Do not use the count status of IPPool as it may not have been set when IPPool
	// first created.
	ranges, err := iprange.Parse(pool.Spec.Ranges...)
	if err != nil {
		return nil, err
	}

	free, err := iprange.Parse(subnet.Status.Free...)
	if err != nil {
		return nil, err
	}
	count := new(big.Int)
	count.SetString(subnet.Status.BlockCount.Free, 10)

	step, err := net.CountFromMaskSize(*subnet.Spec.Version, int(*subnet.Spec.BlockSize))
	if err != nil {
		return nil, err
	}

	return &ipBlockAllocation{
		pool:           pool,
		poolRanges:     ranges,
		poolSize:       int(ranges.Size().Int64()),
		subnet:         subnet,
		freeRanges:     free,
		freeBlockCount: count,
		step:           int(step.Int64()),
		bStep:          step,
	}, nil
}

// scaleDown scales down IPPool and releases unused IPBlocks.
func (r *claimReconciler) scaleDown(ctx context.Context, alloc *ipBlockAllocation, replicas int) error {
	if alloc.pool.Status.Count == nil {
		return newErrorRequeue()
	}

	used, err := strconv.Atoi(alloc.pool.Status.Count.Used)
	if err != nil {
		return err
	}

	// Wait for the replica of workload to converge before scaling down IPPool.
	// The default DeletionGracePeriodSeconds for Pod is 30 seconds.
	if used != replicas {
		return newErrorRequeueAfter(5 * time.Second)
	}

	// Do not get the free IPRanges of IPPool in getIPBlockAllocation, as the status
	// of IPPool may not be ready yet.
	free, err := iprange.Parse(alloc.pool.Status.Free...)
	if err != nil {
		return err
	}

	ranges := alloc.poolRanges.Diff(free)
	alloc.pool.Spec.Ranges = ranges.Strings()
	if err := r.client.Update(ctx, alloc.pool); err != nil {
		return err
	}

	if ranges.Size().Sign() == 0 {
		return r.client.DeleteAllOf(
			ctx,
			&requeueipv1.IPBlock{},
			client.MatchingLabels{
				consts.LabelRefNamespace: alloc.pool.Namespace,
				consts.LabelRefIPPool:    alloc.pool.Name,
			},
		)
	}

	// Never get IPBlocks from the cache.
	var rbList requeueipv1.IPBlockList
	if err := r.reader.List(
		ctx,
		&rbList,
		client.MatchingLabels{
			consts.LabelRefNamespace: alloc.pool.Namespace,
			consts.LabelRefIPPool:    alloc.pool.Name,
		},
		client.UnsafeDisableDeepCopy,
	); err != nil {
		return err
	}

	ipTotal := len(rbList.Items) * alloc.step
	if ipTotal-replicas < alloc.step {
		return nil
	}

	if err := r.releaseIPBlocks(ctx, alloc.pool, rbList.Items); err != nil {
		return fmt.Errorf("failed to release IPBlocks: %v", err)
	}

	return nil
}

// scaleUp claims IPBlocks and scales up IPPool.
func (r *claimReconciler) scaleUp(ctx context.Context, alloc *ipBlockAllocation, replicas int) error {
	// Never get IPBlocks from the cache.
	var rbList requeueipv1.IPBlockList
	if err := r.reader.List(
		ctx,
		&rbList,
		client.MatchingLabels{
			consts.LabelRefNamespace: alloc.pool.Namespace,
			consts.LabelRefIPPool:    alloc.pool.Name,
		},
		client.UnsafeDisableDeepCopy,
	); err != nil {
		return err
	}

	ipTotal := len(rbList.Items) * alloc.step
	if replicas <= ipTotal {
		if err := r.scaleUpWithinIPBlocks(ctx, alloc, replicas, rbList.Items); err != nil {
			return err
		}
		if ipTotal-replicas >= alloc.step {
			return r.releaseIPBlocks(ctx, alloc.pool, rbList.Items)
		}
		return nil
	}

	delta := replicas - ipTotal
	expect := delta / alloc.step
	if delta%alloc.step != 0 {
		expect++
	}

	if alloc.freeBlockCount.Cmp(big.NewInt(int64(expect))) < 0 {
		return errInsufficientIPBlocks
	}

	blocks, err := r.claimIPBlocks(ctx, alloc, len(rbList.Items), expect)
	if err != nil {
		return err
	}

	return r.scaleUpWithinIPBlocks(ctx, alloc, replicas, append(rbList.Items, blocks...))
}

// scaleUpWithinIPBlocks scales up IPPool based on existing IPBlocks.
func (r *claimReconciler) scaleUpWithinIPBlocks(
	ctx context.Context,
	alloc *ipBlockAllocation,
	replicas int,
	blocks []requeueipv1.IPBlock,
) error {
	br, err := parseRangesFromIPBlocks(alloc.pool.Spec.Version, blocks)
	if err != nil {
		return err
	}

	delta := int64(replicas) - alloc.poolRanges.Size().Int64()
	dr := br.Diff(alloc.poolRanges).Slice(consts.BigInt[0], big.NewInt(delta-1))

	// The version of alloc.poolRanges may be Unknown when init an empty IPPool.
	// Never never use alloc.poolRanges.Union(dr).
	old := alloc.pool.DeepCopy()
	alloc.pool.Spec.Ranges = dr.Union(alloc.poolRanges).Strings()

	// No other component attempts to update the IPPool spec, using patch to avoid
	// conflicts.
	return r.client.Patch(ctx, alloc.pool, client.MergeFrom(old))
}

// claimIPBlocks claims the expected number of IPBlocks.
func (r *claimReconciler) claimIPBlocks(
	ctx context.Context,
	alloc *ipBlockAllocation,
	start, expect int,
) ([]requeueipv1.IPBlock, error) {
	h := fnv.New32a()
	var wg sync.WaitGroup
	errCh := make(chan error, expect)
	blockCh := make(chan *requeueipv1.IPBlock, expect)
	for i := 1; i <= expect; i++ {
		id := fmt.Sprintf("%s-%d", alloc.pool.Name, start+i)
		h.Write([]byte(id))
		hash := h.Sum32()
		h.Reset()

		index := new(big.Int).Mod(big.NewInt(int64(hash)), alloc.freeBlockCount)
		index.Add(index, consts.BigInt[1])

		wg.Add(1)
		go func() {
			defer wg.Done()
			block, err := r.claimIPBlock(ctx, alloc, index)
			if err != nil {
				errCh <- err
				return
			}
			blockCh <- block
		}()
	}

	wg.Wait()
	close(errCh)
	close(blockCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		err := utilerrors.NewAggregate(errs)
		if apierrors.IsAlreadyExists(err) {
			return nil, newErrorRequeue()
		}
		return nil, err
	}

	blocks := make([]requeueipv1.IPBlock, 0, expect)
	for block := range blockCh {
		blocks = append(blocks, *block)
	}

	return blocks, nil
}

// claimIPBlock claims IPBlock with specified index that is available under the
// IPRanges of the Subnet.
func (r *claimReconciler) claimIPBlock(
	ctx context.Context,
	alloc *ipBlockAllocation,
	index *big.Int,
) (*requeueipv1.IPBlock, error) {
	iter := alloc.freeRanges.BlockIterator(alloc.bStep)
	block := iter.NextN(index)
	rb := &requeueipv1.IPBlock{
		ObjectMeta: metav1.ObjectMeta{
			Name: net.CIDRStringToName(block.String()),
			Labels: map[string]string{
				consts.LabelRefSubnet:    alloc.pool.Spec.Subnet,
				consts.LabelRefNamespace: alloc.pool.Namespace,
				consts.LabelRefIPPool:    alloc.pool.Name,
			},
		},
	}
	if err := r.client.Create(ctx, rb); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, err
		}

		// Linear probing.
		block = iter.Next()
		if block == nil {
			iter.Reset()
			block = iter.Next()
		}

		rb.Name = net.CIDRStringToName(block.String())
		if err := r.client.Create(ctx, rb); err != nil {
			return nil, err
		}
	}

	return rb, nil
}

// releaseIPBlocks releases unused IPBlocks.
func (r *claimReconciler) releaseIPBlocks(ctx context.Context, pool *requeueipv1.IPPool, blocks []requeueipv1.IPBlock) error {
	ranges, err := iprange.Parse(pool.Spec.Ranges...)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(blocks))
	for i := 0; i < len(blocks); i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ipNet, err := net.NameToCIDR(pool.Spec.Version, blocks[i].Name)
			if err != nil {
				errCh <- err
				return
			}
			br, err := iprange.Parse(ipNet.String())
			if err != nil {
				errCh <- err
				return
			}
			if br.Intersect(ranges).Size().Sign() == 0 {
				errCh <- r.client.Delete(ctx, &blocks[i])
			}
		}()
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	return nil
}

// getLastScaleDownTime gets the current status of IPPoolClaim.
func getIPPoolClaimStatus(alloc *ipBlockAllocation, claim *requeueipv1.IPPoolClaim) (*requeueipv1.IPPoolClaimStatus, error) {
	ranges, err := iprange.Parse(alloc.pool.Spec.Ranges...)
	if err != nil {
		return nil, err
	}

	poolSize := ranges.Size().Int64()
	next, err := getNextScaleDownTime(claim, int(poolSize), alloc.step)
	if err != nil {
		return nil, err
	}

	return &requeueipv1.IPPoolClaimStatus{
		Subnet:            &alloc.pool.Spec.Subnet,
		PoolSize:          ptr.To(int32(poolSize)),
		NextScaleDownTime: next,
	}, nil
}

// getNextScaleDownTime gets the next time for scaling down.
func getNextScaleDownTime(claim *requeueipv1.IPPoolClaim, poolSize, step int) (*metav1.Time, error) {
	// Delay scaling down not enabled.
	if *claim.Spec.ScaleDownDelay == "0" {
		return nil, nil
	}

	// No wasted IPBlocks or IPPool has just completed scaling up/down.
	if poolSize-int(claim.Spec.Replicas) < step {
		return nil, nil
	}

	// Do not re-update to now, this may lead to malicious delayed scaling down.
	if claim.Status.NextScaleDownTime != nil {
		return claim.Status.NextScaleDownTime, nil
	}

	delay, err := str2duration.ParseDuration(*claim.Spec.ScaleDownDelay)
	if err != nil {
		return nil, err
	}
	next := metav1.NewTime(metav1.Now().Add(delay))

	return &next, nil
}
