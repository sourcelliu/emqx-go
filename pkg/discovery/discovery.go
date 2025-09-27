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

// Package discovery defines a generic interface for service discovery mechanisms,
// allowing the cluster to find other peer nodes in various environments. This
// is a key component for automatic clustering.
package discovery

import "context"

// Peer represents a discovered peer node in the cluster. It contains the
// necessary information to establish a connection with the peer.
type Peer struct {
	// ID is the unique identifier of the peer node.
	ID string
	// Address is the network address (e.g., "ip:port") where the peer's
	// cluster service is listening.
	Address string
}

// Discovery is the interface that all service discovery implementations must
// satisfy. It provides a mechanism for a node to find the addresses of its
// peers in a dynamic environment.
type Discovery interface {
	// DiscoverPeers queries the discovery service to get a list of all peer
	// nodes that are part of the cluster.
	//
	// - ctx: A context for managing the discovery request, including timeouts.
	//
	// Returns a slice of Peer objects or an error if the discovery process fails.
	DiscoverPeers(ctx context.Context) ([]Peer, error)
}