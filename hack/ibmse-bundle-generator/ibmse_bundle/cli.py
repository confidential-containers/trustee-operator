"""
cli.py — Entry point for the IBM SE bundle generator.

Usage:
    python -m ibmse_bundle.cli \\
        --worker-ips 192.168.1.10 192.168.1.11 \\
        --hkd-cert /path/to/hkd.crt \\
        --hdr-bin /path/to/hdr.bin \\
        --se-message /path/to/se-message.json \\
        --namespace trustee-operator-system \\
        [--kubeconfig ~/.kube/config] \\
        [--dry-run]
"""

import argparse
import logging
import sys
from pathlib import Path

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)-8s %(name)s: %(message)s",
)
log = logging.getLogger(__name__)


def _build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        description="Generate the IBM SE bundle and push required Secrets to Kubernetes.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument(
        "--worker-ips",
        nargs="+",
        required=True,
        metavar="IP",
        help="Worker node IP addresses to include in TLS SAN (one or more)",
    )
    p.add_argument(
        "--hkd-cert",
        required=True,
        type=Path,
        metavar="PATH",
        help="Path to the IBM Host Key Document certificate (.crt)",
    )
    p.add_argument(
        "--hdr-bin",
        required=True,
        type=Path,
        metavar="PATH",
        help="Path to hdr.bin produced by pvextract-hdr (must be exactly 640 bytes)",
    )
    p.add_argument(
        "--se-message",
        required=True,
        type=Path,
        metavar="PATH",
        help="Path to se-message JSON file containing se.attestation_phkh, se.image_phkh, se.tag",
    )
    p.add_argument(
        "--namespace",
        default="trustee-operator-system",
        help="Kubernetes namespace where Secrets are created (default: trustee-operator-system)",
    )
    p.add_argument(
        "--kubeconfig",
        type=Path,
        default=None,
        metavar="PATH",
        help="Path to kubeconfig file (defaults to in-cluster config, then ~/.kube/config)",
    )
    p.add_argument(
        "--dry-run",
        action="store_true",
        help="Validate and assemble the bundle but do not push any Secrets to Kubernetes",
    )
    p.add_argument(
        "--cert-checksums",
        nargs="*",
        default=[],
        metavar="NAME=SHA256",
        help="Optional SHA-256 checksums for downloaded certs/CRLs, e.g. DigiCertCA.crt=abc123",
    )
    return p


def _parse_checksums(raw: list[str]) -> dict[str, str]:
    result = {}
    for item in raw:
        if "=" not in item:
            log.warning("Ignoring malformed checksum entry (expected NAME=SHA256): %s", item)
            continue
        name, digest = item.split("=", 1)
        result[name.strip()] = digest.strip()
    return result


def main(argv: list[str] | None = None) -> int:
    args = _build_parser().parse_args(argv)

    # ── validate inputs ──────────────────────────────────────────────────────
    errors: list[str] = []
    if not args.hkd_cert.is_file():
        errors.append(f"HKD cert not found: {args.hkd_cert}")
    if not args.hdr_bin.is_file():
        errors.append(f"hdr.bin not found: {args.hdr_bin}")
    if not args.se_message.is_file():
        errors.append(f"se-message JSON not found: {args.se_message}")
    if errors:
        for e in errors:
            log.error(e)
        return 1

    checksums = _parse_checksums(args.cert_checksums)

    # ── imports deferred so --help works without deps installed ──────────────
    from ibmse_bundle import crypto, downloader, bundler, k8s, policy

    # 1. Download certs and CRLs
    log.info("Downloading IBM Z certificates and CRLs...")
    certs = downloader.download_certs(checksums)
    crls = downloader.download_crls(checksums)

    # 2. Generate TLS cert + key in memory
    log.info("Generating TLS certificate for worker IPs: %s", args.worker_ips)
    https_cert, https_key = crypto.generate_tls_cert_and_key(args.worker_ips)

    # 3. Generate RSA keypair in memory
    log.info("Generating RSA-4096 keypair in memory...")
    rsa_private, rsa_public = crypto.generate_rsa_keypair()

    # 4. Read HKD cert and hdr.bin from disk
    hkd_cert_bytes = args.hkd_cert.read_bytes()
    hdr_bin_bytes = args.hdr_bin.read_bytes()

    # 5. Assemble and tar the bundle
    log.info("Assembling ibmse bundle...")
    tar_gz, sha256 = bundler.create_bundle(
        https_key=https_key,
        https_cert=https_cert,
        certs=certs,
        crls=crls,
        hdr_bin=hdr_bin_bytes,
        hkd_cert_name=args.hkd_cert.name,
        hkd_cert=hkd_cert_bytes,
        rsa_private_key=rsa_private,
        rsa_public_key=rsa_public,
    )
    log.info("Bundle SHA-256: %s", sha256)

    # 6. Parse se-message JSON
    se_message_json = args.se_message.read_text()
    rendered_policy = policy.render_attestation_policy(se_message_json)
    log.info("Rendered attestation policy:\n%s", rendered_policy)

    if args.dry_run:
        log.info("--dry-run: skipping Kubernetes Secret push")
        return 0

    # 7. Push Secrets to Kubernetes
    kubeconfig_str = str(args.kubeconfig) if args.kubeconfig else None
    log.info("Pushing Secrets to namespace %s...", args.namespace)
    k8s.push_secrets(
        namespace=args.namespace,
        tar_gz=tar_gz,
        tar_sha256=sha256,
        https_key=https_key,
        https_cert=https_cert,
        kubeconfig=kubeconfig_str,
    )
    policy.push_attestation_policy_secret(
        namespace=args.namespace,
        se_message_json=se_message_json,
        kubeconfig=kubeconfig_str,
    )

    log.info("Done. 4 Secrets created/updated in namespace %s.", args.namespace)
    log.info("  ibmse-bundle, kbs-https-key, kbs-https-certificate, ibmse-se-message")
    return 0


if __name__ == "__main__":
    sys.exit(main())
