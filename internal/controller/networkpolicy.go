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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NetworkPolicy names for the KBS operand.
const (
	kbsNetworkPolicyDenyAll           = "kbs-deny-all"
	kbsNetworkPolicyAllowIngress      = "kbs-allow-ingress"
	kbsNetworkPolicyAllowEgressDNS    = "kbs-allow-egress-dns"
	kbsNetworkPolicyAllowEgressAttest = "kbs-allow-egress-attestation"

	// Cluster DNS namespaces. OpenShift runs CoreDNS in openshift-dns; vanilla
	// Kubernetes runs it in kube-system. Selected at runtime via IsOpenShift.
	openShiftDNSNamespace = "openshift-dns"
	upstreamDNSNamespace  = "kube-system"

	// openShiftRouterNamespaceLabel selects the OpenShift ingress router namespace.
	// The router is host-networked, so it is not covered by an "any in-cluster pod"
	// ingress rule and must be admitted explicitly. There is no equivalent on
	// vanilla Kubernetes.
	openShiftRouterNamespaceLabel = "policy-group.network.openshift.io/ingress"
)

// The KBS operand pods (Deployment/Service) are selected by this label,
// see newKbsDeployment/newKbsService.
var kbsPodSelector = metav1.LabelSelector{
	MatchLabels: map[string]string{
		"app": "kbs",
	},
}

// deployOrUpdateKbsNetworkPolicies creates or updates the set of operator-owned
// NetworkPolicies that protect the KBS operand pods.
//
// The policies are scoped to the operand pods by label
// (app=kbs) rather than to the whole namespace. Every policy is owner-referenced to the
// KbsConfig CR and watched via Owns() in SetupWithManager, so a user who edits or
// deletes one has it restored on the next reconcile.
func (r *KbsConfigReconciler) deployOrUpdateKbsNetworkPolicies(ctx context.Context) error {
	policies, err := r.newKbsNetworkPolicies()
	if err != nil {
		return err
	}

	for _, desired := range policies {
		found := &networkingv1.NetworkPolicy{}
		err := r.Get(ctx, client.ObjectKey{
			Namespace: desired.Namespace,
			Name:      desired.Name,
		}, found)

		if err != nil && k8serrors.IsNotFound(err) {
			// Create the NetworkPolicy
			r.log.Info("Creating a new NetworkPolicy", "NetworkPolicy.Namespace", desired.Namespace, "NetworkPolicy.Name", desired.Name)
			if err := r.Create(ctx, desired); err != nil {
				r.Recorder.Eventf(r.kbsConfig, nil, corev1.EventTypeWarning, "NetworkPolicyCreateFailed", "NetworkPolicyCreateFailed", err.Error())
				return err
			}
			r.Recorder.Eventf(r.kbsConfig, nil, corev1.EventTypeNormal, "NetworkPolicyCreated", "NetworkPolicyCreated", "KBS NetworkPolicy %s created successfully", desired.Name)
			continue
		} else if err != nil {
			return err
		}

		// NetworkPolicy already exists: reconcile it back to the desired spec if it
		// drifted (e.g. a user edited it). This is what enforces operator ownership.
		if apiequality.Semantic.DeepEqual(found.Spec, desired.Spec) {
			continue
		}
		r.log.Info("Updating NetworkPolicy", "NetworkPolicy.Namespace", desired.Namespace, "NetworkPolicy.Name", desired.Name)
		found.Spec = desired.Spec
		if err := r.Update(ctx, found); err != nil {
			r.Recorder.Eventf(r.kbsConfig, nil, corev1.EventTypeWarning, "NetworkPolicyUpdateFailed", "NetworkPolicyUpdateFailed", err.Error())
			return err
		}
	}
	return nil
}

