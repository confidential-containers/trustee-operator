"""
crypto.py — TLS certificate/key generation and RSA keypair generation.

All private key material is kept in memory as bytes and never written to disk.
"""

import datetime
import ipaddress
import os
import logging

from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import NameOID

log = logging.getLogger(__name__)


def generate_tls_cert_and_key(worker_ips: list[str]) -> tuple[bytes, bytes]:
    """
    Generate a self-signed TLS certificate and private key for the KBS HTTPS server.

    The certificate includes all provided worker IPs as Subject Alternative Names so
    that Kata agents on those nodes can verify the TLS connection without a CA.

    Returns:
        (cert_pem_bytes, key_pem_bytes) — both as PEM-encoded bytes, never written to disk.
    """
    if not worker_ips:
        raise ValueError("At least one worker IP must be provided for TLS SAN")

    key = rsa.generate_private_key(public_exponent=65537, key_size=4096)

    san_entries: list[x509.GeneralName] = []
    for ip_str in worker_ips:
        try:
            san_entries.append(x509.IPAddress(ipaddress.ip_address(ip_str)))
        except ValueError:
            # treat as DNS name if not parseable as IP
            san_entries.append(x509.DNSName(ip_str))

    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "kbs-service")])
    now = datetime.datetime.now(datetime.timezone.utc)
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now)
        .not_valid_after(now + datetime.timedelta(days=365))
        .add_extension(x509.SubjectAlternativeName(san_entries), critical=False)
        .add_extension(x509.BasicConstraints(ca=True, path_length=None), critical=True)
        .sign(key, hashes.SHA256())
    )

    cert_pem = cert.public_bytes(serialization.Encoding.PEM)
    key_pem = key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    )
    log.info("Generated TLS certificate with SANs: %s", worker_ips)
    return cert_pem, key_pem


def generate_rsa_keypair() -> tuple[bytes, bytes]:
    """
    Generate a 4096-bit RSA keypair for ibmse encryption.

    The private key passphrase is never used — the unencrypted key is returned
    directly in memory.  The caller is responsible for writing to /tmp/ibmse/rsa/
    and for zeroing the memory after use.

    Returns:
        (private_key_pem_bytes, public_key_pem_bytes)
    """
    key = rsa.generate_private_key(public_exponent=65537, key_size=4096)
    private_pem = key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.TraditionalOpenSSL,
        encryption_algorithm=serialization.NoEncryption(),
    )
    public_pem = key.public_key().public_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PublicFormat.SubjectPublicKeyInfo,
    )
    log.info("Generated RSA-4096 keypair in memory")
    return private_pem, public_pem
