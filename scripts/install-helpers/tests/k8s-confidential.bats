#!/usr/bin/env bats
# Copyright Kata Containers Contributors
#
# SPDX-License-Identifier: Apache-2.0
#

load "${BATS_TEST_DIRNAME}/tests_common.sh"

setup() {
    get_pod_config_dir
}

@test "Test unencrypted confidential container launch success and verify that we are running in a secure enclave." {
    # Start the service/deployment/pod
    oc apply -f "${pod_config_dir}/pod-confidential-unencrypted.yaml"

    # Retrieve pod name
    pod_name=$(oc get pod -o wide | grep "confidential-unencrypted" | awk '{print $1;}')

    # Check pod creation
    oc wait --for=condition=Ready --timeout=$timeout pod "${pod_name}"

    coco_enabled=""
    for i in {1..6}; do
        echo "Trying to ssh into ${pod_name}..."
        coco_enabled=$(oc rsh -t ${pod_name} $(get_remote_command_per_hypervisor ${TEE}) 2> /dev/null) && break
        echo "Failed to connect to pod ..."
        sleep 5
    done
    [ -z "$coco_enabled" ] && echo "Confidential compute is expected but not enabled." && exit 1
    echo "ssh client output: ${coco_enabled}"
}

teardown() {
    oc describe "pod/${pod_name}" || true
    oc delete -f "${pod_config_dir}/pod-confidential-unencrypted.yaml" || true
    rm "${pod_config_dir}/pod-confidential-unencrypted.yaml"
}
