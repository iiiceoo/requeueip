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

KIND := kind
HELM := helm
KUBECTL := kubectl

NODE_IMAGE ?= kindest/node:v1.29.2
KIND_CONFIG ?= scripts/kind/kind-config.yaml

.PHONY: deploy.kind
deploy.kind: kind.charts

.PHONY: deploy.clean
deploy.clean: kind.clean

.PHONY: kind.cluster
kind.cluster: tools.verify.kind
	$(eval EXIST := $(shell kind get clusters | grep -q $(GIT_BRANCH) && echo 0 || echo 1))
	@if [ $(EXIST) -ne 1 ]; then \
		echo "kind cluster $(GIT_BRANCH) already exist"; \
	else \
		$(KIND) create cluster --image $(NODE_IMAGE) --config $(KIND_CONFIG) --name $(GIT_BRANCH); \
	fi

.PHONY: kind.clean
kind.clean: tools.verify.kind
	@$(KIND) delete cluster --name $(GIT_BRANCH)

.PHONY: kind.use-context
kind.use-context:
	$(KUBECTL) config use-context kind-$(GIT_BRANCH)

.PHONY: kind.charts
kind.charts: kind.cluster
	@$(HELM) upgrade requeueip charts/requeueip/ -n kube-system \
	--wait --install --kube-context kind-$(GIT_BRANCH)