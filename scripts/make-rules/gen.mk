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
gen.run: gen.manifests gen.deepcopy gen.oapi

.PHONY: gen.verify
gen.verify: gen.manifests.verify gen.oapi.verify

.PHONY: gen.clean
gen.clean: gen.manifests.clean gen.oapi.clean

.PHONY: gen.manifests
gen.manifests: tools.verify.controller-gen
	@echo "==> Generate ClusterRoles and CustomResourceDefinitions"
	@scripts/controller_gen.sh manifests

.PHONY: gen.manifests.verify
gen.manifests.verify: tools.verify.controller-gen
	@echo "==> Verify ClusterRoles and CustomResourceDefinitions"
	@scripts/controller_gen.sh verify

.PHONY: gen.manifests.clean
gen.manifests.clean: tools.verify.controller-gen
	@echo "==> Clean ClusterRoles and CustomResourceDefinitions"
	@scripts/controller_gen.sh clean

.PHONY: gen.deepcopy
gen.deepcopy: tools.verify.controller-gen
	@echo "==> Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations"
	@scripts/controller_gen.sh deepcopy

.PHONY: gen.oapi
gen.oapi: tools.verify.oapi-codegen
	@echo "==> Generate OpenAPI(v3) types, client, server code"
	@scripts/oapi_codegen.sh gen

.PHONY: gen.oapi.verify
gen.oapi.verify: tools.verify.oapi-codegen
	@echo "==> Verify OpenAPI(v3) types, client, server code"
	@scripts/oapi_codegen.sh verify

.PHONY: gen.oapi.clean
gen.oapi.clean: tools.verify.oapi-codegen
	@echo "==> Clean OpenAPI(v3) types, client, server code"
	@scripts/oapi_codegen.sh clean