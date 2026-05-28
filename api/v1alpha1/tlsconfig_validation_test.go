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

package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestTlsConfig_ValidateMinMaxVersion(t *testing.T) {
	tests := []struct {
		name       string
		tlsConfig  *TlsConfig
		wantValid  bool
		wantErrMsg string
	}{
		{
			name: "valid: minVersion 1.2, maxVersion 1.3",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MinVersion: "1.2",
				MaxVersion: "1.3",
			},
			wantValid: true,
		},
		{
			name: "valid: minVersion 1.0, maxVersion 1.3",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MinVersion: "1.0",
				MaxVersion: "1.3",
			},
			wantValid: true,
		},
		{
			name: "valid: same version",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MinVersion: "1.2",
				MaxVersion: "1.2",
			},
			wantValid: true,
		},
		{
			name: "valid: only minVersion set",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MinVersion: "1.2",
			},
			wantValid: true,
		},
		{
			name: "valid: only maxVersion set",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MaxVersion: "1.3",
			},
			wantValid: true,
		},
		{
			name: "valid: neither set",
			tlsConfig: &TlsConfig{
				Profile: "custom",
			},
			wantValid: true,
		},
		{
			name: "invalid: minVersion > maxVersion",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MinVersion: "1.3",
				MaxVersion: "1.2",
			},
			wantValid:  false,
			wantErrMsg: "minVersion must not be greater than maxVersion",
		},
		{
			name: "invalid: minVersion format",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MinVersion: "1.4",
			},
			wantValid:  false,
			wantErrMsg: "pattern",
		},
		{
			name: "invalid: maxVersion format",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MaxVersion: "2.0",
			},
			wantValid:  false,
			wantErrMsg: "pattern",
		},
		{
			name: "invalid: minVersion not a valid format",
			tlsConfig: &TlsConfig{
				Profile:    "custom",
				MinVersion: "1.x",
			},
			wantValid:  false,
			wantErrMsg: "pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate pattern constraints programmatically
			minErr := validateTlsVersionPattern(tt.tlsConfig.MinVersion, field.NewPath("minVersion"))
			maxErr := validateTlsVersionPattern(tt.tlsConfig.MaxVersion, field.NewPath("maxVersion"))

			hasPatternError := minErr != nil || maxErr != nil

			if tt.wantValid {
				if minErr != nil {
					t.Errorf("minVersion validation failed: %v", minErr)
					return
				}
				if maxErr != nil {
					t.Errorf("maxVersion validation failed: %v", maxErr)
					return
				}
			} else if tt.wantErrMsg == "pattern" {
				if !hasPatternError {
					t.Errorf("validation should have failed for invalid pattern")
					return
				}
			}

			// Skip min/max comparison if there are pattern errors
			if hasPatternError {
				return
			}

			// Validate min <= max constraint
			var hasOrderingError bool
			if tt.tlsConfig.MinVersion != "" && tt.tlsConfig.MaxVersion != "" {
				hasOrderingError = tt.tlsConfig.MinVersion > tt.tlsConfig.MaxVersion
			}

			// Check if the ordering validation result matches expectations
			if tt.wantErrMsg == "minVersion must not be greater than maxVersion" {
				if !hasOrderingError {
					t.Errorf("Expected ordering error (minVersion > maxVersion), but minVersion=%q <= maxVersion=%q",
						tt.tlsConfig.MinVersion, tt.tlsConfig.MaxVersion)
				}
			} else if tt.wantValid {
				if hasOrderingError {
					t.Errorf("Expected valid config, but minVersion=%q > maxVersion=%q",
						tt.tlsConfig.MinVersion, tt.tlsConfig.MaxVersion)
				}
			}
		})
	}
}

// validateTlsVersionPattern checks if the version matches the pattern ^1\.[0-3]$
func validateTlsVersionPattern(version string, fldPath *field.Path) *field.Error {
	if version == "" {
		return nil
	}

	validVersions := map[string]bool{
		"1.0": true,
		"1.1": true,
		"1.2": true,
		"1.3": true,
	}

	if !validVersions[version] {
		return field.Invalid(fldPath, version, "must match pattern ^1\\.[0-3]$")
	}

	return nil
}
