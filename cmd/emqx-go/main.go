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

// package main is the entrypoint for the EMQX-Go application.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/discovery"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"google.golang.org/grpc"
)

const (
	// grpcPort is the port on which the gRPC server for cluster communication will listen.
	grpcPort = ":8081"
)

// main is the primary entrypoint for the EMQX-Go application.
// It initializes and starts all the major components of the server, including:
// - The cluster manager for handling peer connections and routing.
// - The MQTT broker for handling client connections and messaging.
// - The gRPC server for inter-node communication.
// - The service discovery mechanism for finding peer nodes.
//
// It also sets up a graceful shutdown mechanism to ensure that the server can
// terminate cleanly upon receiving an interrupt or termination signal.
func main() {
	log.Println("Starting EMQX-GO Broker PoC (Phase 3)...")

	nodeID, _ := os.Hostname()
	log.Printf("Node ID: %s", nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Setup Cluster Manager ---
	clusterMgr := cluster.NewManager(nodeID, fmt.Sprintf("%s%s", nodeID, grpcPort))

	// --- Start Local Broker ---
	b := broker.New(nodeID, clusterMgr)
	go func() {
		if err := b.StartServer(ctx, ":1883"); err != nil {
			log.Fatalf("Broker server failed: %v", err)
		}
	}()

	// --- Start gRPC Server for Clustering ---
	grpcServer := grpc.NewServer()
	clusterServer := cluster.NewServer(nodeID)
	clusterpb.RegisterClusterServiceServer(grpcServer, clusterServer)

	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("Failed to listen for gRPC: %v", err)
	}
	go func() {
		log.Printf("gRPC server listening on %s", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()
	defer grpcServer.Stop()

	// --- Start Discovery and Peer Connection ---
	go startDiscovery(ctx, clusterMgr)

	// --- Wait for Shutdown Signal ---
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)
	<-shutdownChan

	log.Println("Shutdown signal received. Shutting down...")
}

// startDiscovery initializes and runs the service discovery process.
// It periodically queries the discovery service (e.g., Kubernetes API) to find
// peer nodes and instructs the cluster manager to connect to them.
// For local testing, it includes a fallback to simulate peer discovery if a
// Kubernetes environment is not detected.
func startDiscovery(ctx context.Context, mgr *cluster.Manager) {
	// In a real K8s environment, these would be populated from env vars.
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	// The service name should match the headless service for the statefulset
	serviceName := "emqx-go-headless"
	portName := "grpc"

	disc, err := discovery.NewKubeDiscovery(namespace, serviceName, portName)
	if err != nil {
		log.Printf("Could not initialize Kubernetes discovery, will not connect to peers: %v", err)
		// For local testing, we can simulate peers
		// This part would be removed in a real deployment script.
		if strings.Contains(err.Error(), "in-cluster configuration") {
			log.Println("Simulating peer discovery for local testing...")
			go mgr.AddPeer(ctx, "emqx-go-1", "localhost:8082") // Simulate another node
		}
		return
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peers, err := disc.DiscoverPeers(ctx)
			if err != nil {
				log.Printf("Failed to discover peers: %v", err)
				continue
			}
			log.Printf("Discovered %d peers", len(peers))

			for _, peer := range peers {
				go mgr.AddPeer(ctx, peer.ID, peer.Address)
			}
		}
	}
}