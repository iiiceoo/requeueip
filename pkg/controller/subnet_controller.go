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

	var rpList requeueipv1.IPPoolList
	if err := r.client.List(
		ctx,
		&rpList,
		client.MatchingLabels{consts.LabelRefSubnet: rn.Name},
		client.Limit(1),
	); err != nil {
		return ctrl.Result{}, err
	}

	if len(rpList.Items) == 0 && !rn.DeletionTimestamp.IsZero() {
		controllerutil.RemoveFinalizer(&rn, consts.RFinalizer)
		return ctrl.Result{}, r.client.Update(ctx, &rn)
	}

	var rbList requeueipv1.IPBlockList
	if err := r.client.List(ctx, &rbList, client.MatchingLabels{consts.LabelRefSubnet: rn.Name}); err != nil {
		return ctrl.Result{}, err
	}

	step, err := net.CountFromMaskSize(*rn.Spec.Version, int(*rn.Spec.BlockSize))
	if err != nil {
		return ctrl.Result{}, err
	}

	// Calculate the total, used, and available range of the Subnet.
	total, err := iprange.Parse(rn.Spec.CIDR)
	if err != nil {
		return ctrl.Result{}, err
	}
	used, err := parseRangesFromIPBlocks(*rn.Spec.Version, rbList.Items)
	if err != nil {
		return ctrl.Result{}, err
	}
	free := total.DeepCopy().Diff(used)

	// Update Subnet status if its current status has changed.
	status := rn.Status.DeepCopy()
	if status.BlockCount == nil {
		status.BlockCount = &requeueipv1.BlockCount{}
	}
	count := new(big.Int)
	status.Free = free.Strings()
	status.BlockCount.Total = count.Div(total.Size(), step).String()
	status.BlockCount.Used = count.Div(used.Size(), step).String()
	status.BlockCount.Free = count.Div(free.Size(), step).String()
	if reflect.DeepEqual(status, rn.Status) {
		return ctrl.Result{}, nil
	}
	rn.Status = *status

	return ctrl.Result{}, r.client.Status().Update(ctx, &rn)
}
