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
	"errors"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *KbsConfigReconciler) createKbsConfigMapVolume(ctx context.Context, volumeName string) (*corev1.Volume, error) {
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

		volume := corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.kbsConfig.Spec.KbsConfigMapName,
					},
				},
			},
		}
		return &volume, nil
	}
	return nil, errors.New("KbsConfigMapName hasn't been provided")
}

func (r *KbsConfigReconciler) createAuthSecretVolume(ctx context.Context, volumeName string) (*corev1.Volume, error) {
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

		volume := corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: r.kbsConfig.Spec.KbsAuthSecretName,
				},
			},
		}
		return &volume, nil
	}
	return nil, errors.New("KbsAuthSecretName hasn't been provided")
}

func (r *KbsConfigReconciler) createHttpsKeyVolume(ctx context.Context, volumeName string) (*corev1.Volume, error) {
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

	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: r.kbsConfig.Spec.KbsHttpsKeySecretName,
			},
		},
	}

	return &volume, nil
}

func (r *KbsConfigReconciler) createHttpsCertVolume(ctx context.Context, volumeName string) (*corev1.Volume, error) {
	// get the https key and append to volumes
	// get the https certificate and append to volumes
	foundHttpsCertSecret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
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

	volume := corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: r.kbsConfig.Spec.KbsHttpsCertSecretName,
			},
		},
	}

	return &volume, nil
}

// Method to add KbsSecretResources to the KBS volumes
func (r *KbsConfigReconciler) createKbsSecretResourcesVolume(ctx context.Context) ([]corev1.Volume, error) {
	var secretVolumes []corev1.Volume
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

			secretVolumes = append(secretVolumes, corev1.Volume{
				Name: secretResource,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: secretResource,
					},
				},
			})
		}
	}
	return secretVolumes, nil
}

func (r *KbsConfigReconciler) createRvpsRefValuesConfigMapVolume(ctx context.Context, volumeName string) (*corev1.Volume, error) {
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

		volume := corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: referenceValuesMapName,
					},
				},
			},
		}
		return &volume, nil
	}
	return nil, errors.New("KbsRvpsRefValuesConfigMapName hasn't been provided")
}

func (r *KbsConfigReconciler) createAsConfigMapVolume(ctx context.Context, volumeName string) (*corev1.Volume, error) {
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

		volume := corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.kbsConfig.Spec.KbsAsConfigMapName,
					},
				},
			},
		}
		return &volume, nil
	}
	return nil, errors.New("KbsAsConfigMapName hasn't been provided")
}

func (r *KbsConfigReconciler) processRvpsConfigMapVolume(ctx context.Context, volumeName string) (*corev1.Volume, error) {
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

		volume := corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.kbsConfig.Spec.KbsRvpsConfigMapName,
					},
				},
			},
		}
		return &volume, nil
	}
	return nil, errors.New("KbsRvpsConfigMapName hasn't been provided")
}

func createVolumeMount(volumeName string, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: mountPath,
	}
}
