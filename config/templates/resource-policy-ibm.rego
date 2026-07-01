package policy
import rego.v1
# Default deny - only allow if all conditions pass
default allow := false
# Extract IBM SE claims from EAR token using correct path
se_claims := input.submods.cpu0["ear.veraison.annotated-evidence"].se
# Allow access if:
# 1. Plugin is "resource" (LocalFs plugin)
# 2. IBM SE claims exist
# 3. All IBM SE claim values match expected values
allow if {
     data.plugin == "resource"
     se_claims != null
     se_claims.attestation_phkh == "<se.attestation_phkh>"
     se_claims.image_phkh == "<se.image_phkh>"
     se_claims.tag == "<se.tag>"
     se_claims.version == 256
     }
