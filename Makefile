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

# Build all by default, even if it's not first.
.DEFAULT_GOAL := all

.PHONY: all
all: fmt tidy vendor

# ==============================================================================
# Build options

ROOT_PACKAGE=github.com/iiiceoo/requeueip
VERSION_PACKAGE=github.com/iiiceoo/requeueip/internal/version

# ==============================================================================
# Includes

# Make sure include common.mk at the first include line.
include scripts/make-rules/common.mk 
include scripts/make-rules/golang.mk
include scripts/make-rules/image.mk
include scripts/make-rules/deploy.mk
include scripts/make-rules/gen.mk
include scripts/make-rules/release.mk
include scripts/make-rules/docs.mk
include scripts/make-rules/tools.mk

# ==============================================================================
# Usage

## fmt: Reformat package sources (exclude vendor dir if existed).
.PHONY: fmt
fmt: tools.verify.golines tools.verify.goimports
	@echo "==> Formating codes"
	@$(FIND) -type f -name '*.go' | $(XARGS) gofmt -s -w
	@$(FIND) -type f -name '*.go' | $(XARGS) goimports -w -local $(ROOT_PACKAGE)
	@$(FIND) -type f -name '*.go' | $(XARGS) golines -w --max-len=130 --reformat-tags --ignore-generated .
	@$(GO) mod edit -fmt

## tidy: go mod tidy
.PHONY: tidy
tidy:
	@echo "==> go mod tidy"
	@$(GO) mod tidy

## vendor: go mod vendor
.PHONY: vendor
vendor:
	@echo "==> go mod vendor"
	@$(GO) mod vendor

## build: Build source code for host platform.
.PHONY: build
build:
	@$(MAKE) go.build

## build.multiarch: Build source code for multiple platforms.
.PHONY: build.multiarch
build.multiarch:
	@$(MAKE) go.build.multiarch

## image: Build docker images for host arch.
.PHONY: image
image:
	@$(MAKE) image.build

## image.multiarch: Build( and push) docker images for multiple platforms.
.PHONY: image.multiarch
image.multiarch:
	@$(MAKE) image.build.multiarch

## push: Push docker images to registry.
.PHONY: push
push:
	@$(MAKE) image.push

## deploy: Deploy RequeueIP to kind cluster.
.PHONY: deploy
deploy:
	@$(MAKE) deploy.kind

## clean: Remove all files that are created by building and testing.
.PHONY: clean
clean:
	@echo "==> Cleaning all build and test outputs"
	@rm -rf $(OUTPUT_DIR)

## tools: Install dependent tools.
.PHONY: tools
tools:
	@$(MAKE) tools.install

## gen: Generate all necessary files, such as source code, artifacts.
.PHONY: gen
gen:
	@$(MAKE) gen.run

## release.major: Release a major version (v0.0.0 --> v1.0.0).
.PHONY: release.major
release.major:
	@$(MAKE) release.tag.major

## release.minor: Release a minor version (v0.0.0 --> v0.1.0).
.PHONY: release.minor
release.minor:
	@$(MAKE) release.tag.minor

## release.patch: Release a patch version (v0.0.0 --> v0.0.1).
.PHONY: release.patch
release.patch:
	@$(MAKE) release.tag.patch

## verify: Verify various lints or others, it is mainly used for CI.
.PHONY: verify
verify:
	@$(MAKE) gen.verify
	@$(MAKE) release.verify