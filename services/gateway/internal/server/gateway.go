package server

import (
	"context"
	"log/slog"

	commonv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/ingestion/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// GatewayServer implements the gRPC IngestionService.
type GatewayServer struct {
	ingestionv1.UnimplementedIngestionServiceServer
	commonv1.UnimplementedHealthServiceServer

	logger  *slog.Logger
	items   map[string]*ingestionv1.InboxItem
	version string
}

// NewGatewayServer creates a new GatewayServer.
func NewGatewayServer(logger *slog.Logger) *GatewayServer {
	return &GatewayServer{
		logger:  logger,
		items:   make(map[string]*ingestionv1.InboxItem),
		version: "0.1.0",
	}
}

// Check implements the HealthService Check RPC.
func (s *GatewayServer) Check(ctx context.Context, req *commonv1.HealthCheckRequest) (*commonv1.HealthCheckResponse, error) {
	return &commonv1.HealthCheckResponse{
		Status:    commonv1.HealthCheckResponse_SERVING,
		Version:   s.version,
		Timestamp: timestamppb.Now(),
	}, nil
}

// IngestItem implements the IngestionService IngestItem RPC.
func (s *GatewayServer) IngestItem(ctx context.Context, req *ingestionv1.IngestRequest) (*ingestionv1.IngestResponse, error) {
	item := req.GetItem()
	if item == nil {
		return &ingestionv1.IngestResponse{
			Accepted: false,
			Message:  "item is required",
		}, nil
	}

	s.items[item.Id] = item
	s.logger.Info("item ingested", "id", item.Id, "source", item.Source)

	return &ingestionv1.IngestResponse{
		ItemId:   item.Id,
		Accepted: true,
		Message:  "item accepted",
		Status:   commonv1.ProcessingStatus_PROCESSING_STATUS_NEW,
	}, nil
}

// StreamIngest implements the IngestionService StreamIngest RPC.
func (s *GatewayServer) StreamIngest(stream ingestionv1.IngestionService_StreamIngestServer) error {
	var totalReceived, totalAccepted, totalRejected int32
	var rejectedIDs []string

	for {
		req, err := stream.Recv()
		if err != nil {
			// Stream ended
			return stream.SendAndClose(&ingestionv1.IngestSummary{
				TotalReceived: totalReceived,
				TotalAccepted: totalAccepted,
				TotalRejected: totalRejected,
				RejectedIds:   rejectedIDs,
			})
		}

		totalReceived++
		item := req.GetItem()
		if item == nil || item.Content == "" {
			totalRejected++
			if item != nil {
				rejectedIDs = append(rejectedIDs, item.Id)
			}
			continue
		}

		s.items[item.Id] = item
		totalAccepted++
	}
}

// GetItemStatus implements the IngestionService GetItemStatus RPC.
func (s *GatewayServer) GetItemStatus(ctx context.Context, req *ingestionv1.ItemStatusRequest) (*ingestionv1.ItemStatusResponse, error) {
	item, exists := s.items[req.ItemId]
	if !exists {
		return &ingestionv1.ItemStatusResponse{
			ItemId: req.ItemId,
			Status: commonv1.ProcessingStatus_PROCESSING_STATUS_UNSPECIFIED,
		}, nil
	}

	return &ingestionv1.ItemStatusResponse{
		ItemId:      item.Id,
		Status:      commonv1.ProcessingStatus_PROCESSING_STATUS_NEW,
		LastUpdated: item.ReceivedAt,
	}, nil
}

// ListItems implements the IngestionService ListItems RPC.
func (s *GatewayServer) ListItems(ctx context.Context, req *ingestionv1.ListItemsRequest) (*ingestionv1.ListItemsResponse, error) {
	var result []*ingestionv1.InboxItem
	for _, item := range s.items {
		result = append(result, item)
		if len(result) >= int(req.PageSize) && req.PageSize > 0 {
			break
		}
	}

	return &ingestionv1.ListItemsResponse{
		Items:      result,
		TotalCount: int32(len(s.items)),
	}, nil
}

// AddItem adds an item directly (used by webhook handler).
func (s *GatewayServer) AddItem(item *ingestionv1.InboxItem) {
	s.items[item.Id] = item
}
