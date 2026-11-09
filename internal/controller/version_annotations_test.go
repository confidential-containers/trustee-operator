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
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
)

const testVANamespace = "trustee-operator-system"

const secretVersionsAnnotation = "kbs.confidentialcontainers.org/secret-versions"

func newVersionAnnotationsReconciler(t *testing.T, kbsConfig *confidentialcontainersorgv1alpha1.KbsConfig, objs ...client.Object) *KbsConfigReconciler {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := confidentialcontainersorgv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha1 to scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(append(objs, kbsConfig)...).
		Build()

	return &KbsConfigReconciler{
		Client:    fakeClient,
		Scheme:    scheme,
		Recorder:  &events.FakeRecorder{},
		kbsConfig: kbsConfig,
		log:       logr.Discard(),
		namespace: testVANamespace,
	}
}

func newTestSecret(name string, data map[string]string) *corev1.Secret {
	stringData := make(map[string][]byte, len(data))
	for k, v := range data {
		stringData[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testVANamespace,
		},
		Data: stringData,
	}
}

// getVersionAnnotations must track the ResourceVersion of every secret
// referenced by KbsSecretResources so that updating a secret's contents changes
// the pod template and triggers a rolling restart of KBS.
func TestGetVersionAnnotations_TracksSecretResources(t *testing.T) {
	secret1 := newTestSecret("kbsres1", map[string]string{"key1": "value1"})
	secret2 := newTestSecret("kbsres2", map[string]string{"key2": "value2"})

	kbsConfig := &confidentialcontainersorgv1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kbsconfig",
			Namespace: testVANamespace,
			UID:       "test-uid",
		},
		Spec: confidentialcontainersorgv1alpha1.KbsConfigSpec{
			KbsSecretResources: []string{"kbsres1", "kbsres2"},
		},
	}

	r := newVersionAnnotationsReconciler(t, kbsConfig, secret1, secret2)

	annotations := r.getVersionAnnotations(context.Background())
	got, ok := annotations[secretVersionsAnnotation]
	if !ok {
		t.Fatalf("expected annotation %q to be set, got: %v", secretVersionsAnnotation, annotations)
	}
	if !strings.Contains(got, "kbsres1:") || !strings.Contains(got, "kbsres2:") {
		t.Fatalf("expected annotation to reference both secrets, got: %q", got)
	}
}

// Updating the contents of an already-referenced secret must change the tracked
// annotation value so DeepEqual on the pod template detects the drift.
func TestGetVersionAnnotations_SecretContentChangeChangesAnnotation(t *testing.T) {
	secret := newTestSecret("kbsres1", map[string]string{"key1": "value1"})

	kbsConfig := &confidentialcontainersorgv1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kbsconfig",
			Namespace: testVANamespace,
			UID:       "test-uid",
		},
		Spec: confidentialcontainersorgv1alpha1.KbsConfigSpec{
			KbsSecretResources: []string{"kbsres1"},
		},
	}

	r := newVersionAnnotationsReconciler(t, kbsConfig, secret)
	ctx := context.Background()

	before := r.getVersionAnnotations(ctx)[secretVersionsAnnotation]
	if before == "" {
		t.Fatalf("expected a non-empty secret-versions annotation")
	}

	// Update the secret contents (keeping the same name); the fake client bumps
	// the ResourceVersion on update.
	updated := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: testVANamespace, Name: "kbsres1"}, updated); err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	updated.Data["key1"] = []byte("value2")
	if err := r.Update(ctx, updated); err != nil {
		t.Fatalf("failed to update secret: %v", err)
	}

	after := r.getVersionAnnotations(ctx)[secretVersionsAnnotation]
	if after == before {
		t.Fatalf("expected secret-versions annotation to change after content update, got %q both times", after)
	}
}

// A missing referenced secret must not produce an annotation entry or panic.
func TestGetVersionAnnotations_MissingSecretSkipped(t *testing.T) {
	kbsConfig := &confidentialcontainersorgv1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kbsconfig",
			Namespace: testVANamespace,
			UID:       "test-uid",
		},
		Spec: confidentialcontainersorgv1alpha1.KbsConfigSpec{
			KbsSecretResources: []string{"does-not-exist"},
		},
	}

	r := newVersionAnnotationsReconciler(t, kbsConfig)

	annotations := r.getVersionAnnotations(context.Background())
	if _, ok := annotations[secretVersionsAnnotation]; ok {
		t.Fatalf("expected no secret-versions annotation for a missing secret, got: %v", annotations)
	}
}
