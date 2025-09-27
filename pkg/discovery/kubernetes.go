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

// KubeDiscovery provides an implementation of the Discovery interface that uses
// the Kubernetes API to find peer nodes. It works by querying the `Endpoints`
// resource associated with a headless service that exposes the application pods.
type KubeDiscovery struct {
	clientset kubernetes.Interface
	namespace string
	service   string
	portName  string
}

// NewKubeDiscoveryWithClient creates a new KubeDiscovery instance with a
// pre-configured Kubernetes clientset. This is useful for testing or for cases
// where the client is managed externally.
//
// - clientset: An initialized Kubernetes clientset.
// - namespace: The Kubernetes namespace where the service and pods reside.
// - service: The name of the headless service for the application.
// - portName: The name of the port in the service definition for cluster communication.
func NewKubeDiscoveryWithClient(clientset kubernetes.Interface, namespace, service, portName string) *KubeDiscovery {
	return &KubeDiscovery{
		clientset: clientset,
		namespace: namespace,
		service:   service,
		portName:  portName,
	}
}

// NewKubeDiscovery creates a new KubeDiscovery instance, automatically
// configuring the Kubernetes client from within a pod's environment using a
// service account.
//
// - namespace: The Kubernetes namespace where the service and pods reside.
// - service: The name of the headless service for the application.
// - portName: The name of the port in the service definition for cluster communication.
//
// Returns a configured KubeDiscovery instance or an error if in-cluster
// configuration fails.
func NewKubeDiscovery(namespace, service, portName string) (*KubeDiscovery, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("could not get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("could not create clientset: %w", err)
	}

	return NewKubeDiscoveryWithClient(clientset, namespace, service, portName), nil
}

// DiscoverPeers implements the Discovery interface for Kubernetes. It queries the
// Kubernetes API server to find the endpoints of a specific service. It then
// extracts the IP addresses and port numbers of the pods backing that service
// to build a list of peers.
//
// The method automatically excludes the current pod from the returned list to
// prevent a node from trying to connect to itself.
//
// - ctx: A context for managing the API request.
//
// Returns a slice of Peer objects representing the other nodes in the cluster,
// or an error if the API call fails.
func (k *KubeDiscovery) DiscoverPeers(ctx context.Context) ([]Peer, error) {
	endpoints, err := k.clientset.CoreV1().Endpoints(k.namespace).Get(ctx, k.service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints for service %s: %w", k.service, err)
	}

	var peers []Peer
	// In a Kubernetes environment, the pod's hostname is usually available via the HOSTNAME env var.
	// We fall back to os.Hostname() for other cases.
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

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