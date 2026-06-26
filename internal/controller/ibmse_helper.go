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

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

// isIBMSE returns true if IBM SE configuration is specified
func (r *TrusteeConfigReconciler) isIBMSE() bool {
	return r.trusteeConfig.Spec.TeeType == confidentialcontainersorgv1alpha1.TeeTypeIbmSel
}

// getIBMSEPVCName returns the auto-generated PVC name for IBM SE
func (r *TrusteeConfigReconciler) getIBMSEPVCName() string {
	return r.trusteeConfig.Name + "-ibmse-certstore-pvc"
}

// getIBMSEPVName returns the name for the IBM SE PersistentVolume
func (r *TrusteeConfigReconciler) getIBMSEPVName() string {
	return r.trusteeConfig.Name + "-ibmse-pv"
}

// generateIBMSEPV generates the PersistentVolume for IBM SE
func (r *TrusteeConfigReconciler) generateIBMSEPV() *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.getIBMSEPVName(),
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("100Mi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadOnlyMany,
			},
			StorageClassName: "",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				Local: &corev1.LocalVolumeSource{
					Path: "/opt/confidential-containers/ibmse",
				},
			},
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "node-role.kubernetes.io/worker",
									Operator: corev1.NodeSelectorOpExists,
								},
							},
						},
					},
				},
			},
		},
	}
}

// createOrUpdateIBMSEPV creates or updates the PersistentVolume for IBM SE
func (r *TrusteeConfigReconciler) createOrUpdateIBMSEPV(ctx context.Context) error {
	pvName := r.getIBMSEPVName()
	found := &corev1.PersistentVolume{}
	err := r.Get(ctx, client.ObjectKey{Name: pvName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating IBM SE PersistentVolume", "PV.Name", pvName)
		if err := r.Create(ctx, r.generateIBMSEPV()); err != nil {
			return fmt.Errorf("failed to create IBM SE PV: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get IBM SE PV: %w", err)
	}

	if !apiequality.Semantic.DeepEqual(found.Spec, r.generateIBMSEPV().Spec) {
		r.log.Info("Updating IBM SE PersistentVolume", "PV.Name", pvName)
		found.Spec = r.generateIBMSEPV().Spec
		if err := r.Update(ctx, found); err != nil {
			return fmt.Errorf("failed to update IBM SE PV: %w", err)
		}
		return nil
	}

	r.log.V(1).Info("IBM SE PersistentVolume unchanged", "PV.Name", pvName)
	return nil
}

// generateIBMSEPVC generates the PersistentVolumeClaim for IBM SE
func (r *TrusteeConfigReconciler) generateIBMSEPVC() *corev1.PersistentVolumeClaim {
	pvcName := r.getIBMSEPVCName()
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: r.namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadOnlyMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
			VolumeName: r.getIBMSEPVName(),
		},
	}
}

// createOrUpdateIBMSEPVC creates or updates the PersistentVolumeClaim for IBM SE
func (r *TrusteeConfigReconciler) createOrUpdateIBMSEPVC(ctx context.Context) error {
	pvcName := r.getIBMSEPVCName()
	desired := r.generateIBMSEPVC()
	if err := ctrl.SetControllerReference(r.trusteeConfig, desired, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference for IBM SE PVC: %w", err)
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

	if !metav1.IsControlledBy(found, r.trusteeConfig) {
		r.log.Info("Adopting existing IBM SE PersistentVolumeClaim", "PVC.Namespace", r.namespace, "PVC.Name", pvcName)
		found.OwnerReferences = desired.OwnerReferences
		if err := r.Update(ctx, found); err != nil {
			return fmt.Errorf("failed to adopt IBM SE PVC: %w", err)
		}
	}

	if found.Spec.VolumeName != desired.Spec.VolumeName {
		return fmt.Errorf("existing IBM SE PVC %s/%s is bound to volume %q, expected %q", r.namespace, pvcName, found.Spec.VolumeName, desired.Spec.VolumeName)
	}

	r.log.V(1).Info("IBM SE PersistentVolumeClaim reconciled", "PVC.Namespace", r.namespace, "PVC.Name", pvcName)
	return nil
}
