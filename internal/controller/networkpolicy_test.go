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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
)

const testNPNamespace = "trustee-operator-system"

func newNetworkPolicyTestReconciler(t *testing.T, isOpenShift bool, objs ...client.Object) (*KbsConfigReconciler, *confidentialcontainersorgv1alpha1.KbsConfig) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := networkingv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add networkingv1 to scheme: %v", err)
	}
	if err := confidentialcontainersorgv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add v1alpha1 to scheme: %v", err)
	}

	kbsConfig := &confidentialcontainersorgv1alpha1.KbsConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-kbsconfig",
			Namespace: testNPNamespace,
			UID:       "test-uid",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(append(objs, kbsConfig)...).
		Build()

	r := &KbsConfigReconciler{
		Client:      fakeClient,
		Scheme:      scheme,
		Recorder:    &events.FakeRecorder{},
		kbsConfig:   kbsConfig,
		log:         logr.Discard(),
		namespace:   testNPNamespace,
		IsOpenShift: isOpenShift,
	}
	return r, kbsConfig
}

func TestNewKbsNetworkPolicies(t *testing.T) {
	r, kbsConfig := newNetworkPolicyTestReconciler(t, true)

	policies, err := r.newKbsNetworkPolicies()
	if err != nil {
		t.Fatalf("newKbsNetworkPolicies returned error: %v", err)
	}

	expected := map[string]bool{
		kbsNetworkPolicyDenyAll:           false,
		kbsNetworkPolicyAllowIngress:      false,
		kbsNetworkPolicyAllowEgressDNS:    false,
		kbsNetworkPolicyAllowEgressAttest: false,
	}
	if len(policies) != len(expected) {
		t.Fatalf("expected %d policies, got %d", len(expected), len(policies))
	}

	for _, np := range policies {
		if _, ok := expected[np.Name]; !ok {
			t.Errorf("unexpected policy name %q", np.Name)
			continue
		}
		expected[np.Name] = true

		if np.Namespace != testNPNamespace {
			t.Errorf("%s: expected namespace %q, got %q", np.Name, testNPNamespace, np.Namespace)
		}
		// Every policy must be scoped to the operand pods by label, never the whole
		// namespace (layered-product requirement).
		if got := np.Spec.PodSelector.MatchLabels["app"]; got != "kbs" {
			t.Errorf("%s: expected podSelector app=kbs, got %q", np.Name, got)
		}
		// Every policy must be owner-referenced to the KbsConfig CR so it is GC'd
		// with the operand and reverted when tampered with.
		if len(np.OwnerReferences) != 1 {
			t.Errorf("%s: expected 1 owner reference, got %d", np.Name, len(np.OwnerReferences))
			continue
		}
		owner := np.OwnerReferences[0]
		if owner.Name != kbsConfig.Name || owner.Kind != "KbsConfig" {
			t.Errorf("%s: unexpected owner reference %+v", np.Name, owner)
		}
		if owner.Controller == nil || !*owner.Controller {
			t.Errorf("%s: owner reference is not a controller reference", np.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing expected policy %q", name)
		}
	}
}

func TestNewKbsNetworkPolicies_DenyAll(t *testing.T) {
	r, _ := newNetworkPolicyTestReconciler(t, true)
	policies, err := r.newKbsNetworkPolicies()
	if err != nil {
		t.Fatalf("newKbsNetworkPolicies returned error: %v", err)
	}

	np := findPolicy(t, policies, kbsNetworkPolicyDenyAll)
	// Default-deny: both policy types enabled, no ingress/egress rules.
	if !hasPolicyType(np, networkingv1.PolicyTypeIngress) || !hasPolicyType(np, networkingv1.PolicyTypeEgress) {
		t.Errorf("deny-all must enable both Ingress and Egress policy types, got %v", np.Spec.PolicyTypes)
	}
	if len(np.Spec.Ingress) != 0 || len(np.Spec.Egress) != 0 {
		t.Errorf("deny-all must have no rules, got ingress=%d egress=%d", len(np.Spec.Ingress), len(np.Spec.Egress))
	}
}

func TestNewKbsNetworkPolicies_AllowIngress(t *testing.T) {
	tests := []struct {
		name        string
		isOpenShift bool
		wantRules   int
		wantRouter  bool
	}{
		{name: "openshift", isOpenShift: true, wantRules: 2, wantRouter: true},
		{name: "kubernetes", isOpenShift: false, wantRules: 1, wantRouter: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newNetworkPolicyTestReconciler(t, tt.isOpenShift)
			policies, err := r.newKbsNetworkPolicies()
			if err != nil {
				t.Fatalf("newKbsNetworkPolicies returned error: %v", err)
			}

			np := findPolicy(t, policies, kbsNetworkPolicyAllowIngress)
			if !hasPolicyType(np, networkingv1.PolicyTypeIngress) || hasPolicyType(np, networkingv1.PolicyTypeEgress) {
				t.Errorf("allow-ingress must be Ingress-only, got %v", np.Spec.PolicyTypes)
			}
			if len(np.Spec.Ingress) != tt.wantRules {
				t.Fatalf("expected %d ingress rule(s), got %d", tt.wantRules, len(np.Spec.Ingress))
			}
			// Rule 0: any client on 8080 (no from restriction), always present.
			if len(np.Spec.Ingress[0].From) != 0 {
				t.Errorf("first ingress rule must allow any client (empty from), got %v", np.Spec.Ingress[0].From)
			}
			if !ingressRuleHasTCPPort(np.Spec.Ingress[0], 8080) {
				t.Errorf("first ingress rule must open TCP 8080")
			}
			if !tt.wantRouter {
				return
			}
			// Rule 1 (OpenShift only): from the ingress router.
			if len(np.Spec.Ingress[1].From) != 1 || np.Spec.Ingress[1].From[0].NamespaceSelector == nil {
				t.Fatalf("second ingress rule must select the router namespace")
			}
			if _, ok := np.Spec.Ingress[1].From[0].NamespaceSelector.MatchLabels[openShiftRouterNamespaceLabel]; !ok {
				t.Errorf("second ingress rule must select the router via %s", openShiftRouterNamespaceLabel)
			}
			if !ingressRuleHasTCPPort(np.Spec.Ingress[1], 8080) {
				t.Errorf("router ingress rule must open TCP 8080")
			}
		})
	}
}

