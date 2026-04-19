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
	"math/big"
	"net"
	"reflect"
	"strconv"

	"github.com/iiiceoo/iprange"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/metrics"
	rnet "github.com/iiiceoo/requeueip/pkg/net"
)

func NewSubnetReconciler(c client.Client) *subnetReconciler {
	return &subnetReconciler{
		client: c,
	}
}

type subnetReconciler struct {
	client client.Client
}

func (r *subnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.Subnet{}).
		Watches(
			&requeueipv1.IPBlock{},
			handler.EnqueueRequestsFromMapFunc(mapFuncForSubnet),
		).
		Watches(
			&requeueipv1.IPPool{},
			handler.EnqueueRequestsFromMapFunc(mapFuncForSubnet),
			builder.WithPredicates(IPPoolDeletedPredicate{}),
		).
		Complete(r)
}

var mapFuncForSubnet = func(ctx context.Context, o client.Object) []reconcile.Request {
	labels := o.GetLabels()
	if labels == nil {
		return nil
	}

	v, ok := labels[consts.LabelRefSubnet]
	if !ok {
		return nil
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: v}}}
}

type IPPoolDeletedPredicate struct {
	predicate.Funcs
}

func (IPPoolDeletedPredicate) Create(event.CreateEvent) bool {
	return false
}

func (IPPoolDeletedPredicate) Delete(event.DeleteEvent) bool {
	return true
}

func (IPPoolDeletedPredicate) Update(event.UpdateEvent) bool {
	return false
}

func (IPPoolDeletedPredicate) Generic(event.GenericEvent) bool {
	return false
}

var _ reconcile.Reconciler = &subnetReconciler{}

func (r *subnetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var rn requeueipv1.Subnet
	if err := r.client.Get(ctx, req.NamespacedName, &rn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rn.DeletionTimestamp.IsZero() {
		cleaned, err := r.cleanUpSubnet(ctx, &rn)
		if err != nil {
			return ctrl.Result{}, err
		}
		if cleaned {
			return ctrl.Result{}, nil
		}
	}

	var rbList requeueipv1.IPBlockList
	if err := r.client.List(
		ctx,
		&rbList,
		client.MatchingLabels{consts.LabelRefSubnet: rn.Name},
		client.UnsafeDisableDeepCopy,
	); err != nil {
		return ctrl.Result{}, err
	}

	// Calculate the total, used, and available IP range of the Subnet.
	total, err := getTotalIPRanges(&rn)
	if err != nil {
		return ctrl.Result{}, err
	}
	used, err := parseRangesFromIPBlocks(*rn.Spec.Version, rbList.Items)
	if err != nil {
		return ctrl.Result{}, err
	}
	free := total.DeepCopy().Diff(used)

	step, err := rnet.CountFromMaskSize(*rn.Spec.Version, int(*rn.Spec.BlockSize))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update Subnet status if its current status has changed.
	totalCount := new(big.Int).Div(total.Size(), step)
	usedCount := new(big.Int).Div(used.Size(), step)
	blockSize := strconv.Itoa(int(*rn.Spec.BlockSize))
	status := &requeueipv1.SubnetStatus{
		Free: free.Strings(),
		BlockCount: &requeueipv1.BlockCount{
			Total: totalCount.String(),
			Used:  usedCount.String(),
			Free:  new(big.Int).Div(free.Size(), step).String(),
		},
	}

	if reflect.DeepEqual(status, &rn.Status) {
		metrics.SubnetBlockTotal(rn.Name, *rn.Spec.Version, rn.Spec.CIDR, blockSize).Set(float64(totalCount.Int64()))
		metrics.SubnetBlockUsage(rn.Name, *rn.Spec.Version, rn.Spec.CIDR, blockSize).Set(float64(usedCount.Int64()))
		return ctrl.Result{}, nil
	}

	old := rn.DeepCopy()
	rn.Status = *status
	if err := r.client.Status().Patch(ctx, &rn, client.MergeFrom(old)); err != nil {
		return ctrl.Result{}, err
	}

	metrics.SubnetBlockTotal(rn.Name, *rn.Spec.Version, rn.Spec.CIDR, blockSize).Set(float64(totalCount.Int64()))
	metrics.SubnetBlockUsage(rn.Name, *rn.Spec.Version, rn.Spec.CIDR, blockSize).Set(float64(usedCount.Int64()))
	return ctrl.Result{}, nil
}

// cleanUpSubnet removes Subnet's finalizer when it is not referenced by any
// IPPools.
func (r *subnetReconciler) cleanUpSubnet(ctx context.Context, subnet *requeueipv1.Subnet) (cleaned bool, err error) {
	if subnet.Status.BlockCount == nil {
		return false, nil
	}

	// The number of IPBlocks used in the cluster will not exceed the int limit.
	used, err := strconv.Atoi(subnet.Status.BlockCount.Used)
	if err != nil {
		return false, err
	}
	if used > 0 {
		return false, nil
	}

	// Wait until all referenced IPPools disappear before removing the Subnet
	// finalizer. IPPool deletion may lag behind IPBlock deletion, so IPBlock
	// events alone are not sufficient to reliably drive this cleanup. For this
	// reason, the controller also watches IPPool delete events.
	var rpList requeueipv1.IPPoolList
	if err := r.client.List(
		ctx,
		&rpList,
		client.MatchingLabels{consts.LabelRefSubnet: subnet.Name},
		client.Limit(1),
		client.UnsafeDisableDeepCopy,
	); err != nil {
		return false, err
	}

	if len(rpList.Items) != 0 {
		return false, nil
	}

	controllerutil.RemoveFinalizer(subnet, consts.RFinalizer)
	if err := r.client.Update(ctx, subnet); err != nil {
		return false, err
	}

	metrics.DeleteSubnet(subnet.Name)
	return true, nil
}

// getTotalIPRanges returns the total IP ranges after excluding the IPBlocks
// that need to be reserved.
func getTotalIPRanges(subnet *requeueipv1.Subnet) (*iprange.IPRanges, error) {
	total, err := iprange.Parse(subnet.Spec.CIDR)
	if err != nil {
		return nil, err
	}

	if len(subnet.Spec.Excluded) == 0 {
		return total, nil
	}

	excluded, err := iprange.Parse(subnet.Spec.Excluded...)
	if err != nil {
		return nil, err
	}

	// Excluded IP ranges do not overlap with Subnet CIDR.
	if total.DeepCopy().Intersect(excluded).Size().Sign() == 0 {
		return total, nil
	}

	var cidrs []string
	iter := excluded.CIDRIterator()
	for {
		cidr := iter.Next()
		if cidr == nil {
			break
		}

		bits := 32
		if *subnet.Spec.Version == rnet.IPv6 {
			bits = 128
		}

		// Allocation is performed in IPBlock-sized units. If an excluded CIDR
		// is smaller than one IPBlock, round it down to the containing IPBlock
		// so the whole allocation unit is reserved.
		ones, _ := cidr.Mask.Size()
		if ones > int(*subnet.Spec.BlockSize) {
			cidr.Mask = net.CIDRMask(int(*subnet.Spec.BlockSize), bits)
			cidr.IP = cidr.IP.Mask(cidr.Mask)
		}

		// Re-parse the normalized CIDRs as IPRanges before subtracting them
		// from the Subnet's total ranges.
		cidrs = append(cidrs, cidr.String())
	}

	blocked, err := iprange.Parse(cidrs...)
	if err != nil {
		return nil, err
	}

	return total.Diff(blocked), nil
}
