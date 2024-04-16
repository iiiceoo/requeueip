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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/sauto4/requeueip/api/v1"
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
		Owns(&requeueipv1.SautoIP{}).
		Complete(r)
}

var _ reconcile.Reconciler = &sautoIPPoolReconciler{}

func (r *sautoIPPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sp requeueipv1.SautoIPPool
	if err := r.client.Get(ctx, req.NamespacedName, &sp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
