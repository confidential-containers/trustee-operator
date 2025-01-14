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
