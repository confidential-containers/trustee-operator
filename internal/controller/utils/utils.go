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
package utils

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// discoveryTimeout bounds a single API discovery attempt so an unresponsive API
// server cannot block the operator before the manager starts. The overall call
// may make several such attempts (see discoveryBackoff).
const discoveryTimeout = 10 * time.Second

// discoveryBackoff bounds the retries for the one-shot cluster-type detection at
// startup. The API server may be briefly throttling or unreachable right after the
// operator pod is scheduled; retrying transient failures here avoids an otherwise
// certain CrashLoopBackoff. Roughly 0.5s, 1s, 2s, 4s between attempts (~7.5s of
// backoff on top of the per-attempt discoveryTimeout).
var discoveryBackoff = wait.Backoff{
	Steps:    5,
	Duration: 500 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.1,
}

// isRetriableDiscoveryErr reports whether a discovery error is transient and worth
// retrying. Permanent errors (e.g. forbidden/RBAC) are not retried so the operator
// fails fast instead of burning the backoff budget on an inevitable failure.
func isRetriableDiscoveryErr(err error) bool {
	// Transient server-side conditions reported as APIStatus errors.
	if apierrors.IsServerTimeout(err) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsUnexpectedServerError(err) {
		return true
	}

	// Transport-level failures (connection refused, DNS lookup failures, dial
	// timeouts) carry no APIStatus and surface as net errors. These are common
	// while the API server is still coming up right after the operator pod is
	// scheduled, so treat them as retriable too.
	var netErr net.Error
	return errors.As(err, &netErr)
}

// PlatformCapabilities reports which OpenShift-specific API capabilities the
// cluster serves. The two flags are independent because a cluster may expose
// one API group without the other, and different call sites depend on
// different groups (e.g. Route creation vs. reading the APIServer TLS profile).
type PlatformCapabilities struct {
	// HasRouteAPI is true when route.openshift.io is served, i.e. OpenShift
	// Routes can be created (used for operand exposure and OpenShift-specific
	// NetworkPolicy peers such as openshift-dns).
	HasRouteAPI bool
	// HasConfigAPI is true when config.openshift.io is served, i.e. the cluster
	// config API (including the APIServer CR that carries the TLS profile) is
	// available.
	HasConfigAPI bool
}

// DetectPlatformCapabilities queries the cluster's API groups and reports which
// OpenShift API capabilities are available. Each discovery attempt is bounded by
// discoveryTimeout, and transient failures are retried with discoveryBackoff; the
// call also returns promptly if ctx is cancelled. Because both the number of
// attempts and the per-attempt timeout are bounded, the discovery goroutine cannot
// run indefinitely even after the caller has returned.
func DetectPlatformCapabilities(ctx context.Context, config *rest.Config) (PlatformCapabilities, error) {
	caps := PlatformCapabilities{}

	// DiscoveryClient.ServerGroups() has no context-aware variant in this
	// client-go version (it uses context.TODO() internally), so a client-side
	// timeout on a copied config is what actually bounds each HTTP request.
	// Copy the config to avoid mutating the caller's.
	cfg := rest.CopyConfig(config)
	if cfg.Timeout <= 0 || cfg.Timeout > discoveryTimeout {
		cfg.Timeout = discoveryTimeout
	}

	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return caps, err
	}

	// Run the (context-unaware) discovery call in a goroutine so we can return
	// promptly when ctx is cancelled; the per-attempt timeout and the bounded
	// retry steps guarantee the goroutine cannot leak indefinitely. Transient
	// failures (throttling, server timeouts, API server briefly unavailable at
	// startup) are retried with backoff.
	type result struct {
		groups *metav1.APIGroupList
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		var groups *metav1.APIGroupList
		err := retry.OnError(discoveryBackoff, isRetriableDiscoveryErr, func() error {
			var listErr error
			groups, listErr = dc.ServerGroups()
			return listErr
		})
		ch <- result{groups: groups, err: err}
	}()

	select {
	case <-ctx.Done():
		return caps, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return caps, res.err
		}
		for _, group := range res.groups.Groups {
			switch group.Name {
			case "route.openshift.io":
				caps.HasRouteAPI = true
			case "config.openshift.io":
				caps.HasConfigAPI = true
			}
		}
		return caps, nil
	}
}

// IsConsolePluginSupported returns true if the given OCP version supports console plugins (>= 4.22).
func IsConsolePluginSupported(ocpVersion string) bool {
	v, err := k8sversion.ParseGeneric(ocpVersion)
	if err != nil {
		return false
	}
	return v.AtLeast(k8sversion.MajorMinor(4, 22))
}

// GetOpenshiftVersion reads the ClusterVersion CR and returns "major.minor".
func GetOpenshiftVersion(ctx context.Context, c client.Client) (string, error) {
	cv := &configv1.ClusterVersion{}
	if err := c.Get(ctx, client.ObjectKey{Name: "version"}, cv); err != nil {
		return "", fmt.Errorf("reading ClusterVersion: %w", err)
	}
	parts := strings.Split(cv.Status.Desired.Version, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("unexpected ClusterVersion format: %q", cv.Status.Desired.Version)
	}
	return parts[0] + "." + parts[1], nil
}
