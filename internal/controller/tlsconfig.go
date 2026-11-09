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
	"strings"

	configv1 "github.com/openshift/api/config/v1"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
)

// KbsConfigTemplateData holds data for rendering KBS config templates
type KbsConfigTemplateData struct {
	TlsProfile    string
	TlsMinVersion string
	TlsMaxVersion string
	TlsCiphers    string
	TlsGroups     string
}

// GetTLSConfigFromTlsConfig converts TlsConfig to template data
// Returns template data with safe defaults if TlsConfig is nil
func GetTLSConfigFromTlsConfig(tlsConfig *confidentialcontainersorgv1alpha1.TlsConfig) *KbsConfigTemplateData {
	// Default to intermediate profile
	if tlsConfig == nil {
		return newPredefinedProfileData("intermediate")
	}

	profile := tlsConfig.Profile

	// If profile is empty, default to intermediate
	if profile == "" {
		profile = "intermediate"
	}

	// For custom profile, pass through the explicitly provided fields.
	if profile == "custom" {
		data := &KbsConfigTemplateData{
			TlsProfile:    "custom",
			TlsMinVersion: tlsConfig.MinVersion,
			TlsMaxVersion: tlsConfig.MaxVersion,
		}

		if len(tlsConfig.Ciphers) > 0 {
			data.TlsCiphers = convertCiphers(tlsConfig.Ciphers)
		}

		if len(tlsConfig.Groups) > 0 {
			data.TlsGroups = strings.Join(tlsConfig.Groups, ":")
		}

		return data
	}

	// Predefined profiles (old / intermediate / modern).
	return newPredefinedProfileData(profile)
}

// newPredefinedProfileData builds template data for a predefined profile
// (old/intermediate/modern), expanding it into the profile's explicit cipher
// list.
//
// Without this, KBS only receives the profile name and bases its listener on
// OpenSSL's SslAcceptor::mozilla_intermediate_v5, whose cipher list includes
// DHE-RSA ciphers. Passing an explicit cipher list makes KBS serve exactly the
// ciphers we select. The definitions are sourced from the Mozilla-based TLS
// profiles in github.com/openshift/api.
//
// Finite-field DHE ciphers are filtered out: they are not post-quantum relevant,
// are being retired from the Mozilla/OpenShift intermediate profile, and are
// flagged by OpenShift TLS adherence checks. Filtering them here keeps the
// behaviour stable regardless of the github.com/openshift/api version in use
// (older revisions still list DHE in the intermediate profile).
func newPredefinedProfileData(profile string) *KbsConfigTemplateData {
	data := &KbsConfigTemplateData{TlsProfile: profile}

	var profileType configv1.TLSProfileType
	switch profile {
	case "old":
		profileType = configv1.TLSProfileOldType
	case "modern":
		profileType = configv1.TLSProfileModernType
	default:
		profileType = configv1.TLSProfileIntermediateType
	}

	if spec, ok := configv1.TLSProfiles[profileType]; ok {
		// Profile ciphers are already in OpenSSL format.
		data.TlsCiphers = strings.Join(filterDHECiphers(spec.Ciphers), ":")
	}

	return data
}

// filterDHECiphers returns the cipher list with finite-field Diffie-Hellman
// (DHE) ciphers removed. ECDHE (elliptic-curve) ciphers are preserved.
func filterDHECiphers(ciphers []string) []string {
	filtered := make([]string, 0, len(ciphers))
	for _, c := range ciphers {
		// Cipher names may be in OpenSSL ("DHE-RSA-...") or IANA
		// ("TLS_DHE_RSA_...") form. ECDHE ciphers do not match these prefixes.
		if strings.HasPrefix(c, "DHE-") || strings.HasPrefix(c, "TLS_DHE_") {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// convertCiphers converts IANA cipher names to OpenSSL format
// TLS 1.3 ciphers are passed through unchanged
// TLS 1.2 ciphers are converted from IANA to OpenSSL format
func convertCiphers(ciphers []string) string {
	if len(ciphers) == 0 {
		return ""
	}

	converted := make([]string, 0, len(ciphers))
	for _, cipher := range ciphers {
		converted = append(converted, convertSingleCipher(cipher))
	}

	return strings.Join(converted, ":")
}

// convertSingleCipher converts a single cipher from IANA to OpenSSL format
func convertSingleCipher(cipher string) string {
	// TLS 1.3 ciphers: no conversion needed
	if strings.HasPrefix(cipher, "TLS_AES_") ||
		strings.HasPrefix(cipher, "TLS_CHACHA20_") {
		return cipher
	}

	// Explicit mappings for ciphers that don't follow the generic pattern.
	// This includes:
	// - ChaCha20 ciphers (hash is omitted in OpenSSL name)
	// - RSA key exchange ciphers (have different naming in OpenSSL)
	// - Other ciphers where IANA and OpenSSL names differ
	explicitMappings := map[string]string{
		// ChaCha20 ciphers
		"TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256":   "ECDHE-RSA-CHACHA20-POLY1305",
		"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256": "ECDHE-ECDSA-CHACHA20-POLY1305",
		"TLS_DHE_RSA_WITH_CHACHA20_POLY1305_SHA256":     "DHE-RSA-CHACHA20-POLY1305",

		// RSA key exchange ciphers (OpenSSL omits "RSA" prefix)
		"TLS_RSA_WITH_AES_128_CBC_SHA":    "AES128-SHA",
		"TLS_RSA_WITH_AES_256_CBC_SHA":    "AES256-SHA",
		"TLS_RSA_WITH_AES_128_CBC_SHA256": "AES128-SHA256",
		"TLS_RSA_WITH_AES_256_CBC_SHA256": "AES256-SHA256",
		"TLS_RSA_WITH_AES_128_GCM_SHA256": "AES128-GCM-SHA256",
		"TLS_RSA_WITH_AES_256_GCM_SHA384": "AES256-GCM-SHA384",

		// 3DES ciphers
		"TLS_RSA_WITH_3DES_EDE_CBC_SHA":       "DES-CBC3-SHA",
		"TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA": "ECDHE-RSA-DES-CBC3-SHA",
	}

	if mapped, ok := explicitMappings[cipher]; ok {
		return mapped
	}

	// TLS 1.2 ciphers: IANA → OpenSSL conversion
	// Example: TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 → ECDHE-RSA-AES128-GCM-SHA256

	// Strip TLS_ prefix
	result := strings.TrimPrefix(cipher, "TLS_")

	// Replace _WITH_ with -
	result = strings.Replace(result, "_WITH_", "-", 1)

	// Convert remaining parts: remove _ before numbers, replace _ with - elsewhere
	// Split by _ and rejoin intelligently
	parts := strings.Split(result, "_")
	var converted []string
	for i, part := range parts {
		if i > 0 {
			// Check if current part starts with a digit
			if len(part) > 0 && part[0] >= '0' && part[0] <= '9' {
				// Append without separator (e.g., AES + 128 → AES128)
				if len(converted) > 0 {
					converted[len(converted)-1] += part
					continue
				}
			}
			// Otherwise use dash separator
		}
		converted = append(converted, part)
	}

	return strings.Join(converted, "-")
}
