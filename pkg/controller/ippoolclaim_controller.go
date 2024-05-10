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
	"strconv"
	"sync"
	"time"

	"github.com/iiiceoo/iprange"
	"golang.org/x/sync/errgroup"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

func NewIPPoolClaimReconciler(c client.Client, reader client.Reader) *ippoolClaimReconciler {
	return &ippoolClaimReconciler{
		client: c,
		reader: reader,
	}
}

type ippoolClaimReconciler struct {
	client client.Client
	reader client.Reader
}

func (r *ippoolClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.IPPoolClaim{}).
		Complete(r)
}

var _ reconcile.Reconciler = &ippoolClaimReconciler{}

func (r *ippoolClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var claim requeueipv1.IPPoolClaim
	if err := r.client.Get(ctx, req.NamespacedName, &claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !claim.DeletionTimestamp.IsZero() {
		metadata, err := r.getOwnerMetadata(ctx, &claim)
		if err != nil {
			return ctrl.Result{}, err
		}

		if metadata == nil || !metadata.DeletionTimestamp.IsZero() {
			controllerutil.RemoveFinalizer(&claim, consts.RFinalizer)
			return ctrl.Result{}, r.client.Update(ctx, &claim)
		}
	}

	// To ensure that IPBlocks are always correctly recycled, it is necessary
	// to create an empty IPPool with LabelRefSubnet set before claiming IPBlocks.
	pool, err := r.getOrMarkIPPool(ctx, &claim)
	if err != nil {
		return ignoreRequeue(err)
	}

	if err := r.scale(ctx, pool, int(claim.Spec.Replicas)); err != nil {
		return ignoreRequeue(err)
	}

	return ctrl.Result{}, nil
}

func (r *ippoolClaimReconciler) getOwnerMetadata(ctx context.Context, object client.Object) (*metav1.ObjectMeta, error) {
	ref := metav1.GetControllerOf(object)
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

func (r *ippoolClaimReconciler) getOrMarkIPPool(ctx context.Context, claim *requeueipv1.IPPoolClaim) (*requeueipv1.IPPool, error) {
	exist := true
	var rp requeueipv1.IPPool
	if err := r.reader.Get(ctx, types.NamespacedName{
		Namespace: claim.Namespace,
		Name:      claim.Name,
	}, &rp); err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		exist = false
	}

	// The workload has changed the Subnets it expects to assign IP addresses to.
	if exist {
		if slices.Contains(claim.Spec.Subnets, rp.Spec.Subnet) {
			return &rp, nil
		}
		if err := r.client.Delete(ctx, &rp); err != nil {
			return nil, err
		}
	}

	subnet, err := r.selectSubnet(ctx, claim)
	if err != nil {
		return nil, err
	}

	newRP := &requeueipv1.IPPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claim.Name,
			Namespace: claim.Namespace,
			Labels:    map[string]string{consts.LabelRefSubnet: subnet.Name},
		},
		Spec: requeueipv1.IPPoolSpec{
			Version: net.IPv4,
			Subnet:  subnet.Name,
			Ranges:  []string{},
		},
	}
	controllerutil.AddFinalizer(newRP, consts.RFinalizer)
	if err := controllerutil.SetControllerReference(claim, newRP, r.client.Scheme()); err != nil {
		return nil, err
	}
	if err := r.client.Create(ctx, newRP); err != nil {
		return nil, err
	}

	return newRP, nil
}

func (r *ippoolClaimReconciler) selectSubnet(ctx context.Context, claim *requeueipv1.IPPoolClaim) (*requeueipv1.Subnet, error) {
	for _, s := range claim.Spec.Subnets {
		var rn requeueipv1.Subnet
		if err := r.client.Get(ctx, types.NamespacedName{Name: s}, &rn); err != nil {
			if apierrors.IsNotFound(err) {
				// TODO(iiiceoo): Log.
				continue
			}
			return nil, err
		}
		if rn.Status.Count == nil {
			return nil, newErrorRequeue()
		}

		// The number of available IP addresses for a Subnet can be very large.
		// Avoid using strconv.Atoi().
		fc := new(big.Int)
		fc.SetString(rn.Status.Count.Free, 10)
		if fc.Cmp(big.NewInt(int64(claim.Spec.Replicas))) >= 0 {
			return &rn, nil
		}
	}

	return nil, fmt.Errorf("no Subnets are available in %s: %w", claim.Spec.Subnets, errorInsufficientIPBlocks)
}

