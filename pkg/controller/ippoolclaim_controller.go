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
	"errors"
	"fmt"
	"hash/fnv"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/iiiceoo/iprange"
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

var (
	errInsufficientIPBlocks = errors.New("IP blocks are insufficient")
)

var (
	zero = big.NewInt(0)
	one  = big.NewInt(1)
)

func NewIPPoolClaimReconciler(c client.Client) *ippoolClaimReconciler {
	return &ippoolClaimReconciler{
		client: c,
	}
}

type ippoolClaimReconciler struct {
	client client.Client
}

func (r *ippoolClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.IPPoolClaim{}).
		Complete(r)
}

var _ reconcile.Reconciler = &ippoolClaimReconciler{}

func (r *ippoolClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var rpc requeueipv1.IPPoolClaim
	if err := r.client.Get(ctx, req.NamespacedName, &rpc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// The workload has been deleted, do nothing, OwnerReference will ensure
	// that the relevant IPPool is recycled.
	if !rpc.DeletionTimestamp.IsZero() {
		// TODO(iiiceoo): Owner terminating or no longer exists.
		return ctrl.Result{}, nil
	}

	exist := true
	var rp requeueipv1.IPPool
	if err := r.client.Get(ctx, req.NamespacedName, &rp); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		exist = false
	}

	// The workload has changed the Subnets it expects to assign IP addresses to.
	if exist && !slices.Contains(rpc.Spec.Subnets, rp.Labels[consts.LabelRefSubnet]) {
		if err := r.client.Delete(ctx, &rp); err != nil {
			return ctrl.Result{}, err
		}
		exist = false
	}

	// To ensure that IPBlocks are always correctly recycled, it is necessary
	// to create an empty IPPool with LabelRefSubnet set before claiming IPBlocks.
	if !exist {
		var subnet string
		for _, s := range rpc.Spec.Subnets {
			var rn requeueipv1.Subnet
			if err := r.client.Get(ctx, types.NamespacedName{Name: s}, &rn); err != nil {
				if apierrors.IsNotFound(err) {
					// TODO(iiiceoo): Log.
					continue
				}
				return ctrl.Result{}, err
			}
			if rn.Status.Count == nil {
				return ctrl.Result{Requeue: true}, nil
			}

			// The number of available IP addresses for a Subnet can be very large.
			// Avoid using strconv.Atoi().
			fc := new(big.Int)
			fc.SetString(rn.Status.Count.Free, 10)
			if fc.Cmp(big.NewInt(int64(rpc.Spec.Replicas))) >= 0 {
				subnet = s
				break
			}
		}
		if subnet == "" {
			return ctrl.Result{}, fmt.Errorf("no Subnets are available in %s: %w", rpc.Spec.Subnets, errInsufficientIPBlocks)
		}

		newRP := &requeueipv1.IPPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rpc.Name,
				Namespace: rpc.Namespace,
				Labels:    map[string]string{consts.LabelRefSubnet: subnet},
			},
			Spec: requeueipv1.IPPoolSpec{
				Version: net.IPv4,
				Ranges:  []string{},
			},
		}
		controllerutil.AddFinalizer(newRP, consts.RFinalizer)
		if err := controllerutil.SetControllerReference(&rpc, newRP, r.client.Scheme()); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.client.Create(ctx, newRP); err != nil {
			return ctrl.Result{}, err
		}
		rp = *newRP
	}

	// Do not use .status.count.all as it may not have been set correctly when
	// IPPool first created.
	ranges, err := iprange.Parse(rp.Spec.Ranges...)
	if err != nil {
		return ctrl.Result{}, err
	}

	// A workload must not have more than int replicas. For convenience, convert
	// IPPool size to int.
	poolSize := int(ranges.Size().Int64())
	replicas := int(rpc.Spec.Replicas)
	if poolSize == replicas {
		return ctrl.Result{}, nil
	}

	// TODO(iiiceo): IPPool does not have label LabelRefSubnet.
	var rn requeueipv1.Subnet
	if err := r.client.Get(ctx, types.NamespacedName{Name: rp.Labels[consts.LabelRefSubnet]}, &rn); err != nil {
		return ctrl.Result{}, err
	}

	bStep, err := net.CountFromMaskSize(*rn.Spec.Version, int(*rn.Spec.BlockSize))
	if err != nil {
		return ctrl.Result{}, err
	}
	step := int(bStep.Int64())

	var rbList requeueipv1.IPBlockList
	if err := r.client.List(
		ctx,
		&rbList,
		client.MatchingLabels{
			consts.LabelRefNamespace: rp.Namespace,
			consts.LabelRefIPPool:    rp.Name,
		},
	); err != nil {
		return ctrl.Result{}, err
	}
	ipCount := len(rbList.Items) * step

	// Scale down IPPools.
	if replicas < poolSize {
		if rp.Status.Count == nil {
			return ctrl.Result{Requeue: true}, nil
		}

		// Wait for the replicas of workload to converge before scaling down IPPool.
		uc, err := strconv.Atoi(rp.Status.Count.Used)
		if err != nil {
			return ctrl.Result{}, err
		}
		if uc != replicas {
			// The default DeletionGracePeriodSeconds for Pod is 30 seconds.
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		rp.Spec.Ranges = rp.Status.Free
		if err := r.client.Update(ctx, &rp); err != nil {
			return ctrl.Result{}, err
		}

		// Normal redundancy, no need to recycle IPBlocks.
		if ipCount-replicas < step {
			return ctrl.Result{}, nil
		}

		fr, err := iprange.Parse(rp.Status.Free...)
		if err != nil {
			return ctrl.Result{}, err
		}

		// TODO(iiiceoo): Goroutine.
		for _, rb := range rbList.Items {
			ipNet, err := net.NameToCIDR(rp.Spec.Version, rb.Name)
			if err != nil {
				return ctrl.Result{}, err
			}
			br, err := iprange.Parse(ipNet.String())
			if err != nil {
				return ctrl.Result{}, err
			}
			if br.Diff(fr).Size().Sign() > 0 {
				continue
			}
			if err := r.client.Delete(ctx, &rb); err != nil {
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}
		}
	}

	// Scale up IPPools.
	if replicas > poolSize {
		if replicas <= ipCount {
			delta, err := parseRangesFromBlocks(*rn.Spec.Version, rbList.Items)
			if err != nil {
				return ctrl.Result{}, err
			}

			// TODO(iiiceoo): Error.
			// The version of ranges may be Unknown when init an empty IPPool. Never
			// never use ranges.Union(delta).
			delta.Diff(ranges).Slice(zero, big.NewInt(int64(replicas-poolSize-1)))
			rp.Spec.Ranges = delta.Union(ranges).Strings()
			return ctrl.Result{}, r.client.Update(ctx, &rp)
		}

		if rn.Status.Count == nil {
			return ctrl.Result{Requeue: true}, nil
		}

		dv := replicas - ipCount
		expect := dv / step
		if dv%step != 0 {
			expect++
		}

		// The number of available IP addresses for a Subnet can be very large.
		// Avoid using strconv.Atoi().
		fc := new(big.Int)
		fc.SetString(rn.Status.Count.Free, 10)
		if fc.Cmp(big.NewInt(int64(expect*step))) < 0 {
			// TODO(iiiceoo): Deadlock.
			return ctrl.Result{}, fmt.Errorf(
				"unable to scale up IPPool %s in Subnet %s: %w",
				rp.Name,
				rn.Name,
				errInsufficientIPBlocks,
			)
		}
		bc := new(big.Int).Div(fc, bStep)

		free, err := iprange.Parse(rn.Status.Free...)
		if err != nil {
			return ctrl.Result{}, err
		}
		bi := free.BlockIterator(bStep)

		// TODO(iiiceoo): Goroutine.
		complete := true
		h := fnv.New32a()
		for i := 1; i <= expect; i++ {
			h.Reset()
			id := fmt.Sprintf("%s-%d", rp.Name, len(rbList.Items)+i)
			h.Write([]byte(id))
			n := new(big.Int).Mod(big.NewInt(int64(h.Sum32())), bc)
			n.Add(n, one)

			blockStr := bi.NextN(n).String()
			rb := &requeueipv1.IPBlock{
				ObjectMeta: metav1.ObjectMeta{
					Name: net.CIDRStringToName(blockStr),
					Labels: map[string]string{
						consts.LabelRefSubnet:    rn.Name,
						consts.LabelRefNamespace: rp.Namespace,
						consts.LabelRefIPPool:    rp.Name,
					},
				},
			}
			if err := r.client.Create(ctx, rb); err != nil {
				if !apierrors.IsAlreadyExists(err) {
					return ctrl.Result{}, err
				}

				block := bi.Next()
				if block == nil {
					bi.Reset()
					block = bi.Next()
				}
				rb.SetName(net.CIDRStringToName(block.String()))
				if err := r.client.Create(ctx, rb); err != nil {
					if !apierrors.IsAlreadyExists(err) {
						return ctrl.Result{}, err
					}

					// TODO(iiiceoo): Log.
					complete = false
					continue
				}
			}
			rbList.Items = append(rbList.Items, *rb)
		}
		if !complete {
			return ctrl.Result{Requeue: true}, nil
		}

		ranges, err := parseRangesFromBlocks(*rn.Spec.Version, rbList.Items)
		if err != nil {
			return ctrl.Result{}, err
		}

		// TODO(iiiceoo): if !exist --> Patch.
		rp.Spec.Ranges = ranges.Slice(zero, big.NewInt(int64(rpc.Spec.Replicas-1))).Strings()
		if err := r.client.Update(ctx, &rp); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func parseArray(arrStr string) []string {
	if arrStr == "" {
		return nil
	}

	var res []string
	parts := strings.Split(arrStr, ",")
	for _, p := range parts {
		p = strings.Trim(p, " ")
		if p != "" {
			res = append(res, p)
		}
	}

	return res
}
