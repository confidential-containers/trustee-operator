#!/bin/bash

# Defaults
OCP_PULL_SECRET_LOCATION="${OCP_PULL_SECRET_LOCATION:-$HOME/pull-secret.json}"
MIRRORING=false
ADD_IMAGE_PULL_SECRET=false
GA_RELEASE=true
TDX=${TDX:-false}
ITA_KEY="${ITA_KEY:-}"
if [ -n "$ITA_KEY" ]; then
	TDX=true
fi
DEFAULT_IMAGE=quay.io/openshift_sandboxed_containers/kbs:v0.10.1
if [ -n "$ITA_KEY" ]; then
    DEFAULT_IMAGE+="-ita"
fi
TRUSTEE_IMAGE=${TRUSTEE_IMAGE:-$DEFAULT_IMAGE}

# Function to check if the oc command is available
function check_oc() {
    if ! command -v oc &>/dev/null; then
        echo "oc command not found. Please install the oc CLI tool."
        return 1
    fi
}

# Function to check if the jq command is available
function check_jq() {
    if ! command -v jq &>/dev/null; then
        echo "jq command not found. Please install the jq CLI tool."
        return 1
    fi
}

# Function to check if the openssl command is available
function check_openssl() {
    if ! command -v openssl &>/dev/null; then
        echo "openssl command not found. Please install the openssl CLI tool."
        return 1
    fi
}

# Function to wait for the operator deployment object to be ready
function wait_for_deployment() {
    local deployment=$1
    local namespace=$2
    local timeout=300
    local interval=5
    local elapsed=0
    local ready=0

    while [ $elapsed -lt $timeout ]; do
        ready=$(oc get deployment -n "$namespace" "$deployment" -o jsonpath='{.status.readyReplicas}')
        if [ "$ready" == "1" ]; then
            echo "Operator $deployment is ready"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
    done
    echo "Operator $deployment is not ready after $timeout seconds"
    return 1
}

# Function to wait for service endpoints IP to be available
function wait_for_service_ep_ip() {
    local service=$1
    local namespace=$2
    local timeout=300
    local interval=5
    local elapsed=0
    local ip=0

    while [ $elapsed -lt $timeout ]; do
        ip=$(oc get endpoints -n "$namespace" "$service" -o jsonpath='{.subsets[0].addresses[0].ip}')
        if [ -n "$ip" ]; then
            echo "Service $service IP is available"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
    done
    echo "Service $service IP is not available after $timeout seconds"
    return 1
}

# Function to wait for MachineConfigPool (MCP) to be ready
function wait_for_mcp() {
    local mcp=$1
    local timeout=900
    local interval=5
    local elapsed=0
    local ready=0
    while [ $elapsed -lt $timeout ]; do
        if [ "$statusUpdated" == "True" ] && [ "$statusUpdating" == "False" ] && [ "$statusDegraded" == "False" ]; then
            echo "MCP $mcp is ready"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
        statusUpdated=$(oc get mcp "$mcp" -o=jsonpath='{.status.conditions[?(@.type=="Updated")].status}')
        statusUpdating=$(oc get mcp "$mcp" -o=jsonpath='{.status.conditions[?(@.type=="Updating")].status}')
        statusDegraded=$(oc get mcp "$mcp" -o=jsonpath='{.status.conditions[?(@.type=="Degraded")].status}')
    done

}

