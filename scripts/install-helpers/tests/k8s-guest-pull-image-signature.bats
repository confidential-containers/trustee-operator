#!/usr/bin/env bats
# Copyright Kata Containers Contributors
#
# SPDX-License-Identifier: Apache-2.0
#

load "${BATS_TEST_DIRNAME}/tests_common.sh"

setup() {
    original_secrets=$(oc -n trustee-operator-system get kbsconfig -o jsonpath="{.items[0].spec.kbsSecretResources}" | tr -d '[]" ') 

    oc -n trustee-operator-system delete secrets cosign-public-key security-policy || true

    UNSIGNED_UNPROTECTED_REGISTRY_IMAGE="quay.io/prometheus/busybox:latest"
    UNSIGNED_PROTECTED_REGISTRY_IMAGE="ghcr.io/confidential-containers/test-container-image-rs:unsigned"
    COSIGN_SIGNED_PROTECTED_REGISTRY_IMAGE="ghcr.io/confidential-containers/test-container-image-rs:cosign-signed"
    COSIGNED_SIGNED_PROTECTED_REGISTRY_WRONG_KEY_IMAGE="ghcr.io/confidential-containers/test-container-image-rs:cosign-signed-key2"
    SECURITY_POLICY_KBS_URI="kbs:///default/security-policy/test"

    get_pod_config_dir

    test_pod="${pod_config_dir}/pod-signed-tests.yaml"
    pod_name="signed-image-tests"
}

function ensure_kbs_has_up_to_date_secrets() {
    secret_path="$1"
    secret_value="$2"

    slept=0
    interval=5
    while true; do
        trustee_deployment_pod=""
        while [ -z "$trustee_deployment_pod" ]; do
            trustee_deployment_pod=$(oc -n trustee-operator-system get pods --sort-by=.metadata.creationTimestamp -o NAME | grep trustee-deployment | tail -n1)
        done

        trustee_secret=$(oc -n trustee-operator-system exec -it $trustee_deployment_pod -- cat /opt/confidential-containers/kbs/repository/default/$secret_path | tr -d '\r')
        if [ "$trustee_secret" == "$secret_value" ]; then
            break
        fi
        sleep $interval

        # Update elapsed time
        slept=$((slept + interval))

        # Break if timeout is reached
        if [ "$slept" -ge "120" ]; then
            echo "Timeout reached while waiting for Trustee to have the up to date secrets value."
            exit 1
        fi
    done
}

function setup_kbs_image_policy() {
    default_policy="${1:-insecureAcceptAnything}"
    policy_json=$(cat << EOF
{
    "default": [
        {
            "type": "${default_policy}"
        }
    ],
    "transports": {
        "docker": {
            "ghcr.io/confidential-containers/test-container-image-rs": [
                {
                    "type": "sigstoreSigned",
                    "keyPath": "kbs:///default/cosign-public-key/test"
                }
            ],
            "quay.io/prometheus": [
                {
                    "type": "insecureAcceptAnything"
                }
            ]
        }
    }
}
EOF
    )

    # This public key is corresponding to a private key that was generated to test signed images in image-rs CI.
    public_key=$(curl -sSL "https://raw.githubusercontent.com/confidential-containers/guest-components/075b9a9ee77227d9d92b6f3649ef69de5e72d204/image-rs/test_data/signature/cosign/cosign1.pub")

    oc create secret generic security-policy --from-literal test="${policy_json}" -n trustee-operator-system
    oc create secret generic cosign-public-key --from-literal test="${public_key}" -n trustee-operator-system

    oc patch KbsConfig cluster-kbsconfig -n trustee-operator-system --type=json -p="[
    {
        "op": "replace",
        "path": "/spec/kbsSecretResources",
        "value": "[${original_secrets},security-policy,cosign-public-key]"
    }
    ]"

    ensure_kbs_has_up_to_date_secrets "security-policy/test" "${policy_json}"
    ensure_kbs_has_up_to_date_secrets "cosign-public-key/test" "${public_key}"
}

