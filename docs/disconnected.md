# Disconnected enviroment

A disconnected environment is a system that has no direct or continuous connection to the internet or other external networks.
In this guide, we bring an example on how to configure the trustee operator for baking a VCEK certificate into the trustee image. 

## Create the VCEK secret

First of all let's create a local directory containing the certificate:

```
├── certs
│   ├── VCEK.crt
```

Then we create a secret:

```bash
kubectl create secret generic vcek-secret --from-file ./certs -n trustee-operator-system
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
    secretName: vcek-secret
    mountPath: "/etc/kbs/snp/ek"
```

The `VCEK.crt` certificate will be mounted in the trustee `mountPath` directoty.