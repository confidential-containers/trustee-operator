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
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
)

// TrusteeConfigReconciler reconciles a TrusteeConfig object
type TrusteeConfigReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	trusteeConfig *confidentialcontainersorgv1alpha1.TrusteeConfig
	log           logr.Logger
	namespace     string
}

//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=trusteeconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=trusteeconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=trusteeconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=kbsconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=kbsconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *TrusteeConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log = log.FromContext(ctx)
	r.log.Info("Reconciling TrusteeConfig", "TrusteeConfig.Namespace", req.Namespace, "TrusteeConfig.Name", req.Name)

	// Fetch the TrusteeConfig instance
	r.trusteeConfig = &confidentialcontainersorgv1alpha1.TrusteeConfig{}
	err := r.Get(ctx, req.NamespacedName, r.trusteeConfig)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			r.log.Info("TrusteeConfig resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.log.Error(err, "Failed to get TrusteeConfig")
		return ctrl.Result{}, err
	}

	// Set the namespace
	r.namespace = req.Namespace

	// Check if the TrusteeConfig is being deleted
	if r.trusteeConfig.DeletionTimestamp != nil {
		r.log.Info("TrusteeConfig is being deleted")
		return ctrl.Result{}, nil
	}

	// Build the KbsConfigSpec based on TrusteeConfig
	kbsConfigSpec := r.buildKbsConfigSpec(ctx)

	// Create or update the KbsConfig
	kbsConfig := r.createOrUpdateKbsConfig(ctx, kbsConfigSpec)
	if kbsConfig == nil {
		r.log.Info("Failed to create or update KbsConfig")
		return ctrl.Result{}, fmt.Errorf("failed to create or update KbsConfig")
	}

	// Update the TrusteeConfig status
	r.trusteeConfig.Status.IsReady = true

	// Set the KbsConfig reference
	r.trusteeConfig.Status.KbsConfigRef = &corev1.ObjectReference{
		APIVersion: "confidentialcontainers.org/v1alpha1",
		Kind:       "KbsConfig",
		Name:       kbsConfig.Name,
		Namespace:  kbsConfig.Namespace,
	} // Set status description based on KbsConfig status
	r.log.Info("KbsConfig status check", "KbsConfig.IsReady", kbsConfig.Status.IsReady, "KbsConfig.Name", kbsConfig.Name)
	if kbsConfig.Status.IsReady {
		r.trusteeConfig.Status.StatusDescription = "TrusteeConfig is ready and KbsConfig is deployed successfully"
	} else {
		r.trusteeConfig.Status.StatusDescription = "TrusteeConfig is ready but KbsConfig deployment is in progress"
		// Requeue to wait for KbsConfig to become ready
		err = r.Status().Update(ctx, r.trusteeConfig)
		if err != nil {
			r.log.Error(err, "Failed to update TrusteeConfig status")
			return ctrl.Result{}, err
		}
		r.log.Info("KbsConfig not ready yet, requeuing in 10 seconds")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	err = r.Status().Update(ctx, r.trusteeConfig)
	if err != nil {
		r.log.Error(err, "Failed to update TrusteeConfig status")
		return ctrl.Result{}, err
	}

	r.log.Info("Successfully reconciled TrusteeConfig")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrusteeConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&confidentialcontainersorgv1alpha1.TrusteeConfig{}).
		Watches(
			&confidentialcontainersorgv1alpha1.KbsConfig{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &confidentialcontainersorgv1alpha1.TrusteeConfig{}),
		).
		Complete(r)
}

