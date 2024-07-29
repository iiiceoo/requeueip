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

package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/iiiceoo/iprange"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	requeueipv1 "github.com/iiiceoo/requeueip/api/v1"
	"github.com/iiiceoo/requeueip/pkg/consts"
	rnet "github.com/iiiceoo/requeueip/pkg/net"
)

func NewSubnetWebhooker(c client.Client, reader client.Reader) *subnetWebhooker {
	return &subnetWebhooker{
		client: c,
		reader: reader,
	}
}

type subnetWebhooker struct {
	client client.Client
	reader client.Reader
}

func (h *subnetWebhooker) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&requeueipv1.Subnet{}).
		WithDefaulter(h).
		WithValidator(h).
		Complete()
}

var _ webhook.CustomDefaulter = (*subnetWebhooker)(nil)

func (h *subnetWebhooker) Default(ctx context.Context, obj runtime.Object) error {
	rn := obj.(*requeueipv1.Subnet)
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}

	logger := log.FromContext(ctx).WithValues(
		"webhook", "mutating",
		"operation", req.Operation,
	)
	logger.V(5).Info("Request object", "old", req.OldObject, "new", req.Object)

	return h.mutate(log.IntoContext(ctx, logger), rn)
}

var _ webhook.CustomValidator = (*subnetWebhooker)(nil)

func (h *subnetWebhooker) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	rn := obj.(*requeueipv1.Subnet)
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, err
	}

	logger := log.FromContext(ctx).WithValues(
		"webhook", "validating",
		"operation", req.Operation,
	)
	logger.V(5).Info("Request object", "old", req.OldObject, "new", req.Object)

	if errs := h.validateCreate(log.IntoContext(ctx, logger), rn); len(errs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind},
			rn.Name,
			errs,
		)
	}

	return nil, nil
}

func (h *subnetWebhooker) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	old := oldObj.(*requeueipv1.Subnet)
	rn := newObj.(*requeueipv1.Subnet)
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return nil, err
	}

	logger := log.FromContext(ctx).WithValues(
		"webhook", "validating",
		"operation", req.Operation,
	)
	logger.V(5).Info("Request object", "old", req.OldObject, "new", req.Object)

	if !rn.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(rn, consts.RFinalizer) {
			return nil, nil
		}

		return nil, apierrors.NewForbidden(
			schema.GroupResource{Group: req.Resource.Group, Resource: req.Resource.Resource},
			rn.Name,
			errors.New("could not update a terminating Subnet"),
		)
	}

	if errs := h.validateUpdate(log.IntoContext(ctx, logger), old, rn); len(errs) != 0 {
		return nil, apierrors.NewInvalid(
			schema.GroupKind{Group: req.Kind.Group, Kind: req.Kind.Kind},
			rn.Name,
			errs,
		)
	}

	return nil, nil
}

func (h *subnetWebhooker) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (h *subnetWebhooker) mutate(ctx context.Context, subnet *requeueipv1.Subnet) error {
	logger := log.FromContext(ctx)
	if !subnet.DeletionTimestamp.IsZero() {
		logger.Info("Terminating Subnet")
		return nil
	}

	if !controllerutil.ContainsFinalizer(subnet, consts.RFinalizer) {
		controllerutil.AddFinalizer(subnet, consts.RFinalizer)
		logger.Info("Added finalizer", "finalizer", consts.RFinalizer)
	}

	if subnet.Spec.Version == nil {
		ip, _, err := net.ParseCIDR(subnet.Spec.CIDR)
		if err != nil {
			logger.Info("Skipped mutating", "cidr", subnet.Spec.CIDR, "error", err)
			return nil
		}

		if ip.To4() != nil {
			subnet.Spec.Version = ptr.To(rnet.IPv4)
		} else {
			subnet.Spec.Version = ptr.To(rnet.IPv6)
		}
		logger.Info(fmt.Sprintf("Added %s", fieldVersion), "version", *subnet.Spec.Version)
	}

	if subnet.Spec.BlockSize == nil {
		if *subnet.Spec.Version == rnet.IPv4 {
			subnet.Spec.BlockSize = ptr.To(int32(30))
		} else {
			subnet.Spec.BlockSize = ptr.To(int32(126))
		}
		logger.Info(fmt.Sprintf("Added %s", fieldBlockSize), "blockSize", *subnet.Spec.BlockSize)
	}

	return nil
}

var (
	fieldVersion   *field.Path = field.NewPath("spec").Child("version")
	fieldCIDR      *field.Path = field.NewPath("spec").Child("cidr")
	fieldBlockSize *field.Path = field.NewPath("spec").Child("blockSize")
	fieldFree      *field.Path = field.NewPath("status").Child("free")
)

