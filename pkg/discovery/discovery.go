package discovery

import "context"

// Peer represents another node in the cluster.
type Peer struct {
	ID      string
	Address string
}

// Discovery defines the interface for service discovery.
type Discovery interface {
	// DiscoverPeers returns a list of all peer nodes in the cluster.
	DiscoverPeers(ctx context.Context) ([]Peer, error)
}