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
	"math/big"
	"strings"

	"github.com/google/uuid"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	"github.com/iiiceoo/requeueip/pkg/net"
)

func NewScaleReconciler(c client.Client) *scaleReconciler {
	return &scaleReconciler{
		client: c,
	}
}

type scaleReconciler struct {
	client client.Client
}

func (r *scaleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1.Scale{}, builder.WithPredicates(
			predicate.NewPredicateFuncs(func(object client.Object) bool {
				scale := object.(*autoscalingv1.Scale)
				return !strings.Contains(scale.Status.Selector, "pod-template-hash=")
			}),
		)).
		Complete(r)
}

var _ reconcile.Reconciler = &scaleReconciler{}

func (r *scaleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var scale autoscalingv1.Scale
	if err := r.client.Get(ctx, req.NamespacedName, &scale); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// The workload has been deleted, do nothing, OwnerReference will ensure
	// that the relevant IPPools are recycled.
	if !scale.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// Empty workloads, always recycle corresponding IPPools, even if they
	// never exist, which may result in redundant calls to DeleteAllOf.
	if scale.Spec.Replicas == 0 {
		return ctrl.Result{}, r.client.DeleteAllOf(
			ctx,
			&requeueipv1.SautoIPPool{},
			client.MatchingLabels{consts.AnnoWorkloadUID: string(scale.UID)},
		)
	}

	// Get all annotations for IPPool through Pod or Namespace.
	ls, err := metav1.ParseToLabelSelector(scale.Status.Selector)
	if err != nil {
		return ctrl.Result{}, err
	}
	selector, err := metav1.LabelSelectorAsSelector(ls)
	if err != nil {
		return ctrl.Result{}, err
	}

	var podList corev1.PodList
	if err := r.client.List(ctx, &podList, client.MatchingLabelsSelector{Selector: selector}, client.Limit(1)); err != nil {
		return ctrl.Result{}, err
	}

	// TODO(iiiceoo): len(PodList) == 0?
	annos := podList.Items[0].Annotations
	v4subnetsStr, ok := annos[consts.AnnoIPv4Subnets]
	if !ok {
		var ns corev1.Namespace
		if err := r.client.Get(ctx, types.NamespacedName{Name: scale.Namespace}, &ns); err != nil {
			return ctrl.Result{}, err
		}

		v4subnetsStr, ok = ns.Annotations[consts.AnnoIPv4Subnets]
		if !ok {
			return ctrl.Result{}, nil
		}
	}

	// Invalid IPPool annotation, do nothing, RequeueIP CNI will return an error.
	v4sa := parseArray(v4subnetsStr)
	if len(v4sa) == 0 {
		// TODO(iiiceoo): Log.
		return ctrl.Result{}, nil
	}

	var spList requeueipv1.SautoIPPoolList
	if err := r.client.List(ctx, &spList, client.MatchingLabels{consts.AnnoWorkloadUID: string(scale.UID)}); err != nil {
		return ctrl.Result{}, err
	}

	var v4sp *requeueipv1.SautoIPPool
	for _, sp := range spList.Items {
		if !sp.DeletionTimestamp.IsZero() {
			continue
		}
		if *sp.Spec.Version == net.IPv4 {
			// The workload has changed the Subnets it expects to assign IP addresses to.
			if !slices.Contains(v4sa, sp.Name) {
				if err := r.client.Delete(ctx, &sp); err != nil {
					return ctrl.Result{}, err
				}
			} else {
				v4sp = sp.DeepCopy()
			}
		}
	}

	// To ensure that IPBlocks are always correctly recycled, it is necessary
	// to create an empty IPPool with LabelRefSubnet set before applying IPBlocks.
	if v4sp == nil {
		var v4sn string
		for _, s := range v4sa {
			var ss requeueipv1.SautoSubnet
			if err := r.client.Get(ctx, types.NamespacedName{Name: s}, &ss); err != nil {
				if apierrors.IsNotFound(err) {
					// TODO(iiiceoo): Log.
					continue
				}
				return ctrl.Result{}, err
			}

			fc := new(big.Int)
			fc.SetString(ss.Status.Count.Free, 10)
			if fc.Cmp(big.NewInt(int64(scale.Spec.Replicas))) >= 0 {
				v4sn = s
				break
			}
		}
		if v4sn == "" {
			return ctrl.Result{}, fmt.Errorf("no Subnets are available in %s: %w", v4sa, errInsufficientIPBlocks)
		}

		v4sp = &requeueipv1.SautoIPPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scale.Name + "-" + uuid.New().String(),
				Namespace: scale.Namespace,
				Labels:    map[string]string{consts.LabelRefSubnet: v4sn},
			},
			Spec: requeueipv1.SautoIPPoolSpec{
				Version: ptr.To(net.IPv4),
			},
		}
		controllerutil.AddFinalizer(v4sp, consts.RFinalizer)
		if err := controllerutil.SetControllerReference(&scale, v4sp, r.client.Scheme()); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.client.Create(ctx, v4sp); err != nil {
			return ctrl.Result{}, err
		}
	}

	// TODO(iiiceoo): Comment scale IPPools.

	return ctrl.Result{}, nil
}

func parseArray(arrStr string) []string {
	if arrStr == "" {
		return nil
	}

	var res []string
	parts := strings.Split(arrStr, ",")
	for _, p := range parts {
		p = strings.Trim(p, " ")
		if p != "" {
			res = append(res, p)
		}
	}

	return res
}
