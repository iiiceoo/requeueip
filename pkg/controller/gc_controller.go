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
	"fmt"
	"regexp"
	"strconv"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func NewGCReconciler(c client.Client) *gcReconciler {
	return &gcReconciler{
		client: c,
	}
}

type gcReconciler struct {
	client client.Client
}

func (r *gcReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(
			&appsv1.StatefulSet{},
			builder.WithPredicates(StatefulSetReplicasDecreasedPredicate{}),
		).
		Complete(r)
}

type StatefulSetReplicasDecreasedPredicate struct {
	predicate.Funcs
}

func (StatefulSetReplicasDecreasedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil {
		return false
	}
	if e.ObjectNew == nil {
		return false
	}

	old := e.ObjectOld.(*appsv1.StatefulSet)
	sts := e.ObjectNew.(*appsv1.StatefulSet)

	var or, r int32
	if old.Spec.Replicas != nil {
		or = *old.Spec.Replicas
	}
	if sts.Spec.Replicas != nil {
		r = *sts.Spec.Replicas
	}

	return r < or
}

var _ reconcile.Reconciler = &gcReconciler{}

func (r *gcReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sts appsv1.StatefulSet
	if err := r.client.Get(ctx, req.NamespacedName, &sts); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// The StatefulSet has been deleted, do nothing, OwnerReference will ensure that
	// the relevant IPs are recycled.
	if !sts.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	annos := sts.Spec.Template.Annotations
	_, v4Enabled := annos[consts.AnnoIPv4Subnets]
	_, v6Enabled := annos[consts.AnnoIPv6Subnets]
	if !v4Enabled && !v6Enabled {
		var ns corev1.Namespace
		if err := r.client.Get(ctx, types.NamespacedName{Name: sts.Namespace}, &ns); err != nil {
			return ctrl.Result{}, err
		}
		_, v4Enabled = ns.Annotations[consts.AnnoIPv4Subnets]
		_, v6Enabled = ns.Annotations[consts.AnnoIPv6Subnets]
		if !v4Enabled && !v6Enabled {
			return ctrl.Result{}, nil
		}
	}

	var riList requeueipv1.IPList
	if err := r.client.List(
		ctx,
		&riList,
		client.MatchingLabels{consts.LabelRefSTSUID: string(sts.UID)},
		client.InNamespace(sts.Namespace),
		client.UnsafeDisableDeepCopy,
	); err != nil {
		return ctrl.Result{}, err
	}

	deleted := map[string]bool{}
	for i := 0; i < len(riList.Items); i++ {
		ri := &riList.Items[i]
		podName := ri.Labels[consts.LabelRefPod]
		if deleted[podName] {
			continue
		}

		ok, err := isRunningSTSPod(&sts, podName)
		if err != nil {
			return ctrl.Result{}, err
		}

		if !ok {
			if err := r.client.DeleteAllOf(
				ctx,
				&requeueipv1.IP{},
				client.MatchingLabels{
					consts.LabelRefSTSUID: string(sts.UID),
					consts.LabelRefPod:    podName,
				},
				client.InNamespace(sts.Namespace),
			); err != nil {
				return ctrl.Result{}, err
			}
			deleted[podName] = true
		}
	}

	return ctrl.Result{}, nil
}

// isRunningSTSPod reports whether Pod is controlled by StatefulSet and does not
// need to be deleted.
func isRunningSTSPod(sts *appsv1.StatefulSet, podName string) (bool, error) {
	ordinal, err := getSTSPodOrdinal(podName)
	if err != nil {
		return false, err
	}

	end := 0
	if sts.Spec.Replicas != nil {
		end = int(*sts.Spec.Replicas) - 1
	}

	// Ref: https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#start-ordinal
	if sts.Spec.Ordinals != nil {
		start := int(sts.Spec.Ordinals.Start)
		end = start + end
	}

	return ordinal <= end, nil
}

// regexSTSPodName is a regular expression that extracts the parent StatefulSet
// and ordinal from the Name of a Pod.
var regexSTSPodName = regexp.MustCompile("(.*)-([0-9]+)$")

// getSTSPodOrdinal gets Pod's ordinal as extracted from its Name. If the Pod
// was not created by a StatefulSet, its ordinal is considered to be -1.
func getSTSPodOrdinal(podName string) (int, error) {
	matches := regexSTSPodName.FindStringSubmatch(podName)
	if len(matches) < 3 {
		return -1, fmt.Errorf("invalid StatefulSet Pod name %s", podName)
	}

	ordinal, err := strconv.ParseInt(matches[2], 10, 32)
	if err != nil {
		return -1, fmt.Errorf("invalid StatefulSet Pod name %s: %v", podName, err)
	}

	return int(ordinal), nil
}
