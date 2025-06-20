# Introduction

The `trustee-operator` manages the lifecycle of [trustee](https://github.com/confidential-containers/trustee)
along with its configuration when deployed in a Kubernetes cluster

## Description

The operator manages a Kubernetes custom resource named: `KbsConfig`. Following are the key fields of the
`KbsConfig` custom resource definition

```golang
// KbsConfigSpec defines the desired state of KbsConfig
type KbsConfigSpec struct {
  // KbsConfigMapName is the name of the configmap that contains the KBS configuration
  KbsConfigMapName string `json:"kbsConfigMapName,omitempty"`

  // KbsAsConfigMapName is the name of the configmap that contains the KBS AS configuration
  // Required only when MicroservicesDeployment is set
  // +optional
  KbsAsConfigMapName string `json:"kbsAsConfigMapName,omitempty"`

  // KbsRvpsConfigMapName is the name of the configmap that contains the KBS RVPS configuration
  // Required only when MicroservicesDeployment is set
  // +optional
  KbsRvpsConfigMapName string `json:"kbsRvpsConfigMapName,omitempty"`

  // kbsRvpsRefValuesConfigMapName is the name of the configmap that contains the RVPS reference values
  KbsRvpsRefValuesConfigMapName string `json:"kbsRvpsRefValuesConfigMapName,omitempty"`

  // KbsAuthSecretName is the name of the secret that contains the KBS auth secret
  KbsAuthSecretName string `json:"kbsAuthSecretName,omitempty"`

  // KbsServiceType is the type of service to create for KBS
  // Default value is ClusterIP
  // +optional
  KbsServiceType corev1.ServiceType `json:"kbsServiceType,omitempty"`

  // KbsDeploymentType is the type of KBS deployment
  // It can assume one of the following values:
  //    AllInOneDeployment: all the KBS components will be deployed in the same container
  //    MicroservicesDeployment: all the KBS components will be deployed in separate containers
  // +kubebuilder:validation:Enum=AllInOneDeployment;MicroservicesDeployment
  // Default value is AllInOneDeployment
  // +optional
  KbsDeploymentType DeploymentType `json:"kbsDeploymentType,omitempty"`

  // KbsHttpsKeySecretName is the name of the secret that contains the KBS https private key
  KbsHttpsKeySecretName string `json:"kbsHttpsKeySecretName,omitempty"`

  // KbsHttpsCertSecretName is the name of the secret that contains the KBS https certificate
  KbsHttpsCertSecretName string `json:"kbsHttpsCertSecretName,omitempty"`

  // KbsSecretResources is an array of secret names that contain the keys required by clients
  // +optional
  KbsSecretResources []string `json:"kbsSecretResources,omitempty"`

  // KbsAttestationPolicyConfigMapName is the name of the configmap that contains the Attestation Policy
  // +optional
  KbsAttestationPolicyConfigMapName string `json:"kbsAttestationPolicyConfigMapName,omitempty"`

  // KbsResourcePolicyConfigMapName is the name of the configmap that contains the Resource Policy
  // +optional
  KbsResourcePolicyConfigMapName string `json:"kbsResourcePolicyConfigMapName,omitempty"`

  // TdxConfigSpec is the struct that hosts the TDX specific configuration
  // +optional
  TdxConfigSpec TdxConfigSpec `json:"tdxConfigSpec,omitempty"`

  // IbmSEConfigSpec is the struct that hosts the IBMSE specific configuration
  // +optional
  IbmSEConfigSpec IbmSEConfigSpec `json:"ibmSEConfigSpec,omitempty"`

  // KbsLocalCertCacheSpec is the struct for mounting local certificates into trustee file system
  kbsLocalCertCacheSpec kbsLocalCertCacheSpec `json:"kbsLocalCertCacheSpec,omitempty"`  

	// KbsDeploymentSpec is the struct for trustee deployment options
	KbsDeploymentSpec KbsDeploymentSpec `json:"KksDeploymentSpec,omitempty"`
}

// IbmSEConfigSpec defines the desired state for IBMSE configuration
type IbmSEConfigSpec struct {
  // certStorePvc is the name of the PeristentVolumeClaim where certificates/keys are mounted
  // +optional
  CertStorePvc string `json:"certStorePvc,omitempty"`
}

// TdxConfigSpec defines the desired state for TDX configuration
type TdxConfigSpec struct {
  // kbsTdxConfigMapName is the name of the configmap containing sgx_default_qcnl.conf file
  // +optional
  KbsTdxConfigMapName string `json:"kbsTdxConfigMapName,omitempty"`
}

// KbsLocalCertCacheSpec defines the configuration for mounting local certificates into trustee file system
type KbsLocalCertCacheSpec struct {
  // SecretName is the name of the secret that maps to a local directory containing the certificates
  // +optional
  SecretName string `json:"secretName,omitempty"`
  // MountPath is the destination path in the trustee file system
  // +optional
  MountPath string `json:"mountPath,omitempty"`
}

// KbsDeploymentSpec defines the configuration for trustee deployment
type KbsDeploymentSpec struct {
	// Number of desired trustee pods. This is a pointer to distinguish between explicit
	// zero and not specified. Defaults to 1.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}
```

>Note: the default deployment type is ```MicroservicesDeployment```. 
The examples below apply to this mode.

An example configmap for the KBS configuration looks like this:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kbs-config-grpc
  namespace: trustee-operator-system
data:
  kbs-config.toml: |
    [http_server]
    sockets = ["0.0.0.0:8080"]
    insecure_http = true
    [admin]
    insecure_api = true
    auth_public_key = "/etc/auth-secret/kbs.pem"

    [attestation_token]
    insecure_key = true

    [attestation_service]
    type = "coco_as_grpc"
    as_addr = "http://127.0.0.1:50004"

    [[plugins]]
    name = "resource"
    type = "LocalFs"
    dir_path = "/opt/confidential-containers/kbs/repository"

    [policy_engine]
    policy_path = "/opt/confidential-containers/opa/policy.rego"

```

If HTTPS support is not needed, please set `insecure_http=true` and no need to specify the attributes `private_key` and `certificate`.

An example configmap for AS config looks like this:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: as-config-grpc
  namespace: trustee-operator-system
data:
  as-config.json: |
    {
        "work_dir": "/opt/confidential-containers/attestation-service",
        "policy_engine": "opa",
        "rvps_config": {
          "type": "GrpcRemote",
          "address": "http://127.0.0.1:50003"
        },
        "attestation_token_broker": {
          "type": "Ear",
          "policy_dir": "/opt/confidential-containers/attestation-service/policies"
        },
        "attestation_token_config": {
          "duration_min": 5
        }
    }
```

Currently these configmaps needs to be created during deployment.
In subsequent releases we'll look into having these configmaps created by the operator based on user inputs.

A sample `KbsConfig` custom resource

```yaml
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:  
  name: kbsconfig-sample
  namespace: trustee-operator-system
spec:
  # KBS configuration
  kbsConfigMapName: kbs-config
  # AS configuration
  kbsAsConfigMapName: as-config  
  # RVPS configuration
  kbsRvpsConfigMapName: rvps-config-grpc
  # reference values config map
  kbsRvpsReferenceValuesMapName: rvps-reference-values
  # authentication secret
  kbsAuthSecretName: kbs-auth-public-key
  # service type
  kbsServiceType: ClusterIP
  # deployment type
  kbsDeploymentType: MicroservicesDeployment
  # HTTPS support
  kbsHttpsKeySecretName: kbs-https-key
  kbsHttpsCertSecretName: kbs-https-certificate
  # K8s Secrets to be made available to KBS clients
  kbsSecretResources: ["kbsres1"]
  # Attestation policy
  kbsAttestationPolicyConfigMapName: attestation-policy
  # Resource policy
  kbsResourcePolicyConfigMapName: resource-policy
  # TDX settings
  tdxConfigSpec:
    kbsTdxConfigMapName: tdx-config-sample
  # IBMSE settings
  ibmSEConfigSpec:
    certStorePvc: ibmse-pvc
  # Mount VCEK certificate for disconnected environment
  kbsLocalCertCacheSpec:
    secretName: vcek-secret
    mountPath: "/etc/kbs/snp/ek"
```

## Getting Started

You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.

>Note: Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster

Ensure you have `golang`, `kubectl`, `make` available in the `$PATH`.

#### Deploying prebuilt operator image

If you want to deploy latest prebuilt image, then run the following command:

```sh
make deploy IMG=quay.io/confidential-containers/trustee-operator:latest
```

Verify if the controller is running by executing the following command:

```sh
kubectl get pods -n trustee-operator-system --watch
```

You should see a similar output as below:

```sh
NAME                                                   READY   STATUS    RESTARTS   AGE
trustee-operator-controller-manager-6797b78467-zndkv   1/1     Running   0          111s
```

#### Deployment of CRDs, ConfigMaps and Secrets

This is an example deployment. Review the config files and change it as per your requirements.

```sh
cd config/samples/all-in-one
# or config/samples/microservices for the microservices mode

# create authentication keys
openssl genpkey -algorithm ed25519 > privateKey
openssl pkey -in privateKey -pubout -out kbs.pem

# create all the needed resources
kubectl apply -k .
```

Verify if the trustee deployment is running by executing the following command:

```sh
kubectl get pods -n trustee-operator-system --selector=app=kbs
```

You should see a similar output as below:

```sh
NAME                                  READY   STATUS    RESTARTS   AGE
trustee-deployment-78bd97f6d4-nxsbb   3/3     Running   0          4m3s
```

The default installation uses empty reference values. You must add real values by updating
the `rvps-reference-values` ConfigMap like shown in the example below:

``` yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: rvps-reference-values
  namespace: trustee-operator-system
data:
  reference-values.json: |
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: rvps-reference-values
      namespace: trustee-operator-system
    data:
      reference-values.json: |
        [
          {
            "name": "svn",
            "expired": "2026-01-01T00:00:00Z",
            "hash-value": [
              {
                "alg": "sha256",
                "value": "1"
              }
            ]
          }
        ]
```

The default installation creates a sample K8s secret named `kbsres1` to be made available to clients.
Take a look at [patch-kbs-resources.yaml](config/samples/microservices/patch-kbs-resources.yaml) and update it
with the K8s secrets that you want to deliver to clients via Trustee.

#### IBM Secure Execution

For IBM SE specific configuration, please refer to [ibmse.md](docs/ibmse.md).
  
#### ITA configuration

For Intel's ITA specific configuration, please refer to [ita.md](docs/ita.md).

### Mount certificates for disconnected environment

Please refer to [disconnected.md](docs/disconnected.md).

### Uninstallation

Ensure you are in the root folder of the project before running the uninstall commands.

#### Uninstall CRDs

To delete the CRDs from the cluster:

```sh
make uninstall
```

#### Undeploy controller

UnDeploy the controller from the cluster:

```sh
make undeploy
```

## Contributing

Contributions are most welcome. Please take a look at the [guide](https://github.com/confidential-containers/confidential-containers/blob/main/CONTRIBUTING.md) for more details.

### How it works

This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out

- Install the CRDs into the cluster.

  ```sh
  make install
  ```

- Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

  ```sh
  make run
  ```

>Note: You can also run this in one step by running: `make install run`

#### Building your own operator image

If using a remote Kubernetes cluster for testing, then you'll need to
build the controller image and deploy it.

- Export env variables.

  Set `REGISTRY` environment variable to point to your container registry.
  For example:

  ```sh
  export REGISTRY=quay.io/user
  ```

- Build and push your image to the location specified by `IMG`.

  ```sh
  make docker-build docker-push IMG=${REGISTRY}/trustee-operator:latest
  ```

  Change the tag from `latest` to any other based on your requirements.
  Also ensure that the image is public.

- Deploy the controller to the cluster with the image specified by `IMG`.

  ```sh
  make deploy IMG=${REGISTRY}/trustee-operator:latest
  ```

### Integration tests

An attestation with the sample-attester is performed in an ephemeral kind cluster:

Prerequisites:

- [kuttl](https://kuttl.dev/docs/cli.html#setup-the-kuttl-kubectl-plugin) plugin installed
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) installed

Optional: set the env variables KBS_IMAGE_NAME and CLIENT_IMAGE_NAME to override the default trustee/client images

  ```sh
  KBS_IMAGE_NAME=<trustee-image> CLIENT_IMAGE_NAME=<client-image> make test-e2e
  ```

### Modifying the API definitions

If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

>Note: Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright Confidential Containers Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

[http://www.apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0)

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
