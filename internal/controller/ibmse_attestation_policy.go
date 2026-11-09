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
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ibmseAttestationPolicyConfigMapName is the auto-generated ConfigMap name.
	ibmseAttestationPolicyConfigMapName = "ibmse-attestation-policy"

	// seMessageSecretKey is the key inside the se-message Secret.
	seMessageSecretKey = "se-message.json"

	// ibmseAttestationPolicyFilename is the file name inside the ConfigMap.
	ibmseAttestationPolicyFilename = "default.rego"
)

// seMessage holds the IBM SE attestation values extracted from the pvextract-hdr output.
type seMessage struct {
	AttestationPhkh string `json:"se.attestation_phkh"`
	ImagePhkh       string `json:"se.image_phkh"`
	Tag             string `json:"se.tag"`
}

// getIBMSEAttestationPolicyConfigMapName returns the auto-generated ConfigMap name.
func (r *TrusteeConfigReconciler) getIBMSEAttestationPolicyConfigMapName() string {
	return r.trusteeConfig.Name + "-" + ibmseAttestationPolicyConfigMapName
}

// createOrUpdateIBMSEAttestationPolicy reads the se-message Secret and renders
// the IBM SE-specific attestation policy Rego, creating or updating the
// corresponding ConfigMap.  This replaces the manual copy-paste step described
// in the runbook.
//
// The Secret must contain a "se-message.json" key with JSON like:
//
//	{
//	    "se.attestation_phkh": "<hex>",
//	    "se.image_phkh":       "<hex>",
//	    "se.tag":              "<hex>"
//	}
func (r *TrusteeConfigReconciler) createOrUpdateIBMSEAttestationPolicy(ctx context.Context) (string, error) {
	seSecretName := r.trusteeConfig.Spec.IbmSE.SeMessageSecretName

	// Fetch the se-message Secret.
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: seSecretName}, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			return "", fmt.Errorf(
				"se-message Secret %q not found in namespace %q — "+
					"run hack/ibmse-bundle-generator with --se-message to create it",
				seSecretName, r.namespace,
			)
		}
		return "", fmt.Errorf("failed to get se-message Secret %q: %w", seSecretName, err)
	}

	raw, ok := secret.Data[seMessageSecretKey]
	if !ok {
		return "", fmt.Errorf(
			"se-message Secret %q is missing key %q", seSecretName, seMessageSecretKey,
		)
	}

	// Parse and validate.
	var msg seMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", fmt.Errorf("failed to parse se-message JSON: %w", err)
	}
	if msg.AttestationPhkh == "" || msg.ImagePhkh == "" || msg.Tag == "" {
		return "", fmt.Errorf(
			"se-message JSON in Secret %q missing one or more required fields "+
				"(se.attestation_phkh, se.image_phkh, se.tag)",
			seSecretName,
		)
	}

	// Render the Rego policy.
	policy := fmt.Sprintf(`package policy
import rego.v1
default allow = false
converted_version := sprintf("%%v", [input["se.version"]])

allow if {
    input["se.attestation_phkh"] == %q
    input["se.image_phkh"] == %q
    input["se.tag"] == %q
    input["se.user_data"] == "00"
    converted_version == "256"
}
`, msg.AttestationPhkh, msg.ImagePhkh, msg.Tag)

	cmName := r.getIBMSEAttestationPolicyConfigMapName()
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: r.namespace,
		},
		Data: map[string]string{
			ibmseAttestationPolicyFilename: policy,
		},
	}
	if err := ctrl.SetControllerReference(r.trusteeConfig, desired, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set controller reference on IBM SE attestation policy ConfigMap: %w", err)
	}

	found := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: cmName}, found)
	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating IBM SE attestation policy ConfigMap", "ConfigMap.Name", cmName)
		if err := r.Create(ctx, desired); err != nil {
			return "", fmt.Errorf("failed to create IBM SE attestation policy ConfigMap: %w", err)
		}
		return cmName, nil
	} else if err != nil {
		return "", fmt.Errorf("failed to get IBM SE attestation policy ConfigMap: %w", err)
	}

	// Update if the policy content changed (e.g. after a bundle rotation / hdr.bin re-generation).
	if found.Data[ibmseAttestationPolicyFilename] != policy {
		r.log.Info("IBM SE attestation policy changed; updating ConfigMap", "ConfigMap.Name", cmName)
		found.Data = desired.Data
		if err := r.Update(ctx, found); err != nil {
			return "", fmt.Errorf("failed to update IBM SE attestation policy ConfigMap: %w", err)
		}
	} else {
		r.log.V(1).Info("IBM SE attestation policy unchanged", "ConfigMap.Name", cmName)
	}

	return cmName, nil
}
