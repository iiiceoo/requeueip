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

package workload

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewWorkloadReconciler(c client.Client) *workloadReconciler {
	return &workloadReconciler{
		dr: newDeploymentReconciler(c),
		sr: newStatefulSetReconciler(c),
	}
}

type workloadReconciler struct {
	dr *deploymentReconciler
	sr *statefulSetReconciler
}

func (r *workloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.dr.setupWithManager(mgr); err != nil {
		return err
	}

	return r.sr.setupWithManager(mgr)
}