func (h *subnetWebhooker) validateCreate(ctx context.Context, subnet *requeueipv1.Subnet) field.ErrorList {
	return h.validateSubnetSpec(ctx, subnet)
}

func (h *subnetWebhooker) validateUpdate(ctx context.Context, old, subnet *requeueipv1.Subnet) field.ErrorList {
	if errs := h.validateSubnetSpec(ctx, subnet); len(errs) != 0 {
		return errs
	}

	var errs field.ErrorList
	if err := validateIPInUse(old, subnet); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func (h *subnetWebhooker) validateSubnetSpec(ctx context.Context, subnet *requeueipv1.Subnet) field.ErrorList {
	if err := h.validateCIDR(ctx, subnet); err != nil {
		return field.ErrorList{err}
	}

	var errs field.ErrorList
	if err := validateBlockSize(subnet); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func (h *subnetWebhooker) validateCIDR(ctx context.Context, subnet *requeueipv1.Subnet) *field.Error {
	ip, cidr, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return field.Invalid(fieldCIDR, subnet.Spec.CIDR, err.Error())
	}

	if !ip.Equal(cidr.IP) {
		return field.Invalid(fieldCIDR, subnet.Spec.CIDR, "is not a network address")
	}

	if ip.To4() != nil {
		if *subnet.Spec.Version != rnet.IPv4 {
			return field.Invalid(fieldCIDR, subnet.Spec.CIDR, "is not an IPv4 CIDR")
		}
	} else {
		if *subnet.Spec.Version != rnet.IPv6 {
			return field.Invalid(fieldCIDR, subnet.Spec.CIDR, "is not an IPv6 CIDR")
		}
	}

	var rnList requeueipv1.SubnetList
	if err := h.reader.List(ctx, &rnList); err != nil {
		return field.InternalError(fieldCIDR, err)
	}

	for i := 0; i < len(rnList.Items); i++ {
		rn := &rnList.Items[i]
		if rn.Name == subnet.Name {
			continue
		}

		_, otherCIDR, err := net.ParseCIDR(rn.Spec.CIDR)
		if err != nil {
			return field.InternalError(fieldCIDR, err)
		}

		if cidr.Contains(otherCIDR.IP) || otherCIDR.Contains(cidr.IP) {
			return field.Invalid(
				fieldCIDR,
				subnet.Spec.CIDR,
				fmt.Sprintf("overlaps with Subnet %s which CIDR is %s", rn.Name, rn.Spec.CIDR),
			)
		}
	}

	return nil
}

func validateBlockSize(subnet *requeueipv1.Subnet) *field.Error {
	if *subnet.Spec.Version == rnet.IPv4 {
		if *subnet.Spec.BlockSize < 26 || *subnet.Spec.BlockSize > 32 {
			return field.Invalid(fieldBlockSize, *subnet.Spec.BlockSize, "must belong to [26, 32]")
		}
	} else {
		if *subnet.Spec.BlockSize < 122 || *subnet.Spec.BlockSize > 128 {
			return field.Invalid(fieldBlockSize, *subnet.Spec.BlockSize, "must belong to [122, 128]")
		}
	}

	_, cidr, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return field.Invalid(fieldCIDR, subnet.Spec.CIDR, err.Error())
	}

	ones, _ := cidr.Mask.Size()
	if int(*subnet.Spec.BlockSize) < ones {
		return field.Invalid(
			fieldBlockSize,
			*subnet.Spec.BlockSize,
			fmt.Sprintf("is smaller than the CIDR mask length %d", ones),
		)
	}

	return nil
}

func validateIPInUse(old, subnet *requeueipv1.Subnet) *field.Error {
	if subnet.Spec.CIDR == old.Spec.CIDR {
		return nil
	}

	if subnet.Status.Free == nil {
		return field.Forbidden(fieldCIDR, "CIDR should not be modified before Subnet is ready")
	}

	ranges, err := iprange.Parse(subnet.Spec.CIDR)
	if err != nil {
		return field.Invalid(fieldCIDR, subnet.Spec.CIDR, err.Error())
	}
	oldRanges, err := iprange.Parse(old.Spec.CIDR)
	if err != nil {
		return nil
	}

	reduce := oldRanges.Diff(ranges)
	if reduce.Size().Sign() == 0 {
		return nil
	}

	free, err := iprange.Parse(subnet.Status.Free...)
	if err != nil {
		return field.InternalError(fieldFree, err)
	}

	invalid := reduce.Diff(free)
	if invalid.Size().Sign() > 0 {
		return field.Forbidden(fieldCIDR, fmt.Sprintf("remove the Subnet IP ranges %s that are being used", invalid))
	}

	return nil
}