func (r *ippoolClaimReconciler) scale(ctx context.Context, pool *requeueipv1.IPPool, replicas int) error {
	var rn requeueipv1.Subnet
	if err := r.client.Get(ctx, types.NamespacedName{Name: pool.Spec.Subnet}, &rn); err != nil {
		return err
	}
	if rn.Status.Count == nil {
		return newErrorRequeue()
	}

	// Do not use .status.count.all as it may not have been set correctly when
	// IPPool first created.
	ranges, err := iprange.Parse(pool.Spec.Ranges...)
	if err != nil {
		return err
	}

	// The replicas for the workload cannot exceed the maximum value of int.
	poolSize := int(ranges.Size().Int64())
	if replicas < poolSize {
		return r.scaleDown(ctx, &rn, pool, replicas)
	}
	if replicas > poolSize {
		return r.scaleUp(ctx, &rn, pool, replicas)
	}

	return nil
}

func (r *ippoolClaimReconciler) scaleDown(
	ctx context.Context,
	subnet *requeueipv1.Subnet,
	pool *requeueipv1.IPPool,
	replicas int,
) error {
	if pool.Status.Count == nil {
		return newErrorRequeue()
	}

	uc, err := strconv.Atoi(pool.Status.Count.Used)
	if err != nil {
		return err
	}

	// Wait for the replicas of workload to converge before scaling down IPPool.
	// The default DeletionGracePeriodSeconds for Pod is 30 seconds.
	if uc != replicas {
		return newErrorRequeueAfter(10 * time.Second)
	}

	pool.Spec.Ranges = pool.Status.Free
	if err := r.client.Update(ctx, pool); err != nil {
		return err
	}

	var rbList requeueipv1.IPBlockList
	if err := r.reader.List(
		ctx,
		&rbList,
		client.MatchingLabels{
			consts.LabelRefNamespace: pool.Namespace,
			consts.LabelRefIPPool:    pool.Name,
		},
	); err != nil {
		return err
	}

	bs, err := net.CountFromMaskSize(pool.Spec.Version, int(*subnet.Spec.BlockSize))
	if err != nil {
		return err
	}

	step := int(bs.Int64())
	ipTotal := len(rbList.Items) * step
	if ipTotal-replicas >= step {
		return r.recycleIPBlocks(ctx, pool, rbList.Items)
	}

	return nil
}

func (r *ippoolClaimReconciler) scaleUp(
	ctx context.Context,
	subnet *requeueipv1.Subnet,
	pool *requeueipv1.IPPool,
	replicas int,
) error {
	var rbList requeueipv1.IPBlockList
	if err := r.reader.List(
		ctx,
		&rbList,
		client.MatchingLabels{
			consts.LabelRefNamespace: pool.Namespace,
			consts.LabelRefIPPool:    pool.Name,
		},
	); err != nil {
		return err
	}

	bStep, err := net.CountFromMaskSize(pool.Spec.Version, int(*subnet.Spec.BlockSize))
	if err != nil {
		return err
	}

	step := int(bStep.Int64())
	ipTotal := len(rbList.Items) * step
	if replicas <= ipTotal {
		if err := r.scaleUpWithinExistingIPBlocks(ctx, pool, rbList.Items, replicas); err != nil {
			return err
		}
		if ipTotal-replicas >= step {
			return r.recycleIPBlocks(ctx, pool, rbList.Items)
		}
		return nil
	}

	delta := replicas - ipTotal
	expect := delta / step
	if delta%step != 0 {
		expect++
	}

	fc := new(big.Int)
	fc.SetString(subnet.Status.Count.Free, 10)
	if fc.Cmp(big.NewInt(int64(expect*step))) < 0 {
		return fmt.Errorf("unable to scale up IPPool %s in Subnet %s: %w", pool.Name, subnet.Name, errorInsufficientIPBlocks)
	}

	fbc := new(big.Int).Div(fc, bStep)
	free, err := iprange.Parse(subnet.Status.Free...)
	if err != nil {
		return err
	}

	blocks, err := r.claimIPBlocks(ctx, pool, free, len(rbList.Items), expect, fbc, bStep)
	if err != nil {
		return err
	}
	rbList.Items = append(rbList.Items, blocks...)

	return r.scaleUpWithinNewIPBlocks(ctx, pool, rbList.Items, replicas)
}

