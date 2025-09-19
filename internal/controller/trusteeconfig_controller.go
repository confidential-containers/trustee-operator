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
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
// TODO(user): Modify the Reconcile function to compare the state specified by
// the TrusteeConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
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

	// Update existing KbsConfig
	r.log.Info("Updating existing KbsConfig", "KbsConfig.Namespace", r.namespace, "KbsConfig.Name", kbsConfigName)
	found.Spec = spec
	err = r.Update(ctx, found)
	if err != nil {
		r.log.Error(err, "Failed to update KbsConfig")
		return nil
	}

	return found
}

// buildKbsConfigSpec builds the KbsConfigSpec based on TrusteeConfig
func (r *TrusteeConfigReconciler) buildKbsConfigSpec(ctx context.Context) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	spec := confidentialcontainersorgv1alpha1.KbsConfigSpec{}

	// Set service type from TrusteeConfig
	if r.trusteeConfig.Spec.KbsServiceType != "" {
		spec.KbsServiceType = r.trusteeConfig.Spec.KbsServiceType
	}

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
	if r.trusteeConfig.Spec.HttpsSpec.HttpsEnabled {
		spec = r.configureHttps(spec)
	}

	// Configure attestation token verification if specified
	if r.trusteeConfig.Spec.AttestationTokenVerificationSpec.TokenVerificationEnabled {
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

	// Create the RVPS reference values config map
	err = r.createOrUpdateRvpsReferenceValuesConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating RVPS reference values config map", "err", err)
		return spec
	}

	// Set the RVPS reference values config map name in the spec
	spec.KbsRvpsRefValuesConfigMapName = r.getRvpsReferenceValuesConfigMapName()

	// Create the RVPS reference values config map
	err = r.createOrUpdateRvpsReferenceValuesConfigMap(ctx)
	if err != nil {
		r.log.Info("Error creating RVPS reference values config map", "err", err)
		return spec
	}

	// Set the RVPS reference values config map name in the spec
	spec.KbsRvpsRefValuesConfigMapName = r.getRvpsReferenceValuesConfigMapName()
	return spec
}

// configureRestrictedProfile configures KbsConfig for restricted mode
func (r *TrusteeConfigReconciler) configureRestrictedProfile(ctx context.Context, spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// Force HTTPS configuration
	if spec.KbsEnvVars == nil {
		spec.KbsEnvVars = make(map[string]string)
	}

	// Create the resource policy config map
	err := r.createOrUpdateResourcePolicyConfigMap(ctx)
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

	// Set the RVPS reference values config map name in the spec
	spec.KbsRvpsRefValuesConfigMapName = r.getRvpsReferenceValuesConfigMapName()

	// TODO: Configure restricted resource policy and enforce HTTPS
	// This would require creating appropriate ConfigMaps and Secrets

	return spec
}

// configureHttps configures HTTPS settings for KbsConfig
func (r *TrusteeConfigReconciler) configureHttps(spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// TODO: Create secrets from the HTTPS configuration
	// For now, we'll set placeholder values that would need to be created
	httpsSecretName := r.getHttpsSecretName()

	spec.KbsHttpsKeySecretName = httpsSecretName + "-key"
	spec.KbsHttpsCertSecretName = httpsSecretName + "-cert"

	return spec
}

// configureAttestationTokenVerification configures attestation token verification for KbsConfig
func (r *TrusteeConfigReconciler) configureAttestationTokenVerification(spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// TODO: Create secrets from the attestation token verification configuration
	// For now, we'll set placeholder values that would need to be created
	attestationSecretName := r.getAttestationSecretName()

	spec.KbsAttestationPolicyConfigMapName = attestationSecretName + "-policy"

	return spec
}

// getKbsConfigName returns the name for the KbsConfig created by this TrusteeConfig
func (r *TrusteeConfigReconciler) getKbsConfigName() string {
	return r.trusteeConfig.Name + "-kbs-config"
}

// getHttpsSecretName returns the name for the HTTPS secret
func (r *TrusteeConfigReconciler) getHttpsSecretName() string {
	return r.trusteeConfig.Name + "-https-secret"
}

// getAttestationSecretName returns the name for the attestation secret
func (r *TrusteeConfigReconciler) getAttestationSecretName() string {
	return r.trusteeConfig.Name + "-attestation-secret"
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
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: configMapName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating KBS config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
		configMap, err := r.generateKbsConfigMap(ctx)
		if err != nil {
			return err
		}
		return r.Client.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	r.log.Info("Updating KBS config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
	updatedConfigMap, err := r.generateKbsConfigMap(ctx)
	if err != nil {
		return err
	}
	found.Data = updatedConfigMap.Data
	return r.Client.Update(ctx, found)
}

// generateKbsAuthSecret creates a Secret for KBS authentication
func (r *TrusteeConfigReconciler) generateKbsAuthSecret(ctx context.Context) (*corev1.Secret, error) {
	secretName := r.getKbsAuthSecretName()

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		r.log.Error(err, "Failed to generate RSA private key")
		return nil, err
	}

	// Encode private key to PEM format
	privateKeyPEM, err := encodePrivateKeyToPEM(privateKey)
	if err != nil {
		r.log.Error(err, "Failed to encode private key to PEM")
		return nil, err
	}

	// Encode public key to PEM format
	publicKeyPEM, err := encodePublicKeyToPEM(&privateKey.PublicKey)
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

// getKbsAuthSecretName returns the name for the KBS auth secret
func (r *TrusteeConfigReconciler) getKbsAuthSecretName() string {
	return r.trusteeConfig.Name + "-auth-secret"
}

// createOrUpdateKbsAuthSecret creates or updates the KBS auth secret
func (r *TrusteeConfigReconciler) createOrUpdateKbsAuthSecret(ctx context.Context) error {
	secretName := r.getKbsAuthSecretName()

	// Check if the secret already exists
	found := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{
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
		err = r.Client.Create(ctx, secret)
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
		err = r.Client.Update(ctx, found)
		if err != nil {
			return err
		}
	}

	return nil
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
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: configMapName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating resource policy config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
		configMap, err := r.generateResourcePolicyConfigMap(ctx)
		if err != nil {
			return err
		}
		return r.Client.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	r.log.Info("Updating resource policy config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
	updatedConfigMap, err := r.generateResourcePolicyConfigMap(ctx)
	if err != nil {
		return err
	}
	found.Data = updatedConfigMap.Data
	return r.Client.Update(ctx, found)
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
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: configMapName}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("Creating RVPS reference values config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
		configMap, err := r.generateRvpsReferenceValuesConfigMap(ctx)
		if err != nil {
			return err
		}
		return r.Client.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	r.log.Info("Updating RVPS reference values config map", "ConfigMap.Namespace", r.namespace, "ConfigMap.Name", configMapName)
	updatedConfigMap, err := r.generateRvpsReferenceValuesConfigMap(ctx)
	if err != nil {
		return err
	}
	found.Data = updatedConfigMap.Data
	return r.Client.Update(ctx, found)
}
