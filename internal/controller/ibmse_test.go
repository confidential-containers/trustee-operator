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
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
)

func init() {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
}

// ── shared test helpers ───────────────────────────────────────────────────────

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = batchv1.AddToScheme(s)
	_ = confidentialcontainersorgv1alpha1.AddToScheme(s)
	return s
}

func newFakeReconciler(scheme *runtime.Scheme, objs ...client.Object) *TrusteeConfigReconciler {
	fc := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&batchv1.Job{}).WithObjects(objs...).Build()
	return &TrusteeConfigReconciler{
		Client:    fc,
		Scheme:    scheme,
		namespace: "default",
		log:       logf.Log.WithName("ibmse-test"),
	}
}

func makeTCWithIBMSE(name, pvName, bundleSecret, seMessageSecret string) *confidentialcontainersorgv1alpha1.TrusteeConfig {
	return &confidentialcontainersorgv1alpha1.TrusteeConfig{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: confidentialcontainersorgv1alpha1.TrusteeConfigSpec{
			IbmSE: &confidentialcontainersorgv1alpha1.IbmSETeeConfig{
				PVName:              pvName,
				BundleSecretName:    bundleSecret,
				SeMessageSecretName: seMessageSecret,
			},
		},
	}
}

func makeBundleSecret(name, namespace, sha string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data: map[string][]byte{
			"ibmse.tar.gz": []byte("fake-tar-content"),
			"sha256":       []byte(sha),
		},
	}
}

func makeWorkerNode(name string, annotations map[string]string) *corev1.Node {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{ibmseWorkerNodeLabel: ""},
			Annotations: annotations,
		},
	}
	return n
}

func makeSeSecret(name, namespace string, vals map[string]string) *corev1.Secret {
	data, _ := json.Marshal(vals)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Data:       map[string][]byte{seMessageSecretKey: data},
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// ── IBM SE Job tests ──────────────────────────────────────────────────────────

// TestIBMSEJob_MissingBundleSecret — reconcileIBMSEJobs returns an error when
// the ibmse-bundle Secret does not exist.
func TestIBMSEJob_MissingBundleSecret(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc1", "ibmse-pv", "nonexistent-bundle", "")
	worker := makeWorkerNode("worker-1", nil)
	r := newFakeReconciler(scheme, tc, worker)
	r.trusteeConfig = tc

	_, err := r.reconcileIBMSEJobs(context.Background())
	if err == nil {
		t.Fatal("expected error when bundle Secret is missing, got nil")
	}
	if !containsStr(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

// TestIBMSEJob_NoWorkerNodes — reconcileIBMSEJobs returns (false, nil) when no
// worker nodes exist yet (e.g. early in cluster bootstrap).
func TestIBMSEJob_NoWorkerNodes(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc2", "ibmse-pv", "my-bundle", "")
	bundleSec := makeBundleSecret("my-bundle", "default", "abc123")
	r := newFakeReconciler(scheme, tc, bundleSec)
	r.trusteeConfig = tc

	ready, err := r.reconcileIBMSEJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not-ready when no worker nodes found")
	}
}

// TestIBMSEJob_CreatesJobPerNode — a Job is created for each worker node whose
// annotation does not match the current bundle SHA.
func TestIBMSEJob_CreatesJobPerNode(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc3", "ibmse-pv", "my-bundle", "")
	bundleSec := makeBundleSecret("my-bundle", "default", "sha-v1")
	worker1 := makeWorkerNode("worker-1", nil)
	worker2 := makeWorkerNode("worker-2", nil)
	r := newFakeReconciler(scheme, tc, bundleSec, worker1, worker2)
	r.trusteeConfig = tc

	ready, err := r.reconcileIBMSEJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not-ready: Jobs just created, not succeeded yet")
	}

	// Verify one Job was created per node.
	for _, nodeName := range []string{"worker-1", "worker-2"} {
		jobName := r.ibmseJobName(nodeName)
		job := &batchv1.Job{}
		if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: jobName}, job); err != nil {
			t.Errorf("Job for node %q not found: %v", nodeName, err)
			continue
		}
		// Verify pinned to specific node.
		if job.Spec.Template.Spec.NodeName != nodeName {
			t.Errorf("expected Job NodeName=%q, got %q", nodeName, job.Spec.Template.Spec.NodeName)
		}
		// Verify bundle SHA annotation on Job.
		if got := job.Annotations["confidentialcontainers.org/ibmse-bundle-sha"]; got != "sha-v1" {
			t.Errorf("expected Job annotation sha=sha-v1, got %q", got)
		}
		// Verify SHA-256 check in container script.
		cmd := job.Spec.Template.Spec.Containers[0].Command
		found := false
		for _, c := range cmd {
			if containsStr(c, "sha256sum") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("node %q: expected sha256sum in Job container command", nodeName)
		}
		// Verify TTL is set (so completed pods are cleaned up).
		if job.Spec.TTLSecondsAfterFinished == nil {
			t.Errorf("node %q: expected TTLSecondsAfterFinished to be set", nodeName)
		}
		// Verify RestartPolicy is Never (one-shot).
		if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
			t.Errorf("node %q: expected RestartPolicy=Never", nodeName)
		}
	}
}

