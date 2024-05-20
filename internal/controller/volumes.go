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
	"path/filepath"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/kbs-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *KbsConfigReconciler) processAuthSecret(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, error) {
	if r.kbsConfig.Spec.KbsAuthSecretName != "" {
		foundSecret := &corev1.Secret{}
		err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: r.namespace,
			Name:      r.kbsConfig.Spec.KbsAuthSecretName,
		}, foundSecret)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Error(err, "KbsAuthSecretName does not exist", "Secret.Namespace", r.namespace, "Secret.Name", r.kbsConfig.Spec.KbsAuthSecretName)
			return nil, err
		} else if err != nil {
			r.log.Error(err, "Failed to get KBS Auth Secret")
			return nil, err
		}

		volumes = append(volumes, corev1.Volume{
			Name: "auth-secret",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.kbsConfig.Spec.KbsAuthSecretName,
				},
			},
		})
	}
	return volumes, nil
}

func (r *KbsConfigReconciler) processHttpsSecret(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, error) {
	httpsConfigPresent, err := r.httpsConfigPresent()
	if err != nil {
		r.log.Error(err, "Failed to get KBS HTTPS secrets")
		return nil, err
	}
	if httpsConfigPresent {
		// get the https key and append to volumes
		foundHttpsKeySecret := &corev1.Secret{}
		err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: r.namespace,
			Name:      r.kbsConfig.Spec.KbsHttpsKeySecretName,
		}, foundHttpsKeySecret)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Error(err, "KbsHttpsKeySecretName does not exist", "Secret.Namespace", r.namespace, "Secret.Name", r.kbsConfig.Spec.KbsHttpsKeySecretName)
			return nil, err
		} else if err != nil {
			r.log.Error(err, "Failed to get KBS HTTPS key Secret")
			return nil, err
		}

		volumes = append(volumes, corev1.Volume{
			Name: "https-key",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.kbsConfig.Spec.KbsHttpsKeySecretName,
				},
			},
		})
		// get the https certificate and append to volumes
		foundHttpsCertSecret := &corev1.Secret{}
		err = r.Client.Get(ctx, client.ObjectKey{
			Namespace: r.namespace,
			Name:      r.kbsConfig.Spec.KbsHttpsCertSecretName,
		}, foundHttpsCertSecret)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Error(err, "KbsHttpsCertSecretName does not exist", "Secret.Namespace", r.namespace, "Secret.Name", r.kbsConfig.Spec.KbsHttpsCertSecretName)
			return nil, err
		} else if err != nil {
			r.log.Error(err, "Failed to get KBS HTTPS Cert Secret")
			return nil, err
		}

		volumes = append(volumes, corev1.Volume{
			Name: "https-cert",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.kbsConfig.Spec.KbsHttpsCertSecretName,
				},
			},
		})
	}
	return volumes, nil
}

func (r *KbsConfigReconciler) processAsConfigMap(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, error) {
	if r.kbsConfig.Spec.KbsAsConfigMapName != "" {
		foundConfigMap := &corev1.ConfigMap{}
		err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: r.namespace,
			Name:      r.kbsConfig.Spec.KbsAsConfigMapName,
		}, foundConfigMap)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Error(err, "KbsAsConfigMapName does not exist", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", r.kbsConfig.Spec.KbsAsConfigMapName)
			return nil, err
		} else if err != nil {
			r.log.Error(err, "Failed to get KBS AS ConfigMap")
			return nil, err
		}

		volumes = append(volumes, corev1.Volume{
			Name: "as-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.kbsConfig.Spec.KbsAsConfigMapName,
					},
				},
			},
		})
	}
	return volumes, nil
}

func (r *KbsConfigReconciler) processRvpsConfigMap(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, error) {
	if r.kbsConfig.Spec.KbsRvpsConfigMapName != "" {
		foundConfigMap := &corev1.ConfigMap{}
		err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: r.namespace,
			Name:      r.kbsConfig.Spec.KbsRvpsConfigMapName,
		}, foundConfigMap)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Error(err, "KbsRvpsConfigMapName does not exist", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", r.kbsConfig.Spec.KbsRvpsConfigMapName)
			return nil, err
		} else if err != nil {
			r.log.Error(err, "Failed to get KBS RVPS ConfigMap")
			return nil, err
		}

		volumes = append(volumes, corev1.Volume{
			Name: "rvps-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.kbsConfig.Spec.KbsRvpsConfigMapName,
					},
				},
			},
		})
	}
	return volumes, nil
}

func (r *KbsConfigReconciler) processRvpsRefValuesConfigMap(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, error) {
	referenceValuesMapName := r.kbsConfig.Spec.KbsRvpsRefValuesConfigMapName
	if referenceValuesMapName != "" {
		foundConfigMap := &corev1.ConfigMap{}
		err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: r.namespace,
			Name:      referenceValuesMapName,
		}, foundConfigMap)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Error(err, "KbsRvpsReferenceValuesMapName does not exist", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", referenceValuesMapName)
			return nil, err
		} else if err != nil {
			r.log.Error(err, "Failed to get KBS RVPS ReferenceValuesMap")
			return nil, err
		}

		volumes = append(volumes, corev1.Volume{
			Name: "reference-values",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: referenceValuesMapName,
					},
				},
			},
		})
	}
	return volumes, nil
}

