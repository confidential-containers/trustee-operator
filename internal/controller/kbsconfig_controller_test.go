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
	"testing"

	"github.com/go-logr/logr"
	"github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(s); err != nil {
		panic(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		panic(err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}

func testConfigMap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: KbsOperatorNamespace},
	}
}

func testSecret(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: KbsOperatorNamespace},
	}
}

// allInOneConfig returns a minimal KbsConfig in AllInOne mode with a UID set
// so that ctrl.SetControllerReference succeeds without a real API server.
func allInOneConfig(name string) *v1alpha1.KbsConfig {
	return &v1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: KbsOperatorNamespace,
			UID:       types.UID("uid-" + name),
		},
		Spec: v1alpha1.KbsConfigSpec{
			KbsDeploymentType:             v1alpha1.DeploymentTypeAllInOne,
			KbsConfigMapName:              "kbs-config",
			KbsAuthSecretName:             "auth-secret",
			KbsRvpsRefValuesConfigMapName: "ref-values",
		},
	}
}

// reconcilerFor builds a KbsConfigReconciler whose fake client is pre-populated
// with kbs and all ConfigMaps/Secrets its AllInOne spec references.
func reconcilerFor(kbs *v1alpha1.KbsConfig, extra ...client.Object) *KbsConfigReconciler {
	s := testScheme()
	objects := []client.Object{
		kbs,
		testConfigMap(kbs.Spec.KbsConfigMapName),
		testConfigMap(kbs.Spec.KbsRvpsRefValuesConfigMapName),
		testSecret(kbs.Spec.KbsAuthSecretName),
	}
	objects = append(objects, extra...)
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(objects...).Build()
	return &KbsConfigReconciler{
		Client:    fc,
		Scheme:    s,
		kbsConfig: kbs,
		namespace: KbsOperatorNamespace,
		log:       logr.Discard(),
		Recorder:  record.NewFakeRecorder(32),
	}
}

// countingClient wraps a Client and records how many times Update is called.
type countingClient struct {
	client.Client
	updateCount int
}

func (c *countingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.updateCount++
	return c.Client.Update(ctx, obj, opts...)
}

// ---------------------------------------------------------------------------
// Tests: secretToKbsConfigMapper
// ---------------------------------------------------------------------------

// TestSecretMapper_AllReferencedFieldsEnqueueReconcile verifies that every
// secret field in KbsConfigSpec is covered by secretToKbsConfigMapper so that
// changes to any referenced secret trigger a reconcile.
func TestSecretMapper_AllReferencedFieldsEnqueueReconcile(t *testing.T) {
	g := gomega.NewWithT(t)

	const (
		authSecret      = "auth-secret"
		httpsKey        = "https-key"
		httpsCert       = "https-cert"
		attKey          = "attestation-key"
		attCert         = "attestation-cert"
		resourceSecret  = "resource-secret"
		certCacheSecret = "cert-cache-secret"
	)

	kbsConfig := &v1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "test-kbsconfig", Namespace: KbsOperatorNamespace},
		Spec: v1alpha1.KbsConfigSpec{
			KbsAuthSecretName:            authSecret,
			KbsHttpsKeySecretName:        httpsKey,
			KbsHttpsCertSecretName:       httpsCert,
			KbsAttestationKeySecretName:  attKey,
			KbsAttestationCertSecretName: attCert,
			KbsSecretResources:           []string{resourceSecret},
			KbsLocalCertCacheSpec: v1alpha1.KbsLocalCertCacheSpec{
				Secrets: []v1alpha1.KbsLocalCertCacheEntry{{SecretName: certCacheSecret}},
			},
		},
	}

	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(kbsConfig).Build()
	mapper, err := secretToKbsConfigMapper(fc, logr.Discard())
	g.Expect(err).NotTo(gomega.HaveOccurred())

	expected := types.NamespacedName{Name: kbsConfig.Name, Namespace: kbsConfig.Namespace}
	ctx := context.Background()

	cases := []struct {
		field      string
		secretName string
	}{
		{"KbsAuthSecretName", authSecret},
		{"KbsHttpsKeySecretName", httpsKey},
		{"KbsHttpsCertSecretName", httpsCert},
		{"KbsAttestationKeySecretName", attKey},
		{"KbsAttestationCertSecretName", attCert},
		{"KbsSecretResources entry", resourceSecret},
		{"KbsLocalCertCacheSpec secret", certCacheSecret},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			g := gomega.NewWithT(t)
			reqs := mapper(ctx, testSecret(tc.secretName))
			g.Expect(reqs).To(gomega.HaveLen(1))
			g.Expect(reqs[0].NamespacedName).To(gomega.Equal(expected))
		})
	}
}

// TestSecretMapper_UnrelatedSecretEnqueuesNothing ensures the mapper does not
// enqueue a reconcile for a secret that no KbsConfig references.
func TestSecretMapper_UnrelatedSecretEnqueuesNothing(t *testing.T) {
	g := gomega.NewWithT(t)

	kbsConfig := &v1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: KbsOperatorNamespace},
		Spec:       v1alpha1.KbsConfigSpec{KbsAuthSecretName: "auth-secret"},
	}

	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(kbsConfig).Build()
	mapper, err := secretToKbsConfigMapper(fc, logr.Discard())
	g.Expect(err).NotTo(gomega.HaveOccurred())

	reqs := mapper(context.Background(), testSecret("unrelated-secret"))
	g.Expect(reqs).To(gomega.BeEmpty())
}

// ---------------------------------------------------------------------------
// Tests: newKbsDeployment — owner reference
// ---------------------------------------------------------------------------