// TestIBMSEJob_ReadyWhenAllAnnotated — reconcileIBMSEJobs returns (true, nil)
// when every worker node already has the current bundle SHA annotation.
func TestIBMSEJob_ReadyWhenAllAnnotated(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc4", "ibmse-pv", "my-bundle", "")
	bundleSec := makeBundleSecret("my-bundle", "default", "sha-v1")
	// Both workers already annotated with the current SHA — simulates a Trustee restart
	// where the files are already present on the nodes.
	worker1 := makeWorkerNode("worker-1", map[string]string{ibmseNodeBundleSHAAnnotation: "sha-v1"})
	worker2 := makeWorkerNode("worker-2", map[string]string{ibmseNodeBundleSHAAnnotation: "sha-v1"})
	r := newFakeReconciler(scheme, tc, bundleSec, worker1, worker2)
	r.trusteeConfig = tc

	ready, err := r.reconcileIBMSEJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("expected ready: all nodes annotated with current SHA")
	}

	// Verify no Jobs were created — nothing to do.
	jobList := &batchv1.JobList{}
	if err := r.List(context.Background(), jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 0 {
		t.Errorf("expected 0 Jobs (nodes already up to date), got %d", len(jobList.Items))
	}
}

// TestIBMSEJob_NodeRestart — simulates a worker node reboot: annotation is wiped
// (node object loses annotation, hostPath cleared), but the old Job is Succeeded.
// reconcileIBMSEJobs must detect the stale annotation and create a fresh Job.
func TestIBMSEJob_NodeRestart(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc5", "ibmse-pv", "my-bundle", "")
	bundleSec := makeBundleSecret("my-bundle", "default", "sha-v1")
	// Node annotation is missing — simulates post-reboot state where hostPath was wiped.
	worker := makeWorkerNode("worker-1", nil)
	r := newFakeReconciler(scheme, tc, bundleSec, worker)
	r.trusteeConfig = tc

	// Pre-create an old Succeeded Job for this node (left over from before the reboot).
	oldJob := r.generateIBMSEJob("worker-1", "my-bundle", "sha-v1")
	oldJob.Namespace = "default"
	oldJob.Status.Conditions = []batchv1.JobCondition{
		{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
	}
	if err := r.Create(context.Background(), oldJob); err != nil {
		t.Fatalf("create old job: %v", err)
	}

	// reconcileIBMSEJobs must delete the old Succeeded Job and create a new one
	// because the node annotation is absent (files are gone from the node).
	ready, err := r.reconcileIBMSEJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not-ready: new Job created for rebooted node")
	}

	// The new Job should exist (old one was deleted and recreated).
	jobName := r.ibmseJobName("worker-1")
	newJob := &batchv1.Job{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: jobName}, newJob); err != nil {
		t.Fatalf("expected a new Job for the rebooted node, got: %v", err)
	}
}

// TestIBMSEJob_BundleRotation — when the bundle SHA changes (new ibmse-bundle Secret),
// the existing Jobs (from the old bundle) must be deleted and new ones created.
func TestIBMSEJob_BundleRotation(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc6", "ibmse-pv", "my-bundle", "")
	// Secret now has a new SHA.
	bundleSec := makeBundleSecret("my-bundle", "default", "sha-v2")
	// Worker still has annotation from the old SHA.
	worker := makeWorkerNode("worker-1", map[string]string{ibmseNodeBundleSHAAnnotation: "sha-v1"})
	r := newFakeReconciler(scheme, tc, bundleSec, worker)
	r.trusteeConfig = tc

	// Pre-create a Succeeded Job from the old bundle.
	oldJob := r.generateIBMSEJob("worker-1", "my-bundle", "sha-v1")
	oldJob.Namespace = "default"
	oldJob.Status.Conditions = []batchv1.JobCondition{
		{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
	}
	if err := r.Create(context.Background(), oldJob); err != nil {
		t.Fatalf("create old job: %v", err)
	}

	ready, err := r.reconcileIBMSEJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not-ready: new SHA requires re-extraction")
	}

	// New Job must exist and carry the new SHA.
	jobName := r.ibmseJobName("worker-1")
	newJob := &batchv1.Job{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: jobName}, newJob); err != nil {
		t.Fatalf("expected new Job after bundle rotation, got: %v", err)
	}
	if got := newJob.Annotations["confidentialcontainers.org/ibmse-bundle-sha"]; got != "sha-v2" {
		t.Errorf("expected new Job to carry sha-v2, got %q", got)
	}
}

