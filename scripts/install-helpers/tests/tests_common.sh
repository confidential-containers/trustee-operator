#!/usr/bin/env bash
# Copyright Kata Containers Contributors
#
# SPDX-License-Identifier: Apache-2.0
#

timeout=90s

function get_remote_command_per_hypervisor() {
	tee="${1}"

	declare -A REMOTE_COMMAND_PER_HYPERVISOR
	REMOTE_COMMAND_PER_HYPERVISOR[snp]="dmesg | grep SEV-SNP"
	REMOTE_COMMAND_PER_HYPERVISOR[tdx]="cpuid | grep TDX_GUEST"

	echo "${REMOTE_COMMAND_PER_HYPERVISOR[${tee}]}"
}

function get_pod_config_dir() {
	pod_config_dir="${BATS_TEST_DIRNAME}/runtimeclass_workloads"
}

function build_kernel_params() {
    image_policy=${1:-} 

    preset_kernel_params=$(oc get mc 96-kata-kernel-config -o jsonpath='{.spec.config.storage.files[0].contents.source}' | cut -d, -f2 | base64 -d | grep -o '"[^"]*"' | sed 's/"//g')

    kernel_params=""
    if [ -n "$image_policy" ]; then
        kernel_params+=" agent.image_policy_file=${image_policy}"
        kernel_params+=" agent.enable_signature_verification=true"
    fi

    echo "$preset_kernel_params $kernel_params"
}

function assert_create_container_error() {
    pod_name="${1}"

    slept=0
    interval=5
    while true; do
        status=$(oc get pod "${pod_name}" -o jsonpath='{.status.containerStatuses[*].state.waiting.reason}')
        if [[ "$status" == "CreateContainerError" ]]; then
            break
        fi
        sleep $interval

        # Update elapsed time
        slept=$((slept + interval))

        # Break if timeout is reached
        if [ "$slept" -ge "$timeout" ]; then
            echo "Timeout reached while waiting for CreateContainerError."
            break
        fi
    done
}
