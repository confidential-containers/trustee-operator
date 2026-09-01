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
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ibmseJobNamePrefix is prepended to every per-node installer Job name.
	ibmseJobNamePrefix = "ibmse-installer"

	// ibmseHostPath is the directory on the worker node where the bundle is extracted.
	// Kept identical to the previous DaemonSet constant so the hostPath PV still binds.
	ibmseHostPath = "/opt/confidential-containers/ibmse"

	// ibmseInstallerImage is a minimal image that ships tar and sha256sum.
	// Operators may override this by setting the IBMSE_INSTALLER_IMAGE env var on the manager.
	ibmseInstallerImage = "registry.access.redhat.com/ubi9/ubi-minimal:latest"

	// ibmseNodeBundleSHAAnnotation is written to the Node object by the Job after
	// successful extraction.  Its value is the SHA-256 hex of the ibmse.tar.gz that
	// was extracted.  The controller uses it to decide whether a node's hostPath is
	// current (annotation matches bundle Secret SHA) or stale/missing (re-run needed).
	ibmseNodeBundleSHAAnnotation = "confidentialcontainers.org/ibmse-bundle-sha"

	// ibmseJobTTLSeconds is how long a completed (Succeeded or Failed) Job pod lingers
	// before Kubernetes garbage-collects it.  5 minutes gives enough time for log
	// inspection without holding pod slots indefinitely.
	ibmseJobTTLSeconds int32 = 300

	// ibmseWorkerNodeLabel is the well-known label that identifies worker nodes.
	ibmseWorkerNodeLabel = "node-role.kubernetes.io/worker"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// ibmseJobName returns the deterministic Job name for a given node.
// Names are kept below the 63-char DNS label limit by truncating the node name.
func (r *TrusteeConfigReconciler) ibmseJobName(nodeName string) string {
	prefix := r.trusteeConfig.Name + "-" + ibmseJobNamePrefix + "-"
	maxNode := 63 - len(prefix)
	if len(nodeName) > maxNode {
		nodeName = nodeName[:maxNode]
	}
	return prefix + nodeName
}

// ibmseCurrentBundleSHA reads the sha256 key from the ibmse-bundle Secret and
// returns it as a trimmed hex string.  This is the source-of-truth version token.
func (r *TrusteeConfigReconciler) ibmseCurrentBundleSHA(ctx context.Context) (string, error) {
	bundleSecretName := r.trusteeConfig.Spec.IbmSE.BundleSecretName
	sec := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: bundleSecretName}, sec); err != nil {
		if k8serrors.IsNotFound(err) {
			return "", fmt.Errorf(
				"ibmse bundle Secret %q not found in namespace %q — "+
					"run hack/ibmse-bundle-generator first to create it",
				bundleSecretName, r.namespace,
			)
		}
		return "", fmt.Errorf("failed to get ibmse bundle Secret %q: %w", bundleSecretName, err)
	}
	raw, ok := sec.Data["sha256"]
	if !ok {
		return "", fmt.Errorf("ibmse bundle Secret %q has no \"sha256\" key", bundleSecretName)
	}
	return strings.TrimSpace(string(raw)), nil
}

// ibmseListWorkerNodes returns all nodes with the worker role label.
func (r *TrusteeConfigReconciler) ibmseListWorkerNodes(ctx context.Context) ([]corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := r.List(ctx, nodeList, client.MatchingLabels{ibmseWorkerNodeLabel: ""}); err != nil {
		return nil, fmt.Errorf("failed to list worker nodes: %w", err)
	}
	return nodeList.Items, nil
}

// ibmseNodeBundleSHA returns the SHA-256 annotation value on the given node,
// or "" if the annotation is absent.
func ibmseNodeBundleSHA(node corev1.Node) string {
	if node.Annotations == nil {
		return ""
	}
	return node.Annotations[ibmseNodeBundleSHAAnnotation]
}

// ── Job generation ────────────────────────────────────────────────────────────

