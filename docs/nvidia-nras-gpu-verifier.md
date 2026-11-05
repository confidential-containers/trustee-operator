# NVIDIA GPU attestation (NRAS remote verifier)

Blackwell (B200/B300) and other recent NVIDIA GPUs may fail local SPDM
verification with *Device architecture not supported*. Use the NRAS remote
verifier in the attestation service configuration.

NRAS requires a licensing agreement with NVIDIA. See
[trustee NVIDIA verifier documentation](https://github.com/confidential-containers/trustee/blob/main/deps/verifier/src/nvidia/README.md).

## Microservices sample

The [`as-config.yaml`](../config/samples/microservices/as-config.yaml) sample
includes:

```json
"verifier_config": {
  "nvidia_verifier": {
    "type": "Remote",
    "verifier_url": "https://nras.attestation.nvidia.com/v4/attest"
  }
}
```

Apply custom attestation policies after AS rollout if embedded defaults are
insufficient — see [attestation-service policy docs](https://github.com/confidential-containers/trustee/blob/main/attestation-service/docs/policy.md).
