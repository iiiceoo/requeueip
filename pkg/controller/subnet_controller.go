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

func NewSubnetReconciler(c client.Client) *SubnetReconciler {
	return &SubnetReconciler{
		client: c,
	}
}

type SubnetReconciler struct {
	client client.Client
}

func (r *SubnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.Subnet{}).
		Watches(&requeueipv1.IPBlock{}, handler.EnqueueRequestsFromMapFunc(mapFuncForSubnet)).
		Complete(r)
}

var _ reconcile.Reconciler = &SubnetReconciler{}

func (r *SubnetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var rn requeueipv1.Subnet
	if err := r.client.Get(ctx, req.NamespacedName, &rn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var rbList requeueipv1.IPBlockList
	if err := r.client.List(ctx, &rbList, client.MatchingLabels{consts.LabelRefSubnet: rn.Name}); err != nil {
		return ctrl.Result{}, err
	}

	if len(rbList.Items) == 0 && !rn.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(&rn, consts.RFinalizer)
		return ctrl.Result{}, r.client.Update(ctx, &rn)
	}

	blockStrs := make([]string, 0, len(rbList.Items))
	for i := 0; i < len(rbList.Items); i++ {
		block, err := net.NameToCIDR(*rn.Spec.Version, rbList.Items[i].Name)
		if err != nil {
			// TODO(iiiceoo): Log.
			continue
		}
		blockStrs = append(blockStrs, block.String())
	}

	// Calculate the entire, used, and available range of the Subnet.
	all, err := iprange.Parse(rn.Spec.CIDR)
	if err != nil {
		return ctrl.Result{}, err
	}
	used, err := iprange.Parse(blockStrs...)
	if err != nil {
		return ctrl.Result{}, err
	}
	free := all.DeepCopy().Diff(used)

	// Update Subnet status if its current status has changed.
	status := rn.Status.DeepCopy()
	if status.Count == nil {
		status.Count = &requeueipv1.Count{}
	}
	status.Count.All = all.Size().String()
	status.Count.Used = used.Size().String()
	status.Count.Free = free.Size().String()
	status.Free = free.Strings()
	if reflect.DeepEqual(status, rn.Status) {
		return ctrl.Result{}, nil
	}
	rn.Status = *status

	return ctrl.Result{}, r.client.Status().Update(ctx, &rn)
}