# Function to set additional cluster-wide image pull secret
# Requires PULL_SECRET_JSON environment variable to be set
# eg. PULL_SECRET_JSON='{"my.registry.io": {"auth": "ABC"}}'
function add_image_pull_secret() {
    # Check if SECRET_JSON is set
    if [ -z "$PULL_SECRET_JSON" ]; then
        echo "PULL_SECRET_JSON environment variable is not set"
        echo "example PULL_SECRET_JSON='{\"my.registry.io\": {\"auth\": \"ABC\"}}'"
        return 1
    fi

    # Get the existing secret
    oc get -n openshift-config secret/pull-secret -ojson | jq -r '.data.".dockerconfigjson"' | base64 -d | jq '.' >cluster-pull-secret.json ||
        return 1

    # Add the new secret to the existing secret
    jq --argjson data "$PULL_SECRET_JSON" '.auths |= ($data + .)' cluster-pull-secret.json >cluster-pull-secret-mod.json || return 1

    # Set the image pull secret
    oc set data secret/pull-secret -n openshift-config --from-file=.dockerconfigjson=cluster-pull-secret-mod.json || return 1

}

#Function to create Trustee artefacts secret
function create_trustee_artefacts() {
    local kbs_cm="kbs-cm.yaml"
    local rvps_cm="rvps-cm.yaml"
    local resource_policy_cm="resource-policy-cm.yaml"
    local tdx_coco_as_cm=""
    local config="kbsconfig.yaml"
    if [ "$TDX" = "true" ]; then
        if [ -n "$ITA_KEY" ]; then
            kbs_cm="tdx-ita-$kbs_cm"
            resource_policy_cm="tdx-ita-$resource_policy_cm"
            config="tdx-ita-$config"

            sed -i -e "s/tBfd5kKX2x9ahbodKV1.../${ITA_KEY}/g" $kbs_cm
	else
            tdx_coco_as_cm="tdx-coco-as-cm.yaml"

            sed -i -e "s/\# tdxConfigSpec/tdxConfigSpec/g" $config
            sed -i -e "s/\#   kbsTdxConfigMapName/    kbsTdxConfigMapName/g" $config
        fi
    fi

    # Create secret
    openssl genpkey -algorithm ed25519 >privateKey
    openssl pkey -in privateKey -pubout -out publicKey

    # Create kbs-auth-public-key secret if it doesn't exist
    if ! oc get secret kbs-auth-public-key -n trustee-operator-system &>/dev/null; then
        oc create secret generic kbs-auth-public-key --from-file=publicKey -n trustee-operator-system || return 1
        echo "Secret kbs-auth-public-key created successfully"
    else
        echo "Secret kbs-auth-public-key already exists, skipping creation"
    fi

    # Create KBS configmap
    oc apply -f "$kbs_cm" || return 1

    # Create RVPS configmap
    oc apply -f "$rvps_cm" || return 1

    # Create resource policy configmap
    oc apply -f "$resource_policy_cm" || return 1

    # Create few secrets to serve via Trustee
    # Create kbsres1 secret only if it doesn't exist
    if ! oc get secret kbsres1 -n trustee-operator-system &>/dev/null; then

        oc create secret generic kbsres1 --from-literal key1=res1val1 -n trustee-operator-system || return 1
        echo "Secret kbsres1 created successfully"
    else
        echo "Secret kbsres1 already exists, skipping creation"
    fi

    # Create TDX configmap
    if [ -n "$tdx_coco_as_cm" ]; then
        oc apply -f "$tdx_coco_as_cm" || return 1
    fi

    # Create KBSConfig
    oc apply -f "$config" || return 1

}

# Function to apply the operator manifests
function apply_operator_manifests() {
    # Apply the manifests, error exit if any of them fail
    oc apply -f ns.yaml || return 1
    oc apply -f og.yaml || return 1
    if [[ "$GA_RELEASE" == "true" ]]; then
        oc apply -f subs-ga.yaml || return 1
    else
        oc apply -f trustee_catalog.yaml || return 1
        oc apply -f subs-upstream.yaml || return 1
    fi

}

# Function to apply the operator manifests
function override_trustee_image() {
    if [ -n "$TRUSTEE_IMAGE" ]; then
        CSV=$(oc get csv -n trustee-operator-system -o name -l operators.coreos.com/trustee-operator.trustee-operator-system)
        oc patch -n trustee-operator-system $CSV --type=json -p="[
        {
            "op": "replace",
            "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/1/env/1/value",
            "value": "$TRUSTEE_IMAGE"
        }
        ]"
    fi
}

