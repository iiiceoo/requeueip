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

	ctrl "sigs.k8s.io/controller-runtime"
)

type requeueError struct {
	result ctrl.Result
}

func (e *requeueError) Error() string {
	return "requeue"
}

func (e *requeueError) Result() ctrl.Result {
	return e.result
}

// newRequeueError returns an error carrying a reconcile result.
func newRequeueError() error {
	return &requeueError{
		result: ctrl.Result{Requeue: true},
	}
}

// ignoreRequeue converts error to the result of Reconciler.
func ignoreRequeue(err error) (ctrl.Result, error) {
	if err == nil {
		return ctrl.Result{}, nil
	}

	var requeue interface{ Result() ctrl.Result }
	if errors.As(err, &requeue) {
		return requeue.Result(), nil
	}

	return ctrl.Result{}, err
}
