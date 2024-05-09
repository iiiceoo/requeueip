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
	errorInsufficientIPBlocks = errors.New("IP blocks are insufficient")
)

func newErrorRequeue() error {
	return &errorRequeue{Requeue: true}
}

func newErrorRequeueAfter(delay time.Duration) error {
	return &errorRequeue{RequeueAfter: delay}
}

type errorRequeue ctrl.Result

func (e *errorRequeue) Error() string {
	return "requeue"
}

func ignoreRequeue(err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeue *errorRequeue
	if errors.As(err, &requeue) {
		return ctrl.Result(*requeue), nil
	}

	return ctrl.Result{}, err
}
