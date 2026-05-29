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
	"testing"
)

func TestMergeTlsSettings(t *testing.T) {
	existingConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
insecure_http = false
private_key = "/etc/https-key/privateKey"
certificate = "/etc/https-cert/certificate"
worker_count = 4

# TLS configuration - Mozilla modern profile

tls_profile = "modern"

[admin]
type = "DenyAll"
insecure_api = false
auth_public_key = "/etc/auth-secret/publicKey"
`

	newConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
insecure_http = false
private_key = "/etc/https-key/privateKey"
certificate = "/etc/https-cert/certificate"
worker_count = 4

# TLS configuration - Mozilla intermediate profile

tls_profile = "intermediate"

[admin]
type = "DenyAll"
`

	result := mergeTlsSettings(existingConfig, newConfig)

	// Should have intermediate profile now
	if !strings.Contains(result, `tls_profile = "intermediate"`) {
		t.Errorf("Expected intermediate profile in result, got:\n%s", result)
	}

	// Should NOT have modern profile
	if strings.Contains(result, `tls_profile = "modern"`) {
		t.Errorf("Expected modern profile to be replaced, got:\n%s", result)
	}

	// Should preserve admin section
	if !strings.Contains(result, `auth_public_key = "/etc/auth-secret/publicKey"`) {
		t.Errorf("Expected admin section to be preserved, got:\n%s", result)
	}

	// Should preserve http_server non-TLS settings
	if !strings.Contains(result, `worker_count = 4`) {
		t.Errorf("Expected worker_count to be preserved, got:\n%s", result)
	}
}

func TestMergeTlsSettings_CustomProfile(t *testing.T) {
	existingConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
insecure_http = false
worker_count = 4

# TLS configuration - Mozilla modern profile

tls_profile = "modern"

[admin]
type = "DenyAll"
`

	newConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
insecure_http = false
worker_count = 4

# TLS configuration - Mozilla custom profile

tls_min_version = "1.2"
tls_max_version = "1.3"
tls_ciphers = "TLS_AES_128_GCM_SHA256:ECDHE-RSA-AES128-GCM-SHA256"
tls_groups = "x25519:secp256r1"

[admin]
type = "DenyAll"
`

	result := mergeTlsSettings(existingConfig, newConfig)

	// Should have custom TLS settings
	if !strings.Contains(result, `tls_min_version = "1.2"`) {
		t.Errorf("Expected tls_min_version in result, got:\n%s", result)
	}
	if !strings.Contains(result, `tls_ciphers = "TLS_AES_128_GCM_SHA256:ECDHE-RSA-AES128-GCM-SHA256"`) {
		t.Errorf("Expected tls_ciphers in result, got:\n%s", result)
	}

	// Should NOT have old profile setting
	if strings.Contains(result, `tls_profile = "modern"`) {
		t.Errorf("Expected old tls_profile to be removed, got:\n%s", result)
	}

	// Should preserve admin section
	if !strings.Contains(result, `[admin]`) {
		t.Errorf("Expected admin section to be preserved, got:\n%s", result)
	}
}

func TestMergeTlsSettings_NoTlsInNew(t *testing.T) {
	existingConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
worker_count = 4

# TLS configuration - Mozilla modern profile

tls_profile = "modern"

[admin]
type = "DenyAll"
`

	newConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
worker_count = 4

[admin]
type = "DenyAll"
`

	result := mergeTlsSettings(existingConfig, newConfig)

	// Should preserve existing config unchanged when new has no TLS
	if result != existingConfig {
		t.Errorf("Expected existing config to be preserved when new has no TLS, got:\n%s", result)
	}
}

func TestMergeTlsSettings_Idempotent(t *testing.T) {
	// Config with TLS block including blank lines
	config := `[http_server]
sockets = ["0.0.0.0:8080"]
insecure_http = false
worker_count = 4

# TLS configuration - Mozilla intermediate profile

tls_profile = "intermediate"

[admin]
type = "DenyAll"
`

	// Merge with the same config (simulating reconciliation)
	result := mergeTlsSettings(config, config)

	// Should be idempotent - result should equal input
	if result != config {
		t.Errorf("Merge is not idempotent.\nExpected:\n%s\nGot:\n%s", config, result)
	}

	// Apply merge again - should still be idempotent
	result2 := mergeTlsSettings(result, config)
	if result2 != config {
		t.Errorf("Second merge is not idempotent.\nExpected:\n%s\nGot:\n%s", config, result2)
	}
}

func TestMergeTlsSettings_BlankLinesInTlsBlock(t *testing.T) {
	existingConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
worker_count = 4

# TLS configuration - Mozilla modern profile

tls_profile = "modern"

[admin]
type = "DenyAll"
`

	newConfig := `[http_server]
sockets = ["0.0.0.0:8080"]
worker_count = 4

# TLS configuration - Mozilla intermediate profile

tls_profile = "intermediate"

[admin]
type = "DenyAll"
`

	result := mergeTlsSettings(existingConfig, newConfig)

	// Should have intermediate profile
	if !strings.Contains(result, `tls_profile = "intermediate"`) {
		t.Errorf("Expected intermediate profile in result")
	}

	// Should NOT have modern profile
	if strings.Contains(result, `tls_profile = "modern"`) {
		t.Errorf("Expected modern profile to be removed")
	}

	// Should NOT have duplicate blank lines
	lines := strings.Split(result, "\n")
	consecutiveBlankCount := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			consecutiveBlankCount++
			if consecutiveBlankCount > 2 {
				t.Errorf("Found more than 2 consecutive blank lines, merge is accumulating blank lines:\n%s", result)
				break
			}
		} else {
			consecutiveBlankCount = 0
		}
	}
}

func TestIsTlsLine(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		{`tls_profile = "modern"`, true},
		{`tls_min_version = "1.2"`, true},
		{`tls_max_version = "1.3"`, true},
		{`tls_ciphers = "..."`, true},
		{`tls_groups = "..."`, true},
		{`# TLS configuration - Mozilla modern profile`, true},
		{`worker_count = 4`, false},
		{`insecure_http = false`, false},
		{`[admin]`, false},
	}

	for _, tt := range tests {
		result := isTlsLine(tt.line)
		if result != tt.expected {
			t.Errorf("isTlsLine(%q) = %v, expected %v", tt.line, result, tt.expected)
		}
	}
}
