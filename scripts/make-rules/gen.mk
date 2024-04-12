# Copyright 2024 The RequeueIP Authors.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

#     http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

.PHONY: gen.run
gen.run: gen.manifests gen.deepcopy

.PHONY: gen.verify
gen.verify: gen.manifests-verify

.PHONY: gen.clean
gen.clean: gen.manifests-clean

.PHONY: gen.manifests
gen.manifests:
	@echo "==> Generate ClusterRoles and CustomResourceDefinitions"
	@scripts/controller_gen.sh manifests

.PHONY: gen.manifests-verify
gen.manifests-verify:
	@echo "==> Verify ClusterRoles and CustomResourceDefinitions"
	@scripts/controller_gen.sh verify

.PHONY: gen.manifests-clean
gen.manifests-clean:
	@echo "==> Clean ClusterRoles and CustomResourceDefinitions"
	@scripts/controller_gen.sh clean

.PHONY: gen.deepcopy
gen.deepcopy:
	@echo "==> Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations"
	@scripts/controller_gen.sh deepcopy