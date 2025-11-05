# HTTPS configuration

This guide provides the steps for generating a TLS self-signed certificate with cert-manager and integrate it in the trustee-operator HTTPS configuration.

## Helm installation

```bash
helm install   cert-manager jetstack/cert-manager   --namespace cert-manager   --create-namespace   --version v1.19.1   --set crds.enabled=true
```

## TLS certificate generation

```bash
kubectl apply -f - << EOF
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: kbs-https
  namespace: trustee-operator-system
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: kbs-https
  namespace: trustee-operator-system
spec:
  dnsNames:
    - kbs-service
  secretName: trustee-tls-cert
  issuerRef:
    name: kbs-https
EOF
```

## TrusteeConfig CR with HTTPS config

```bash
kubectl apply -f - << EOF
apiVersion: confidentialcontainers.org/v1alpha1
kind: TrusteeConfig
metadata:
  labels:
    app.kubernetes.io/name: trusteeconfig
    app.kubernetes.io/instance: trusteeconfig-sample
    app.kubernetes.io/part-of: trustee-operator
    app.kubernetes.io/managed-by: kustomize
    app.kubernetes.io/created-by: trustee-operator
  name: trusteeconfig-sample
  namespace: trustee-operator-system
spec:
  profileType: Restricted
  kbsServiceType: ClusterIP
  httpsSpec:
    tlsSecretName: trustee-tls-cert
EOF
```

