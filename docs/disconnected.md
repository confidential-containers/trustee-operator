# Disconnected environment

A disconnected environment is a system that has no direct or continuous connection to the internet or other external networks.
In this guide, we bring an example on how to configure the trustee operator for baking VCEK certificates into the trustee image.

## Create the VCEK secret

Please refer to this [guide](https://github.com/confidential-containers/trustee/blob/main/attestation-service/docs/amd-offline-certificate-cache.md) for more details.

First of all let's create a local directory containing the certificates.

Trustee supports two naming layouts per hardware ID. When both are present the TCB-prefixed file takes precedence:

```
vcek/
├── <hardware-id-1>/
│   └── bl02_tee00_snp06_ucode21_vcek.der   # preferred: TCB-prefixed filename
├── <hardware-id-2>/
│   └── vcek.der                             # legacy flat layout (fallback)
├── <hardware-id-3>/
│   ├── bl03_tee01_snp08_ucode15_fmc05_vcek.der  # multiple VCEKs per host
│   └── bl02_tee00_snp06_ucode21_vcek.der
```

The TCB prefix format is `bl{BL}_tee{TEE}_snp{SNP}_ucode{UCODE}` with each parameter zero-padded to 2 digits.
Turin processors append an additional `_fmc{FMC}` field.
Using TCB-prefixed filenames is recommended as it allows pre-loading certificates for multiple firmware versions per host.

**Note** The hardware-id must be lowercase.

Then we create a secret (one per hardware ID):

```bash
kubectl create secret generic vcek-secret1 --from-file ./vcek/<hardware-id-1> -n trustee-operator-system
kubectl create secret generic vcek-secret2 --from-file ./vcek/<hardware-id-2> -n trustee-operator-system
```

All files in the hardware ID directory (whether `vcek.der` or TCB-prefixed) are included in the secret automatically.

## KbsConfig

The KbsConfig CR needs to specify the `kbsLocalCertCacheSpec` option:

```yaml
apiVersion: confidentialcontainers.org/v1alpha1
kind: KbsConfig
metadata:  
  name: kbsconfig-sample
  namespace: trustee-operator-system
spec:
  # omitted all the rest of config
  # ...
  kbsLocalCertCacheSpec:
    secrets:
    - secretName: vcek-secret1
      mountPath: "/opt/confidential-containers/attestation-service/kds-store/vcek/<hardware-id-1>"
    - secretName: vcek-secret2
      mountPath: "/opt/confidential-containers/attestation-service/kds-store/vcek/<hardware-id-2>"
```

The VCEK certificates are mounted in the trustee `mountPath` directory.
The `mountPath` directory defaults to `/opt/confidential-containers/attestation-service/kds-store/vcek` if not provided by the user.
