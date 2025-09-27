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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPeer(t *testing.T) {
	p := Peer{ID: "node1", Address: "127.0.0.1:8080"}
	assert.Equal(t, "node1", p.ID)
	assert.Equal(t, "127.0.0.1:8080", p.Address)
}

func TestKubeDiscovery_DiscoverPeers(t *testing.T) {
	// Set the hostname for the current "pod" to test the exclusion logic
	originalHostname, _ := os.Hostname()
	require.NoError(t, os.Setenv("HOSTNAME", "pod-1"))
	t.Cleanup(func() { _ = os.Setenv("HOSTNAME", originalHostname) })

	clientset := fake.NewSimpleClientset(&v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "emqx-go",
			Namespace: "default",
		},
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{IP: "10.0.0.1", Hostname: "pod-1"}, // Current pod
					{IP: "10.0.0.2", Hostname: "pod-2"},
				},
				Ports: []v1.EndpointPort{
					{Name: "grpc", Port: 8081},
				},
			},
		},
	})

	kd := NewKubeDiscoveryWithClient(clientset, "default", "emqx-go", "grpc")
	peers, err := kd.DiscoverPeers(context.Background())
	require.NoError(t, err)

	require.Len(t, peers, 1)
	assert.Equal(t, "pod-2", peers[0].ID)
	assert.Equal(t, "10.0.0.2:8081", peers[0].Address)

	// Test case where the named port is not found
	kd.portName = "unknown-port"
	peers, err = kd.DiscoverPeers(context.Background())
	require.NoError(t, err)
	assert.Len(t, peers, 0)
}

func TestNewKubeDiscovery(t *testing.T) {
	// This test will only pass in an environment where it cannot get an in-cluster config,
	// which is the case for this test runner.
	_, err := NewKubeDiscovery("default", "emqx-go", "grpc")
	assert.Error(t, err)
}