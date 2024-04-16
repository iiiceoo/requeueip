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

DOCKER := docker

REGISTRY_PREFIX ?= ghcr.io/sauto4/requeueip
BUILDER_IMAGE ?= golang:$(GO_VERSION)
BASE_IMAGE ?= gcr.io/distroless/static:nonroot

# Determine image files by looking into images/*/Dockerfile.
IMAGES_DIR ?= $(wildcard ${ROOT_DIR}/images/*)
# Determine images names by stripping out the dir names.
IMAGES ?= $(filter-out tools,$(foreach image,${IMAGES_DIR},$(notdir ${image})))

ifeq (${IMAGES},)
  $(error Could not determine IMAGES, set ROOT_DIR or run in source dir)
endif

.PHONY: image.build
image.build: $(addprefix image.build., $(addprefix $(IMAGE_PLAT)., $(IMAGES)))

.PHONY: image.build.multiarch
image.build.multiarch: $(addprefix image.build., $(addprefix $(subst $(SPACE),$(COMMA),$(PLATFORMS))., $(IMAGES)))

.PHONY: image.build.%
image.build.%:
	$(eval IMAGE := $(word 2,$(subst ., ,$*)))
	$(eval PLATFORMS := $(word 1,$(subst ., ,$*)))
	$(eval IMAGE_PLATS := $(subst _,/,$(PLATFORMS)))
	$(eval BUILD_SUFFIX := -f $(ROOT_DIR)/images/$(IMAGE)/Dockerfile)
	$(eval BUILD_SUFFIX += --build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) --build-arg BASE_IMAGE=$(BASE_IMAGE))
	$(eval BUILD_SUFFIX += -t $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION) $(ROOT_DIR))
	@echo "==> Building docker image $(IMAGE) $(VERSION) for $(IMAGE_PLATS)"
	$(DOCKER) buildx build --platform $(IMAGE_PLATS) $(BUILD_SUFFIX) $(if $(findstring $(COMMA),$(IMAGE_PLATS)),--push --builder multi-platform,--load --builder default) 

.PHONY: image.push
image.push: $(addprefix image.push., $(IMAGES))

.PHONY: image.push.%
image.push.%:
	$(eval IMAGE := $*)
	@echo "==> Pushing image $(IMAGE) $(VERSION) to $(REGISTRY_PREFIX)"
	$(DOCKER) push $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION)

.PHONY: image.load
image.load: $(addprefix image.load., $(IMAGES))

.PHONY: image.load.%
image.load.%: kind.cluster
	$(eval IMAGE := $*)
	@echo "==> Loading image $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION) to kind cluster $(GIT_BRANCH)"
	$(KIND) load docker-image $(REGISTRY_PREFIX)/$(IMAGE):$(VERSION) -n $(GIT_BRANCH)