// createOrUpdateKbsConfig creates or updates a KbsConfig based on TrusteeConfig
func (r *TrusteeConfigReconciler) createOrUpdateKbsConfig(ctx context.Context, spec confidentialcontainersorgv1alpha1.KbsConfigSpec) *confidentialcontainersorgv1alpha1.KbsConfig {
	kbsConfigName := r.getKbsConfigName()

	// Check if KbsConfig already exists
	found := &confidentialcontainersorgv1alpha1.KbsConfig{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      kbsConfigName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create new KbsConfig
		r.log.Info("Creating new KbsConfig", "KbsConfig.Namespace", r.namespace, "KbsConfig.Name", kbsConfigName)
		kbsConfig := &confidentialcontainersorgv1alpha1.KbsConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kbsConfigName,
				Namespace: r.namespace,
			},
			Spec: spec,
		}

		// Set TrusteeConfig as the owner
		err = ctrl.SetControllerReference(r.trusteeConfig, kbsConfig, r.Scheme)
		if err != nil {
			r.log.Error(err, "Failed to set controller reference")
			return nil
		}

		err = r.Create(ctx, kbsConfig)
		if err != nil {
			r.log.Error(err, "Failed to create KbsConfig")
			return nil
		}

		return kbsConfig
	} else if err != nil {
		r.log.Error(err, "Failed to get KbsConfig")
		return nil
	}

	// Check if KbsConfig has been manually modified by comparing with generated spec
	// and checking if user-configurable fields differ from what we would generate
	hasManualChanges := r.detectManualChanges(found.Spec, spec)

	if hasManualChanges {
		r.log.Info("Manual changes detected in KbsConfig, performing smart merge",
			"KbsConfig.Namespace", r.namespace, "KbsConfig.Name", kbsConfigName)
		mergedSpec := r.mergeKbsConfigSpecs(spec, found.Spec)
		found.Spec = mergedSpec
	} else {
		r.log.Info("No manual changes detected, applying generated spec")
		found.Spec = spec
	}

	// Update existing KbsConfig
	r.log.Info("Updating existing KbsConfig", "KbsConfig.Namespace", r.namespace, "KbsConfig.Name", kbsConfigName)
	err = r.Update(ctx, found)
	if err != nil {
		r.log.Error(err, "Failed to update KbsConfig")
		return nil
	}

	// Refresh the KbsConfig to get the latest status
	err = r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      kbsConfigName,
	}, found)
	if err != nil {
		r.log.Error(err, "Failed to refresh KbsConfig after update")
	}

	return found
}

// detectManualChanges detects if user has made manual changes to configurable fields
func (r *TrusteeConfigReconciler) detectManualChanges(current, generated confidentialcontainersorgv1alpha1.KbsConfigSpec) bool {
	// Check user-configurable fields that should be preserved
	userConfigurableFields := []bool{
		// Deployment spec
		current.KbsDeploymentSpec.Replicas != nil &&
			(generated.KbsDeploymentSpec.Replicas == nil || *current.KbsDeploymentSpec.Replicas != *generated.KbsDeploymentSpec.Replicas),

		// Custom environment variables
		len(current.KbsEnvVars) > 0 && !r.mapsEqual(current.KbsEnvVars, generated.KbsEnvVars),

		// Custom HTTPS config
		current.KbsHttpsKeySecretName != "" && current.KbsHttpsKeySecretName != generated.KbsHttpsKeySecretName,
		current.KbsHttpsCertSecretName != "" && current.KbsHttpsCertSecretName != generated.KbsHttpsCertSecretName,

		// Custom secret resources
		len(current.KbsSecretResources) > 0 && !r.stringSlicesEqual(current.KbsSecretResources, generated.KbsSecretResources),

		// Custom local cert cache
		len(current.KbsLocalCertCacheSpec.Secrets) > 0 && !r.certCacheSpecsEqual(current.KbsLocalCertCacheSpec, generated.KbsLocalCertCacheSpec),

		// Custom IBM SE config
		current.IbmSEConfigSpec.CertStorePvc != "" && current.IbmSEConfigSpec.CertStorePvc != generated.IbmSEConfigSpec.CertStorePvc,

		// Custom attestation policy
		current.KbsAttestationPolicyConfigMapName != "" && current.KbsAttestationPolicyConfigMapName != generated.KbsAttestationPolicyConfigMapName,
	}

	// Return true if any user-configurable field has been modified
	for _, hasChange := range userConfigurableFields {
		if hasChange {
			return true
		}
	}
	return false
}

// mapsEqual compares two string maps for equality
func (r *TrusteeConfigReconciler) mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if bValue, exists := b[key]; !exists || bValue != value {
			return false
		}
	}
	return true
}

// stringSlicesEqual compares two string slices for equality
func (r *TrusteeConfigReconciler) stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// certCacheSpecsEqual compares two KbsLocalCertCacheSpec for equality
func (r *TrusteeConfigReconciler) certCacheSpecsEqual(a, b confidentialcontainersorgv1alpha1.KbsLocalCertCacheSpec) bool {
	if len(a.Secrets) != len(b.Secrets) {
		return false
	}
	for i := range a.Secrets {
		if a.Secrets[i].SecretName != b.Secrets[i].SecretName || a.Secrets[i].MountPath != b.Secrets[i].MountPath {
			return false
		}
	}
	return true
}

