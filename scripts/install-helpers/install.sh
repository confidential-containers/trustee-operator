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

DEFAULT_IMAGE=quay.io/redhat-user-workloads/ose-osc-tenant/trustee/trustee:345aef3985efea5d4f91ffbffb597cb44087b96a
DEFAULT_TRUSTEE_OPERATOR_CSV=trustee-operator.v0.4.2

if [ -n "$ITA_KEY" ]; then
    DEFAULT_IMAGE+="-ita"
fi

TRUSTEE_IMAGE=${TRUSTEE_IMAGE:-$DEFAULT_IMAGE}
TRUSTEE_OPERATOR_CSV=${TRUSTEE_OPERATOR_CSV:-$DEFAULT_TRUSTEE_OPERATOR_CSV}

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

# Function to check if the git command is available
function check_git() {
    if ! command -v git &>/dev/null; then
        echo "git command not found. Please install git."
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

# Function to approve installPlan tied to specific CSV to be available in specific namespace
approve_installplan_for_target_csv() {
    local ns="$1"
    local target_csv="$2"
    local timeout=300
    local interval=5
    local elapsed=0

    echo "Waiting for InstallPlan with CSV '$target_csv' in namespace '$ns'..."

    while [ $elapsed -lt "$timeout" ]; do
        installplans=$(oc get installplan -n "$ns" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null)
        for ip in $installplans; do
            csvs=$(oc get installplan "$ip" -n "$ns" -o jsonpath="{.spec.clusterServiceVersionNames[*]}" 2>/dev/null)
            for csv in $csvs; do
                if [ "$csv" == "$target_csv" ]; then
                    echo "Found matching InstallPlan: $ip"
                    echo "Approving InstallPlan: $ip"
                    oc patch installplan "$ip" -n "$ns" -p '{"spec":{"approved":true}}' --type merge || return 1
                    return 0
                fi
            done
        done
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    echo "Timed out waiting for InstallPlan with CSV '$target_csv' in namespace '$ns'"
    return 1
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

    # Workaround the fact that the trustee-deployment isn't getting these from
    # the cluster-wide proxy deployment
    CLUSTER_HTTPS_PROXY="$(oc get proxy/cluster -o jsonpath={.spec.httpsProxy})"
    CLUSTER_HTTP_PROXY="$(oc get proxy/cluster -o jsonpath={.spec.httpProxy})"
    CLUSTER_NO_PROXY="$(oc get proxy/cluster -o jsonpath={.spec.noProxy})"

    echo "
  KbsEnvVars:
    HTTPS_PROXY: \"${CLUSTER_HTTPS_PROXY}\"
    HTTP_PROXY: \"${CLUSTER_HTTP_PROXY}\"
    NO_PROXY: \"${CLUSTER_NO_PROXY}\"" >> $config

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

function set_fbc_catalog_image() {
    latest_fbc_commit=$(git ls-remote https://github.com/openshift/trustee-fbc.git HEAD | cut -f 1)
    ocp_version=$(oc version --output json | jq '.openshiftVersion')
    image_prefix=quay.io/redhat-user-workloads/ose-osc-tenant
    if [[ "$ocp_version" =~ 4\.15.* ]] ;
    then
        FBC_IMAGE=$image_prefix/trustee-fbc-4-15/trustee-fbc-4-15
    elif [[ "$ocp_version" =~ 4\.16.* ]] ;
    then
        FBC_IMAGE=$image_prefix/trustee-fbc/trustee-fbc-4-16
    elif [[ "$ocp_version" =~ 4\.17.* ]] ;
    then
        FBC_IMAGE=$image_prefix/trustee-fbc-4-17
    elif [[ "$ocp_version" =~ 4\.18.* ]] ;
    then
        FBC_IMAGE=$image_prefix/trustee-fbc-4-18
    elif [[ "$ocp_version" =~ 4\.19.* ]] ;
    then
        FBC_IMAGE=$image_prefix/trustee-fbc-4-19
    elif [[ "$ocp_version" =~ 4\.20.* ]] ;
    then
        FBC_IMAGE=$image_prefix/trustee-fbc-4-20
    else
        echo "OCP version "$ocp_version" not supported yet!"
        exit 1
    fi
    export FBC_IMAGE="$FBC_IMAGE:$latest_fbc_commit"
}

# Function to apply the operator manifests
function apply_operator_manifests() {
    # Apply the manifests, error exit if any of them fail
    oc apply -f ns.yaml || return 1
    oc apply -f og.yaml || return 1
    if [[ "$GA_RELEASE" == "true" ]]; then
        oc apply -f subs-ga.yaml || return 1
        approve_installplan_for_target_csv trustee-operator-system "$TRUSTEE_OPERATOR_CSV" || return 1
    else
        set_fbc_catalog_image
        envsubst < "trustee_catalog.yaml.in" > "trustee_catalog.yaml"
        oc apply -f trustee_catalog.yaml || return 1
        rm -f trustee_catalog.yaml
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
            "path": "/spec/install/spec/deployments/0/spec/template/spec/containers/0/env/1/value",
            "value": "$TRUSTEE_IMAGE"
        }
        ]"
    fi
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

# Check if git command is available
check_git || exit 1

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
