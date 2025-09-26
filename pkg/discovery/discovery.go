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

// package discovery provides interfaces and implementations for service discovery.
// This allows the EMQX-Go cluster to dynamically find and connect to peer nodes
// in various environments, such as Kubernetes.
package discovery

import "context"

// Peer represents a discovered peer node in the cluster.
// It contains the necessary information to establish a connection to the peer.
type Peer struct {
	// ID is the unique identifier of the peer node.
	ID string
	// Address is the network address of the peer node, typically in "host:port"
	// format.
	Address string
}

// Discovery defines the interface for service discovery mechanisms.
// Implementations of this interface are responsible for finding peer nodes
// in a specific environment.
type Discovery interface {
	// DiscoverPeers queries the discovery service and returns a list of all
	// discovered peer nodes in the cluster.
	DiscoverPeers(ctx context.Context) ([]Peer, error)
}