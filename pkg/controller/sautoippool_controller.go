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

func NewSautoIPPoolReconciler(c client.Client) *sautoIPPoolReconciler {
	return &sautoIPPoolReconciler{
		client: c,
	}
}

type sautoIPPoolReconciler struct {
	client client.Client
}

func (r *sautoIPPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&requeueipv1.SautoIPPool{}).
		Watches(&requeueipv1.SautoIP{}, handler.EnqueueRequestsFromMapFunc(mapFuncForSautoIPPool)).
		Complete(r)
}

var _ reconcile.Reconciler = &sautoIPPoolReconciler{}

func (r *sautoIPPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sp requeueipv1.SautoIPPool
	if err := r.client.Get(ctx, req.NamespacedName, &sp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var siList requeueipv1.SautoIPList
	if err := r.client.List(ctx, &siList, client.MatchingLabels{consts.ManagedByIPPool: sp.Name}); err != nil {
		return ctrl.Result{}, err
	}

	if len(siList.Items) == 0 && !sp.DeletionTimestamp.IsZero() {
		// TODO(iiiceoo): Owner terminating or no longer exists.
		controllerutil.RemoveFinalizer(&sp, consts.RFinalizer)
		return ctrl.Result{}, r.client.Update(ctx, &sp)
	}

	ips := make([]string, 0, len(siList.Items))
	for _, si := range siList.Items {
		ip, err := net.NameToIP(*sp.Spec.Version, si.Name)
		if err != nil {
			// TODO(iiiceoo): Log.
			continue
		}
		ips = append(ips, ip.String())
	}

	use, err := iprange.Parse(ips...)
	if err != nil {
		return ctrl.Result{}, err
	}
	all, err := iprange.Parse(sp.Spec.Ranges...)
	if err != nil {
		return ctrl.Result{}, err
	}
	exclusion, err := iprange.Parse(sp.Spec.Exclusion...)
	if err != nil {
		return ctrl.Result{}, err
	}

	free := all.Diff(exclusion).Diff(use)
	if reflect.DeepEqual(free.Strings(), sp.Status.Free) {
		return ctrl.Result{}, nil
	}

	sp.Status.Free = free.Strings()
	if err := r.client.Status().Update(ctx, &sp); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