@test "Create a pod from an unsigned image, on an insecureAcceptAnything registry works" {
    # We want to set the default policy to be reject to rule out false positives
    setup_kbs_image_policy "reject"

    export CONTAINER_IMAGE="${UNSIGNED_UNPROTECTED_REGISTRY_IMAGE}"
    export KERNEL_PARAMS=$(build_kernel_params "${SECURITY_POLICY_KBS_URI}")

    envsubst < "${test_pod}.in" > "${test_pod}" 

    cat "${test_pod}"

    oc apply -f "${test_pod}"
    oc wait --for=condition=Ready --timeout=$timeout pod "${pod_name}"
}

@test "Create a pod from an unsigned image, on a 'restricted registry' is rejected" {
    # We want to leave the default policy to be insecureAcceptAnything to rule out false negatives
    setup_kbs_image_policy

    export CONTAINER_IMAGE="${UNSIGNED_PROTECTED_REGISTRY_IMAGE}"
    export KERNEL_PARAMS=$(build_kernel_params "${SECURITY_POLICY_KBS_URI}")

    envsubst < "${test_pod}.in" > "${test_pod}" 

    cat "${test_pod}"

    oc apply -f "${test_pod}"
    assert_create_container_error "${pod_name}"
    oc describe pod "${pod_name}" | grep "Security validate failed: Validate image failed: Cannot pull manifest"
}

@test "Create a pod from a signed image, on a 'restricted registry' is successful" {
    # We want to set the default policy to be reject to rule out false positives
    setup_kbs_image_policy "reject"

    export CONTAINER_IMAGE="${COSIGN_SIGNED_PROTECTED_REGISTRY_IMAGE}"
    export KERNEL_PARAMS=$(build_kernel_params "${SECURITY_POLICY_KBS_URI}")

    envsubst < "${test_pod}.in" > "${test_pod}" 

    cat "${test_pod}"

    oc apply -f "${test_pod}"
    oc wait --for=condition=Ready --timeout=$timeout pod "${pod_name}"
}

@test "Create a pod from a signed image, on a 'restricted registry', but with the wrong key is rejected" {
    # We want to leave the default policy to be insecureAcceptAnything to rule out false negatives
    setup_kbs_image_policy

    export CONTAINER_IMAGE="${COSIGNED_SIGNED_PROTECTED_REGISTRY_WRONG_KEY_IMAGE}"
    export KERNEL_PARAMS=$(build_kernel_params "${SECURITY_POLICY_KBS_URI}")

    envsubst < "${test_pod}.in" > "${test_pod}" 

    cat "${test_pod}"

    oc apply -f "${test_pod}"
    assert_create_container_error "${pod_name}"
    oc describe pod "${pod_name}" | grep "Security validate failed: Validate image failed: \[PublicKeyVerifier"
}

@test "Create a pod from an unsigned image, on a 'restricted registry' works if policy files are not set" {
    # We want to set the default policy to be reject to rule out false positives
    setup_kbs_image_policy "reject"

    export CONTAINER_IMAGE="${UNSIGNED_PROTECTED_REGISTRY_IMAGE}"
    export KERNEL_PARAMS=$(build_kernel_params)

    envsubst < "${test_pod}.in" > "${test_pod}" 

    cat "${test_pod}"

    oc apply -f "${test_pod}"
    oc wait --for=condition=Ready --timeout=$timeout pod "${pod_name}"
}

teardown() {
    oc -n trustee-operator-system delete secrets cosign-public-key security-policy || true

    oc describe pod "${pod_name}" || true
    oc delete -f "${test_pod}" || true

    rm "${test_pod}"

    oc patch KbsConfig cluster-kbsconfig -n trustee-operator-system --type=json -p="[
    {
        "op": "replace",
        "path": "/spec/kbsSecretResources",
        "value": "[${original_secrets}]"
    }
    ]"
}
