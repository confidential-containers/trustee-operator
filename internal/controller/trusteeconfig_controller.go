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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
	"github.com/go-logr/logr"
)

// TrusteeConfigReconciler reconciles a TrusteeConfig object
type TrusteeConfigReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	trusteeConfig *confidentialcontainersorgv1alpha1.TrusteeConfig
	log           logr.Logger
	namespace     string
}

const (
	TrusteeFinalizerName = "trustee.confidentialcontainers.org/finalizer"
)

//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=trusteeconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=trusteeconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=trusteeconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=confidentialcontainers.org,resources=kbsconfigs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *TrusteeConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.log.Info("Reconciling TrusteeConfig")

	// Get the TrusteeConfig instance
	r.trusteeConfig = &confidentialcontainersorgv1alpha1.TrusteeConfig{}
	err := r.Client.Get(ctx, req.NamespacedName, r.trusteeConfig)
	// If the TrusteeConfig instance is not found, then just return
	// and do nothing
	if err != nil && k8serrors.IsNotFound(err) {
		r.log.Info("TrusteeConfig not found")
		return ctrl.Result{}, nil
	}
	// If there is an error other than the TrusteeConfig instance not found,
	// then return with the error
	if err != nil {
		r.log.Info("Getting TrusteeConfig failed with error", "err", err)
		return ctrl.Result{}, err
	}

	// TrusteeConfig instance is found, so continue with rest of the processing

	// Check if the TrusteeConfig object is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	isTrusteeConfigMarkedToBeDeleted := r.trusteeConfig.GetDeletionTimestamp() != nil
	if isTrusteeConfigMarkedToBeDeleted {
		if contains(r.trusteeConfig.GetFinalizers(), TrusteeFinalizerName) {
			// Run finalization logic for trusteeFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			err := r.finalizeTrusteeConfig(ctx)
			if err != nil {
				r.log.Info("Error in finalizeTrusteeConfig", "err", err)
				return ctrl.Result{}, err
			}
		}
		// Remove trusteeFinalizer. Once all finalizers have been
		// removed, the object will be deleted.
		r.log.Info("Removing trusteeFinalizer")
		r.trusteeConfig.SetFinalizers(remove(r.trusteeConfig.GetFinalizers(), TrusteeFinalizerName))
		err := r.Update(ctx, r.trusteeConfig)
		if err != nil {
			r.log.Info("Failed to update TrusteeConfig after removing trusteeFinalizer", "err", err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Create or update the KbsConfig based on TrusteeConfig
	err = r.deployOrUpdateKbsConfig(ctx)
	if err != nil {
		r.log.Info("Error in creating/updating KbsConfig", "err", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// finalizeTrusteeConfig deletes the KbsConfig created by this TrusteeConfig
func (r *TrusteeConfigReconciler) finalizeTrusteeConfig(ctx context.Context) error {
	// Delete the KbsConfig
	r.log.Info("Deleting the KbsConfig created by TrusteeConfig")
	kbsConfigName := r.getKbsConfigName()
	kbsConfig := &confidentialcontainersorgv1alpha1.KbsConfig{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      kbsConfigName,
	}, kbsConfig)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	if err == nil {
		err = r.Client.Delete(ctx, kbsConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

// deployOrUpdateKbsConfig creates or updates a KbsConfig based on the TrusteeConfig
func (r *TrusteeConfigReconciler) deployOrUpdateKbsConfig(ctx context.Context) error {
	kbsConfigName := r.getKbsConfigName()

	// Check if the KbsConfig already exists
	found := &confidentialcontainersorgv1alpha1.KbsConfig{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: r.namespace,
		Name:      kbsConfigName,
	}, found)

	if err != nil && k8serrors.IsNotFound(err) {
		// Create the KbsConfig
		r.log.Info("Creating a new KbsConfig", "KbsConfig.Namespace", r.namespace, "KbsConfig.Name", kbsConfigName)
		kbsConfig := r.newKbsConfig(ctx)
		if kbsConfig == nil {
			return fmt.Errorf("failed to get KbsConfig definition")
		}
		err = r.Client.Create(ctx, kbsConfig)
		if err != nil {
			return err
		}
		// Add the trusteeFinalizer to the TrusteeConfig if it doesn't already exist
		return r.addTrusteeConfigFinalizer(ctx)
	} else if err != nil {
		return err
	}

	// KbsConfig already exists, so update it
	r.log.Info("Updating the KbsConfig", "KbsConfig.Namespace", r.namespace, "KbsConfig.Name", kbsConfigName)
	kbsConfig := r.newKbsConfig(ctx)
	if kbsConfig == nil {
		return fmt.Errorf("failed to get KbsConfig definition")
	}
	// Preserve the existing ObjectMeta
	kbsConfig.ObjectMeta = found.ObjectMeta
	// Update the spec
	found.Spec = kbsConfig.Spec
	err = r.Client.Update(ctx, found)
	if err != nil {
		return err
	}
	return nil
}

// newKbsConfig creates a new KbsConfig based on the TrusteeConfig
func (r *TrusteeConfigReconciler) newKbsConfig(ctx context.Context) *confidentialcontainersorgv1alpha1.KbsConfig {
	kbsConfigName := r.getKbsConfigName()

	// Create the KbsConfig spec based on TrusteeConfig
	kbsConfigSpec := r.buildKbsConfigSpec()

	// Create a new KbsConfig
	kbsConfig := &confidentialcontainersorgv1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.namespace,
			Name:      kbsConfigName,
		},
		Spec: kbsConfigSpec,
	}

	// Set TrusteeConfig instance as the owner and controller
	err := ctrl.SetControllerReference(r.trusteeConfig, kbsConfig, r.Scheme)
	if err != nil {
		r.log.Info("Error in setting the controller reference for the KbsConfig", "err", err)
		return nil
	}
	return kbsConfig
}

// buildKbsConfigSpec builds the KbsConfigSpec based on TrusteeConfig
func (r *TrusteeConfigReconciler) buildKbsConfigSpec() confidentialcontainersorgv1alpha1.KbsConfigSpec {
	spec := confidentialcontainersorgv1alpha1.KbsConfigSpec{}

	// Set service type from TrusteeConfig
	if r.trusteeConfig.Spec.KbsServiceType != "" {
		spec.KbsServiceType = r.trusteeConfig.Spec.KbsServiceType
	}

	// Configure based on profile type
	switch r.trusteeConfig.Spec.Profile {
	case confidentialcontainersorgv1alpha1.ProfileTypePermissive:
		r.log.Info("Configuring KbsConfig for Permissive profile")
		spec = r.configurePermissiveProfile(spec)
	case confidentialcontainersorgv1alpha1.ProfileTypeRestrictive:
		r.log.Info("Configuring KbsConfig for Restricted profile")
		spec = r.configureRestrictedProfile(spec)
	default:
		r.log.Info("No profile specified, using default configuration")
	}

	// Configure HTTPS if specified
	if r.trusteeConfig.Spec.HttpsSpec.Certificate != "" && r.trusteeConfig.Spec.HttpsSpec.PrivateKey != "" {
		spec = r.configureHttps(spec)
	}

	// Configure attestation token verification if specified
	if r.trusteeConfig.Spec.AttestationTokenVerificationSpec.Certificate != "" {
		// TODO: Configure attestation token verification
	}

	return spec
}

// configurePermissiveProfile configures KbsConfig for permissive mode
func (r *TrusteeConfigReconciler) configurePermissiveProfile(spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// Set environment variables for permissive mode
	if spec.KbsEnvVars == nil {
		spec.KbsEnvVars = make(map[string]string)
	}
	spec.KbsEnvVars["RUST_LOG"] = "debug"

	// TODO: Configure permissive resource policy
	// This would require creating appropriate ConfigMaps

	return spec
}

// configureRestrictedProfile configures KbsConfig for restricted mode
func (r *TrusteeConfigReconciler) configureRestrictedProfile(spec confidentialcontainersorgv1alpha1.KbsConfigSpec) confidentialcontainersorgv1alpha1.KbsConfigSpec {
	// Force HTTPS configuration
	if spec.KbsEnvVars == nil {
		spec.KbsEnvVars = make(map[string]string)
	}

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

// getKbsConfigName returns the name for the KbsConfig created by this TrusteeConfig
func (r *TrusteeConfigReconciler) getKbsConfigName() string {
	return r.trusteeConfig.Name + "-kbsconfig"
}

// getHttpsSecretName returns the name for the HTTPS secrets
func (r *TrusteeConfigReconciler) getHttpsSecretName() string {
	return r.trusteeConfig.Name + "-https"
}

// addTrusteeConfigFinalizer adds the finalizer to TrusteeConfig
func (r *TrusteeConfigReconciler) addTrusteeConfigFinalizer(ctx context.Context) error {
	if !contains(r.trusteeConfig.GetFinalizers(), TrusteeFinalizerName) {
		r.log.Info("Adding trusteeFinalizer to TrusteeConfig")
		r.trusteeConfig.SetFinalizers(append(r.trusteeConfig.GetFinalizers(), TrusteeFinalizerName))
		err := r.Update(ctx, r.trusteeConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrusteeConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Get the namespace that the controller is running in
	r.namespace = getOperatorNamespace()

	// Create a logr instance and assign it to r.log
	r.log = ctrl.Log.WithName("trusteeconfig-controller")
	r.log = r.log.WithValues("trusteeconfig", r.namespace)

	// Create a new controller and add a watch for TrusteeConfig
	return ctrl.NewControllerManagedBy(mgr).
		For(&confidentialcontainersorgv1alpha1.TrusteeConfig{}).
		Owns(&confidentialcontainersorgv1alpha1.KbsConfig{}).
		Complete(r)
}
