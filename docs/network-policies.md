# Trustee Network Policies

This document is the design and hand-off reference for the NetworkPolicies that
protect the Trustee operands from unintended data leaks. It inventories the
operands, maps their ingress/egress, records the policy model.

## 1. Operand inventory (`trustee-operator-system`)

All operands run in the operator's namespace (default `trustee-operator-system`)
and carry the label `app: kbs`.

| Component | Container | Role | Listens |
|---|---|---|---|
| KBS/AS/RVPS | `kbs` | Serves keys/resources to attesters | TCP 8080 |
| secret-converter | init container | Prepares mounted secrets before KBS starts | none |

## 2. Per-operand traffic matrix

Direction is relative to the `app: kbs` operand pod.

| # | Dir | Peer | Port/proto | Purpose | Policy |
|---|-----|------|-----------|---------|--------|
| 1 | Ingress | Any client (attesters, callers) | TCP 8080 | KBS API / key brokering | `kbs-allow-ingress` |
| 2 | Ingress | OpenShift ingress router *(OCP only)* | — | External access via Route | `kbs-allow-ingress` (router rule) |
| 3 | Egress | Cluster DNS namespace | UDP/TCP 53, UDP/TCP 5353 | Name resolution | `kbs-allow-egress-dns` |
| 4 | Egress | External attestation services | TCP 443 | Evidence/collateral verification | `kbs-allow-egress-attestation` |
| — | Both | Everything else | — | Denied by default | `kbs-deny-all` |

External attestation peers on port 443 (connected profile):

| Vendor | Endpoint |
|---|---|
| AMD KDS | `kdsintf.amd.com` |
| Intel PCS / Trust Authority | `api.trustedservices.intel.com` |
| NVIDIA NRAS | `nras.attestation.nvidia.com` |

Notes:
- DNS covers both `53` (standard) and `5353` (OpenShift's `openshift-dns`
  CoreDNS bind port), and both UDP and TCP (TCP is required for large/truncated
  responses). Egress is restricted to the cluster DNS namespace; the resolver
  performs the external lookup on the pod's behalf.
- NetworkPolicy cannot match destinations by DNS name, so the attestation egress
  allows TCP 443 to any destination as a portable baseline.

## 3. The four operand policies

Implemented in `internal/controller/networkpolicy.go`, one set per `KbsConfig`.
Each is label-scoped to `podSelector: app=kbs` and lives in the operator
namespace.

1. **`kbs-deny-all`** — default-deny for `app=kbs` (`PolicyTypes: [Ingress, Egress]`,
   no rules). Everything below is an additive allow on top of this.
2. **`kbs-allow-ingress`** — ingress TCP 8080 from any client. On OpenShift, an
   extra rule allows the ingress router namespace (`policy-group.network.openshift.io/ingress: ""`).
3. **`kbs-allow-egress-dns`** — egress UDP/TCP 53 + 5353 to the DNS namespace
   (`kube-system` on Kubernetes, `openshift-dns` on OpenShift).
4. **`kbs-allow-egress-attestation`** — egress TCP 443 (connected profile).

Platform differences (DNS namespace, router ingress rule) are selected at runtime
via the reconciler's `IsOpenShift` field, which defaults to `false` (vanilla
Kubernetes).

## 4. Operator contract

The controller guarantees the following for every operand policy (verified in
`networkpolicy_test.go` and against a live cluster):

- **Create** — all four policies are created when a `KbsConfig` is reconciled,
  before the KBS deployment, so isolation is in place as the operand starts.
- **Reconcile / immutable** — the desired spec is compared with `DeepEqual`;
  drift is reverted. Deleting a policy triggers recreation via the `Owns` watch.
  Users effectively cannot edit or delete the policies.
- **Garbage collection** — owner references tie the policies to the `KbsConfig`,
  so they are removed with it.

## Appendix: validation summary

Enforcement was validated on kind + Calico (kindnet does **not** enforce
NetworkPolicy). From an `app=kbs` pod: DNS resolution and TCP 443 to
AMD/Intel/NVIDIA succeeded; egress on other ports (e.g. 80) and ingress on
non-8080 ports timed out; an unlabeled pod reached the same ports freely
(confirming isolation applies only to operands). Self-heal (delete) and
drift-revert (edit) were confirmed on any CNI.