// TestNewKbsDeployment_SetsControllerOwnerReference verifies that the Deployment
// produced by newKbsDeployment carries a controller owner reference pointing at
// the KbsConfig so that Owns() watch and GC work correctly.
func TestNewKbsDeployment_SetsControllerOwnerReference(t *testing.T) {
	g := gomega.NewWithT(t)

	kbs := allInOneConfig("owner-ref-test")
	r := reconcilerFor(kbs)

	deployment, err := r.newKbsDeployment(context.Background())
	g.Expect(err).NotTo(gomega.HaveOccurred())

	g.Expect(deployment.OwnerReferences).To(gomega.HaveLen(1))
	g.Expect(deployment.OwnerReferences[0].Name).To(gomega.Equal(kbs.Name))
	g.Expect(deployment.OwnerReferences[0].UID).To(gomega.Equal(kbs.UID))
	g.Expect(*deployment.OwnerReferences[0].Controller).To(gomega.BeTrue())
}

// ---------------------------------------------------------------------------
// Tests: updateKbsDeployment — idempotency
// ---------------------------------------------------------------------------

// TestUpdateKbsDeployment_SkipsUpdateWhenSpecUnchanged verifies that
// updateKbsDeployment does not call r.Update when the existing Deployment's
// PodSpec and Replicas already match the desired state. Without this guard,
// adding Owns(&Deployment{}) would cause an infinite reconcile loop.
func TestUpdateKbsDeployment_SkipsUpdateWhenSpecUnchanged(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	kbs := allInOneConfig("idem-deployment-test")
	r := reconcilerFor(kbs)

	// Generate and store the desired deployment so updateKbsDeployment has
	// something to compare against and a valid object to Update if needed.
	desired, err := r.newKbsDeployment(ctx)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(r.Create(ctx, desired)).To(gomega.Succeed())

	cc := &countingClient{Client: r.Client}
	r.Client = cc

	existing := &appsv1.Deployment{}
	g.Expect(r.Client.Get(ctx, types.NamespacedName{
		Name: KbsDeploymentName, Namespace: KbsOperatorNamespace,
	}, existing)).To(gomega.Succeed())

	g.Expect(r.updateKbsDeployment(ctx, existing)).To(gomega.Succeed())
	g.Expect(cc.updateCount).To(gomega.Equal(0), "Update must not be called when spec is unchanged")
}

// TestUpdateKbsDeployment_CallsUpdateWhenReplicasDiffer verifies that
// updateKbsDeployment calls r.Update when Replicas differ from the desired state.
func TestUpdateKbsDeployment_CallsUpdateWhenReplicasDiffer(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	kbs := allInOneConfig("changed-replicas-test")
	r := reconcilerFor(kbs)

	desired, err := r.newKbsDeployment(ctx)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(r.Create(ctx, desired)).To(gomega.Succeed())

	cc := &countingClient{Client: r.Client}
	r.Client = cc

	existing := &appsv1.Deployment{}
	g.Expect(r.Client.Get(ctx, types.NamespacedName{
		Name: KbsDeploymentName, Namespace: KbsOperatorNamespace,
	}, existing)).To(gomega.Succeed())

	// Deviate replicas from what the KbsConfig specifies.
	different := int32(5)
	existing.Spec.Replicas = &different

	g.Expect(r.updateKbsDeployment(ctx, existing)).To(gomega.Succeed())
	g.Expect(cc.updateCount).To(gomega.Equal(1), "Update must be called when Replicas differ")
}

// ---------------------------------------------------------------------------
// Tests: deployOrUpdateKbsService — update correctness and idempotency
// ---------------------------------------------------------------------------

// TestDeployOrUpdateKbsService_SkipsUpdateWhenServiceUnchanged verifies that
// deployOrUpdateKbsService does not call r.Update when the existing service
// spec already matches the desired state. Without this guard, adding
// Owns(&Service{}) would cause an infinite reconcile loop.
func TestDeployOrUpdateKbsService_SkipsUpdateWhenServiceUnchanged(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	kbs := allInOneConfig("idem-service-test")
	r := reconcilerFor(kbs)

	// First call: creates the service.
	g.Expect(r.deployOrUpdateKbsService(ctx)).To(gomega.Succeed())

	// Second call: service exists with an identical spec.
	cc := &countingClient{Client: r.Client}
	r.Client = cc
	g.Expect(r.deployOrUpdateKbsService(ctx)).To(gomega.Succeed())
	g.Expect(cc.updateCount).To(gomega.Equal(0), "Update must not be called when service spec is unchanged")
}

// TestDeployOrUpdateKbsService_UpdatesServiceTypeInPlace verifies that when
// KbsServiceType changes the controller updates the existing service's type
// using found (preserving ResourceVersion) rather than a freshly built object.
func TestDeployOrUpdateKbsService_UpdatesServiceTypeInPlace(t *testing.T) {
	g := gomega.NewWithT(t)
	ctx := context.Background()

	kbs := allInOneConfig("service-type-change-test")
	r := reconcilerFor(kbs)

	// Create the initial service (ClusterIP is the default).
	g.Expect(r.deployOrUpdateKbsService(ctx)).To(gomega.Succeed())

	// Change the desired service type and reconcile again.
	kbs.Spec.KbsServiceType = corev1.ServiceTypeNodePort
	g.Expect(r.deployOrUpdateKbsService(ctx)).To(gomega.Succeed())

	// The service in the store must now reflect the new type.
	svc := &corev1.Service{}
	g.Expect(r.Get(ctx, types.NamespacedName{
		Name: KbsServiceName, Namespace: KbsOperatorNamespace,
	}, svc)).To(gomega.Succeed())
	g.Expect(svc.Spec.Type).To(gomega.Equal(corev1.ServiceTypeNodePort))
}
