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
	"reflect"
	"strconv"

	"github.com/iiiceoo/iprange"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/metrics"
)

func NewIPPoolReconciler(c client.Client) *poolReconciler {
	return &poolReconciler{
		client: c,
	}
}

type poolReconciler struct {
	client client.Client
}

func (r *poolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.IPPool{}).
		Watches(&requeueipv1.IP{}, handler.EnqueueRequestsFromMapFunc(mapFuncForIPPool)).
		Complete(r)
}

var mapFuncForIPPool = func(ctx context.Context, o client.Object) []reconcile.Request {
	labels := o.GetLabels()
	if labels == nil {
		return nil
	}

	v, ok := labels[consts.LabelRefIPPool]
	if !ok {
		return nil
	}

	return []reconcile.Request{{NamespacedName: types.NamespacedName{
		Namespace: o.GetNamespace(),
		Name:      v,
	}}}
}

var _ reconcile.Reconciler = &poolReconciler{}

func (r *poolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var rp requeueipv1.IPPool
	if err := r.client.Get(ctx, req.NamespacedName, &rp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !rp.DeletionTimestamp.IsZero() {
		ok, err := r.cleanUpIPool(ctx, &rp)
		if err != nil {
			return ctrl.Result{}, err
		}
		if ok {
			return ctrl.Result{}, nil
		}
	}

	var riList requeueipv1.IPList
	if err := r.client.List(
		ctx,
		&riList,
		client.MatchingLabels{consts.LabelRefIPPool: rp.Name},
		client.InNamespace(rp.Namespace),
		client.UnsafeDisableDeepCopy,
	); err != nil {
		return ctrl.Result{}, err
	}

	// Calculate the total, used, and available range of the IPPool.
	total, err := iprange.Parse(rp.Spec.Ranges...)
	if err != nil {
		return ctrl.Result{}, err
	}
	used, err := parseRangesFromIPs(rp.Spec.Version, riList.Items)
	if err != nil {
		return ctrl.Result{}, err
	}
	free := total.DeepCopy().Diff(used)

	// Update IPPool status if its current status has changed.
	totalSize := total.Size()
	usedSize := used.Size()
	status := &requeueipv1.IPPoolStatus{
		Free: free.Strings(),
		Count: &requeueipv1.Count{
			Total: totalSize.String(),
			Used:  usedSize.String(),
			Free:  free.Size().String(),
		},
	}

	if reflect.DeepEqual(status, &rp.Status) {
		return ctrl.Result{}, nil
	}

	old := rp.DeepCopy()
	rp.Status = *status
	if err := r.client.Status().Patch(ctx, &rp, client.MergeFrom(old)); err != nil {
		return ctrl.Result{}, err
	}

	// Set IPPool metrics.
	metrics.IPPoolIPTotal(rp.Namespace, rp.Name, rp.Spec.Version, rp.Spec.Subnet).Set(float64(totalSize.Int64()))
	metrics.IPPoolIPUsage(rp.Namespace, rp.Name, rp.Spec.Version, rp.Spec.Subnet).Set(float64(usedSize.Int64()))

	return ctrl.Result{}, nil
}

// cleanUpIPool removes IPPool's finalizer when it is not referenced by any IP
// and releases its corresponding IPBlocks.
func (r *poolReconciler) cleanUpIPool(ctx context.Context, pool *requeueipv1.IPPool) (bool, error) {
	if pool.Status.Count == nil {
		return false, nil
	}

	used, err := strconv.Atoi(pool.Status.Count.Used)
	if err != nil {
		return false, err
	}
	if used > 0 {
		return false, nil
	}

	if err := r.client.DeleteAllOf(
		ctx,
		&requeueipv1.IPBlock{},
		client.MatchingLabels{
			consts.LabelRefNamespace: pool.Namespace,
			consts.LabelRefIPPool:    pool.Name,
		},
	); err != nil {
		return false, err
	}

	controllerutil.RemoveFinalizer(pool, consts.RFinalizer)
	if err := r.client.Update(ctx, pool); err != nil {
		return false, err
	}

	// Delete IPPool metrics.
	metrics.DeleteIPPool(pool.Namespace, pool.Name)

	return true, nil
}
