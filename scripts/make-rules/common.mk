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

SHELL := /bin/bash

# ROOT_DIR
COMMON_SELF_DIR := $(dir $(lastword $(MAKEFILE_LIST)))

ifeq ($(origin ROOT_DIR),undefined)
ROOT_DIR := $(abspath $(shell cd $(COMMON_SELF_DIR)/../.. && pwd -P))
endif

# OUTPUT_DIR
ifeq ($(origin OUTPUT_DIR),undefined)
OUTPUT_DIR := $(ROOT_DIR)/_output
$(shell mkdir -p $(OUTPUT_DIR))
endif

# VERSION
ifeq ($(origin VERSION), undefined)
VERSION := $(shell git describe --tags --always --match='v*')
endif

# APP VERSION
ifeq ($(origin APP_VERSION), undefined)
APP_VERSION := $(shell git describe --abbrev=0 --tags --match='v*' | tr -d 'v')
endif

# Copy githook scripts when execute makefile.
ifneq ($(wildcard .git/hooks),)
COPY_GITHOOK := $(shell cp -f scripts/githooks/* .git/hooks/)
endif

# Show current commit ID.
GIT_COMMIT := $(shell git rev-parse HEAD)

# Show current branch name.
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD)

# Set a specific PLATFORM.
PLATFORMS ?= linux_amd64 linux_arm64
ifeq ($(origin PLATFORM), undefined)
	ifeq ($(origin GOOS), undefined)
		GOOS := $(shell go env GOOS)
	endif
	ifeq ($(origin GOARCH), undefined)
		GOARCH := $(shell go env GOARCH)
	endif
	PLATFORM := $(GOOS)_$(GOARCH)
	# Use linux as the default OS when building images.
	IMAGE_PLAT := linux_$(GOARCH)
else
	GOOS := $(word 1, $(subst _, ,$(PLATFORM)))
	GOARCH := $(word 2, $(subst _, ,$(PLATFORM)))
	IMAGE_PLAT := $(PLATFORM)
endif

# Linux command settings.
FIND := find . ! -path './vendor/*'
XARGS := xargs --no-run-if-empty

# List --> String
COMMA := ,
EMPTY :=
SPACE := $(EMPTY) $(EMPTY)