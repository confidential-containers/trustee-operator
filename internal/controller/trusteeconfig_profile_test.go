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

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
)

// TestValidateProfileType guards the fail-closed behavior for profileType:
// only the exact strings "Permissive" and "Restricted" are accepted. An
// omitted, misspelled or arbitrarily-cased value must produce an error, never
// a silent fallback to the Permissive (test-only) configuration.
func TestValidateProfileType(t *testing.T) {
	testCases := []struct {
		name    string
		profile confidentialcontainersorgv1alpha1.ProfileType
		wantErr bool
	}{
		{"exact Permissive", confidentialcontainersorgv1alpha1.ProfileTypePermissive, false},
		{"exact Restricted", confidentialcontainersorgv1alpha1.ProfileTypeRestrictive, false},
		{"omitted (empty string)", "", true},
		{"lowercase restricted", "restricted", true},
		{"mixed case Restrictive", "Restrictive", true},
		{"lowercase permissive", "permissive", true},
		{"arbitrary string", "banana", true},
		{"leading whitespace", " Permissive", true},
		{"trailing whitespace", "Restricted ", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProfileType(tc.profile)
			if tc.wantErr && err == nil {
				t.Errorf("validateProfileType(%q) = nil, want error", tc.profile)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateProfileType(%q) = %v, want nil", tc.profile, err)
			}
		})
	}
}

// TestProfileTransitionDescription checks the status message written when an
// in-place profileType change is detected: it must name both profiles and the
// delete-and-recreate requirement.
func TestProfileTransitionDescription(t *testing.T) {
	got := profileTransitionDescription(
		confidentialcontainersorgv1alpha1.ProfileTypePermissive,
		confidentialcontainersorgv1alpha1.ProfileTypeRestrictive)
	for _, want := range []string{
		`profileType is "Restricted"`,
		`generated for profile "Permissive"`,
		"delete and recreate",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("profileTransitionDescription message %q does not contain %q", got, want)
		}
	}
}
