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

	"github.com/iiiceoo/iprange"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

func NewIPPoolReconciler(c client.Client) *IPPoolReconciler {
	return &IPPoolReconciler{
		client: c,
	}
}

type IPPoolReconciler struct {
	client client.Client
}

func (r *IPPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.IPPool{}).
		Watches(&requeueipv1.IP{}, handler.EnqueueRequestsFromMapFunc(mapFuncForIPPool)).
		Complete(r)
}

var _ reconcile.Reconciler = &IPPoolReconciler{}

func (r *IPPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var rp requeueipv1.IPPool
	if err := r.client.Get(ctx, req.NamespacedName, &rp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var riList requeueipv1.IPList
	if err := r.client.List(ctx, &riList, client.MatchingLabels{consts.LabelRefIPPool: rp.Name}); err != nil {
		return ctrl.Result{}, err
	}

	if len(riList.Items) == 0 && !rp.DeletionTimestamp.IsZero() {
		// TODO(iiiceoo): Owner terminating or no longer exists.
		if err := r.client.DeleteAllOf(
			ctx,
			&requeueipv1.IPBlock{},
			client.MatchingLabels{
				consts.LabelRefNamespace: rp.Namespace,
				consts.LabelRefIPPool:    rp.Name,
			},
		); err != nil {
			return ctrl.Result{}, err
		}

		controllerutil.RemoveFinalizer(&rp, consts.RFinalizer)
		return ctrl.Result{}, r.client.Update(ctx, &rp)
	}

	ipStrs := make([]string, 0, len(riList.Items))
	for i := 0; i < len(riList.Items); i++ {
		ip, err := net.NameToIP(rp.Spec.Version, riList.Items[i].Name)
		if err != nil {
			// TODO(iiiceoo): Log.
			continue
		}
		ipStrs = append(ipStrs, ip.String())
	}

	// Calculate the entire, used, and available range of the IPPool.
	all, err := iprange.Parse(rp.Spec.Ranges...)
	if err != nil {
		return ctrl.Result{}, err
	}
	used, err := iprange.Parse(ipStrs...)
	if err != nil {
		return ctrl.Result{}, err
	}
	free := all.DeepCopy().Diff(used)

	// Update IPPool status if its current status has changed.
	status := rp.Status.DeepCopy()
	if status.Count == nil {
		status.Count = &requeueipv1.Count{}
	}
	status.Count.All = all.Size().String()
	status.Count.Used = used.Size().String()
	status.Count.Free = free.Size().String()
	status.Free = free.Strings()
	if reflect.DeepEqual(status, rp.Status) {
		return ctrl.Result{}, nil
	}
	rp.Status = *status

	return ctrl.Result{}, r.client.Status().Update(ctx, &rp)
}
