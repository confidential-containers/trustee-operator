# IBM Secure Execution (SE)

## Download certificate and keys

In order to Trustee to work properly with IBM SE, a directory containing certificates and keys needs to be mounted in the trustee pod file system.
More information about the IBM download process can be found [here](https://github.com/confidential-containers/trustee/blob/main/deps/verifier/src/se/README.md#download-certs-crls).

By the end of the aforementioned procedure, you should end up having a directory like the following:

```
├── certs
│   ├── ibm-z-host-key-signing-gen2.crt
|   └── DigiCertCA.crt
├── crls
│   └── ibm-z-host-key-gen2.crl
│   └── DigiCertTrustedRootG4.crl
│   └── DigiCertTrustedG4CodeSigningRSA4096SHA3842021CA1.crl
├── hdr
│   └── hdr.bin
├── hkds
│   └── HKD-3931-0275D38.crt
└── rsa
    ├── encrypt_key.pem
    └── encrypt_key.pub
```

Place this directory at `/opt/confidential-containers/ibmse` on every worker node that will run the trustee pod, and ensure the correct permissions are set:

```bash
sudo chmod -R 755 /opt/confidential-containers/ibmse/
```

---

## Configuring IBM SE via TrusteeConfig (recommended)

The `TrusteeConfig` CR provides the simplest way to deploy trustee for IBM SE. When `ibmSE` is set, the operator:

- Creates a `PersistentVolumeClaim` named `<trusteeconfig-name>-ibmse-certstore-pvc`, bound to the PV specified in `ibmSE.pvName`, and wires it into the generated `KbsConfig`
- Skips CPU/GPU attestation policy ConfigMaps, which are not applicable to IBM SE

> **Note:** The `PersistentVolume` is **not** created or deleted by the operator. It must be pre-created by the cluster administrator before applying the `TrusteeConfig` (see Step 1 below). `PersistentVolume` is a cluster-scoped Kubernetes resource and cannot be owned by a namespace-scoped CR.

### Step 1 – Create the PersistentVolume

Apply the following manifest once per cluster. You can use the sample at `config/samples/all-in-one/ibmse-pv.yaml` as a starting point.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ibmse-pv
spec:
  capacity:
    storage: 100Mi
  accessModes:
    - ReadOnlyMany
  storageClassName: ""
  local:
    path: /opt/confidential-containers/ibmse
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
            - key: node-role.kubernetes.io/worker
              operator: Exists
```

### Step 2 – Apply the TrusteeConfig

Set `ibmSE.pvName` to the name of the PV created in Step 1. The operator will create a PVC named `trusteeconfig-ibmse-ibmse-certstore-pvc` that binds to it.

```yaml
apiVersion: confidentialcontainers.org/v1alpha1
kind: TrusteeConfig
metadata:
  name: trusteeconfig-ibmse
  namespace: trustee-operator-system
spec:
  ibmSE:
    pvName: ibmse-pv
  profileType: Restricted
  httpsSpec:
    tlsSecretName: kbs-https-certificate
  kbsServiceType: NodePort
```

### Step 3 – Update the IBM SE policy ConfigMap

Update the resource policy ConfigMap by following the sample at `config/templates/resource-policy-ibm.rego`.

> **Note:** Replace `<se.attestation_phkh>`, `<se.image_phkh>`, and `<se.tag>` with the values for your workload. Refer to [Retrieve-the-attestation-policy-fields-for-ibm-se](https://github.com/confidential-containers/trustee/blob/main/deps/verifier/src/se/README.md#retrive-the-attestation-policy-fields-for-ibm-se) for details.

---

## Configuring IBM SE via KbsConfig (advanced)

If you manage the `KbsConfig` resource directly (without `TrusteeConfig`), you must create the PV, PVC, and all ConfigMaps manually.

### Persistent Volume

Create a `PersistentVolume` backed by the local IBM SE certificate directory (cluster-scoped, no namespace):

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ibmse-pv
spec:
  capacity:
    storage: 100Mi
  accessModes:
    - ReadOnlyMany
  storageClassName: ""
  local:
    path: /opt/confidential-containers/ibmse
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
            - key: node-role.kubernetes.io/worker
              operator: Exists
```

### PersistentVolumeClaim

Create the PVC in the same namespace as the `KbsConfig`. Setting `volumeName` ensures static binding to the PV above.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ibmse-pvc
  namespace: trustee-operator-system
spec:
  accessModes:
    - ReadOnlyMany
  storageClassName: ""
  volumeName: ibmse-pv
  resources:
    requests:
      storage: 100Mi
```

### KbsConfig CR

```yaml
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:
  name: kbsconfig-sample
  namespace: trustee-operator-system
spec:
  # omitted all the rest of config
  # ...
  kbsResourcePolicyConfigMapName: ibmse-resource-policy
  kbsServiceType: NodePort
  # IBMSE settings
  ibmSEConfigSpec:
    certStorePvc: ibmse-pvc
```

**Notes:**

- `kbsResourcePolicyConfigMapName` must reference a ConfigMap whose `policy.rego` follows the `config/templates/resource-policy-ibm.rego` template.
- `certStorePvc` must match the PVC name created above.
- If HTTPS is enabled, include the worker node IPs in the `[alt_names]` section of your certificate. See [Generate a self-signed certificate](https://github.com/confidential-containers/trustee/blob/main/kbs/docs/self-signed-https.md#generate-a-self-signed-certificate) for details.
  ```ini
  [alt_names]
  DNS.1 = kbs-service
  IP.1 = <ocp-worker-node-0-ip>
  IP.2 = <ocp-worker-node-1-ip>
  ```
