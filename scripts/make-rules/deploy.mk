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

NODE_VERSION ?= v1.29.2
NODE_IMAGE ?= kindest/node:$(NODE_VERSION)
KIND_CONFIG ?= scripts/kind/kind-config.yaml

CALICO_VERSION ?= v3.28.x
CALICO_MANIFESTS ?= scripts/kind/calico/calico-$(CALICO_VERSION).yaml

.PHONY: deploy.kind
deploy.kind: kind.charts

.PHONY: deploy.clean
deploy.clean: kind.clean

.PHONY: kind.cluster
kind.cluster: tools.verify.kind
	$(eval EXIST := $(shell kind get clusters | grep -q $(GIT_BRANCH) && echo 0 || echo 1))
	@if [ $(EXIST) -ne 1 ]; then \
		echo "kind cluster $(GIT_BRANCH) already exist"; exit 0; \
	else \
		$(KIND) create cluster --image $(NODE_IMAGE) --config $(KIND_CONFIG) --name $(GIT_BRANCH); \
		echo "Install Calico $(CALICO_VERSION) ..."; \
		$(KUBECTL) apply -f $(CALICO_MANIFESTS); \
		echo "Wait all calico-node Pods ready ..."; \
		$(KUBECTL) wait po -l k8s-app=calico-node -n kube-system --for=condition=Ready --timeout 2m; \
	fi

.PHONY: kind.clean
kind.clean: tools.verify.kind
	@$(KIND) delete cluster --name $(GIT_BRANCH)

.PHONY: kind.use-context
kind.use-context:
	$(KUBECTL) config use-context kind-$(GIT_BRANCH)

.PHONY: kind.charts
kind.charts: kind.cluster
	@$(HELM) upgrade requeueip $(CHARTS_DIR) -n kube-system \
	--wait --timeout 1m --install --kube-context kind-$(GIT_BRANCH) \
	--set daemon.image.registry=$(REGISTRY_PREFIX) \
	--set daemon.image.tag=$(VERSION) \
	--set controller.image.registry=$(REGISTRY_PREFIX) \
	--set controller.image.tag=$(VERSION)