// mergeKbsConfigSpecs intelligently merges TrusteeConfig-generated spec with manually modified spec
func (r *TrusteeConfigReconciler) mergeKbsConfigSpecs(generatedSpec, manualSpec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	merged := generatedSpec

	// Preserve user-configurable fields from manual edits
	// KbsDeploymentSpec.Replicas: preserve user override if different from generated default
	if manualSpec.KbsDeploymentSpec.Replicas != nil &&
		(generatedSpec.KbsDeploymentSpec.Replicas == nil || *manualSpec.KbsDeploymentSpec.Replicas != *generatedSpec.KbsDeploymentSpec.Replicas) {
		merged.KbsDeploymentSpec.Replicas = manualSpec.KbsDeploymentSpec.Replicas
	}

	// Merge environment variables - preserve manual ones, add generated ones
	if merged.KbsEnvVars == nil {
		merged.KbsEnvVars = make(map[string]string)
	}
	if manualSpec.KbsEnvVars != nil {
		for key, value := range manualSpec.KbsEnvVars {
			merged.KbsEnvVars[key] = value
		}
	}

	// Preserve manual HTTPS configuration
	if manualSpec.KbsHttpsKeySecretName != "" {
		merged.KbsHttpsKeySecretName = manualSpec.KbsHttpsKeySecretName
	}
	if manualSpec.KbsHttpsCertSecretName != "" {
		merged.KbsHttpsCertSecretName = manualSpec.KbsHttpsCertSecretName
	}

	// Preserve manual secret resources
	if len(manualSpec.KbsSecretResources) > 0 {
		merged.KbsSecretResources = manualSpec.KbsSecretResources
	}

	// Preserve manual local cert cache configuration
	if len(manualSpec.KbsLocalCertCacheSpec.Secrets) > 0 {
		merged.KbsLocalCertCacheSpec.Secrets = manualSpec.KbsLocalCertCacheSpec.Secrets
	}

	// Preserve manual IBM SE configuration
	if manualSpec.IbmSEConfigSpec.CertStorePvc != "" {
		merged.IbmSEConfigSpec.CertStorePvc = manualSpec.IbmSEConfigSpec.CertStorePvc
	}

	// Preserve manual attestation policy
	if manualSpec.KbsAttestationPolicyConfigMapName != "" {
		merged.KbsAttestationPolicyConfigMapName = manualSpec.KbsAttestationPolicyConfigMapName
	}

	r.log.Info("Merged KbsConfig specs", "preservedFields", []string{
		"KbsDeploymentSpec", "KbsEnvVars", "KbsHttpsKeySecretName", "KbsHttpsCertSecretName",
		"KbsSecretResources", "KbsLocalCertCacheSpec", "IbmSEConfigSpec",
		"KbsAttestationPolicyConfigMapName",
	})

	return merged
}

// buildKbsConfigSpec builds the KbsConfigSpec based on TrusteeConfig
func (r *TrusteeConfigReconciler) buildKbsConfigSpec(ctx context.Context) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	spec := confidentialcontainersorgv1alpha1.KbsConfigSpec{}

	// Set service type from TrusteeConfig
	if r.trusteeConfig.Spec.KbsServiceType != "" {
		spec.KbsServiceType = r.trusteeConfig.Spec.KbsServiceType
	}

	spec.KbsDeploymentType = confidentialcontainersorgv1alpha1.DeploymentTypeAllInOne

	// Set default replicas to 1
	defaultReplicas := int32(1)
	spec.KbsDeploymentSpec.Replicas = &defaultReplicas

	// Configure based on profile type
	switch r.trusteeConfig.Spec.Profile {
	case confidentialcontainersorgv1alpha1.ProfileTypePermissive:
		r.log.Info("Configuring KbsConfig for Permissive profile")
		spec = r.configurePermissiveProfile(ctx, spec)
	case confidentialcontainersorgv1alpha1.ProfileTypeRestrictive:
		r.log.Info("Configuring KbsConfig for Restricted profile")
		spec = r.configureRestrictedProfile(ctx, spec)
	default:
		r.log.Info("Unknown profile type, using default Permissive profile")
		spec = r.configurePermissiveProfile(ctx, spec)
	}

	// Configure HTTPS if specified
	if r.trusteeConfig.Spec.HttpsSpec.TlsSecretName != "" {
		err := r.createOrUpdateHttpsSecrets(ctx)
		if err != nil {
			r.log.Error(err, "Error creating HTTPS secrets")
			return spec
		}
		spec = r.configureHttps(spec)
	}

	// Configure attestation token verification if specified
	if r.trusteeConfig.Spec.AttestationTokenVerificationSpec.TlsSecretName != "" {
		err := r.createOrUpdateAttestationSecrets(ctx)
		if err != nil {
			r.log.Error(err, "Error creating attestation secrets")
			return spec
		}
		spec = r.configureAttestationTokenVerification(spec)
	}

	return spec
}

