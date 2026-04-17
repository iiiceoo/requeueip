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

package workload

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func newStatefulSetReconciler(c client.Client) *statefulSetReconciler {
	return &statefulSetReconciler{
		client:    c,
		rpcClient: newRPCClient(c),
	}
}

type statefulSetReconciler struct {
	client    client.Client
	rpcClient *rpcClient
}

func (r *statefulSetReconciler) setupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}, builder.WithPredicates(
			predicate.GenerationChangedPredicate{},
		)).
		Complete(r)
}

var _ reconcile.Reconciler = &statefulSetReconciler{}

func (r *statefulSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sts appsv1.StatefulSet
	if err := r.client.Get(ctx, req.NamespacedName, &sts); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// The workload has been deleted, do nothing, OwnerReference will ensure
	// that the relevant IPPoolClaims are recycled.
	if !sts.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	claims, err := r.rpcClient.parseClaims(
		ctx,
		sts.Namespace,
		sts.Spec.Template.ObjectMeta.Annotations,
		*sts.Spec.Replicas,
	)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.rpcClient.ensureClaims(ctx, claims, &sts); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
