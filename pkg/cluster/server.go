package cluster

import (
	"context"
	"log"

	pb "github.com/turtacn/emqx-go/pkg/proto/cluster"
)

// Server implements the gRPC ClusterService server.
type Server struct {
	pb.UnimplementedClusterServiceServer
	NodeID string
}

// NewServer creates a new cluster server.
func NewServer(nodeID string) *Server {
	return &Server{NodeID: nodeID}
}

func (s *Server) Join(ctx context.Context, req *pb.JoinRequest) (*pb.JoinResponse, error) {
	log.Printf("Received Join request from node %s at %s", req.Node.NodeId, req.Node.Address)
	return &pb.JoinResponse{
		Success: true,
		Message: "Welcome to the cluster!",
		// In a real implementation, we would return the actual list of nodes.
		ClusterNodes: []*pb.NodeInfo{{NodeId: s.NodeID}},
	}, nil
}

func (s *Server) Leave(ctx context.Context, req *pb.LeaveRequest) (*pb.LeaveResponse, error) {
	log.Printf("Received Leave request from node %s", req.NodeId)
	return &pb.LeaveResponse{Success: true}, nil
}

func (s *Server) SyncNodeStatus(ctx context.Context, req *pb.SyncNodeStatusRequest) (*pb.SyncNodeStatusResponse, error) {
	log.Printf("Received SyncNodeStatus request from node %s", req.Node.NodeId)
	return &pb.SyncNodeStatusResponse{Success: true}, nil
}

func (s *Server) SyncSubscriptions(ctx context.Context, req *pb.SyncSubscriptionsRequest) (*pb.SyncSubscriptionsResponse, error) {
	log.Printf("Received SyncSubscriptions request from node %s with %d subscriptions", req.NodeId, len(req.Subscriptions))
	return &pb.SyncSubscriptionsResponse{Success: true, ReceivedCount: int32(len(req.Subscriptions))}, nil
}

func (s *Server) ForwardPublish(ctx context.Context, req *pb.PublishForward) (*pb.ForwardAck, error) {
	log.Printf("Received ForwardPublish request for topic %s from node %s", req.Topic, req.FromNode)
	return &pb.ForwardAck{MessageId: req.MessageId, Success: true}, nil
}

func (s *Server) BatchUpdateRoutes(ctx context.Context, req *pb.BatchUpdateRoutesRequest) (*pb.BatchUpdateRoutesResponse, error) {
	log.Printf("Received BatchUpdateRoutes request from node %s", req.FromNode)
	return &pb.BatchUpdateRoutesResponse{Success: true, UpdatedCount: int32(len(req.Routes))}, nil
}

func (s *Server) SyncConfig(ctx context.Context, req *pb.SyncConfigRequest) (*pb.SyncConfigResponse, error) {
	log.Printf("Received SyncConfig request from node %s", req.NodeId)
	return &pb.SyncConfigResponse{Success: true}, nil
}

func (s *Server) GetClusterStats(ctx context.Context, req *pb.ClusterStatsRequest) (*pb.ClusterStatsResponse, error) {
	log.Printf("Received GetClusterStats request from node %s", req.NodeId)
	return &pb.ClusterStatsResponse{Success: true}, nil
}