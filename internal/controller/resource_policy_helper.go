/*
Copyright Confidential Containers Contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"os"
)

// generateResourcePolicyRego generates the Rego policy content based on profile type
func generateResourcePolicyRego(profileType string) (string, error) {
	var templateFile string

	// Select template file based on profile type
	switch profileType {
	case "Restricted":
		templateFile = "/config/templates/resource-policy-restrictive.rego"
	case "Permissive":
		templateFile = "/config/templates/resource-policy-permissive.rego"
	default:
		templateFile = "/config/templates/resource-policy-permissive.rego"
	}

	// Read the template file
	policyBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return "", err
	}

	return string(policyBytes), nil
}

// generateCpuAttestationPolicyRego generates the Rego policy content for CPU attestation policy
// Uses the same policy template for both permissive and restrictive profiles
func generateCpuAttestationPolicyRego(profileType string) (string, error) {
	// Use the same attestation policy template for all profiles
	templateFile := "/config/templates/ear_default_attestation_policy_cpu.rego"

	// Read the template file
	policyBytes, err := os.ReadFile(templateFile)
	if err != nil {
		return "", err
	}

	return string(policyBytes), nil
}
