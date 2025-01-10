# Introduction

These are helper scripts to setup Trustee operator on OpenShift cluster

## Prerequisites

- `oc`, `jq` and `openssl` CLI

## Install Trustee operator GA release

- Update `startingCSV` key in the `subs-ga.yaml` file to use the GA release you need.

- Kickstart the installation by running the following:

```sh
./install.sh
```

This will install the Trustee operator and create `KbsConfig` CRD with an opinionated
configuration. Specifically it will create a key-pair for authentication to KBS,
create configMaps for KBS, RVPS, resource policy and a sample K8s secret `kbsres1`.
All the resources will be created under the `trustee-operator-system` namespace.

## Install Trustee operator pre-GA release

- Update trustee_catalog.yaml to point to the pre-GA catalog
  
- The pre-GA build images are in an authenticated registry, so you'll need to
  set the `PULL_SECRET_JSON` variable with the registry credentials. Following is an example:

  ```sh
  export PULL_SECRET_JSON='{"brew.registry.redhat.io": {"auth": "abcd1234"}, "registry.redhat.io": {"auth": "abcd1234"}}'
  ```

- Kickstart the installation by running the following:

  ```sh
  ./install.sh -m -s -b
  ```

  This will deploy the pre-GA release of Trustee operator.

## Un-installation

Run the following command to uninstall

```sh
./install.sh -u
```

## Optional configurations

### Permissive resource policy

You can edit the `resource-policy` configMap and set `default allow = true`.

### TDX configuration

Create the TDX configmap

```sh
oc apply -f tdx-coco-as-cm.yaml
```

Update the KbsConfig CR

```sh
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:
  labels:
    app.kubernetes.io/name: kbsconfig
    app.kubernetes.io/instance: kbsconfig
    app.kubernetes.io/part-of: trustee-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: trustee-operator
  name: cluster-kbsconfig
  namespace: trustee-operator-system
spec:
  kbsConfigMapName: kbs-config-cm
  kbsAuthSecretName: kbs-auth-public-key
  kbsDeploymentType: AllInOneDeployment
  kbsRvpsRefValuesConfigMapName: rvps-reference-values
  kbsSecretResources: ["kbsres1"]
  kbsResourcePolicyConfigMapName: resource-policy

  # TDX specific configuration
  tdxConfigSpec:
     kbsTdxConfigMapName: tdx-config
 
  ```

### Custom attestation policy

Create the attestation policy configmap

```sh
oc apply -f attestation-policy.yaml
```

Update the CR

```sh
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:
  labels:
    app.kubernetes.io/name: kbsconfig
    app.kubernetes.io/instance: kbsconfig
    app.kubernetes.io/part-of: trustee-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: trustee-operator
  name: cluster-kbsconfig
  namespace: trustee-operator-system
spec:
  kbsConfigMapName: kbs-config-cm
  kbsAuthSecretName: kbs-auth-public-key
  kbsDeploymentType: AllInOneDeployment
  kbsRvpsRefValuesConfigMapName: rvps-reference-values
  kbsSecretResources: ["kbsres1"]  
  kbsResourcePolicyConfigMapName: resource-policy

  # Override attestation policy (optional)
  kbsAttestationPolicyConfigMapName: attestation-policy
```