// configurePermissiveProfile configures KbsConfig for permissive mode
func (r *TrusteeConfigReconciler) configurePermissiveProfile(ctx context.Context, spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// Set environment variables for permissive mode
	if spec.KbsEnvVars == nil {
		spec.KbsEnvVars = make(map[string]string)
	}
	spec.KbsEnvVars["RUST_LOG"] = "debug"

	// Create the KBS config map
	err := r.createOrUpdateKbsConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating KBS config map", "err", err)
		return spec
	}

	// Create the KBS auth secret
	err = r.createOrUpdateKbsAuthSecret(ctx)
	if err != nil {
		r.log.Info("Error creating KBS auth secret", "err", err)
		return spec
	}

	// Create the sample secret kbsre1
	err = r.createOrUpdateKbsSampleSecret(ctx)
	if err != nil {
		r.log.Info("Error creating KBS sample secret", "err", err)
		return spec
	}

	// Create the resource policy config map
	err = r.createOrUpdateResourcePolicyConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating resource policy config map", "err", err)
		return spec
	}

	// Set the config map, auth secret, and resource policy config map names in the spec
	spec.KbsConfigMapName = r.getKbsConfigMapName()
	spec.KbsAuthSecretName = r.getKbsAuthSecretName()
	spec.KbsResourcePolicyConfigMapName = r.getResourcePolicyConfigMapName()
	// Set the sample secret
	spec.KbsSecretResources = append(spec.KbsSecretResources, r.getKbsSampleSecretName())

	// Create the RVPS reference values config map
	err = r.createOrUpdateRvpsReferenceValuesConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating RVPS reference values config map", "err", err)
		return spec
	}

	// Set the RVPS reference values config map name in the spec
	spec.KbsRvpsRefValuesConfigMapName = r.getRvpsReferenceValuesConfigMapName()

	// Create the TDX config map
	err = r.createOrUpdateTdxConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating TDX config map", "err", err)
		return spec
	}

	// Set the TDX config map name in the spec
	spec.TdxConfigSpec.KbsTdxConfigMapName = r.getTdxConfigMapName()
	return spec
}

// configureRestrictedProfile configures KbsConfig for restricted mode
func (r *TrusteeConfigReconciler) configureRestrictedProfile(ctx context.Context, spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	if spec.KbsEnvVars == nil {
		spec.KbsEnvVars = make(map[string]string)
	}

	// Create the KBS config map
	err := r.createOrUpdateKbsConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating KBS config map", "err", err)
		return spec
	}

	// Create the KBS auth secret
	err = r.createOrUpdateKbsAuthSecret(ctx)
	if err != nil {
		r.log.Info("Error creating KBS auth secret", "err", err)
		return spec
	}

	// Create the sample secret kbsres1
	err = r.createOrUpdateKbsSampleSecret(ctx)
	if err != nil {
		r.log.Info("Error creating KBS sample secret", "err", err)
		return spec
	}

	// Set the config map, auth secret, and resource policy config map names in the spec
	spec.KbsConfigMapName = r.getKbsConfigMapName()
	spec.KbsAuthSecretName = r.getKbsAuthSecretName()
	spec.KbsSecretResources = append(spec.KbsSecretResources, r.getKbsSampleSecretName())

	// Create the resource policy config map
	err = r.createOrUpdateResourcePolicyConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating resource policy config map", "err", err)
		return spec
	}

	// Set the resource policy config map name in the spec
	spec.KbsResourcePolicyConfigMapName = r.getResourcePolicyConfigMapName()

	// Create the RVPS reference values config map
	err = r.createOrUpdateRvpsReferenceValuesConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating RVPS reference values config map", "err", err)
		return spec
	}

	// Set the RVPS reference values config map name in the spec
	spec.KbsRvpsRefValuesConfigMapName = r.getRvpsReferenceValuesConfigMapName()

	// Create the TDX config map
	err = r.createOrUpdateTdxConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating TDX config map", "err", err)
		return spec
	}

	// Set the TDX config map name in the spec
	spec.TdxConfigSpec.KbsTdxConfigMapName = r.getTdxConfigMapName()

	return spec
}

// configureHttps configures HTTPS settings for KbsConfig
func (r *TrusteeConfigReconciler) configureHttps(spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// Set the secret names for key and certificate
	spec.KbsHttpsKeySecretName = r.getHttpsKeySecretName()
	spec.KbsHttpsCertSecretName = r.getHttpsCertSecretName()

	return spec
}

// configureAttestationTokenVerification configures attestation token verification for KbsConfig
func (r *TrusteeConfigReconciler) configureAttestationTokenVerification(spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// Set the secret names for key and certificate
	spec.KbsAttestationKeySecretName = r.getAttestationKeySecretName()
	spec.KbsAttestationCertSecretName = r.getAttestationCertSecretName()

	return spec
}

// getKbsConfigName returns the name for the KbsConfig created by this TrusteeConfig
func (r *TrusteeConfigReconciler) getKbsConfigName() string {
	return r.trusteeConfig.Name + "-kbs-config"
}

