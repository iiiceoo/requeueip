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
CONTROLLER_GEN_TMP_DIR=${CONTROLLER_GEN_TMP_DIR:-${PROJECT_ROOT}/.controller_gen_tmp}
CODEGEN_PKG=${CODEGEN_PKG:-$(cd ${PROJECT_ROOT}; ls -d -1 ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen)}

# ENV
# Defines the output path for the artifacts controller-gen generates
OUTPUT_BASE_DIR=${OUTPUT_BASE_DIR:-${PROJECT_ROOT}/charts/requeueip}
# Defines tmp path of the current artifacts for diffing
OUTPUT_TMP_DIR=${OUTPUT_TMP_DIR:-${CONTROLLER_GEN_TMP_DIR}/old}
# Defines the output path of the latest artifacts for diffing
OUTPUT_DIFF_DIR=${OUTPUT_DIFF_DIR:-${CONTROLLER_GEN_TMP_DIR}/new}



controller-gen() {
  go run ${PROJECT_ROOT}/${CODEGEN_PKG}/main.go $@
}

manifests_clean() {
  rm -rf ${OUTPUT_BASE_DIR}/crds/*
  rm -rf ${OUTPUT_BASE_DIR}/templates/rbac/role.yaml
}

manifests_gen() {
  output_dir=$1

  controller-gen \
  crd:generateEmbeddedObjectMeta=true \
  rbac:roleName="__TEMPLATE__-admin" \
  paths="${PWD}/${PROJECT_ROOT}/api/v1" \
  output:crd:artifacts:config="${output_dir}/crds" \
  output:rbac:artifacts:config="${output_dir}/templates/rbac"
  
  sed -i 's/__TEMPLATE__/{{ include "requeueip.fullname" . }}/g' ${output_dir}/templates/rbac/role.yaml
}

deepcopy_gen() {
  tmp_header_file=${CONTROLLER_GEN_TMP_DIR}/boilerplate.go.txt
  cat ${PROJECT_ROOT}/scripts/boilerplate.txt | sed -E 's?(.*)?// \1?' > ${tmp_header_file}

  controller-gen \
    object:headerFile="${tmp_header_file}" \
    paths="${PWD}/${PROJECT_ROOT}/api/v1"
}

manifests_verify() {
  # Aggregate the artifacts currently in use
  mkdir -p ${OUTPUT_TMP_DIR}/templates/rbac
  if [ "$(ls -A ${OUTPUT_BASE_DIR}/crds)" ]; then
    cp -a ${OUTPUT_BASE_DIR}/crds ${OUTPUT_TMP_DIR}
  fi

  if [ "$(ls -A ${OUTPUT_BASE_DIR}/templates/rbac)" ]; then
    cp -a ${OUTPUT_BASE_DIR}/templates/rbac/role.yaml ${OUTPUT_TMP_DIR}/templates/rbac
  fi

  # Generator the latest artifacts
  manifests_gen ${OUTPUT_DIFF_DIR}

  # Diff
  ret=0
  diff -Naupr ${OUTPUT_TMP_DIR} ${OUTPUT_DIFF_DIR} || ret=$?

  if [[ $ret -eq 0 ]];then
    echo "The Artifacts is up to date."
  else
    echo "Error: The Artifacts is out of date! Please run 'make manifests'."
    exit 1
  fi
}

cleanup() {
  rm -rf ${CONTROLLER_GEN_TMP_DIR}
}

help() {
  controller-gen -h
}

main() {
  trap "cleanup" EXIT SIGINT
  cleanup
  mkdir -p ${CONTROLLER_GEN_TMP_DIR}

  case ${1:-none} in
    clean)
      manifests_clean
      ;;
    manifests)
      manifests_clean
      manifests_gen ${OUTPUT_BASE_DIR}
      ;;
    deepcopy)
      deepcopy_gen
      ;;
    verify)
      manifests_verify
      ;;
    *|help|-h|--help)
      help
      ;;
  esac
}

main "$*"