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

// KubeDiscovery implements the Discovery interface for discovering peer nodes
// in a Kubernetes environment. It uses the Kubernetes API to find the endpoints
// of a service, which correspond to the pods running the EMQX-Go application.
type KubeDiscovery struct {
	clientset *kubernetes.Clientset
	namespace string
	service   string
	portName  string
}

// NewKubeDiscovery creates a new Kubernetes discovery client.
// It is intended to be run from within a Kubernetes pod, as it uses the
// in-cluster configuration to create a Kubernetes clientset.
//
// namespace is the Kubernetes namespace where the service is located.
// service is the name of the Kubernetes service to discover endpoints for.
// portName is the name of the port in the service definition to use for the peer address.
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

// DiscoverPeers queries the Kubernetes API for the endpoints of the configured
// service and returns a list of discovered peers. It excludes the current pod
// from the list of peers.
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