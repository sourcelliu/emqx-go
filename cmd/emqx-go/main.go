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

// Package main serves as the entry point for the EMQX-Go application. It is
// responsible for initializing and orchestrating all the major components of the
// broker, including the MQTT server, the gRPC server for clustering, the
// metrics server, and the peer discovery mechanism. It also handles graceful
// shutdown on receiving termination signals.
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
	"github.com/turtacn/emqx-go/pkg/metrics"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
	"google.golang.org/grpc"
)

const (
	// grpcPort is the port on which the gRPC server listens for inter-node
	// communication.
	grpcPort = ":8081"
	// metricsPort is the port on which the Prometheus metrics server listens.
	metricsPort = ":8082"
)

// main is the primary function that starts the EMQX-Go broker.
// It follows these steps:
// 1. Sets up a unique node ID.
// 2. Initializes the main context for graceful shutdown.
// 3. Creates the cluster manager and the main broker, linking them together.
// 4. Starts the MQTT broker server in a goroutine.
// 5. Starts the gRPC server for cluster communication in a goroutine.
// 6. Starts the Prometheus metrics server in a goroutine.
// 7. Starts the peer discovery process in a goroutine.
// 8. Blocks and waits for a shutdown signal (SIGINT or SIGTERM) to gracefully
//    terminate the application.
func main() {
	log.Println("Starting EMQX-GO Broker PoC (Phase 3)...")

	nodeID, _ := os.Hostname()
	if nodeID == "" {
		nodeID = "local-node"
	}
	log.Printf("Node ID: %s", nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Setup Broker and Cluster Manager ---
	// The broker needs to be created first to pass its publish function to the manager.
	var b *broker.Broker
	clusterMgr := cluster.NewManager(nodeID, fmt.Sprintf("%s%s", nodeID, grpcPort), func(topic string, payload []byte) {
		if b != nil {
			b.RouteToLocalSubscribers(topic, payload)
		}
	})

	// --- Start Local Broker ---
	b = broker.New(nodeID, clusterMgr)
	go func() {
		if err := b.StartServer(ctx, ":1883"); err != nil {
			log.Fatalf("Broker server failed: %v", err)
		}
	}()

	// --- Start gRPC Server for Clustering ---
	grpcServer := grpc.NewServer()
	clusterServer := cluster.NewServer(nodeID, clusterMgr)
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

	// --- Start Metrics Server ---
	go metrics.Serve(metricsPort)

	// --- Start Discovery and Peer Connection ---
	go startDiscovery(ctx, clusterMgr)

	// --- Wait for Shutdown Signal ---
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)
	<-shutdownChan

	log.Println("Shutdown signal received. Shutting down...")
}

// startDiscovery initializes and runs the peer discovery process. It is designed
// to work within a Kubernetes environment by default, using a headless service
// to find other broker pods. If not in a Kubernetes environment, it logs this
// fact and does nothing.
//
// The discovery process runs in a loop, periodically querying for peers and
// attempting to add them to the cluster manager.
//
// - ctx: The main application context. The discovery loop will terminate when this
//   context is canceled.
// - mgr: The cluster manager to which discovered peers will be added.
func startDiscovery(ctx context.Context, mgr *cluster.Manager) {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	serviceName := "emqx-go-headless"
	portName := "grpc"

	disc, err := discovery.NewKubeDiscovery(namespace, serviceName, portName)
	if err != nil {
		log.Printf("Could not initialize Kubernetes discovery: %v", err)
		if strings.Contains(err.Error(), "in-cluster configuration") {
			log.Println("This is not a Kubernetes environment. Skipping peer discovery.")
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