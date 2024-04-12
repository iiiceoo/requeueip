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

DEP_TOOLS ?= gsemver goimports golines golangci-lint
OTHER_TOOLS ?= ginkgo helm-docs kind

.PHONY: tools.install
tools.install: $(addprefix tools.install., $(DEP_TOOLS) ${OTHER_TOOLS})

.PHONY: tools.install.%
tools.install.%:
	@echo "==> Installing $*"
	@$(MAKE) install.$*

.PHONY: tools.verify.%
tools.verify.%:
	@if ! which $* &>/dev/null; then $(MAKE) tools.install.$*; fi

.PHONY: install.gsemver
install.gsemver:
	@$(GO) install github.com/arnaud-deprez/gsemver@latest

.PHONY: install.goimports
install.goimports:
	@$(GO) install golang.org/x/tools/cmd/goimports@latest

.PHONY: install.golines
install.golines:
	@$(GO) install github.com/segmentio/golines@latest

.PHONY: install.golangci-lint
install.golangci-lint:
	@$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.55.2
	@golangci-lint completion bash > $(HOME)/.golangci-lint.bash
	@if ! grep -q .golangci-lint.bash $(HOME)/.bashrc; then echo "source \$$HOME/.golangci-lint.bash" >> $(HOME)/.bashrc; fi

.PHONY: install.ginkgo
install.ginkgo:
	@$(GO) install github.com/onsi/ginkgo/v2/ginkgo@latest

.PHONY: install.helm-docs
install.helm-docs:
	@$(GO) install github.com/norwoodj/helm-docs/cmd/helm-docs@latest

.PHONY: install.kind
install.kind:
	@$(GO) install sigs.k8s.io/kind@v0.22.0

.PHONY: install.markdownlint-cli2
install.markdownlint-cli2:
	@npm install markdownlint-cli2 --global