// getHttpsKeySecretName returns the name for the HTTPS key secret
func (r *TrusteeConfigReconciler) getHttpsKeySecretName() string {
	return r.trusteeConfig.Name + "-https-key-secret"
}

// getHttpsCertSecretName returns the name for the HTTPS certificate secret
func (r *TrusteeConfigReconciler) getHttpsCertSecretName() string {
	return r.trusteeConfig.Name + "-https-cert-secret"
}

// generateKbsTomlConfig generates the TOML configuration for KBS
func (r *TrusteeConfigReconciler) generateKbsTomlConfig() (string, error) {
	var templateFile string

	// Select template file based on profile type
	switch r.trusteeConfig.Spec.Profile {
	case confidentialcontainersorgv1alpha1.ProfileTypeRestrictive:
		templateFile = "/config/templates/kbs-config-restricted.toml"
		r.log.Info("Using restricted configuration template")
	case confidentialcontainersorgv1alpha1.ProfileTypePermissive:
		templateFile = "/config/templates/kbs-config-permissive.toml"
		r.log.Info("Using permissive configuration template")
	default:
		templateFile = "/config/templates/kbs-config-permissive.toml"
		r.log.Info("Using default permissive configuration template")
	}

	// Read the template file
	configBytes, err := os.ReadFile(templateFile)
	if err != nil {
		r.log.Error(err, "Failed to read config template", "template", templateFile)
		return "", err
	}

	return string(configBytes), nil
}

// generateKbsConfigMap creates a ConfigMap for KBS configuration
func (r *TrusteeConfigReconciler) generateKbsConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	configToml, err := r.generateKbsTomlConfig()
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getKbsConfigMapName(),
			Namespace: r.namespace,
		},
		Data: map[string]string{
			"kbs-config.toml": configToml,
		},
	}

	err = ctrl.SetControllerReference(r.trusteeConfig, configMap, r.Scheme)
	if err != nil {
		return nil, err
	}

	return configMap, nil
}

// getKbsConfigMapName returns the name for the KBS config map
func (r *TrusteeConfigReconciler) getKbsConfigMapName() string {
	return r.trusteeConfig.Name + "-kbs-config"
}

