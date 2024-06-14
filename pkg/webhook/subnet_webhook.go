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

func NewSubnetWebhooker(c client.Client) *subnetWebhooker {
	return &subnetWebhooker{
		client: c,
	}
}

type subnetWebhooker struct {
	client client.Client
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

func (h *subnetWebhooker) mutate(ctx context.Context, rn *requeueipv1.Subnet) error {
	logger := log.FromContext(ctx)
	if !rn.DeletionTimestamp.IsZero() {
		logger.Info("Terminating Subnet")
		return nil
	}

	if !controllerutil.ContainsFinalizer(rn, consts.RFinalizer) {
		controllerutil.AddFinalizer(rn, consts.RFinalizer)
		logger.Info("Added finalizer", "finalizer", consts.RFinalizer)
	}

	if rn.Spec.Version == nil {
		ip, _, err := net.ParseCIDR(rn.Spec.CIDR)
		if err != nil {
			logger.Info("Skipped mutating", "cidr", rn.Spec.CIDR, "error", err)
			return nil
		}

		if ip.To4() != nil {
			rn.Spec.Version = ptr.To(rnet.IPv4)
		} else {
			rn.Spec.Version = ptr.To(rnet.IPv6)
		}
		logger.Info(fmt.Sprintf("Added %s", fieldVersion), "version", *rn.Spec.Version)
	}

	if rn.Spec.BlockSize == nil {
		if *rn.Spec.Version == rnet.IPv4 {
			rn.Spec.BlockSize = ptr.To(int32(30))
		} else {
			rn.Spec.BlockSize = ptr.To(int32(126))
		}
		logger.Info(fmt.Sprintf("Added %s", fieldBlockSize), "blockSize", *rn.Spec.BlockSize)
	}

	return nil
}

var (
	fieldVersion   *field.Path = field.NewPath("spec").Child("version")
	fieldCIDR      *field.Path = field.NewPath("spec").Child("cidr")
	fieldBlockSize *field.Path = field.NewPath("spec").Child("blockSize")
)

func (h *subnetWebhooker) validateCreate(ctx context.Context, rn *requeueipv1.Subnet) field.ErrorList {
	return validateSubnetSpec(rn)
}

func (h *subnetWebhooker) validateUpdate(ctx context.Context, old, rn *requeueipv1.Subnet) field.ErrorList {
	return validateSubnetSpec(rn)
}

func validateSubnetSpec(rn *requeueipv1.Subnet) field.ErrorList {
	if err := validateCIDR(rn); err != nil {
		return field.ErrorList{err}
	}

	var errs field.ErrorList
	if err := validateBlockSize(*rn.Spec.Version, *rn.Spec.BlockSize); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func validateCIDR(rn *requeueipv1.Subnet) *field.Error {
	ip, _, err := net.ParseCIDR(rn.Spec.CIDR)
	if err != nil {
		return field.Invalid(fieldCIDR, rn.Spec.CIDR, err.Error())
	}

	if ip.To4() != nil {
		if *rn.Spec.Version != rnet.IPv4 {
			return field.Invalid(fieldCIDR, rn.Spec.CIDR, "is not an IPv4 CIDR")
		}
	} else {
		if *rn.Spec.Version != rnet.IPv6 {
			return field.Invalid(fieldCIDR, rn.Spec.CIDR, "is not an IPv6 CIDR")
		}
	}

	return nil
}

func validateBlockSize(version string, blockSize int32) *field.Error {
	if version == rnet.IPv4 {
		if blockSize < 26 || blockSize > 32 {
			return field.Forbidden(fieldBlockSize, "IPv4 block size must belong to [26, 32]")
		}
	} else {
		if blockSize < 122 || blockSize > 128 {
			return field.Forbidden(fieldBlockSize, "IPv6 block size must belong to [122, 128]")
		}
	}

	return nil
}
