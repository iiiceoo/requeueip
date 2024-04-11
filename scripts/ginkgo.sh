#!/usr/bin/env bash

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

set -o errexit
set -o nounset
set -o pipefail

# CONST
PROJECT_ROOT=$(dirname ${BASH_SOURCE[0]})/..
GINKGO_PKG=${GINKGO_PKG:-$(cd ${PROJECT_ROOT}; ls -d -1 ./vendor/github.com/onsi/ginkgo/v2/ginkgo)}

ginkgo() {
  go run ${PROJECT_ROOT}/${GINKGO_PKG}/main.go $@
}

main() {
  ginkgo $@
}

main "$*"