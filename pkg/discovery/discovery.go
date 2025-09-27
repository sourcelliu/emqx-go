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
package discovery

import "context"

// Peer represents a discovered peer node in the cluster.
type Peer struct {
	ID      string
	Address string
}

// Discovery defines the interface for service discovery mechanisms.
type Discovery interface {
	// DiscoverPeers returns a list of all peer nodes in the cluster.
	DiscoverPeers(ctx context.Context) ([]Peer, error)
}