// TestIBMSEJob_FailedJobRecreated — a Failed Job is deleted and recreated on the
// next reconcile instead of being left in a terminal error state.
func TestIBMSEJob_FailedJobRecreated(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc7", "ibmse-pv", "my-bundle", "")
	bundleSec := makeBundleSecret("my-bundle", "default", "sha-v1")
	worker := makeWorkerNode("worker-1", nil)
	r := newFakeReconciler(scheme, tc, bundleSec, worker)
	r.trusteeConfig = tc

	// Pre-create a Failed Job.
	failedJob := r.generateIBMSEJob("worker-1", "my-bundle", "sha-v1")
	failedJob.Namespace = "default"
	failedJob.Status.Conditions = []batchv1.JobCondition{
		{Type: batchv1.JobFailed, Status: corev1.ConditionTrue},
	}
	if err := r.Create(context.Background(), failedJob); err != nil {
		t.Fatalf("create failed job: %v", err)
	}

	ready, err := r.reconcileIBMSEJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not-ready after failed Job recreation")
	}

	// A new Job must have been created (the failed one was deleted and re-created).
	jobName := r.ibmseJobName("worker-1")
	newJob := &batchv1.Job{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: jobName}, newJob); err != nil {
		t.Fatalf("expected a new Job after failed Job recreation, got: %v", err)
	}
}

// TestIBMSEJob_Idempotent — calling reconcileIBMSEJobs twice without any state
// change must not create duplicate Jobs.
func TestIBMSEJob_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc8", "ibmse-pv", "my-bundle", "")
	bundleSec := makeBundleSecret("my-bundle", "default", "sha-v1")
	worker := makeWorkerNode("worker-1", nil)
	r := newFakeReconciler(scheme, tc, bundleSec, worker)
	r.trusteeConfig = tc

	// First call — creates the Job.
	if _, err := r.reconcileIBMSEJobs(context.Background()); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	// Second call — Job already exists with same SHA, should be a no-op.
	if _, err := r.reconcileIBMSEJobs(context.Background()); err != nil {
		t.Fatalf("second call error: %v", err)
	}

	jobList := &batchv1.JobList{}
	if err := r.List(context.Background(), jobList, client.InNamespace("default")); err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobList.Items) != 1 {
		t.Errorf("expected exactly 1 Job after two idempotent calls, got %d", len(jobList.Items))
	}
}

// TestIBMSEJob_PartialReady — one node annotated (current), one node still running
// a Job → reconcileIBMSEJobs returns (false, nil), not ready yet.
func TestIBMSEJob_PartialReady(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc9", "ibmse-pv", "my-bundle", "")
	bundleSec := makeBundleSecret("my-bundle", "default", "sha-v1")
	// worker-1 is done, worker-2 is not yet annotated.
	worker1 := makeWorkerNode("worker-1", map[string]string{ibmseNodeBundleSHAAnnotation: "sha-v1"})
	worker2 := makeWorkerNode("worker-2", nil)
	r := newFakeReconciler(scheme, tc, bundleSec, worker1, worker2)
	r.trusteeConfig = tc

	ready, err := r.reconcileIBMSEJobs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not-ready: worker-2 not yet done")
	}

	// Job must exist only for worker-2.
	job2Name := r.ibmseJobName("worker-2")
	job2 := &batchv1.Job{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: job2Name}, job2); err != nil {
		t.Fatalf("expected Job for worker-2, got: %v", err)
	}
	job1Name := r.ibmseJobName("worker-1")
	job1 := &batchv1.Job{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: job1Name}, job1); err == nil {
		t.Error("expected no Job for worker-1 (already annotated)")
	}
}

// ── IBM SE Attestation Policy tests ──────────────────────────────────────────
// (unchanged from original — attestation policy logic was not modified)

