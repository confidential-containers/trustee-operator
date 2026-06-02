# Trustee Operator Helm Chart

Helm chart for deploying the Trustee Operator, which manages the lifecycle of
[Trustee](https://github.com/confidential-containers/trustee) components (KBS, AS, RVPS)
in a Kubernetes cluster.

## Prerequisites

Before installing this chart, ensure you have:

- **Helm** v3.x or later installed ([installation guide](https://helm.sh/docs/intro/install/))
- **Kubernetes cluster** v1.24+ with appropriate access
- **kubeconfig** configured to access your cluster

## Quick Start

### Install Operator Only

Deploy the operator without creating a Trustee instance (you'll manage TrusteeConfig CRs manually):

```bash
helm install trustee-operator ./charts/trustee-operator \
  --set trustee.enabled=false \
  -n trustee-operator-system \
  --create-namespace
```

### Install Operator + Trustee Instance

Deploy the operator and automatically create a TrusteeConfig CR for a working Trustee deployment:

```bash
helm install trustee-operator ./charts/trustee-operator \
  -n trustee-operator-system \
  --create-namespace
```

This creates:
- Trustee Operator deployment
- All required RBAC (ClusterRoles, RoleBindings, ServiceAccount)
- CRDs (KbsConfig, TrusteeConfig)
- A TrusteeConfig CR named `trustee-sample` (configurable)

### Install with Custom Configuration

```bash
helm install trustee-operator ./charts/trustee-operator \
  --set trustee.profileType=Restrictive \
  --set kbs.serviceType=NodePort \
  --set kbs.https.enabled=true \
  --set kbs.https.tlsSecretName=kbs-tls-cert \
  -n trustee-operator-system \
  --create-namespace
```

## Configuration

All configuration options are defined in [`values.yaml`](./values.yaml) with detailed
comments explaining each field.

You can override any value by either:
1. Using `--set key=value` flags
2. Creating your own values file and passing it with `-f custom-values.yaml`

## Uninstall

```bash
helm uninstall trustee-operator -n trustee-operator-system
```