// generateIBMSEJob builds the Job manifest that installs the ibmse bundle onto
// a single named worker node.
//
// The Job container:
//  1. Verifies the SHA-256 of ibmse.tar.gz against the digest in the Secret.
//  2. Extracts the tar to the node's hostPath via a hostPath volume.
//  3. Patches the Node annotation to record which SHA was installed.
//     This annotation is the "files are current" signal read back by the controller.
//
// Because the Job is pinned to one node via spec.template.spec.nodeName, Kubernetes
// runs exactly one pod on that specific node — no nodeSelector ambiguity.
//
// ttlSecondsAfterFinished cleans up the pod automatically, leaving no long-running
// pods on worker nodes once extraction completes.
func (r *TrusteeConfigReconciler) generateIBMSEJob(nodeName, bundleSecretName, bundleSHA string) *batchv1.Job {
	jobName := r.ibmseJobName(nodeName)
	ttl := ibmseJobTTLSeconds
	privileged := true
	hostPathType := corev1.HostPathDirectoryOrCreate

	// The Job pod patches its own Node's annotation using kubectl.
	// It uses the downward API to discover its own node name so the patch
	// command stays generic and doesn't need the node name baked in.
	extractScript := fmt.Sprintf(`
set -e
BUNDLE=/bundle/ibmse.tar.gz
EXPECTED_SHA=$(cat /bundle/sha256)

echo "Verifying SHA-256 of ibmse bundle..."
ACTUAL_SHA=$(sha256sum "$BUNDLE" | awk '{print $1}')
if [ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]; then
  echo "ERROR: SHA-256 mismatch. Expected=$EXPECTED_SHA Actual=$ACTUAL_SHA"
  exit 1
fi
echo "SHA-256 verified OK"

echo "Extracting ibmse bundle to /host-ibmse..."
tar -xzf "$BUNDLE" -C /host-ibmse --strip-components=1
chmod -R 755 /host-ibmse
echo "Extraction complete."

echo "Annotating node $(NODE_NAME) with bundle SHA..."
kubectl annotate node "$(NODE_NAME)" \
  %s=%s \
  --overwrite
echo "Node annotation written."
`, ibmseNodeBundleSHAAnnotation, bundleSHA)

	labels := map[string]string{
		"app.kubernetes.io/managed-by": "trustee-operator",
		"app.kubernetes.io/part-of":    "trustee",
		"app.kubernetes.io/component":  "ibmse-node-installer",
		"app.kubernetes.io/instance":   r.trusteeConfig.Name,
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.namespace,
			Labels:    labels,
			// Store the target node and bundle SHA as annotations on the Job so
			// the controller can match them without re-parsing the Job name.
			Annotations: map[string]string{
				"confidentialcontainers.org/ibmse-target-node":   nodeName,
				"confidentialcontainers.org/ibmse-bundle-sha":    bundleSHA,
				"confidentialcontainers.org/ibmse-bundle-secret": bundleSecretName,
			},
		},
		Spec: batchv1.JobSpec{
			// Never retry: if the Job fails the operator will re-create it on
			// the next reconcile, which gives a clean audit trail per attempt.
			BackoffLimit:            ptr(int32(0)),
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					// Pin to the specific worker node — this is what makes the
					// Job equivalent to a DaemonSet pod for a single node.
					NodeName: nodeName,
					// Tolerate all taints so the pod can land even on tainted workers.
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists},
					},
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "trustee-operator-controller-manager",
					Volumes: []corev1.Volume{
						{
							// ibmse.tar.gz + sha256 from the Secret.
							Name: "ibmse-bundle",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: bundleSecretName,
								},
							},
						},
						{
							// The target host directory — maps to the real filesystem.
							Name: "ibmse-host",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: ibmseHostPath,
									Type: &hostPathType,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "extract-ibmse-bundle",
							Image:   ibmseInstallerImage,
							Command: []string{"/bin/sh", "-c", extractScript},
							Env: []corev1.EnvVar{
								{
									// Downward API: inject the node name so the
									// kubectl annotate command knows which node to patch.
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "ibmse-bundle", MountPath: "/bundle", ReadOnly: true},
								{Name: "ibmse-host", MountPath: "/host-ibmse"},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
							},
						},
					},
				},
			},
		},
	}
}

// ── Reconcile logic ───────────────────────────────────────────────────────────