// newKbsNetworkPolicies builds the full set of NetworkPolicies for the KBS operand.
// Each policy selects the operand pods (app=kbs) in the controller namespace and is
// owner-referenced to the KbsConfig CR so it is garbage collected with the operand.
func (r *KbsConfigReconciler) newKbsNetworkPolicies() ([]*networkingv1.NetworkPolicy, error) {
	tcp := corev1.ProtocolTCP
	udp := corev1.ProtocolUDP
	port8080 := intstr.FromInt(8080)
	port53 := intstr.FromInt(53)
	port5353 := intstr.FromInt(5353)
	port443 := intstr.FromInt(443)

	// 1. Default-deny baseline. Selecting the operand pods and enabling both policy
	// types with NO rules denies all ingress and egress; every allowed flow below is
	// re-opened explicitly by a sibling policy.
	denyAll := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbsNetworkPolicyDenyAll,
			Namespace: r.namespace,
			Labels:    standardLabels(r.kbsConfig.Name, "network-policy"),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: kbsPodSelector,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	// 2. Ingress to the KBS attestation/resource endpoint (:8080). KBS is the
	// guest-facing trust endpoint: guest attestation agents and clients live in
	// arbitrary namespaces, so ingress is allowed from any in-cluster peer.
	ingressRules := []networkingv1.NetworkPolicyIngressRule{
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: &port8080},
			},
		},
	}
	// On OpenShift, the ingress router is host-networked, which the any-client
	// rule above does not cover on OVN-Kubernetes; admit the router namespace
	// so external access via kbs-route works. Vanilla Kubernetes has no such router.
	if r.IsOpenShift {
		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							openShiftRouterNamespaceLabel: "",
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcp, Port: &port8080},
			},
		})
	}
	allowIngress := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbsNetworkPolicyAllowIngress,
			Namespace: r.namespace,
			Labels:    standardLabels(r.kbsConfig.Name, "network-policy"),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: kbsPodSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     ingressRules,
		},
	}

	// 3. Egress to cluster DNS. KBS needs name resolution to reach external
	// attestation peers and any in-cluster service it is configured against. The DNS
	// namespace differs by platform (openshift-dns vs kube-system).
	dnsNamespace := upstreamDNSNamespace
	if r.IsOpenShift {
		dnsNamespace = openShiftDNSNamespace
	}
	allowEgressDNS := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbsNetworkPolicyAllowEgressDNS,
			Namespace: r.namespace,
			Labels:    standardLabels(r.kbsConfig.Name, "network-policy"),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: kbsPodSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"kubernetes.io/metadata.name": dnsNamespace,
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &port5353},
						{Protocol: &tcp, Port: &port5353},
						{Protocol: &udp, Port: &port53},
						{Protocol: &tcp, Port: &port53},
					},
				},
			},
		},
	}

	// 4. Egress to the external attestation/verification peers (HTTPS :443) used in
	// the connected profile: AMD KDS, Intel PCS/Trust Authority, NVIDIA NRAS.
	// NetworkPolicy cannot match destinations by DNS name, so this allows :443 to any
	// destination as a portable baseline.
	allowEgressAttestation := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kbsNetworkPolicyAllowEgressAttest,
			Namespace: r.namespace,
			Labels:    standardLabels(r.kbsConfig.Name, "network-policy"),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: kbsPodSelector,
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &tcp, Port: &port443},
					},
				},
			},
		},
	}

	policies := []*networkingv1.NetworkPolicy{
		denyAll,
		allowIngress,
		allowEgressDNS,
		allowEgressAttestation,
	}

	// Set KbsConfig instance as the owner and controller of every policy so they are
	// garbage collected with the operand and restored by Owns() if tampered with.
	for _, np := range policies {
		if err := ctrl.SetControllerReference(r.kbsConfig, np, r.Scheme); err != nil {
			r.log.Info("Error in setting the controller reference for the KBS NetworkPolicy", "NetworkPolicy.Name", np.Name, "err", err)
			return nil, err
		}
	}

	return policies, nil
}