func TestNewKbsNetworkPolicies_EgressDNS(t *testing.T) {
	tests := []struct {
		name        string
		isOpenShift bool
		wantNS      string
	}{
		{name: "openshift", isOpenShift: true, wantNS: openShiftDNSNamespace},
		{name: "kubernetes", isOpenShift: false, wantNS: upstreamDNSNamespace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newNetworkPolicyTestReconciler(t, tt.isOpenShift)
			policies, err := r.newKbsNetworkPolicies()
			if err != nil {
				t.Fatalf("newKbsNetworkPolicies returned error: %v", err)
			}

			np := findPolicy(t, policies, kbsNetworkPolicyAllowEgressDNS)
			if !hasPolicyType(np, networkingv1.PolicyTypeEgress) || hasPolicyType(np, networkingv1.PolicyTypeIngress) {
				t.Errorf("allow-egress-dns must be Egress-only, got %v", np.Spec.PolicyTypes)
			}
			if len(np.Spec.Egress) != 1 || len(np.Spec.Egress[0].To) != 1 {
				t.Fatalf("egress-dns must have a single rule targeting the DNS namespace")
			}
			sel := np.Spec.Egress[0].To[0].NamespaceSelector
			if sel == nil || sel.MatchLabels["kubernetes.io/metadata.name"] != tt.wantNS {
				t.Errorf("egress-dns must target the %s namespace, got %+v", tt.wantNS, sel)
			}
			// Must open both UDP and TCP on 53 and 5353.
			for _, want := range []struct {
				proto corev1.Protocol
				port  int32
			}{
				{corev1.ProtocolUDP, 53}, {corev1.ProtocolTCP, 53},
				{corev1.ProtocolUDP, 5353}, {corev1.ProtocolTCP, 5353},
			} {
				if !egressRuleHasPort(np.Spec.Egress[0], want.proto, want.port) {
					t.Errorf("egress-dns must open %s/%d", want.proto, want.port)
				}
			}
		})
	}
}

