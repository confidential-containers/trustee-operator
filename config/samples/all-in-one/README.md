# TrusteeConfig All-in-One Samples

This directory contains sample configurations for deploying Trustee (KBS) using the **recommended TrusteeConfig approach**.

## Quick Start

### Basic Deployment (Permissive Profile)

For development and testing, use the permissive profile which auto-generates all required resources:

```bash
kubectl apply -k config/samples/all-in-one/
```

This deploys:
- TrusteeConfig with permissive profile
- Auto-generated KBS configuration
- Sample secrets and policies for testing
- Debug logging enabled

### Production Deployment (Restricted Profile)

For production use, first create your TLS certificates:

```bash
# Create HTTPS certificate secret
kubectl create secret tls kbs-https-certificate \
  --cert=path/to/tls.crt \
  --key=path/to/tls.key \
  -n trustee-operator-system

# Create attestation token certificate secret (optional but recommended)
kubectl create secret tls attestation-token-certificate \
  --cert=path/to/token.crt \
  --key=path/to/token.key \
  -n trustee-operator-system
```

Then edit `kustomization.yaml` to use the restricted profile:

```yaml
resources:
- trusteeconfig-restricted.yaml
```

Apply:

```bash
kubectl apply -k config/samples/all-in-one/
```

## What TrusteeConfig Auto-Generates

TrusteeConfig automatically creates and manages the following resources:

### ConfigMaps
- **kbs-config**: Main KBS configuration (kbs-config.toml)
- **attestation-policy**: CPU attestation policy (OPA/Rego)
- **gpu-attestation-policy**: GPU attestation policy
- **resource-policy**: Resource access policy
- **rvps-reference-values**: RVPS reference values (JSON)
- **tdx-config**: TDX-specific configuration

### Secrets
- **kbs-auth-public-key**: KBS authentication public key (auto-generated)
- **kbs-sample-secret**: Sample secret for testing (permissive profile only)
- **kbs-https-key**: HTTPS private key (when httpsSpec is configured)
- **kbs-https-cert**: HTTPS certificate (when httpsSpec is configured)
- **kbs-attestation-key**: Attestation token signing key (when attestationTokenVerificationSpec is configured)
- **kbs-attestation-cert**: Attestation token certificate (when attestationTokenVerificationSpec is configured)

### KbsConfig
- A KbsConfig CR that manages the actual KBS deployment

## Profile Types

### Permissive Profile
**Use for**: Development, testing, demos

**Features**:
- Insecure HTTP allowed
- Debug logging enabled (RUST_LOG=debug)
- Permissive resource policy (allows access with affirming attestation)
- Auto-generated sample secrets
- Insecure API enabled
- Insecure attestation keys

### Restricted Profile
**Use for**: Production, security-critical deployments

**Features**:
- HTTPS required (must provide TLS certificate)
- Restrictive resource policy
- No debug logging
- Secure API settings (insecure_api=false, insecure_key=false)
- Attestation token verification with trusted CA (recommended)

## Advanced Customization

While TrusteeConfig auto-generates configurations, you can still customize the underlying KbsConfig:

1. Apply your TrusteeConfig
2. Wait for it to create the KbsConfig
3. Edit the KbsConfig directly for advanced settings:
   - Replica count
   - Environment variables
   - Secret resources
   - Local certificate cache
   - Platform-specific configurations (IBM SE, TDX)

TrusteeConfig will preserve your manual customizations during reconciliation.

## Platform-Specific Examples

### IBM Secure Execution (SE)

For IBM SE support, you need to provide a PersistentVolume with certificates:

1. Uncomment the IBM SE resources in `kustomization.yaml`:
   ```yaml
   resources:
   - ibmse-pv.yaml
   - ibmse-pvc.yaml
   ```

2. Edit `ibmse-pv.yaml` to point to your certificate storage

3. After deploying TrusteeConfig, edit the generated KbsConfig to add:
   ```yaml
   spec:
     ibmSEConfigSpec:
       certStorePvc: ibmse-cert-store
   ```

## Migration from Legacy KbsConfig

If you were using the old approach with manual ConfigMaps and Secrets:

### Old Approach (Deprecated)
```yaml
# Had to manually create:
- kbsconfig_sample.yaml
- kbs-config.yaml (ConfigMap)
- attestation-policy.yaml (ConfigMap)
- resource-policy.yaml (ConfigMap)
- rvps-reference-values.yaml (ConfigMap)
- tdx-config.yaml (ConfigMap)
- Secrets via kustomize secretGenerator
```

### New Approach (Recommended)
```yaml
# Just create:
- trusteeconfig-basic.yaml (or trusteeconfig-restricted.yaml)
# Everything else is auto-generated!
```

### Migration Steps

1. **Back up your existing configuration**:
   ```bash
   kubectl get kbsconfig -n trustee-operator-system -o yaml > backup.yaml
   ```

2. **Note any custom values** in your ConfigMaps and Secrets

3. **Delete the old KbsConfig**:
   ```bash
   kubectl delete kbsconfig kbsconfig-sample -n trustee-operator-system
   ```

4. **Apply TrusteeConfig**:
   ```bash
   kubectl apply -f trusteeconfig-basic.yaml
   ```

5. **Re-apply customizations** (if needed) to the auto-generated KbsConfig


## Troubleshooting

### Check TrusteeConfig Status
```bash
kubectl get trusteeconfig -n trustee-operator-system
kubectl describe trusteeconfig trusteeconfig-basic -n trustee-operator-system
```

### Check Auto-Generated KbsConfig
```bash
kubectl get kbsconfig -n trustee-operator-system
kubectl describe kbsconfig <name> -n trustee-operator-system
```

### View Auto-Generated Resources
```bash
# ConfigMaps
kubectl get configmap -n trustee-operator-system | grep -E "(kbs-config|attestation-policy|resource-policy|rvps|tdx)"

# Secrets
kubectl get secret -n trustee-operator-system | grep kbs
```

### Check Logs
```bash
# Trustee operator logs
kubectl logs -n trustee-operator-system deployment/trustee-operator-controller-manager

# KBS pods
kubectl logs -n trustee-operator-system -l app=kbs
```

## Further Reading

- [TrusteeConfig API Reference](../../api/v1alpha1/kbsconfig_types.go)
- [Trustee Operator Documentation](../../../README.md)
- [Confidential Containers Documentation](https://github.com/confidential-containers/documentation)