// createOrUpdateKbsConfigMap creates or updates the KBS ConfigMap
func (r *TrusteeConfigReconciler) createOrUpdateKbsConfigMap(ctx context.Context) error {
	configMapName := r.getKbsConfigMapName()
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: configMapName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating KBS config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
		configMap, err := r.generateKbsConfigMap(ctx)
		if err != nil {
			return err
		}
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	r.log.Info("Updating KBS config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
	updatedConfigMap, err := r.generateKbsConfigMap(ctx)
	if err != nil {
		return err
	}
	found.Data = updatedConfigMap.Data
	return r.Update(ctx, found)
}

// generateKbsAuthSecret creates a Secret for KBS authentication
func (r *TrusteeConfigReconciler) generateKbsAuthSecret(ctx context.Context) (*corev1.Secret, error) {
	secretName := r.getKbsAuthSecretName()

	// Generate Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		r.log.Error(err, "Failed to generate Ed25519 key pair")
		return nil, err
	}

	// Encode private key to PEM format
	privateKeyPEM, err := encodeEd25519PrivateKeyToPEM(privateKey)
	if err != nil {
		r.log.Error(err, "Failed to encode private key to PEM")
		return nil, err
	}

	// Encode public key to PEM format
	publicKeyPEM, err := encodeEd25519PublicKeyToPEM(publicKey)
	if err != nil {
		r.log.Error(err, "Failed to encode public key to PEM")
		return nil, err
	}

	// Prepare secret data
	data := make(map[string][]byte)
	data["publicKey"] = publicKeyPEM
	data["privateKey"] = privateKeyPEM

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set TrusteeConfig as the owner
	err = ctrl.SetControllerReference(r.trusteeConfig, secret, r.Scheme)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// generateKbsSampleSecret creates a sample Secret for KBS
func (r *TrusteeConfigReconciler) generateKbsSampleSecret(ctx context.Context) (*corev1.Secret, error) {
	secretName := r.getKbsSampleSecretName()

	// Prepare secret data
	data := make(map[string][]byte)
	data["key1"] = []byte("res1val1")
	data["key2"] = []byte("res1val2")

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set TrusteeConfig as the owner
	err := ctrl.SetControllerReference(r.trusteeConfig, secret, r.Scheme)
	if err != nil {
		return nil, err
	}

	return secret, nil
}

// getKbsAuthSecretName returns the name for the KBS auth secret
func (r *TrusteeConfigReconciler) getKbsAuthSecretName() string {
	return r.trusteeConfig.Name + "-auth-secret"
}

// getKbsSampleSecretName returns the name for the KBS sample secret
func (r *TrusteeConfigReconciler) getKbsSampleSecretName() string {
	return "kbsres1"
}

// createOrUpdateKbsAuthSecret creates or updates the KBS auth secret
func (r *TrusteeConfigReconciler) createOrUpdateKbsAuthSecret(ctx context.Context) error {
	secretName := r.getKbsAuthSecretName()

	// Check if the secret already exists
	found := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      secretName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create the secret
		r.log.Info("Creating KBS auth secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		secret, err := r.generateKbsAuthSecret(ctx)
		if err != nil {
			return err
		}
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update the secret
		r.log.Info("Updating KBS auth secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		updatedSecret, err := r.generateKbsAuthSecret(ctx)
		if err != nil {
			return err
		}
		found.Data = updatedSecret.Data
		err = r.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
}

// createOrUpdateKbsSampleSecret creates or updates the KBS sample secret
func (r *TrusteeConfigReconciler) createOrUpdateKbsSampleSecret(ctx context.Context) error {
	secretName := r.getKbsSampleSecretName()

	// Check if the secret already exists
	found := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      secretName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create the secret
		r.log.Info("Creating KBS sample secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		secret, err := r.generateKbsSampleSecret(ctx)
		if err != nil {
			return err
		}
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update the secret
		r.log.Info("Updating KBS auth secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		updatedSecret, err := r.generateKbsSampleSecret(ctx)
		if err != nil {
			return err
		}
		found.Data = updatedSecret.Data
		err = r.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
}

// createOrUpdateHttpsSecrets creates or updates the HTTPS key and certificate secrets from the TLS secret
func (r *TrusteeConfigReconciler) createOrUpdateHttpsSecrets(ctx context.Context) error {
	// Read the TLS secret
	tlsSecret := &corev1.Secret{}
	tlsSecretName := r.trusteeConfig.Spec.HttpsSpec.TlsSecretName
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      tlsSecretName,
	}, tlsSecret)
	if err != nil {
		r.log.Error(err, "Failed to get TLS secret", "Secret.Namespace", r.namespace, "Secret.Name", tlsSecretName)
		return err
	}

	// Verify it's a TLS secret
	if tlsSecret.Type != corev1.SecretTypeTLS {
		err := fmt.Errorf("secret %s is not of type %s, got %s", tlsSecretName, corev1.SecretTypeTLS, tlsSecret.Type)
		r.log.Error(err, "Invalid secret type")
		return err
	}

	// Extract tls.key and tls.crt
	tlsKey, exists := tlsSecret.Data["tls.key"]
	if !exists {
		err := fmt.Errorf("tls.key not found in TLS secret %s", tlsSecretName)
		r.log.Error(err, "Missing tls.key")
		return err
	}

	tlsCert, exists := tlsSecret.Data["tls.crt"]
	if !exists {
		err := fmt.Errorf("tls.crt not found in TLS secret %s", tlsSecretName)
		r.log.Error(err, "Missing tls.crt")
		return err
	}

	// Create or update the key secret
	err = r.createOrUpdateHttpsKeySecret(ctx, tlsKey)
	if err != nil {
		return err
	}

	// Create or update the cert secret
	err = r.createOrUpdateHttpsCertSecret(ctx, tlsCert)
	if err != nil {
		return err
	}

	return nil
}

// createOrUpdateHttpsKeySecret creates or updates the HTTPS key secret
func (r *TrusteeConfigReconciler) createOrUpdateHttpsKeySecret(ctx context.Context, keyData []byte) error {
	secretName := r.getHttpsKeySecretName()

	// Check if the secret already exists
	found := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      secretName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create the secret
		r.log.Info("Creating HTTPS key secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		secret := r.generateHttpsKeySecret(keyData)
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update the secret
		r.log.Info("Updating HTTPS key secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		found.Data = map[string][]byte{
			"privateKey": keyData,
		}
		err = r.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
}

// createOrUpdateHttpsCertSecret creates or updates the HTTPS certificate secret
func (r *TrusteeConfigReconciler) createOrUpdateHttpsCertSecret(ctx context.Context, certData []byte) error {
	secretName := r.getHttpsCertSecretName()

	// Check if the secret already exists
	found := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      secretName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create the secret
		r.log.Info("Creating HTTPS certificate secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		secret := r.generateHttpsCertSecret(certData)
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update the secret
		r.log.Info("Updating HTTPS certificate secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		found.Data = map[string][]byte{
			"certificate": certData,
		}
		err = r.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
}

// generateHttpsKeySecret creates a Secret for HTTPS private key
func (r *TrusteeConfigReconciler) generateHttpsKeySecret(keyData []byte) *corev1.Secret {
	secretName := r.getHttpsKeySecretName()

	data := make(map[string][]byte)
	data["privateKey"] = keyData

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set TrusteeConfig as the owner
	_ = ctrl.SetControllerReference(r.trusteeConfig, secret, r.Scheme)

	return secret
}

// generateHttpsCertSecret creates a Secret for HTTPS certificate
func (r *TrusteeConfigReconciler) generateHttpsCertSecret(certData []byte) *corev1.Secret {
	secretName := r.getHttpsCertSecretName()

	data := make(map[string][]byte)
	data["certificate"] = certData

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set TrusteeConfig as the owner
	_ = ctrl.SetControllerReference(r.trusteeConfig, secret, r.Scheme)

	return secret
}

// createOrUpdateAttestationSecrets creates or updates the attestation key and certificate secrets from the TLS secret
func (r *TrusteeConfigReconciler) createOrUpdateAttestationSecrets(ctx context.Context) error {
	// Read the TLS secret
	tlsSecret := &corev1.Secret{}
	tlsSecretName := r.trusteeConfig.Spec.AttestationTokenVerificationSpec.TlsSecretName
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      tlsSecretName,
	}, tlsSecret)
	if err != nil {
		r.log.Error(err, "Failed to get TLS secret", "Secret.Namespace", r.namespace, "Secret.Name", tlsSecretName)
		return err
	}

	// Verify it's a TLS secret
	if tlsSecret.Type != corev1.SecretTypeTLS {
		err := fmt.Errorf("secret %s is not of type %s, got %s", tlsSecretName, corev1.SecretTypeTLS, tlsSecret.Type)
		r.log.Error(err, "Invalid secret type")
		return err
	}

	// Extract tls.key and tls.crt
	tlsKey, exists := tlsSecret.Data["tls.key"]
	if !exists {
		err := fmt.Errorf("tls.key not found in TLS secret %s", tlsSecretName)
		r.log.Error(err, "Missing tls.key")
		return err
	}

	tlsCert, exists := tlsSecret.Data["tls.crt"]
	if !exists {
		err := fmt.Errorf("tls.crt not found in TLS secret %s", tlsSecretName)
		r.log.Error(err, "Missing tls.crt")
		return err
	}

	// Create or update the key secret
	err = r.createOrUpdateAttestationKeySecret(ctx, tlsKey)
	if err != nil {
		return err
	}

	// Create or update the cert secret
	err = r.createOrUpdateAttestationCertSecret(ctx, tlsCert)
	if err != nil {
		return err
	}

	return nil
}

// createOrUpdateAttestationKeySecret creates or updates the attestation key secret
func (r *TrusteeConfigReconciler) createOrUpdateAttestationKeySecret(ctx context.Context, keyData []byte) error {
	secretName := r.getAttestationKeySecretName()

	// Check if the secret already exists
	found := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      secretName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create the secret
		r.log.Info("Creating attestation key secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		secret := r.generateAttestationKeySecret(keyData)
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update the secret
		r.log.Info("Updating attestation key secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		found.Data = map[string][]byte{
			"token.key": keyData,
		}
		err = r.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
}

// createOrUpdateAttestationCertSecret creates or updates the attestation certificate secret
func (r *TrusteeConfigReconciler) createOrUpdateAttestationCertSecret(ctx context.Context, certData []byte) error {
	secretName := r.getAttestationCertSecretName()

	// Check if the secret already exists
	found := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      secretName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create the secret
		r.log.Info("Creating attestation certificate secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		secret := r.generateAttestationCertSecret(certData)
		err = r.Create(ctx, secret)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update the secret
		r.log.Info("Updating attestation certificate secret", "Secret.Namespace", r.namespace, "Secret.Name", secretName)
		found.Data = map[string][]byte{
			"token.crt": certData,
		}
		err = r.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
}

// generateAttestationCertSecret creates a Secret for attestation certificate
func (r *TrusteeConfigReconciler) generateAttestationCertSecret(certData []byte) *corev1.Secret {
	secretName := r.getAttestationCertSecretName()

	data := make(map[string][]byte)
	data["token.crt"] = certData

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set TrusteeConfig as the owner
	_ = ctrl.SetControllerReference(r.trusteeConfig, secret, r.Scheme)

	return secret
}

// getAttestationKeySecretName returns the name for the attestation key secret
func (r *TrusteeConfigReconciler) getAttestationKeySecretName() string {
	return r.trusteeConfig.Name + "-attestation-key-secret"
}

// getAttestationCertSecretName returns the name for the attestation certificate secret
func (r *TrusteeConfigReconciler) getAttestationCertSecretName() string {
	return r.trusteeConfig.Name + "-attestation-cert-secret"
}

// generateAttestationKeySecret creates a Secret for attestation private key
func (r *TrusteeConfigReconciler) generateAttestationKeySecret(keyData []byte) *corev1.Secret {
	secretName := r.getAttestationKeySecretName()

	data := make(map[string][]byte)
	data["token.key"] = keyData

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set TrusteeConfig as the owner
	_ = ctrl.SetControllerReference(r.trusteeConfig, secret, r.Scheme)

	return secret
}

// generateResourcePolicyConfigMap creates a ConfigMap for resource policy
func (r *TrusteeConfigReconciler) generateResourcePolicyConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	policyRego, err := generateResourcePolicyRego(string(r.trusteeConfig.Spec.Profile))
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getResourcePolicyConfigMapName(),
			Namespace: r.namespace,
		},
		Data: map[string]string{
			"policy.rego": policyRego,
		},
	}

	err = ctrl.SetControllerReference(r.trusteeConfig, configMap, r.Scheme)
	if err != nil {
		return nil, err
	}

	return configMap, nil
}

// getResourcePolicyConfigMapName returns the name for the resource policy config map
func (r *TrusteeConfigReconciler) getResourcePolicyConfigMapName() string {
	return r.trusteeConfig.Name + "-resource-policy"
}

// createOrUpdateResourcePolicyConfigMap creates or updates the resource policy ConfigMap
func (r *TrusteeConfigReconciler) createOrUpdateResourcePolicyConfigMap(ctx context.Context) error {
	configMapName := r.getResourcePolicyConfigMapName()
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: configMapName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating resource policy config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
		configMap, err := r.generateResourcePolicyConfigMap(ctx)
		if err != nil {
			return err
		}
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	r.log.Info("Updating resource policy config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
	updatedConfigMap, err := r.generateResourcePolicyConfigMap(ctx)
	if err != nil {
		return err
	}
	found.Data = updatedConfigMap.Data
	return r.Update(ctx, found)
}

// generateRvpsReferenceValuesConfigMap creates a ConfigMap for RVPS reference values
func (r *TrusteeConfigReconciler) generateRvpsReferenceValuesConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	referenceValuesJson, err := generateRvpsReferenceValues()
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getRvpsReferenceValuesConfigMapName(),
			Namespace: r.namespace,
		},
		Data: map[string]string{
			"reference-values.json": referenceValuesJson,
		},
	}

	err = ctrl.SetControllerReference(r.trusteeConfig, configMap, r.Scheme)
	if err != nil {
		return nil, err
	}

	return configMap, nil
}

