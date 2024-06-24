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
	"errors"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// The Subnet does not have enough IP blocks for IP address assignments.
	errInsufficientIPBlocks = errors.New("IPBlocks are insufficient")
)

type errRequeue ctrl.Result

func (e *errRequeue) Error() string {
	return "requeue"
}

// newErrorRequeue returns an error for requeuing.
func newErrorRequeue() error {
	return &errRequeue{Requeue: true}
}

// newErrorRequeueAfter returns an error for delayed requeuing.
func newErrorRequeueAfter(delay time.Duration) error {
	return &errRequeue{RequeueAfter: delay}
}

// ignoreRequeue converts error to the result of Reconciler.
func ignoreRequeue(err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeue *errRequeue
	if errors.As(err, &requeue) {
		return ctrl.Result(*requeue), nil
	}

	return ctrl.Result{}, err
}
