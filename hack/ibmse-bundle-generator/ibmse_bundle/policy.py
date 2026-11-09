"""
policy.py — Parse se-message JSON produced by pvextract-hdr / GetRvps.sh
and render the IBM SE attestation policy ConfigMap data.

The se-message file contains JSON like:
    {
        "se.attestation_phkh": "<hex>",
        "se.image_phkh":       "<hex>",
        "se.tag":              "<hex>",
        "se.version":          256
    }

This module reads those values and renders the attestation-policy Rego that
the trustee-operator's TrusteeConfig controller will mount into the AS pod.
"""

import json
import logging

from kubernetes import client, config
from kubernetes.client.rest import ApiException

log = logging.getLogger(__name__)

_POLICY_TEMPLATE = """\
package policy
import rego.v1
default allow = false
converted_version := sprintf("%v", [input["se.version"]])

allow if {{
    input["se.attestation_phkh"] == "{attestation_phkh}"
    input["se.image_phkh"] == "{image_phkh}"
    input["se.tag"] == "{tag}"
    input["se.user_data"] == "00"
    converted_version == "256"
}}
"""

# Keys expected in the se-message JSON.
_REQUIRED_KEYS = ("se.attestation_phkh", "se.image_phkh", "se.tag")


def parse_se_message(se_message_json: str) -> dict[str, str]:
    """
    Parse se-message JSON and return the three phkh/tag values.

    Raises:
        ValueError: if any required key is missing.
    """
    data = json.loads(se_message_json)
    missing = [k for k in _REQUIRED_KEYS if k not in data]
    if missing:
        raise ValueError(f"se-message JSON missing required keys: {missing}")
    return {
        "attestation_phkh": data["se.attestation_phkh"],
        "image_phkh": data["se.image_phkh"],
        "tag": data["se.tag"],
    }


def render_attestation_policy(se_message_json: str) -> str:
    """
    Render the ibmse-attestation-policy Rego string from se-message JSON.
    """
    values = parse_se_message(se_message_json)
    return _POLICY_TEMPLATE.format(**values)


def push_attestation_policy_secret(
    *,
    namespace: str,
    se_message_json: str,
    kubeconfig: str | None = None,
) -> None:
    """
    Create or update a Secret named ``ibmse-se-message`` in *namespace*
    that holds the raw se-message JSON.

    The trustee-operator controller reads this Secret to auto-generate
    the ``ibmse-attestation-policy`` ConfigMap without any manual copy-paste.
    """
    if kubeconfig:
        config.load_kube_config(config_file=kubeconfig)
    else:
        try:
            config.load_incluster_config()
        except config.ConfigException:
            config.load_kube_config()

    # Validate the JSON before pushing.
    parse_se_message(se_message_json)

    v1 = client.CoreV1Api()
    secret = client.V1Secret(
        metadata=client.V1ObjectMeta(name="ibmse-se-message", namespace=namespace),
        type="Opaque",
        string_data={"se-message.json": se_message_json},
    )
    try:
        v1.read_namespaced_secret("ibmse-se-message", namespace)
        v1.replace_namespaced_secret("ibmse-se-message", namespace, secret)
        log.info("Updated Secret ibmse-se-message in %s", namespace)
    except ApiException as exc:
        if exc.status == 404:
            v1.create_namespaced_secret(namespace, secret)
            log.info("Created Secret ibmse-se-message in %s", namespace)
        else:
            raise