func (r *ippoolClaimReconciler) scaleUpWithinExistingIPBlocks(
	ctx context.Context,
	pool *requeueipv1.IPPool,
	blocks []requeueipv1.IPBlock,
	replicas int,
) error {
	ranges, err := iprange.Parse(pool.Spec.Ranges...)
	if err != nil {
		return err
	}

	br, err := parseRangesFromIPBlocks(pool.Spec.Version, blocks)
	if err != nil {
		return err
	}

	// The version of ranges may be Unknown when init an empty IPPool. Never
	// never use ranges.Union(dr).
	delta := int64(replicas) - ranges.Size().Int64()
	dr := br.Diff(ranges).Slice(zero, big.NewInt(delta-1))

	old := pool.DeepCopy()
	pool.Spec.Ranges = dr.Union(ranges).Strings()

	return r.client.Patch(ctx, pool, client.MergeFrom(old))
}

func (r *ippoolClaimReconciler) claimIPBlocks(
	ctx context.Context,
	pool *requeueipv1.IPPool,
	free *iprange.IPRanges,
	start, expect int,
	total, step *big.Int,
) ([]requeueipv1.IPBlock, error) {
	var wg sync.WaitGroup
	h := fnv.New32a()
	errCh := make(chan error, expect)
	blockCh := make(chan *requeueipv1.IPBlock, expect)
	for i := 1; i <= expect; i++ {
		id := fmt.Sprintf("%s-%d", pool.Name, start+i)
		h.Write([]byte(id))
		hash := h.Sum32()
		h.Reset()

		index := new(big.Int).Mod(big.NewInt(int64(hash)), total)
		index.Add(index, one)

		wg.Add(1)
		go func() {
			defer wg.Done()
			block, err := r.claimIPBlock(ctx, pool, free, step, index)
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

	for err := range errCh {
		if err != nil {
			return nil, newErrorRequeue()
		}
	}

	blocks := make([]requeueipv1.IPBlock, 0, expect)
	for block := range blockCh {
		blocks = append(blocks, *block)
	}

	return blocks, nil
}

func (r *ippoolClaimReconciler) claimIPBlock(
	ctx context.Context,
	pool *requeueipv1.IPPool,
	free *iprange.IPRanges,
	step, index *big.Int,
) (*requeueipv1.IPBlock, error) {
	iter := free.BlockIterator(step)
	block := iter.NextN(index)
	rb := &requeueipv1.IPBlock{
		ObjectMeta: metav1.ObjectMeta{
			Name: net.CIDRStringToName(block.String()),
			Labels: map[string]string{
				consts.LabelRefSubnet:    pool.Spec.Subnet,
				consts.LabelRefNamespace: pool.Namespace,
				consts.LabelRefIPPool:    pool.Name,
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
		rb.SetName(net.CIDRStringToName(block.String()))
		if err := r.client.Create(ctx, rb); err != nil {
			// TODO(iiiceoo): Log.
			return nil, err
		}
	}

	return rb, nil
}

func (r *ippoolClaimReconciler) scaleUpWithinNewIPBlocks(
	ctx context.Context,
	pool *requeueipv1.IPPool,
	blocks []requeueipv1.IPBlock,
	replicas int,
) error {
	br, err := parseRangesFromIPBlocks(pool.Spec.Version, blocks)
	if err != nil {
		return err
	}

	ranges := br.Slice(zero, big.NewInt(int64(replicas-1))).Merge()
	old := pool.DeepCopy()
	pool.Spec.Ranges = ranges.Strings()

	return r.client.Patch(ctx, pool, client.MergeFrom(old))
}

func (r *ippoolClaimReconciler) recycleIPBlocks(ctx context.Context, pool *requeueipv1.IPPool, blocks []requeueipv1.IPBlock) error {
	ranges, err := iprange.Parse(pool.Spec.Ranges...)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	for i := 0; i < len(blocks); i++ {
		i := i
		eg.Go(func() error {
			ipNet, err := net.NameToCIDR(pool.Spec.Version, blocks[i].Name)
			if err != nil {
				return err
			}
			br, err := iprange.Parse(ipNet.String())
			if err != nil {
				return err
			}
			if br.Intersect(ranges).Size().Sign() == 0 {
				return r.client.Delete(ctx, &blocks[i])
			}
			return nil
		})
	}

	return eg.Wait()
}
