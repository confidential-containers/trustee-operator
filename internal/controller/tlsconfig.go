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
		return &KbsConfigTemplateData{
			TlsProfile: "intermediate",
		}
	}

	data := &KbsConfigTemplateData{
		TlsProfile: tlsConfig.Profile,
	}

	// If profile is empty, default to intermediate
	if data.TlsProfile == "" {
		data.TlsProfile = "intermediate"
	}

	// For custom profile, include additional fields
	if tlsConfig.Profile == "custom" {
		data.TlsMinVersion = tlsConfig.MinVersion
		data.TlsMaxVersion = tlsConfig.MaxVersion

		if len(tlsConfig.Ciphers) > 0 {
			data.TlsCiphers = convertCiphers(tlsConfig.Ciphers)
		}

		if len(tlsConfig.Groups) > 0 {
			data.TlsGroups = strings.Join(tlsConfig.Groups, ":")
		}
	}

	return data
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
