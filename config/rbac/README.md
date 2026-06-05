# Tenant RBAC Examples

This directory contains example RBAC manifests for granting tenant users permission to create TrusteeConfigs in their namespace.

## Setup

```bash
# Apply tenant ClusterRole (once per cluster)
kubectl apply -f tenant-role.yaml

# Create tenant namespace with Pod Security Admission labels
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: <tenant-namespace>
  labels:
    pod-security.kubernetes.io/enforce: privileged
    pod-security.kubernetes.io/audit: privileged
    pod-security.kubernetes.io/warn: privileged
EOF

# Use a RoleBinding on the ClusterRole to limit permissions to the given Namespace
# This is the CLI equivelant of tenant-rolebinding.yaml
# For a User (--user) or ServiceAccount (--serviceaccount)
kubectl create rolebinding tenant-user-trustee-config-creator \
  --clusterrole=trustee-config-creator \
  --user=<tenant-namespace>:<username> \
  --serviceaccount=tenant-a:tenant-sa \
  -n <tenant-namespace>

# Tenant can now create TrusteeConfig in their namespace
```
