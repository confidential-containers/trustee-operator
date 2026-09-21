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
	"errors"
	"net"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
)

// discoveryBackoff bounds the retries for the one-shot cluster-type detection at
// startup. The API server may be briefly throttling or unreachable right after the
// operator pod is scheduled; retrying transient failures here avoids an otherwise
// certain CrashLoopBackoff. Roughly 0.5s, 1s, 2s, 4s between attempts (~7.5s total).
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

// IsOpenShift checks if the cluster is running OpenShift
func IsOpenShift(config *rest.Config) (bool, error) {
	// Create a discovery client using the REST config
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false, err
	}

	// Query the API groups available in the cluster, retrying transient failures
	// (throttling, server timeouts, API server briefly unavailable at startup).
	var apiGroupList *metav1.APIGroupList
	err = retry.OnError(discoveryBackoff, isRetriableDiscoveryErr, func() error {
		var listErr error
		apiGroupList, listErr = dc.ServerGroups()
		return listErr
	})
	if err != nil {
		return false, err
	}

	// Check if any API group belongs to OpenShift
	for _, group := range apiGroupList.Groups {
		if group.Name == "route.openshift.io" || group.Name == "config.openshift.io" {
			return true, nil
		}
	}

	return false, nil
}
