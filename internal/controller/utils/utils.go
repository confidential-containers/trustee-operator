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
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// IsOpenShift checks if the cluster is running OpenShift
func IsOpenShift(config *rest.Config) (bool, error) {
	// Create a discovery client using the REST config
	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false, err
	}

	// Query the API groups available in the cluster
	apiGroupList, err := dc.ServerGroups()
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
