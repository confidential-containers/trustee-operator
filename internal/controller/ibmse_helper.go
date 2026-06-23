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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// isIBMSE returns true if IBM SE configuration is specified
func (r *TrusteeConfigReconciler) isIBMSE() bool {
	return r.trusteeConfig.Spec.IbmSE != nil
}

// getIBMSEPVCName returns the auto-generated PVC name for IBM SE
func (r *TrusteeConfigReconciler) getIBMSEPVCName() string {
	return r.trusteeConfig.Name + "-ibmse-certstore-pvc"
}

// generateIBMSEPVC generates the PersistentVolumeClaim for IBM SE.
// The PVC binds to the PV named in spec.ibmSEPVName, which must be
// pre-created by the cluster administrator.
func (r *TrusteeConfigReconciler) generateIBMSEPVC() *corev1.PersistentVolumeClaim {
	pvcName := r.getIBMSEPVCName()
	// Match the PV's empty storageClassName to avoid default StorageClass injection.
	sc := ""
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: r.namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &sc,
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadOnlyMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			VolumeName: r.trusteeConfig.Spec.IbmSE.PVName,
		},
	}
}

// validateIBMSEPV checks that the PV named in spec.ibmSE.pvName actually exists.
// This gives the user an actionable error early rather than leaving the PVC in
// Pending state indefinitely with no explanation.
func (r *TrusteeConfigReconciler) validateIBMSEPV(ctx context.Context) error {
	pvName := r.trusteeConfig.Spec.IbmSE.PVName
	if pvName == "" {
		return fmt.Errorf("spec.ibmSE.pvName must be set when ibmSE is configured")
	}
	pv := &corev1.PersistentVolume{}
	err := r.Get(ctx, client.ObjectKey{Name: pvName}, pv)
	if k8serrors.IsNotFound(err) {
		return fmt.Errorf("PersistentVolume %q not found — create the PV before applying the TrusteeConfig", pvName)
	}
	if err != nil {
		return fmt.Errorf("failed to get PersistentVolume %q: %w", pvName, err)
	}
	return nil
}

// createOrUpdateIBMSEPVC creates or updates the PersistentVolumeClaim for IBM SE.
// The PVC is owned by the TrusteeConfig CR and is garbage-collected when it is deleted.
func (r *TrusteeConfigReconciler) createOrUpdateIBMSEPVC(ctx context.Context) error {
	if err := r.validateIBMSEPV(ctx); err != nil {
		return err
	}

	pvcName := r.getIBMSEPVCName()
	desired := r.generateIBMSEPVC()

	if err := ctrl.SetControllerReference(r.trusteeConfig, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on IBM SE PVC: %w", err)
	}

	found := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      pvcName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating IBM SE PersistentVolumeClaim", "PVC.Namespace", r.namespace, "PVC.Name", pvcName)
		if err := r.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create IBM SE PVC: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get IBM SE PVC: %w", err)
	}

	if found.Spec.VolumeName != desired.Spec.VolumeName {
		return fmt.Errorf("existing IBM SE PVC %s/%s is bound to volume %q, expected %q", r.namespace, pvcName, found.Spec.VolumeName, desired.Spec.VolumeName)
	}

	r.log.V(1).Info("IBM SE PersistentVolumeClaim reconciled", "PVC.Namespace", r.namespace, "PVC.Name", pvcName)
	return nil
}
