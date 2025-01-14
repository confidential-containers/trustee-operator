#!/usr/bin/env bats
# Copyright Kata Containers Contributors
#
# SPDX-License-Identifier: Apache-2.0
#

load "${BATS_TEST_DIRNAME}/tests_common.sh"

setup() {
    get_pod_config_dir
}

@test "Get preset secret" {
    # Start the service/deployment/pod
    oc apply -f "${pod_config_dir}/pod-attestable.yaml"

    # Retrieve pod name
    pod_name=$(oc get pod -o wide | grep "aa-test-cc" | awk '{print $1;}')

    # Check pod creation
    oc wait --for=condition=Ready --timeout=$timeout pod "${pod_name}"

    oc exec "${pod_name}" -- curl "http://127.0.0.1:8006/cdh/resource/default/kbsres1/key1" | grep "res1val1"
}

teardown() {
    oc describe "pod/${pod_name}" || true
    oc delete -f "${pod_config_dir}/pod-attestable.yaml" || true
    rm "${pod_config_dir}/pod-attestable.yaml"
}
