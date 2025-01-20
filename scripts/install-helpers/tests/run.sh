#!/bin/bash
#
# Copyright Kata Containers Contributors
#
# SPDX-License-Identifier: Apache-2.0
#

set -e

TEE=${TEE}

[ -z "${TEE}" ] && echo "TEE env var must be set to either \"snp\" or \"tdx\"" && exit 1

cleanup() {
    # Clean up all node debugger pods whose name starts with `custom-node-debugger` if pods exist
    pods_to_be_deleted=$(oc get pods -n kube-system --no-headers -o custom-columns=:metadata.name \
        | grep '^custom-node-debugger' || true)
    [ -n "$pods_to_be_deleted" ] && oc delete pod -n kube-system $pods_to_be_deleted || true
}

trap cleanup EXIT

K8S_TEST_DEBUG="${K8S_TEST_DEBUG:-false}"

if [ -n "${K8S_TEST_UNION:-}" ]; then
    K8S_TEST_UNION=($K8S_TEST_UNION)
else
    K8S_TEST_UNION=( \
        "k8s-confidential.bats" \ 
        "k8s-confidential-attestation.bats" \
        "k8s-guest-pull-image-signature.bats" \
    )
fi

echo "Running tests with bats version: $(bats --version)"
echo "Running tests with yq: $(yq --version)"

for file in $(ls runtimeclass_workloads/*.in); do
    envsubst < ${file} > ${file%.in}
done

# Run the tests from the default namespace
oc project default || true

tests_fail=()
for K8S_TEST_ENTRY in ${K8S_TEST_UNION[@]}
do
    echo "$(oc get pods --all-namespaces 2>&1)"
    echo "Executing ${K8S_TEST_ENTRY}"
    if ! bats --show-output-of-passing-tests "${K8S_TEST_ENTRY}"; then
        tests_fail+=("${K8S_TEST_ENTRY}")
        [ "${K8S_TEST_FAIL_FAST}" = "yes" ] && break
    fi
done

[ ${#tests_fail[@]} -ne 0 ] && echo "Tests FAILED from suites: ${tests_fail[*]}" && exit 1

echo "All tests SUCCEEDED"
