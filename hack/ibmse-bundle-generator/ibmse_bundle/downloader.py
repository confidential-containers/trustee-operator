"""
downloader.py — Download IBM/DigiCert certificates and CRLs with retry,
SHA-256 checksum verification, and CRL expiry validation.
"""

import datetime
import hashlib
import logging
import time
import urllib.request
import urllib.error

from cryptography import x509
from cryptography.hazmat.primitives.serialization import Encoding

log = logging.getLogger(__name__)

# Public URLs for IBM Z host-key certificates and CRLs.
CERT_URLS: dict[str, str] = {
    "ibm-z-host-key-signing-gen2.crt": (
        "https://www.ibm.com/security/cryptocards/pciecc4/pdf/"
        "ibm-z-host-key-signing-gen2.crt"
    ),
    "DigiCertCA.crt": (
        "https://cacerts.digicert.com/DigiCertTrustedG4RSA4096SHA256TimeStampingCA.crt.pem"
    ),
}

CRL_URLS: dict[str, str] = {
    "ibm-z-host-key-gen2.crl": (
        "http://crl3.digicert.com/ibm-z-host-key-gen2.crl"
    ),
    "DigiCertTrustedRootG4.crl": (
        "http://crl3.digicert.com/DigiCertTrustedRootG4.crl"
    ),
    "DigiCertTrustedG4CodeSigningRSA4096SHA3842021CA1.crl": (
        "http://crl3.digicert.com/DigiCertTrustedG4CodeSigningRSA4096SHA3842021CA1.crl"
    ),
}

# Minimum days before CRL nextUpdate for the CRL to be considered valid.
CRL_EXPIRY_WARN_DAYS = 7


def _fetch_with_retry(url: str, retries: int = 3, delay: float = 2.0) -> bytes:
    """Fetch URL content with up to *retries* attempts."""
    last_exc: Exception | None = None
    for attempt in range(1, retries + 1):
        try:
            with urllib.request.urlopen(url, timeout=30) as resp:  # noqa: S310
                if resp.status != 200:
                    raise IOError(f"HTTP {resp.status} fetching {url}")
                data = resp.read()
                log.debug("Fetched %s (%d bytes)", url, len(data))
                return data
        except Exception as exc:
            last_exc = exc
            log.warning("Attempt %d/%d failed for %s: %s", attempt, retries, url, exc)
            if attempt < retries:
                time.sleep(delay)
    raise IOError(f"Failed to fetch {url} after {retries} attempts") from last_exc


def _verify_checksum(data: bytes, expected_sha256: str | None, name: str) -> None:
    """Verify SHA-256 checksum when an expected value is provided."""
    if not expected_sha256:
        return
    actual = hashlib.sha256(data).hexdigest()
    if actual != expected_sha256.lower():
        raise ValueError(
            f"Checksum mismatch for {name}: expected {expected_sha256}, got {actual}"
        )


def _validate_crl_expiry(crl_data: bytes, name: str) -> None:
    """
    Parse the CRL and fail if it is already expired or expires within
    CRL_EXPIRY_WARN_DAYS days.
    """
    try:
        crl = x509.load_der_x509_crl(crl_data)
    except Exception:
        # Some CRLs are PEM-encoded; try that.
        try:
            crl = x509.load_pem_x509_crl(crl_data)
        except Exception as exc:
            raise ValueError(f"Cannot parse CRL {name}: {exc}") from exc

    next_update = crl.next_update_utc
    now = datetime.datetime.now(datetime.timezone.utc)
    remaining = next_update - now
    if remaining.total_seconds() <= 0:
        raise ValueError(f"CRL {name} has expired (nextUpdate={next_update})")
    if remaining.days < CRL_EXPIRY_WARN_DAYS:
        raise ValueError(
            f"CRL {name} expires in {remaining.days} day(s) (<{CRL_EXPIRY_WARN_DAYS}). "
            "Refresh the CRL before proceeding."
        )
    log.info("CRL %s valid until %s (%d days)", name, next_update.date(), remaining.days)


def download_certs(checksums: dict[str, str] | None = None) -> dict[str, bytes]:
    """
    Download all IBM Z / DigiCert signing certificates.

    Args:
        checksums: Optional mapping of filename → expected SHA-256 hex digest.

    Returns:
        dict mapping filename → raw bytes.
    """
    checksums = checksums or {}
    result: dict[str, bytes] = {}
    for name, url in CERT_URLS.items():
        data = _fetch_with_retry(url)
        _verify_checksum(data, checksums.get(name), name)
        result[name] = data
        log.info("Downloaded cert %s (%d bytes)", name, len(data))
    return result


def download_crls(checksums: dict[str, str] | None = None) -> dict[str, bytes]:
    """
    Download all required CRLs and validate their expiry.

    Args:
        checksums: Optional mapping of filename → expected SHA-256 hex digest.

    Returns:
        dict mapping filename → raw bytes.
    """
    checksums = checksums or {}
    result: dict[str, bytes] = {}
    for name, url in CRL_URLS.items():
        data = _fetch_with_retry(url)
        _verify_checksum(data, checksums.get(name), name)
        _validate_crl_expiry(data, name)
        result[name] = data
        log.info("Downloaded CRL %s (%d bytes)", name, len(data))
    return result