# Function to workaround an operator issue, as the operator does not
# properly take the cluster wide proxy set into consideration.
set_proxy_vars_for_trustee_deployment() {
    CLUSTER_HTTPS_PROXY="$(oc get proxy/cluster -o jsonpath={.spec.httpsProxy})"
    CLUSTER_HTTP_PROXY="$(oc get proxy/cluster -o jsonpath={.spec.httpProxy})"
    CLUSTER_NO_PROXY="$(oc get proxy/cluster -o jsonpath={.spec.noProxy})"

    # Here were coming with the assumption that the env is always not there, which is
    # the case in a normal deployment.
    #
    # ideally, we should double check whether it's empty, and yatta yatta ... but if
    # this is just a workaround till the Operator knows how to properly do this.
    oc patch -n trustee-operator-system deployment trustee-deployment --type=json -p="[
    {
        "op": "add",
        "path": "/spec/template/spec/containers/0/env",
        "value": [
            { "name": "HTTPS_PROXY", "value": \""$CLUSTER_HTTPS_PROXY"\" },
            { "name": "HTTP_PROXY", "value": \""$CLUSTER_HTTP_PROXY"\" },
            { "name": "NO_PROXY", "value": \""$CLUSTER_NO_PROXY"\" }
         ]
    }
    ]"
}

# Function to uninstall the installed artifacts
# It won't delete the cluster
function uninstall() {

    echo "Uninstalling all the artifacts"

    # Delete kbsconfig cluster-kbsconfig if it exists
    oc get kbsconfig -n trustee-operator-system cluster-kbsconfig &>/dev/null
    return_code=$?
    if [ $return_code -eq 0 ]; then
        oc delete kbsconfig -n trustee-operator-system cluster-kbsconfig || return 1
    fi

    # Delete trustee-upstream-catalog CatalogSource if it exists
    oc get catalogsource trustee-upstream-catalog -n openshift-marketplace &>/dev/null
    return_code=$?
    if [ $return_code -eq 0 ]; then
        oc delete catalogsource trustee-upstream-catalog -n openshift-marketplace || return 1
    fi

    # Delete ImageTagMirrorSet trustee-registry if it exists
    oc get imagetagmirrorset trustee-registry &>/dev/null
    return_code=$?
    if [ $return_code -eq 0 ]; then
        oc delete imagetagmirrorset trustee-registry || return 1
    fi

    # Delete ImageDigestMirrorSet trustee-registry if it exists
    oc get imagedigestmirrorset trustee-registry &>/dev/null
    return_code=$?
    if [ $return_code -eq 0 ]; then
        oc delete imagedigestmirrorset trustee-registry || return 1
    fi

    # Delete the namespace trustee-operator-system if it exists
    oc get ns trustee-operator-system &>/dev/null
    return_code=$?
    if [ $return_code -eq 0 ]; then
        oc delete ns trustee-operator-system || return 1
    fi

    echo "Waiting for MCP to be READY"

    # Wait for sometime before checking for MCP
    sleep 10
    wait_for_mcp master || return 1
    wait_for_mcp worker || return 1

    echo "Uninstall completed successfully"
}

function display_help() {
    echo "Usage: install.sh [-h] [-m] [-s] [-b] [-u]"
    echo "Options:"
    echo "  -h Display help"
    echo "  -m Install the image mirroring config"
    echo "  -s Set additional cluster-wide image pull secret."
    echo "     Requires the secret to be set in PULL_SECRET_JSON environment variable"
    echo "     Example PULL_SECRET_JSON='{\"my.registry.io\": {\"auth\": \"ABC\"}}'"
    echo "  -b Use pre-ga operator bundles"
    echo "  -u Uninstall the installed artifacts"
    # Add some example usage options
    echo " "
    echo "Example usage:"
    echo "# Install the GA operator"
    echo " ./install.sh "
    echo " "
    echo "# Install the GA operator with ITA support"
    echo " ITA_KEY="tBfd5kKX2x9ahbodKV1..." ./install.sh"
    echo " "
    echo "# Install the GA operator with DCAP support"
    echo " TDX=true ./install.sh"
    echo " "
    echo "# Install the GA operator with image mirroring"
    echo " ./install.sh -m"
    echo " "
    echo "# Install the GA operator with additional cluster-wide image pull secret"
    echo " export PULL_SECRET_JSON='{"brew.registry.redhat.io": {"auth": "abcd1234"}, "registry.redhat.io": {"auth": "abcd1234"}}'"
    echo " ./install.sh -s"
    echo " "
    echo "# Install the GA operator with a custom trustee image"
    echo " TRUSTEE_IMAGE=<trustee-image> ./install.sh"
    echo " "
    echo "# Install the pre-GA operator with image mirroring and additional cluster-wide image pull secret"
    echo " ./install.sh -m -s -b"
    echo " "
    echo "# Deploy the pre-GA operator with image mirroring and additional cluster-wide image pull secret"
    echo " export PULL_SECRET_JSON='{"brew.registry.redhat.io": {"auth": "abcd1234"}, "registry.redhat.io": {"auth": "abcd1234"}}'"
    echo " ./install.sh -m -s -b"
    echo " "
}

while getopts "hmsbu" opt; do
    case $opt in
    h)
        display_help
        exit 0
        ;;
    m)
        echo "Mirroring option passed"
        # Set global variable to indicate mirroring option is passed
        MIRRORING=true
        ;;
    s)
        echo "Setting additional cluster-wide image pull secret"
        # Check if jq command is available
        ADD_IMAGE_PULL_SECRET=true
        ;;
    b)
        echo "Using non-ga operator bundles"
        GA_RELEASE=false
        ;;
    u)
        echo "Uninstalling"
        uninstall || exit 1
        exit 0
        ;;

    \?)
        echo "Invalid option: -$OPTARG" >&2
        display_help
        exit 1
        ;;
    esac
