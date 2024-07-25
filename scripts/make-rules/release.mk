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

HELM_REPO ?=
CHARTS_DIR := charts/requeueip

.PHONY: release.tag.%
release.tag.%: tools.verify.gsemver
	$(eval APP_VERSION := $(shell gsemver bump $*))
	@echo "==> Update charts version to $(APP_VERSION)"
	@sed -E -i 's?^version: .*?version: $(APP_VERSION)?g' $(CHARTS_DIR)/Chart.yaml
	@sed -E -i 's?^appVersion: .*?appVersion: $(APP_VERSION)?g' $(CHARTS_DIR)/Chart.yaml
	@$(MAKE) charts.docs

.PHONY: release.verify
release.verify: charts.tpl.verify charts.app.verify charts.docs.verify

.PHONY: release.push
release.push:
	$(HELM) push $(CHARTS_DIR) $(HELM_REPO)

.PHONY: charts.docs
charts.docs: tools.verify.helm-docs
	@echo "==> Update charts README.md"
	@helm-docs -l error

.PHONY: charts.docs.verify
charts.docs.verify: tools.verify.helm-docs
	@helm-docs -d -l error > $(OUTPUT_DIR)/charts-docs.md
	@trap "rm -f $(OUTPUT_DIR)/charts-docs.md" EXIT SIGINT; \
	diff -Naup $(CHARTS_DIR)/README.md $(OUTPUT_DIR)/charts-docs.md
	@echo "==> Latest charts README.md"

.PHONY: charts.app.verify
charts.app.verify:
	@grep -E "^appVersion: \"$(APP_VERSION)\"" $(CHARTS_DIR)/Chart.yaml &>/dev/null || \
	{ echo "Mismatched charts APP version"; exit 1; }
	@echo "==> Matched charts APP version $(APP_VERSION)"

.PHONY: charts.tpl.verify
charts.tpl.verify:
	@$(HELM) lint $(CHARTS_DIR) --quiet
	@echo "==> Syntactically correct charts templates"