// getRvpsReferenceValuesConfigMapName returns the name for the RVPS reference values config map
func (r *TrusteeConfigReconciler) getRvpsReferenceValuesConfigMapName() string {
	return r.trusteeConfig.Name + "-rvps-reference-values"
}

// createOrUpdateRvpsReferenceValuesConfigMap creates or updates the RVPS reference values ConfigMap
func (r *TrusteeConfigReconciler) createOrUpdateRvpsReferenceValuesConfigMap(ctx context.Context) error {
	configMapName := r.getRvpsReferenceValuesConfigMapName()
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: configMapName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating RVPS reference values config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
		configMap, err := r.generateRvpsReferenceValuesConfigMap(ctx)
		if err != nil {
			return err
		}
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	r.log.Info("Updating RVPS reference values config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
	updatedConfigMap, err := r.generateRvpsReferenceValuesConfigMap(ctx)
	if err != nil {
		return err
	}
	found.Data = updatedConfigMap.Data
	return r.Update(ctx, found)
}

// generateTdxConfigMap creates a ConfigMap for TDX configuration
func (r *TrusteeConfigReconciler) generateTdxConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	tdxConfigJson, err := generateTdxConfigJson()
	if err != nil {
		return nil, err
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getTdxConfigMapName(),
			Namespace: r.namespace,
		},
		Data: map[string]string{
			tdxConfigFile: tdxConfigJson,
		},
	}

	err = ctrl.SetControllerReference(r.trusteeConfig, configMap, r.Scheme)
	if err != nil {
		return nil, err
	}

	return configMap, nil
}

// getTdxConfigMapName returns the name for the TDX config map
func (r *TrusteeConfigReconciler) getTdxConfigMapName() string {
	return r.trusteeConfig.Name + "-tdx-config"
}

// createOrUpdateTdxConfigMap creates or updates the TDX ConfigMap
func (r *TrusteeConfigReconciler) createOrUpdateTdxConfigMap(ctx context.Context) error {
	configMapName := r.getTdxConfigMapName()
	found := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: configMapName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating TDX config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
		configMap, err := r.generateTdxConfigMap(ctx)
		if err != nil {
			return err
		}
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	r.log.Info("Updating TDX config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
	updatedConfigMap, err := r.generateTdxConfigMap(ctx)
	if err != nil {
		return err
	}
	found.Data = updatedConfigMap.Data
	return r.Update(ctx, found)
}
