"""
bundler.py — Assemble the validated ibmse directory tree and produce ibmse.tar.gz
in memory.

The full required tree:
  ibmse/
  ├── https.key
  ├── https.crt
  ├── certs/
  │   ├── ibm-z-host-key-signing-gen2.crt
  │   └── DigiCertCA.crt
  ├── crls/
  │   ├── ibm-z-host-key-gen2.crl
  │   ├── DigiCertTrustedRootG4.crl
  │   └── DigiCertTrustedG4CodeSigningRSA4096SHA3842021CA1.crl
  ├── hdr/
  │   └── hdr.bin
  ├── hkds/
  │   └── <hkd_cert>.crt
  └── rsa/
      ├── encrypt_key.pem
      └── encrypt_key.pub
"""

import hashlib
import io
import logging
import os
import tarfile
import tempfile
from pathlib import Path

log = logging.getLogger(__name__)

# Expected size of hdr.bin as documented in the IBM SE runbook.
HDR_BIN_SIZE = 640

REQUIRED_TREE_ENTRIES = [
    "ibmse/https.key",
    "ibmse/https.crt",
    "ibmse/certs/ibm-z-host-key-signing-gen2.crt",
    "ibmse/certs/DigiCertCA.crt",
    "ibmse/crls/ibm-z-host-key-gen2.crl",
    "ibmse/crls/DigiCertTrustedRootG4.crl",
    "ibmse/crls/DigiCertTrustedG4CodeSigningRSA4096SHA3842021CA1.crl",
    "ibmse/hdr/hdr.bin",
    "ibmse/rsa/encrypt_key.pem",
    "ibmse/rsa/encrypt_key.pub",
]


def create_bundle(
    *,
    https_key: bytes,
    https_cert: bytes,
    certs: dict[str, bytes],
    crls: dict[str, bytes],
    hdr_bin: bytes,
    hkd_cert_name: str,
    hkd_cert: bytes,
    rsa_private_key: bytes,
    rsa_public_key: bytes,
) -> tuple[bytes, str]:
    """
    Assemble the ibmse directory tree inside a TemporaryDirectory, validate
    all required files are present, and produce a gzip-compressed tar archive.

    Returns:
        (tar_gz_bytes, sha256_hex) — the archive bytes and its SHA-256 digest.
    """
    with tempfile.TemporaryDirectory(prefix="ibmse-bundle-") as tmpdir:
        base = Path(tmpdir) / "ibmse"

        # create subdirectories
        for subdir in ("certs", "crls", "hdr", "hkds", "rsa"):
            (base / subdir).mkdir(parents=True)

        # write files
        (base / "https.key").write_bytes(https_key)
        (base / "https.crt").write_bytes(https_cert)

        for name, data in certs.items():
            (base / "certs" / name).write_bytes(data)

        for name, data in crls.items():
            (base / "crls" / name).write_bytes(data)

        _validate_hdr_bin(hdr_bin)
        (base / "hdr" / "hdr.bin").write_bytes(hdr_bin)

        (base / "hkds" / hkd_cert_name).write_bytes(hkd_cert)

        (base / "rsa" / "encrypt_key.pem").write_bytes(rsa_private_key)
        (base / "rsa" / "encrypt_key.pub").write_bytes(rsa_public_key)

        # validate required entries exist before tarring
        _validate_tree(base, hkd_cert_name)

        # create tar in memory
        buf = io.BytesIO()
        with tarfile.open(fileobj=buf, mode="w:gz") as tar:
            tar.add(str(base), arcname="ibmse")

        tar_bytes = buf.getvalue()
        sha256 = hashlib.sha256(tar_bytes).hexdigest()
        log.info(
            "Created ibmse.tar.gz: %d bytes, SHA-256=%s", len(tar_bytes), sha256
        )
        return tar_bytes, sha256


def _validate_hdr_bin(hdr_bin: bytes) -> None:
    """Verify hdr.bin is exactly HDR_BIN_SIZE bytes as the runbook specifies."""
    if len(hdr_bin) != HDR_BIN_SIZE:
        raise ValueError(
            f"hdr.bin must be exactly {HDR_BIN_SIZE} bytes, got {len(hdr_bin)}"
        )


def _validate_tree(base: Path, hkd_cert_name: str) -> None:
    """
    Verify that all required files exist inside the assembled tree before
    the tar is created.  Fails fast with a clear error listing missing files.
    """
    required = REQUIRED_TREE_ENTRIES + [f"ibmse/hkds/{hkd_cert_name}"]
    missing = []
    for rel in required:
        # rel is relative to the parent of base (i.e. the tempdir)
        full = base.parent / rel
        if not full.exists():
            missing.append(rel)
    if missing:
        raise FileNotFoundError(
            f"ibmse bundle validation failed — missing files: {missing}"
        )
    log.info("ibmse tree validated: all %d required files present", len(required))