func TestIBMSEAttestationPolicy_MissingSecret(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc10", "", "", "missing-se-message")
	r := newFakeReconciler(scheme, tc)
	r.trusteeConfig = tc

	_, err := r.createOrUpdateIBMSEAttestationPolicy(context.Background())
	if err == nil {
		t.Fatal("expected error when se-message Secret is missing, got nil")
	}
	if !containsStr(err.Error(), "not found") {
		t.Fatalf("expected error containing 'not found', got: %v", err)
	}
}

func TestIBMSEAttestationPolicy_MissingFields(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc11", "", "", "se-msg")
	sec := makeSeSecret("se-msg", "default", map[string]string{
		"se.attestation_phkh": "aabb",
		// image_phkh and tag intentionally missing
	})
	r := newFakeReconciler(scheme, tc, sec)
	r.trusteeConfig = tc

	_, err := r.createOrUpdateIBMSEAttestationPolicy(context.Background())
	if err == nil {
		t.Fatal("expected error for missing fields, got nil")
	}
	if !containsStr(err.Error(), "missing one or more required fields") {
		t.Fatalf("expected error containing 'missing one or more required fields', got: %v", err)
	}
}

func TestIBMSEAttestationPolicy_CreatesConfigMap(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc12", "", "", "se-msg")
	sec := makeSeSecret("se-msg", "default", map[string]string{
		"se.attestation_phkh": "aabbcc",
		"se.image_phkh":       "112233",
		"se.tag":              "deadbeef",
	})
	r := newFakeReconciler(scheme, tc, sec)
	r.trusteeConfig = tc

	cmName, err := r.createOrUpdateIBMSEAttestationPolicy(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "tc12-ibmse-attestation-policy"; cmName != want {
		t.Fatalf("expected ConfigMap name %q, got %q", want, cmName)
	}

	cm := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: cmName}, cm); err != nil {
		t.Fatalf("ConfigMap not found: %v", err)
	}

	policy := cm.Data[ibmseAttestationPolicyFilename]
	for _, want := range []string{"aabbcc", "112233", "deadbeef", "package policy"} {
		if !containsStr(policy, want) {
			t.Errorf("expected policy to contain %q\npolicy:\n%s", want, policy)
		}
	}
}

func TestIBMSEAttestationPolicy_UpdatesConfigMap(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc13", "", "", "se-msg")
	sec := makeSeSecret("se-msg", "default", map[string]string{
		"se.attestation_phkh": "old-a",
		"se.image_phkh":       "old-b",
		"se.tag":              "old-c",
	})
	r := newFakeReconciler(scheme, tc, sec)
	r.trusteeConfig = tc

	cmName, err := r.createOrUpdateIBMSEAttestationPolicy(context.Background())
	if err != nil {
		t.Fatalf("first create error: %v", err)
	}

	// Update the Secret with new phkh values.
	updated, _ := json.Marshal(map[string]string{
		"se.attestation_phkh": "new-a",
		"se.image_phkh":       "new-b",
		"se.tag":              "new-c",
	})
	existingSec := &corev1.Secret{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "se-msg"}, existingSec); err != nil {
		t.Fatalf("get secret: %v", err)
	}
	existingSec.Data[seMessageSecretKey] = updated
	if err := r.Update(context.Background(), existingSec); err != nil {
		t.Fatalf("update secret: %v", err)
	}

	if _, err = r.createOrUpdateIBMSEAttestationPolicy(context.Background()); err != nil {
		t.Fatalf("second update error: %v", err)
	}

	cm := &corev1.ConfigMap{}
	if err := r.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: cmName}, cm); err != nil {
		t.Fatalf("get cm: %v", err)
	}
	if !containsStr(cm.Data[ibmseAttestationPolicyFilename], "new-a") {
		t.Errorf("expected updated phkh in policy, got:\n%s", cm.Data[ibmseAttestationPolicyFilename])
	}
}

func TestIBMSEAttestationPolicy_Idempotent(t *testing.T) {
	scheme := newTestScheme()
	tc := makeTCWithIBMSE("tc14", "", "", "se-msg")
	sec := makeSeSecret("se-msg", "default", map[string]string{
		"se.attestation_phkh": "x1",
		"se.image_phkh":       "x2",
		"se.tag":              "x3",
	})
	r := newFakeReconciler(scheme, tc, sec)
	r.trusteeConfig = tc

	for i := 0; i < 3; i++ {
		if _, err := r.createOrUpdateIBMSEAttestationPolicy(context.Background()); err != nil {
			t.Fatalf("call %d error: %v", i+1, err)
		}
	}
}
