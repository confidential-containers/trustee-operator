# TrusteeConfig Reconciliation Loop Behavior

This document explains how the TrusteeConfig reconciliation loop works, including when it's triggered, how it handles manual changes, and potential reconciliation loops.

## Overview

The TrusteeConfig controller manages KbsConfig resources as owned resources. The reconciliation loop ensures that the KbsConfig matches the desired state defined in TrusteeConfig, while preserving user-configurable fields that have been manually modified.

## Reconciliation Triggers

The TrusteeConfig reconciliation is triggered in the following scenarios:

### 1. TrusteeConfig Changes
- **Direct changes**: Any modification to the TrusteeConfig resource (spec, metadata, etc.)
- **Initial creation**: When a new TrusteeConfig is created
- **Status updates**: When the TrusteeConfig status is updated

### 2. KbsConfig Changes
- **Manual modifications**: When the KbsConfig resource is manually modified (via `kubectl edit`, API calls, etc.)
- **Status changes**: When the KbsConfig status changes (e.g., `IsReady` field)
- **Any field change**: The controller watches KbsConfig using `EnqueueRequestForOwner`, so any change triggers reconciliation

**Note**: The watch is configured in `SetupWithManager` using `handler.EnqueueRequestForOwner`, which enqueues the owner (TrusteeConfig) whenever the owned resource (KbsConfig) changes.

## Reconciliation Flow

The reconciliation process follows these steps:

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Reconcile Triggered                                      │
│    - TrusteeConfig changed OR                               │
│    - KbsConfig changed (via EnqueueRequestForOwner)         │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Fetch TrusteeConfig                                      │
│    - Get current TrusteeConfig state                        │
│    - Handle deletion (if DeletionTimestamp is set)          │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Build KbsConfigSpec                                      │
│    - Generate spec from TrusteeConfig                       │
│    - Apply profile-specific configuration                   │
│    - Configure HTTPS/Attestation if specified               │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Create or Update KbsConfig                               │
│    ┌──────────────────────────────────────────┐            │
│    │ 4a. KbsConfig doesn't exist?             │            │
│    │     → Create new KbsConfig               │            │
│    │     → Set TrusteeConfig as owner         │            │
│    │     → Exit (no merge needed)             │            │
│    └──────────────────────────────────────────┘            │
│    ┌──────────────────────────────────────────┐            │
│    │ 4b. KbsConfig exists?                    │            │
│    │     → Detect manual changes              │            │
│    │     → If manual changes: merge specs     │            │
│    │     → If no manual changes: apply spec   │            │
│    │     → Update KbsConfig                   │            │
│    └──────────────────────────────────────────┘            │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Update TrusteeConfig Status                              │
│    - Set IsReady = true                                     │
│    - Set KbsConfigRef                                       │
│    - Check KbsConfig.IsReady                                │
│    - If not ready: requeue after 10 seconds                 │
└─────────────────────────────────────────────────────────────┘
```

## Manual Change Detection

When a KbsConfig already exists, the controller detects manual changes by comparing the current KbsConfig spec with the generated spec:

### Detection Logic

1. **Compare user-configurable fields** (see [kbs-config-merge-strategy.md](./kbs-config-merge-strategy.md)):
   - `KbsDeploymentSpec.Replicas`
   - `KbsEnvVars`
   - `KbsSecretResources`
   - `KbsLocalCertCacheSpec`
   - `IbmSEConfigSpec`
   - `KbsAttestationPolicyConfigMapName`

2. **If differences detected**:
   - Perform a smart merge (preserve manual changes, apply generated values)
   - Update KbsConfig with merged spec

3. **If no differences detected**:
   - Apply the generated spec directly (overwrites any changes to managed fields)

### Merge Strategy

The merge process:
- **Starts with generated spec** (from TrusteeConfig)
- **Preserves manual overrides** for user-configurable fields
- **Overwrites managed fields** (e.g., `KbsConfigMapName`, `KbsAuthSecretName`, etc.)

## Related Documentation

- [KbsConfig Merge Strategy](./kbs-config-merge-strategy.md) - Details on which fields are preserved vs. overwritten
- [TrusteeConfig API Reference](../api/v1alpha1/trusteeconfig_types.go) - TrusteeConfig resource definition
- [KbsConfig API Reference](../api/v1alpha1/kbsconfig_types.go) - KbsConfig resource definition

