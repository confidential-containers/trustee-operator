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

## Persistent Volume creation

For mounting the above directory to the trustee pod filesystem, we'd need to create a Persistent Volume (PV) and a Persistent Volume Claim (PVC).
The configuration of PV/PVC is deployment specific (e.g. dependent on cloud provider), so it is not reported here in this guide.

In a development environment, you may want to create a PV/PVC that makes use of a local directory. This approach is not recommended for production environments:

PersistentVolume:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: ibmse-pv
  namespace: kbs-operator-system
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
Note: the `path` has to match a local directory on the worker node.

PersistentVolumeClaim:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ibmse-pvc
  namespace: kbs-operator-system
spec:
  accessModes:
    - ReadOnlyMany
  storageClassName: ""
  resources:
    requests:
      storage: 100Mi
```

## KBS config CRD

For enabling IBM specific configuration in trustee pod, the `KbsConfig` custom resource should have the `ibmSEConfigSpec` section populated as in the following example:

```yaml
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:  
  name: kbsconfig-sample
  namespace: kbs-operator-system
spec:
  # omitted all the rest of config
  # ...

  # IBMSE settings
  ibmSEConfigSpec:
    certStorePvc: ibmse-pvc
```

The `certStorePvc` has to match the aforementioned PVC name.