// reconcileIBMSEJobs is the top-level entry point called from buildKbsConfigSpec.
// It replaces the DaemonSet-based approach with per-node Jobs.
//
// Algorithm (per reconcile):
//  1. Read the current bundle SHA from the ibmse-bundle Secret.
//  2. List all worker nodes.
//  3. For each worker node:
//     a. If the node annotation already matches the current SHA → files are current, skip.
//     b. Otherwise (annotation absent, stale, or node just rebooted):
//   - If an existing Job for this node exists AND is for the same SHA AND is still
//     active (not Failed) → leave it running, just wait.
//   - If an existing Job for this node exists but is for a different SHA (bundle
//     rotation) OR has Failed → delete it and create a fresh Job.
//   - If no Job exists → create one.
//  4. Return allReady=true only when every worker node's annotation matches the
//     current SHA and no Job is still Running/Pending for any node.
func (r *TrusteeConfigReconciler) reconcileIBMSEJobs(ctx context.Context) (bool, error) {
	bundleSHA, err := r.ibmseCurrentBundleSHA(ctx)
	if err != nil {
		return false, err
	}

	workers, err := r.ibmseListWorkerNodes(ctx)
	if err != nil {
		return false, err
	}
	if len(workers) == 0 {
		r.log.Info("IBM SE: no worker nodes found yet; waiting")
		return false, nil
	}

	allReady := true

	for _, node := range workers {
		nodeSHA := ibmseNodeBundleSHA(node)

		if nodeSHA == bundleSHA {
			// Node is current — files are present and match the bundle Secret.
			r.log.V(1).Info("IBM SE: node bundle up to date", "node", node.Name, "sha", bundleSHA)
			continue
		}

		// Node needs (re-)installation.  Check whether a Job already exists.
		allReady = false
		jobName := r.ibmseJobName(node.Name)
		existingJob := &batchv1.Job{}
		getErr := r.Get(ctx, client.ObjectKey{Namespace: r.namespace, Name: jobName}, existingJob)

		if getErr != nil && !k8serrors.IsNotFound(getErr) {
			return false, fmt.Errorf("failed to get Job %q: %w", jobName, getErr)
		}

		if k8serrors.IsNotFound(getErr) {
			// No Job exists for this node — create one.
			if err := r.createIBMSEJob(ctx, node.Name, bundleSHA); err != nil {
				return false, err
			}
			r.log.Info("IBM SE: created installer Job", "node", node.Name, "job", jobName)
			continue
		}

		// A Job exists.  Decide whether to reuse it or replace it.
		existingSHA := existingJob.Annotations["confidentialcontainers.org/ibmse-bundle-sha"]
		jobFailed := isJobFailed(existingJob)
		bundleChanged := existingSHA != bundleSHA

		if bundleChanged || jobFailed {
			reason := "bundle rotated"
			if jobFailed {
				reason = "previous Job failed"
			}
			r.log.Info("IBM SE: deleting and recreating installer Job",
				"node", node.Name, "job", jobName, "reason", reason)
			if err := r.deleteIBMSEJob(ctx, existingJob); err != nil {
				return false, err
			}
			if err := r.createIBMSEJob(ctx, node.Name, bundleSHA); err != nil {
				return false, err
			}
			continue
		}

		// Job exists, same SHA, and has not failed — it is either Running or Succeeded.
		if isJobSucceeded(existingJob) {
			// Job succeeded but the node annotation is still absent/stale — this can
			// happen if the kubectl annotate command succeeded but the node object
			// hasn't propagated yet.  Re-check on next reconcile.
			r.log.V(1).Info("IBM SE: Job succeeded but node annotation not yet visible; requeuing",
				"node", node.Name)
		} else {
			r.log.V(1).Info("IBM SE: installer Job still running", "node", node.Name, "job", jobName)
		}
	}

	return allReady, nil
}

// createIBMSEJob creates a new installer Job for the given node.
func (r *TrusteeConfigReconciler) createIBMSEJob(ctx context.Context, nodeName, bundleSHA string) error {
	bundleSecretName := r.trusteeConfig.Spec.IbmSE.BundleSecretName
	job := r.generateIBMSEJob(nodeName, bundleSecretName, bundleSHA)
	if err := ctrl.SetControllerReference(r.trusteeConfig, job, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on Job %q: %w", job.Name, err)
	}
	if err := r.Create(ctx, job); err != nil {
		return fmt.Errorf("failed to create IBM SE installer Job for node %q: %w", nodeName, err)
	}
	return nil
}

// deleteIBMSEJob deletes the given Job and its pods (foreground cascade propagation).
func (r *TrusteeConfigReconciler) deleteIBMSEJob(ctx context.Context, job *batchv1.Job) error {
	propagation := metav1.DeletePropagationForeground
	if err := r.Delete(ctx, job, &client.DeleteOptions{
		PropagationPolicy: &propagation,
	}); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete IBM SE installer Job %q: %w", job.Name, err)
	}
	return nil
}

// ── Status helpers ────────────────────────────────────────────────────────────

func isJobSucceeded(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func isJobFailed(job *batchv1.Job) bool {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// ptr returns a pointer to the given int32 value.
func ptr(i int32) *int32 { return &i }