func TestNewKbsNetworkPolicies_EgressAttestation(t *testing.T) {
	r, _ := newNetworkPolicyTestReconciler(t, true)
	policies, err := r.newKbsNetworkPolicies()
	if err != nil {
		t.Fatalf("newKbsNetworkPolicies returned error: %v", err)
	}

	np := findPolicy(t, policies, kbsNetworkPolicyAllowEgressAttest)
	if !hasPolicyType(np, networkingv1.PolicyTypeEgress) || hasPolicyType(np, networkingv1.PolicyTypeIngress) {
		t.Errorf("allow-egress-attestation must be Egress-only, got %v", np.Spec.PolicyTypes)
	}
	if len(np.Spec.Egress) != 1 {
		t.Fatalf("expected 1 egress rule, got %d", len(np.Spec.Egress))
	}
	// Baseline: no ipBlock/namespace restriction yet (pinned at discovery), TCP 443 only.
	if len(np.Spec.Egress[0].To) != 0 {
		t.Errorf("attestation egress baseline must not restrict destinations, got %v", np.Spec.Egress[0].To)
	}
	if !egressRuleHasPort(np.Spec.Egress[0], corev1.ProtocolTCP, 443) {
		t.Errorf("attestation egress must open TCP 443")
	}
}

func TestDeployOrUpdateKbsNetworkPolicies_Creates(t *testing.T) {
	r, _ := newNetworkPolicyTestReconciler(t, true)

	if err := r.deployOrUpdateKbsNetworkPolicies(context.Background()); err != nil {
		t.Fatalf("deployOrUpdateKbsNetworkPolicies returned error: %v", err)
	}

	for _, name := range []string{
		kbsNetworkPolicyDenyAll,
		kbsNetworkPolicyAllowIngress,
		kbsNetworkPolicyAllowEgressDNS,
		kbsNetworkPolicyAllowEgressAttest,
	} {
		got := &networkingv1.NetworkPolicy{}
		err := r.Get(context.Background(), types.NamespacedName{Name: name, Namespace: testNPNamespace}, got)
		if err != nil {
			t.Errorf("expected policy %q to be created: %v", name, err)
		}
	}
}

func TestDeployOrUpdateKbsNetworkPolicies_RevertsDrift(t *testing.T) {
	r, _ := newNetworkPolicyTestReconciler(t, true)
	ctx := context.Background()

	if err := r.deployOrUpdateKbsNetworkPolicies(ctx); err != nil {
		t.Fatalf("initial reconcile returned error: %v", err)
	}

	// Simulate a user tampering with the default-deny policy by punching a hole in it.
	tampered := &networkingv1.NetworkPolicy{}
	if err := r.Get(ctx, types.NamespacedName{Name: kbsNetworkPolicyDenyAll, Namespace: testNPNamespace}, tampered); err != nil {
		t.Fatalf("failed to get deny-all policy: %v", err)
	}
	tampered.Spec.Ingress = []networkingv1.NetworkPolicyIngressRule{{}}
	if err := r.Update(ctx, tampered); err != nil {
		t.Fatalf("failed to tamper with deny-all policy: %v", err)
	}

	// Reconcile must revert the drift.
	if err := r.deployOrUpdateKbsNetworkPolicies(ctx); err != nil {
		t.Fatalf("second reconcile returned error: %v", err)
	}

	reverted := &networkingv1.NetworkPolicy{}
	if err := r.Get(ctx, types.NamespacedName{Name: kbsNetworkPolicyDenyAll, Namespace: testNPNamespace}, reverted); err != nil {
		t.Fatalf("failed to get reverted policy: %v", err)
	}
	if len(reverted.Spec.Ingress) != 0 {
		t.Errorf("expected deny-all to be reverted to no ingress rules, got %d", len(reverted.Spec.Ingress))
	}
}

// --- helpers ---

func findPolicy(t *testing.T, policies []*networkingv1.NetworkPolicy, name string) *networkingv1.NetworkPolicy {
	t.Helper()
	for _, np := range policies {
		if np.Name == name {
			return np
		}
	}
	t.Fatalf("policy %q not found", name)
	return nil
}

func hasPolicyType(np *networkingv1.NetworkPolicy, pt networkingv1.PolicyType) bool {
	for _, t := range np.Spec.PolicyTypes {
		if t == pt {
			return true
		}
	}
	return false
}

func ingressRuleHasTCPPort(rule networkingv1.NetworkPolicyIngressRule, port int32) bool {
	for _, p := range rule.Ports {
		if p.Port != nil && p.Port.IntVal == port && p.Protocol != nil && *p.Protocol == corev1.ProtocolTCP {
			return true
		}
	}
	return false
}

func egressRuleHasPort(rule networkingv1.NetworkPolicyEgressRule, proto corev1.Protocol, port int32) bool {
	for _, p := range rule.Ports {
		if p.Port != nil && p.Port.IntVal == port && p.Protocol != nil && *p.Protocol == proto {
			return true
		}
	}
	return false
}