done

# Check if oc command is available
check_oc || exit 1

# Check if openssl command is available
check_openssl || exit 1

# Apply the operator manifests
apply_operator_manifests || exit 1

# If MIRRORING is true, then create the image mirroring config
if [ "$MIRRORING" = true ]; then
    echo "Creating image mirroring config"
    oc apply -f image_mirroring.yaml || exit 1

    # Sleep for sometime before checking MCP status
    sleep 10

    echo "Waiting for MCP to be ready"
    wait_for_mcp master || exit 1
    wait_for_mcp worker || exit 1
fi

# If ADD_IMAGE_PULL_SECRET is true, then add additional cluster-wide image pull secret
if [ "$ADD_IMAGE_PULL_SECRET" = true ]; then
    echo "Adding additional cluster-wide image pull secret"
    # Check if jq command is available
    check_jq || exit 1
    add_image_pull_secret || exit 1

    # Sleep for sometime before checking MCP status
    sleep 10

    echo "Waiting for MCP to be ready"
    wait_for_mcp master || exit 1
    wait_for_mcp worker || exit 1

fi

# Apply the operator manifests
apply_operator_manifests || exit 1

wait_for_deployment trustee-operator-controller-manager trustee-operator-system || exit 1

# Override trustee image
if [ "$TRUSTEE_IMAGE" != "" ]; then
    override_trustee_image || exit 1
fi
wait_for_deployment trustee-operator-controller-manager trustee-operator-system || exit 1

# Create Trustee artefacts
create_trustee_artefacts || exit 1

# Ensure that proxy is set via this script, at least till
# the operator does this in the proper way.
set_proxy_vars_for_trustee_deployment || exit 1

# Ensure we restart the deployment after the proxy patches were applied
oc rollout restart deployment -n trustee-operator-system trustee-deployment || exit 1
