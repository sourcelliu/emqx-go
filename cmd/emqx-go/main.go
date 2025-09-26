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

	"google.golang.org/grpc"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/cluster"
	"github.com/turtacn/emqx-go/pkg/discovery"
	clusterpb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

const (
	grpcPort = ":8081"
)

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