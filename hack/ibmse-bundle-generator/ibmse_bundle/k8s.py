"""
k8s.py — Kubernetes client helpers for creating Secrets used by the
trustee-operator IBM SE workflow.

Three Secrets are created/updated:
  - ibmse-bundle          : the raw ibmse.tar.gz + its SHA-256 digest
  - kbs-https-key         : the HTTPS private key consumed by the operator
  - kbs-https-certificate : the HTTPS certificate consumed by the operator
"""

import base64
import logging

from kubernetes import client, config
from kubernetes.client.rest import ApiException

log = logging.getLogger(__name__)


def _load_kube_config(kubeconfig: str | None) -> None:
    """Load kube config from file or in-cluster environment."""
    if kubeconfig:
        config.load_kube_config(config_file=kubeconfig)
    else:
        try:
            config.load_incluster_config()
        except config.ConfigException:
            config.load_kube_config()


def _apply_secret(
    v1: client.CoreV1Api,
    namespace: str,
    name: str,
    data: dict[str, bytes],
    labels: dict[str, str] | None = None,
) -> None:
    """Create or replace an Opaque Secret."""
    b64_data = {k: base64.b64encode(v).decode() for k, v in data.items()}
    secret = client.V1Secret(
        api_version="v1",
        kind="Secret",
        metadata=client.V1ObjectMeta(
            name=name,
            namespace=namespace,
            labels=labels or {},
        ),
        type="Opaque",
        data=b64_data,
    )
    try:
        v1.read_namespaced_secret(name=name, namespace=namespace)
        v1.replace_namespaced_secret(name=name, namespace=namespace, body=secret)
        log.info("Updated Secret %s/%s", namespace, name)
    except ApiException as exc:
        if exc.status == 404:
            v1.create_namespaced_secret(namespace=namespace, body=secret)
            log.info("Created Secret %s/%s", namespace, name)
        else:
            raise


def push_secrets(
    *,
    namespace: str,
    tar_gz: bytes,
    tar_sha256: str,
    https_key: bytes,
    https_cert: bytes,
    kubeconfig: str | None = None,
) -> None:
    """
    Push three Secrets to the cluster in the given namespace:

    ibmse-bundle:
        ibmse.tar.gz  — raw bundle bytes
        sha256        — hex digest for verification by the DaemonSet

    kbs-https-key:
        privateKey    — TLS private key (consumed by trustee-operator)

    kbs-https-certificate:
        certificate   — TLS certificate (consumed by trustee-operator)
    """
    _load_kube_config(kubeconfig)
    v1 = client.CoreV1Api()

    common_labels = {
        "app.kubernetes.io/managed-by": "ibmse-bundle-generator",
        "app.kubernetes.io/part-of": "trustee",
    }

    _apply_secret(
        v1,
        namespace,
        "ibmse-bundle",
        {
            "ibmse.tar.gz": tar_gz,
            "sha256": tar_sha256.encode(),
        },
        labels=common_labels,
    )

    _apply_secret(
        v1,
        namespace,
        "kbs-https-key",
        {"privateKey": https_key},
        labels=common_labels,
    )

    _apply_secret(
        v1,
        namespace,
        "kbs-https-certificate",
        {"certificate": https_cert},
        labels=common_labels,
    )

    log.info(
        "Pushed 3 Secrets to namespace %s: ibmse-bundle, kbs-https-key, kbs-https-certificate",
        namespace,
    )
