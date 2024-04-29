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
	"errors"
	"fmt"
	"hash/fnv"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iiiceoo/iprange"
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
	requeueipnet "github.com/iiiceoo/requeueip/pkg/net"
)

var (
	errInsufficientIPBlocks = errors.New("IP blocks are insufficient")
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
	if err := r.client.List(
		ctx,
		&podList,
		client.MatchingLabelsSelector{Selector: selector},
		client.Limit(1),
	); err != nil {
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
	v4candidates := parseArray(v4subnetsStr)
	if len(v4candidates) == 0 {
		// TODO(iiiceoo): Log.
		return ctrl.Result{}, nil
	}

	var spList requeueipv1.SautoIPPoolList
	if err := r.client.List(
		ctx,
		&spList,
		client.MatchingLabels{consts.AnnoWorkloadUID: string(scale.UID)},
	); err != nil {
		return ctrl.Result{}, err
	}

	var v4sp *requeueipv1.SautoIPPool
	for _, sp := range spList.Items {
		if !sp.DeletionTimestamp.IsZero() {
			continue
		}
		if *sp.Spec.Version == requeueipnet.IPv4 {
			// The workload has changed the Subnets it expects to assign IP addresses to.
			if !slices.Contains(v4candidates, sp.Name) {
				if err := r.client.Delete(ctx, &sp); err != nil {
					return ctrl.Result{}, err
				}
			} else {
				v4sp = sp.DeepCopy()
			}
		}
	}

	// To ensure that IPBlocks are always correctly recycled, it is necessary
	// to create an empty IPPool with LabelRefSubnet set before claiming IPBlocks.
	if v4sp == nil {
		var v4sn string
		for _, c := range v4candidates {
			var ss requeueipv1.SautoSubnet
			if err := r.client.Get(ctx, types.NamespacedName{Name: c}, &ss); err != nil {
				if apierrors.IsNotFound(err) {
					// TODO(iiiceoo): Log.
					continue
				}
				return ctrl.Result{}, err
			}
			if ss.Status.Count == nil {
				return ctrl.Result{Requeue: true}, nil
			}

			// The number of available IP addresses for a Subnet can be very large.
			// Avoid using strconv.Atoi().
			fc := new(big.Int)
			fc.SetString(ss.Status.Count.Free, 10)
			if fc.Cmp(big.NewInt(int64(scale.Spec.Replicas))) >= 0 {
				v4sn = c
				break
			}
		}
		if v4sn == "" {
			return ctrl.Result{}, fmt.Errorf("no Subnets are available in %s: %w", v4candidates, errInsufficientIPBlocks)
		}

		v4sp = &requeueipv1.SautoIPPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      scale.Name + "-" + uuid.New().String(),
				Namespace: scale.Namespace,
				Labels:    map[string]string{consts.LabelRefSubnet: v4sn},
			},
			Spec: requeueipv1.SautoIPPoolSpec{
				Version: ptr.To(requeueipnet.IPv4),
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

	// Do not use .status.count.all as it may not have been set correctly when
	// IPPool first created.
	v4ranges, err := iprange.Parse(v4sp.Spec.Ranges...)
	if err != nil {
		return ctrl.Result{}, err
	}

	// A workload must not have more than int replicas. For convenience, convert
	// IPPool size to int.
	v4poolSize := int(v4ranges.Size().Int64())
	replicas := int(scale.Spec.Replicas)
	if v4poolSize == replicas {
		return ctrl.Result{}, nil
	}

	// TODO(iiiceo): IPPool does not have label LabelRefSubnet.
	var v4ss requeueipv1.SautoSubnet
	if err := r.client.Get(ctx, types.NamespacedName{Name: v4sp.Labels[consts.LabelRefSubnet]}, &v4ss); err != nil {
		return ctrl.Result{}, err
	}

	v4bStep, err := requeueipnet.CountFromMaskSize(*v4ss.Spec.Version, int(*v4ss.Spec.BlockSize))
	if err != nil {
		return ctrl.Result{}, err
	}
	v4step := int(v4bStep.Int64())

	var sbList requeueipv1.SautoIPBlockList
	if err := r.client.List(
		ctx,
		&sbList,
		client.MatchingLabels{
			consts.LabelRefNamespace: v4sp.Namespace,
			consts.LabelRefIPPool:    v4sp.Name,
		},
	); err != nil {
		return ctrl.Result{}, err
	}
	v4ipCount := len(sbList.Items) * v4step

	// Scale down IPPools.
	if replicas < v4poolSize {
		if v4sp.Status.Count == nil {
			return ctrl.Result{Requeue: true}, nil
		}

		// Wait for the replicas of workload to converge before scaling down IPPool.
		uc, err := strconv.Atoi(v4sp.Status.Count.Used)
		if err != nil {
			return ctrl.Result{}, err
		}
		if uc != replicas {
			// The default DeletionGracePeriodSeconds for Pod is 30 seconds.
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		v4sp.Spec.Ranges = v4sp.Status.Free
		if err := r.client.Update(ctx, v4sp); err != nil {
			return ctrl.Result{}, err
		}

		// Normal redundancy, no need to recycle IPBlocks.
		if v4ipCount-replicas < v4step {
			return ctrl.Result{}, nil
		}

		fr, err := iprange.Parse(v4sp.Status.Free...)
		if err != nil {
			return ctrl.Result{}, err
		}

		// TODO(iiiceoo): Goroutine.
		for _, sb := range sbList.Items {
			ipNet, err := requeueipnet.NameToCIDR(*v4sp.Spec.Version, sb.Name)
			if err != nil {
				return ctrl.Result{}, err
			}
			br, err := iprange.Parse(ipNet.String())
			if err != nil {
				return ctrl.Result{}, err
			}
			if br.Diff(fr).Size().Sign() > 0 {
				continue
			}
			if err := r.client.Delete(ctx, &sb); err != nil {
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}
		}
	}

	// Scale up IPPools.
	if replicas > v4poolSize {
		if replicas <= v4ipCount {
			delta, err := parseIPRangesFromIPBlocks(sbList.Items)
			if err != nil {
				return ctrl.Result{}, err
			}

			delta.Diff(v4ranges).Slice(big.NewInt(0), big.NewInt(int64(replicas-v4poolSize)))
			v4sp.Spec.Ranges = v4ranges.Union(delta).Strings()
			return ctrl.Result{}, r.client.Update(ctx, v4sp)
		}

		if v4ss.Status.Count == nil {
			return ctrl.Result{Requeue: true}, nil
		}

		dv := replicas - v4ipCount
		expect := dv / v4step
		if dv%v4step != 0 {
			expect++
		}

		// The number of available IP addresses for a Subnet can be very large.
		// Avoid using strconv.Atoi().
		fc := new(big.Int)
		fc.SetString(v4ss.Status.Count.Free, 10)
		if fc.Cmp(big.NewInt(int64(expect*v4step))) < 0 {
			return ctrl.Result{}, fmt.Errorf(
				"unable to scale up IPPool %s in Subnet %s: %w",
				v4sp.Name,
				v4ss.Name,
				errInsufficientIPBlocks,
			)
		}
		bc := new(big.Int).Div(fc, v4bStep)

		free, err := iprange.Parse(v4ss.Status.Free...)
		if err != nil {
			return ctrl.Result{}, err
		}
		bi := free.BlockIterator(v4bStep)

		// TODO(iiiceoo): Goroutine.
		complete := true
		h := fnv.New32a()
		for i := 1; i <= expect; i++ {
			h.Reset()
			id := fmt.Sprintf("%s-%s-%d", v4sp.Name, *v4sp.Spec.Version, len(sbList.Items)+i)
			h.Write([]byte(id))
			n := new(big.Int).Mod(big.NewInt(int64(h.Sum32())), bc)
			n.Add(n, big.NewInt(1))

			blockStr := bi.NextN(n).String()
			sb := &requeueipv1.SautoIPBlock{
				ObjectMeta: metav1.ObjectMeta{
					Name: requeueipnet.CIDRStringToName(blockStr),
					Labels: map[string]string{
						consts.LabelRefNamespace: v4sp.Namespace,
						consts.LabelRefIPPool:    v4sp.Name,
					},
				},
			}
			if err := r.client.Create(ctx, sb); err != nil {
				if !apierrors.IsAlreadyExists(err) {
					return ctrl.Result{}, err
				}
			}

			blockStr = bi.Next().String()
			sb.SetName(requeueipnet.CIDRStringToName(blockStr))
			if err := r.client.Create(ctx, sb); err != nil {
				if !apierrors.IsAlreadyExists(err) {
					return ctrl.Result{}, err
				}

				// TODO(iiiceoo): Log.
				complete = false
				continue
			}
			sbList.Items = append(sbList.Items, *sb)
		}

		if !complete {
			return ctrl.Result{Requeue: true}, nil
		}

		ranges, err := parseIPRangesFromIPBlocks(sbList.Items)
		if err != nil {
			return ctrl.Result{}, err
		}
		v4sp.Spec.Ranges = ranges.Slice(big.NewInt(0), big.NewInt(int64(scale.Spec.Replicas))).Strings()
		if err := r.client.Update(ctx, v4sp); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func parseIPRangesFromIPBlocks(blocks []requeueipv1.SautoIPBlock) (*iprange.IPRanges, error) {
	blockStrs := make([]string, 0, len(blocks))
	for i := 0; i < len(blocks); i++ {
		blockStrs = append(blockStrs, blocks[i].Name)
	}

	return iprange.Parse(blockStrs...)
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
