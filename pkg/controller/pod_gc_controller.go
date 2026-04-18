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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/metrics"
)

func NewPodGCReconciler(c client.Client) *podGCReconciler {
	return &podGCReconciler{
		client: c,
	}
}

type podGCReconciler struct {
	client client.Client
}

func (r *podGCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(
			PodTerminatingPredicate{},
		)).
		Complete(r)
}

type PodTerminatingPredicate struct {
	predicate.Funcs
}

func (PodTerminatingPredicate) Create(event.CreateEvent) bool {
	return false
}

func (PodTerminatingPredicate) Delete(event.DeleteEvent) bool {
	return true
}

func (PodTerminatingPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectNew == nil {
		return false
	}
	pod := e.ObjectNew.(*corev1.Pod)

	return !pod.DeletionTimestamp.IsZero()
}

func (PodTerminatingPredicate) Generic(event.GenericEvent) bool {
	return false
}

var _ reconcile.Reconciler = &podGCReconciler{}

// Reconcile garbage-collects IP CRs for Pods that remain in terminating state
// beyond their grace period. When the Pod has already been deleted (IP CR is
// reclaimed automatically through OwnerReference), it only removes the
// corresponding Pod metrics.
func (r *podGCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod corev1.Pod
	if err := r.client.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			metrics.DeletePodIP(req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if pod.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	if !isRequeueIPPod(&pod) {
		return ctrl.Result{}, nil
	}

	gracePeriodSeconds := int64(0)
	if pod.DeletionGracePeriodSeconds != nil {
		gracePeriodSeconds = *pod.DeletionGracePeriodSeconds
	}
	gracePeriod := time.Duration(gracePeriodSeconds) * time.Second
	terminatedTime := pod.DeletionTimestamp.Time.Add(gracePeriod)

	now := time.Now()
	if now.Before(terminatedTime) {
		return ctrl.Result{RequeueAfter: terminatedTime.Sub(now)}, nil
	}

	if err := r.client.DeleteAllOf(
		ctx,
		&requeueipv1.IP{},
		client.MatchingLabels{consts.LabelRefPodUID: string(pod.UID)},
		client.InNamespace(pod.Namespace),
	); err != nil {
		return ctrl.Result{}, err
	}

	metrics.DeletePodIP(pod.Namespace, pod.Name)
	return ctrl.Result{}, nil
}

// isRequeueIPPod reports whether Pod is using RequeueIP. StatefulSet Pods are
// excluded because their IP GC is handled by sts_gc_controller, which needs
// ordinal-aware scale-down semantics instead of Pod-level terminating events.
func isRequeueIPPod(pod *corev1.Pod) bool {
	owner := metav1.GetControllerOf(pod)
	if owner == nil {
		return false
	}

	if owner.APIVersion == appsv1.SchemeGroupVersion.String() &&
		owner.Kind == consts.KindStatefulSet {
		return false
	}

	_, v4Subnet := pod.Annotations[consts.AnnoIPv4Subnets]
	_, v6Subnet := pod.Annotations[consts.AnnoIPv6Subnets]
	_, v4IPPool := pod.Annotations[consts.AnnoIPv4IPPools]
	_, v6IPPool := pod.Annotations[consts.AnnoIPv6IPPools]

	v4Enabled := v4Subnet || v4IPPool
	v6Enabled := v6Subnet || v6IPPool

	return v4Enabled || v6Enabled
}
