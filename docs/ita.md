# Intel Trust Authority (ITA)

For details on how to enroll to Intel Trust Authority / Intel Tiber Trusted Services, please, visit:
https://www.intel.com/content/www/us/en/security/trust-authority.html

## OpenShift deployment

The following instructions are assuming an OpenShift cluster is running, and Trustee Operator already installed.

### Check Trustee Operator installation

Now it is time to check if the Trustee operator has been installed properly, by running the command:

```bash
oc get csv -n trustee-operator-system
```

We should expect something like:

```bash
NAME                      DISPLAY            VERSION   REPLACES   PHASE
trustee-operator.v0.1.0   Trustee Operator   0.1.0                Succeeded
```

### Use an ITA / ITTS capable KBS_IMAGE_NAME

```bash
oc -n trustee-operator-system edit trustee-operator.v0.1.0 
```

Search for the `KBS_IMAGE_NAME`, and replace the image used to `quay.io/fidencio/trustee:v0.10.1.1`.
Note that the image used consists in `ghcr.io/confidential-containers/key-broker-service:ita-as-v0.10.1` (the official released image for v0.10.1), plus one patch that avoids a breaking change on the ITA / ITTS side (https://github.com/confidential-containers/trustee/commit/10c689540305b39a951511fecf90babc444eb8a5)

## Configuration

The Trustee Operator configuration requires a few steps. Some of the steps are provided as an example, but you may want to customize the examples for your real requirements.

### Authorization key-pair generation

First of all, we’d need to create the key pairs for Trustee authorization. The public key is used by Trustee for client authorization, the private key is used by the client to prove its identity and register keys/secrets.

Create secret for client authorization:

```bash
openssl genpkey -algorithm ed25519 > privateKey
openssl pkey -in privateKey -pubout -out publicKey
oc create secret generic kbs-auth-public-key --from-file=publicKey -n trustee-operator-system
```

### Trustee ConfigMap object

This command will create the ConfigMap object that provides Trustee all the needed configuration.

Note: you'd probably need to change the Intel API key. Now it is set to `api_key": "tBfd5kKX2x9ahbodKV1..."`

```bash
oc apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: kbs-config
  namespace: trustee-operator-system
data:
  kbs-config.json: |
    {
        "insecure_http" : true,
        "sockets": ["0.0.0.0:8080"],
        "auth_public_key": "/etc/auth-secret/publicKey",
        "attestation_token_config": {
          "attestation_token_type": "Jwt",
          "trusted_certs_paths": ["https://portal.trustauthority.intel.com"]
        },
        "repository_config": {
          "type": "LocalFs",
          "dir_path": "/opt/confidential-containers/kbs/repository"
        },
        "as_config": {
          "work_dir": "/opt/confidential-containers/attestation-service",
          "policy_engine": "opa",
          "attestation_token_broker": "Simple",
          "attestation_token_config": {
            "duration_min": 5
          },
          "rvps_config": {
            "store_type": "LocalJson",
            "store_config": {
              "file_path": "/opt/confidential-containers/rvps/reference-values/reference-values.json"
            }
          }
        },
        "policy_engine_config": {
          "policy_path": "/opt/confidential-containers/opa/policy.rego"
        },
        "intel_trust_authority_config" : {
          "base_url": "https://api.trustauthority.intel.com",
          "api_key": "tBfd5kKX2x9ahbodKV1...",
          "certs_file": "https://portal.trustauthority.intel.com"
        }
    }
EOF
```

### Trustee attestation policies

#### RVPS reference values

```bash
oc apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: rvps-reference-values
  namespace: trustee-operator-system
data:
  reference-values.json: |
    [
    ]
EOF
```

#### Trustee resource policy

```bash
oc apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: resource-policy
  namespace: trustee-operator-system
data:
  policy.rego:
    package policy
    default allow = false
    allow {
      input["attester_type"] != "sample"
    }
EOF
```

### Create secrets

How to create secrets to be shared with the attested clients?
In this example we create a secret *kbsres1* with two entries. These resources (key1, key2) can be retrieved by the Trustee clients.
You can add more secrets as per your requirements.

```bash
oc create secret generic kbsres1 --from-literal key1=res1val1 --from-literal key2=res1val2 -n trustee-operator-system
```

### Create KbsConfig CRD

Finally, the CRD for the operator is created:

```bash
oc apply -f - << EOF
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:
  labels:
    app.kubernetes.io/name: kbsconfig
    app.kubernetes.io/instance: kbsconfig
    app.kubernetes.io/part-of: trustee-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: trustee-operator
  name: kbsconfig
  namespace: trustee-operator-system
spec:
  kbsConfigMapName: kbs-config-cm
  kbsAuthSecretName: kbs-auth-public-key
  kbsDeploymentType: AllInOneDeployment
  kbsRvpsRefValuesConfigMapName: rvps-reference-values
  kbsSecretResources: ["kbsres1"]
  kbsServiceType: NodePort
  kbsResourcePolicyConfigMapName: resource-policy
EOF
```

### Set default project

```bash
oc project trustee-operator-system
```

### Check if the PODs are running

```bash
oc get pods -n trustee-operator-system
NAME                                                   READY   STATUS    RESTARTS   AGE
trustee-deployment-7bdc6858d7-bdncx                    1/1     Running   0          69s
trustee-operator-controller-manager-6c584fc969-8dz2d   2/2     Running   0          4h7m
```

Also, the log should report something like:

```bash
POD_NAME=$(kubectl get pods -l app=kbs -o jsonpath='{.items[0].metadata.name}' -n trustee-operator-system)
oc logs -n trustee-operator-system $POD_NAME
[2024-06-10T13:38:01Z INFO  kbs] Using config file /etc/kbs-config/kbs-config.json
[2024-06-10T13:38:01Z WARN  attestation_service::rvps] No RVPS address provided and will launch a built-in rvps
[2024-06-10T13:38:01Z INFO  attestation_service::token::simple] No Token Signer key in config file, create an ephemeral key and without CA pubkey cert
[2024-06-10T13:38:01Z INFO  api_server] Starting HTTPS server at [0.0.0.0:8080]
[2024-06-10T13:38:01Z INFO  actix_server::builder] starting 12 workers
[2024-06-10T13:38:01Z INFO  actix_server::server] Tokio runtime found; starting in existing Tokio runtime
```