func (r *KbsConfigReconciler) processKbsConfigMap(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, error) {
	if r.kbsConfig.Spec.KbsConfigMapName != "" {
		foundConfigMap := &corev1.ConfigMap{}
		err := r.Client.Get(ctx, client.ObjectKey{
			Namespace: r.namespace,
			Name:      r.kbsConfig.Spec.KbsConfigMapName,
		}, foundConfigMap)
		if err != nil && k8serrors.IsNotFound(err) {
			r.log.Error(err, "KbsConfigMapName does not exist", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", r.kbsConfig.Spec.KbsConfigMapName)
			return nil, err
		} else if err != nil {
			r.log.Error(err, "Failed to get KBS ConfigMap")
			return nil, err
		}

		volumes = append(volumes, corev1.Volume{
			Name: "kbs-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.kbsConfig.Spec.KbsConfigMapName,
					},
				},
			},
		})
	}
	return volumes, nil
}

// Method to add KbsSecretResources to the KBS volumes
func (r *KbsConfigReconciler) processKbsSecretResources(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, error) {
	if r.kbsConfig.Spec.KbsSecretResources != nil {
		for _, secretResource := range r.kbsConfig.Spec.KbsSecretResources {
			foundSecret := &corev1.Secret{}
			err := r.Client.Get(ctx, client.ObjectKey{
				Namespace: r.namespace,
				Name:      secretResource,
			}, foundSecret)
			if err != nil && k8serrors.IsNotFound(err) {
				r.log.Error(err, "KbsSecretResource does not exist", "Secret.Namespace", r.namespace, "Secret.Name", secretResource)
				return nil, err
			} else if err != nil {
				r.log.Error(err, "Failed to get KBS Secret Resource")
				return nil, err
			}

			volumes = append(volumes, corev1.Volume{
				Name: secretResource,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretResource,
					},
				},
			})
		}
	}
	return volumes, nil
}

func (r *KbsConfigReconciler) buildKbsVolumeMounts(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, []corev1.VolumeMount, error) {
	var kbsEtcVolumes, kbsSecretResourceVolumes []corev1.Volume
	kbsEtcVolumes, err := r.processKbsConfigMap(ctx, kbsEtcVolumes)
	if err != nil {
		return nil, nil, err
	}
	kbsEtcVolumes, err = r.processAuthSecret(ctx, kbsEtcVolumes)
	if err != nil {
		return nil, nil, err
	}
	kbsEtcVolumes, err = r.processHttpsSecret(ctx, kbsEtcVolumes)
	if err != nil {
		return nil, nil, err
	}
	// All the above kbsVolumes gets mounted under "/etc" directory
	volumeMounts := volumesToVolumeMounts(kbsEtcVolumes, kbsDefaultConfigPath)
	volumes = append(volumes, kbsEtcVolumes...)

	kbsSecretResourceVolumes, err = r.processKbsSecretResources(ctx, kbsSecretResourceVolumes)
	if err != nil {
		return nil, nil, err
	}

	// Add the kbsSecretResourceVolumes to the volumesMounts
	volumeMounts = append(volumeMounts, volumesToVolumeMounts(kbsSecretResourceVolumes, kbsResourcesPath)...)
	volumes = append(volumes, kbsSecretResourceVolumes...)

	// For the DeploymentTypeAllInOne case, if reference-values.json file is provided must be mounted as a kbs volume
	if r.kbsConfig.Spec.KbsDeploymentType == confidentialcontainersorgv1alpha1.DeploymentTypeAllInOne {
		var rvpsRefValuesVolumes []corev1.Volume
		rvpsRefValuesVolumes, err = r.processRvpsRefValuesConfigMap(ctx, rvpsRefValuesVolumes)
		if err != nil {
			return nil, nil, err
		}
		volumeMounts = append(volumeMounts, volumesToVolumeMounts(rvpsRefValuesVolumes, rvpsReferenceValuesPath)...)
		volumes = append(volumes, rvpsRefValuesVolumes...)

	}

	return volumes, volumeMounts, nil
}

func (r *KbsConfigReconciler) buildAsVolumesMounts(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, []corev1.VolumeMount, error) {
	var asVolumes []corev1.Volume
	asVolumes, err := r.processAsConfigMap(ctx, asVolumes)
	if err != nil {
		return nil, nil, err
	}
	volumeMounts := volumesToVolumeMounts(asVolumes, asDefaultConfigPath)
	volumes = append(volumes, asVolumes...)
	return volumes, volumeMounts, nil
}

func (r *KbsConfigReconciler) buildRvpsVolumesMounts(ctx context.Context, volumes []corev1.Volume) ([]corev1.Volume, []corev1.VolumeMount, error) {
	var rvpsVolumes []corev1.Volume
	rvpsVolumes, err := r.processRvpsConfigMap(ctx, rvpsVolumes)
	if err != nil {
		return nil, nil, err
	}
	var referenceValuesVolumes []corev1.Volume
	referenceValuesVolumes, err = r.processRvpsRefValuesConfigMap(ctx, referenceValuesVolumes)
	if err != nil {
		return nil, nil, err
	}
	volumeMounts := volumesToVolumeMounts(rvpsVolumes, rvpsDefaultConfigPath)
	volumeRefValuesMounts := volumesToVolumeMounts(referenceValuesVolumes, rvpsReferenceValuesPath)
	volumeMounts = append(volumeMounts, volumeRefValuesMounts...)
	volumes = append(volumes, rvpsVolumes...)
	volumes = append(volumes, referenceValuesVolumes...)
	return volumes, volumeMounts, nil
}

// Method to add volumeMounts for KBS under custom directory
func volumesToVolumeMounts(volumes []corev1.Volume, mountPath string) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{}
	for _, volume := range volumes {
		// Create MountPath ensuring file path separators are handled correctly
		mountPath := filepath.Join(mountPath, volume.Name)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volume.Name,
			MountPath: mountPath,
		})
	}
	return volumeMounts
}
