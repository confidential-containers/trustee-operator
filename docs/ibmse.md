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

## Configuring IBM SE via TrusteeConfig (recommended)

The `TrusteeConfig` CR provides the simplest way to deploy trustee for IBM SE. When `teeType: IbmSel` is specified, the operator **automatically**:

- Creates a `PersistentVolume` (`<name>-ibmse-pv`) backed by the local path `/opt/confidential-containers/ibmse` on worker nodes
- Creates a `PersistentVolumeClaim` (`<name>-ibmse-certstore-pvc`) and wires it into the generated `KbsConfig`
- Skips CPU/GPU attestation policy ConfigMaps, which are not applicable to IBM SE

### Step 1 – Apply the TrusteeConfig

```yaml
apiVersion: confidentialcontainers.org/v1alpha1
kind: TrusteeConfig
metadata:
  name: trusteeconfig-ibmse
  namespace: trustee-operator-system
spec:
  teeType: IbmSel
  profileType: Restricted
  httpsSpec:
    tlsSecretName: kbs-https-certificate
  kbsServiceType: NodePort
```

### Step 2 – Update the IBM SE policy ConfigMap

Update the resource policy configmap by following sample `config/templates/resource-policy-ibm.rego`.

> **Note:** Replace `<se.attestation_phkh>`, `<se.image_phkh>`, and `<se.tag>` with the values for your workload. Refer to [Retrive-the-attestation-policy-fields-for-ibm-se](https://github.com/confidential-containers/trustee/blob/main/deps/verifier/src/se/README.md#retrive-the-attestation-policy-fields-for-ibm-se) for details.

---

## Configuring IBM SE via KbsConfig (advanced)

If you manage the `KbsConfig` resource directly (without `TrusteeConfig`), you must create the PV, PVC, and all ConfigMaps manually.

### Persistent Volume and Claim

Create a `PersistentVolume` backed by the local IBM SE certificate directory:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ibmse-pv
  namespace: trustee-operator-system
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

Create the corresponding `PersistentVolumeClaim`:

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

- `kbsResourcePolicyConfigMapName` must be `config/templates/resource-policy-ibm.rego` format.
- `certStorePvc` must match the PVC name created above.
- If HTTPS is enabled, include the worker node IPs in the `[alt_names]` section of your certificate. See [Generate a self-signed certificate](https://github.com/confidential-containers/trustee/blob/main/kbs/docs/self-signed-https.md#generate-a-self-signed-certificate) for details.
  ```ini
  [alt_names]
  DNS.1 = kbs-service
  IP.1 = <ocp-worker-node-0-ip>
  IP.2 = <ocp-worker-node-1-ip>
  ```
