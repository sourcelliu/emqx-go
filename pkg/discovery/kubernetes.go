// Copyright 2023 The emqx-go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package discovery

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubeDiscovery implements the Discovery interface using the Kubernetes API.
type KubeDiscovery struct {
	clientset *kubernetes.Clientset
	namespace string
	service   string
	portName  string
}

// NewKubeDiscovery creates a new Kubernetes discovery client.
// It attempts to configure itself from within a pod using a service account.
func NewKubeDiscovery(namespace, service, portName string) (*KubeDiscovery, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not create clientset: %w", err)
	}

	return &KubeDiscovery{
		clientset: clientset,
		namespace: namespace,
		service:   service,
		portName:  portName,
	}, nil
}

// DiscoverPeers finds other pods belonging to the same service by querying
// the Kubernetes Endpoints API.
func (k *KubeDiscovery) DiscoverPeers(ctx context.Context) ([]Peer, error) {
	endpoints, err := k.clientset.CoreV1().Endpoints(k.namespace).Get(ctx, k.service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints for service %s: %w", k.service, err)
	}

	var peers []Peer
	hostname, _ := os.Hostname() // Used to identify and exclude the current pod

	for _, subset := range endpoints.Subsets {
		var port int32
		for _, p := range subset.Ports {
			if p.Name == k.portName {
				port = p.Port
				break
			}
		}
		if port == 0 {
			continue // Skip subsets that don't have the named port
		}

		for _, addr := range subset.Addresses {
			// Exclude the current pod from the list of peers
			if addr.Hostname != "" && addr.Hostname == hostname {
				continue
			}
			peers = append(peers, Peer{
				ID:      addr.Hostname, // Pod hostname is a good unique ID
				Address: fmt.Sprintf("%s:%d", addr.IP, port),
			})
		}
	}

	return peers, nil
}