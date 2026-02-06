# Disconnected enviroment

A disconnected environment is a system that has no direct or continuous connection to the internet or other external networks.
In this guide, we bring an example on how to configure the trustee operator for baking a VCEK certificate into the trustee image. 

## Create the VCEK secret

Please refer to this [guide](https://github.com/confidential-containers/trustee/blob/main/attestation-service/docs/amd-offline-certificate-cache.md) for more deatails.


First of all let's create a local directory containing the certificates (one per node):

```
├── vcek
│   ├── <hardware-id-1>
│      ├── vcek.der
│   ├── <hardware-id-2>
│      ├── vcek.der
```

**Note** The hardware-id must be lowercase.

Then we create a secret (one per node):

```bash
kubectl create secret generic vcek-secret1 --from-file ./vcek/<hardware-id-1> -n trustee-operator-system
kubectl create secret generic vcek-secret2 --from-file ./vcek/<hardware-id-2> -n trustee-operator-system
```

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

The `vcek.der` certificate will be mounted in the trustee `mountPath` directory.
The `mountPath` directory defaults to `/opt/confidential-containers/attestation-service/kds-store/vcek` if not provided by the user.
