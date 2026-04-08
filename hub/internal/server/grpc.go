package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/regentmarkets/agents-datahub/common/metrics"
	pb "github.com/regentmarkets/agents-datahub/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// queryCounter is a process-wide atomic counter for generating unique query IDs.
var queryCounter uint64

// nextQueryID returns a unique query ID using an atomic counter and process start time.
func nextQueryID() string {
	return fmt.Sprintf("q-%d-%d", time.Now().Unix(), atomic.AddUint64(&queryCounter, 1))
}

// GRPCServer implements the DataHub gRPC service
type GRPCServer struct {
	pb.UnimplementedDataHubServer
	server         *Server
	activeStreams  map[string]pb.DataHub_ConnectSourceServer
	streamsMu      sync.RWMutex
	pendingQueries map[string]chan *pb.QueryResult
	queriesMu      sync.RWMutex
}

// NewGRPCServer creates a new gRPC server
func NewGRPCServer(server *Server) *GRPCServer {
	return &GRPCServer{
		server:         server,
		activeStreams:  make(map[string]pb.DataHub_ConnectSourceServer),
		pendingQueries: make(map[string]chan *pb.QueryResult),
	}
}

// ConnectSource handles bidirectional streaming from sources
func (g *GRPCServer) ConnectSource(stream pb.DataHub_ConnectSourceServer) error {
	// Get peer info
	peerInfo, _ := peer.FromContext(stream.Context())
	remoteAddr := "unknown"
	if peerInfo != nil {
		remoteAddr = peerInfo.Addr.String()
	}

	var sourceName string
	var sourceLabel string
	var operations []string

	// Wait for registration
	msg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "failed to receive registration: %v", err)
	}

	reg := msg.GetRegistration()
	if reg == nil {
		return status.Errorf(codes.InvalidArgument, "first message must be registration")
	}

	// Authenticate source
	if !g.server.authorizer.AuthenticateSource(reg.Name, reg.AuthToken) {
		g.server.logger.Warn("source_auth_failed", "Source authentication failed", map[string]interface{}{
			"name":        reg.Name,
			"remote_addr": remoteAddr,
		})
		return status.Errorf(codes.Unauthenticated, "invalid authentication token")
	}

	sourceName = reg.Name
	sourceLabel = reg.Label

	// Extract operation names
	for _, op := range reg.Operations {
		operations = append(operations, op.Name)
	}

	// Register source in router
	g.streamsMu.Lock()
	g.activeStreams[sourceName] = stream
	g.streamsMu.Unlock()

	err = g.server.router.RegisterSource(sourceName, sourceLabel, operations, stream)
	if err != nil {
		g.streamsMu.Lock()
		delete(g.activeStreams, sourceName)
		g.streamsMu.Unlock()
		return status.Errorf(codes.Internal, "failed to register source: %v", err)
	}

	// Update metrics
	g.server.metrics.SourcesConnected.Inc()
	g.server.metrics.SourcesHealthy.Inc()

	// Send Datadog metrics
	if g.server.datadogClient != nil {
		g.server.datadogClient.Count("sources.connected", 1)
		g.server.datadogClient.Gauge("sources.total", float64(g.server.metrics.SourcesConnected.Get()))
	}

	// Send registration acknowledgment
	ack := &pb.HubMessage{
		Message: &pb.HubMessage_RegistrationAck{
			RegistrationAck: &pb.RegistrationAck{
				Success:    true,
				Message:    "Registration successful",
				AssignedId: sourceName,
			},
		},
	}

	if err := stream.Send(ack); err != nil {
		g.cleanup(sourceName)
		return status.Errorf(codes.Internal, "failed to send ack: %v", err)
	}

	// Log connection
	g.server.connLogger.LogSourceConnected(sourceName, sourceLabel, operations, remoteAddr)

	// Handle messages from source
	for {
		msg, err := stream.Recv()
		if err != nil {
			g.server.logger.Info("source_disconnected", "Source disconnected", map[string]interface{}{
				"name":  sourceName,
				"error": err.Error(),
			})
			g.cleanup(sourceName)
			return err
		}

		// Handle different message types
		switch m := msg.Message.(type) {
		case *pb.SourceMessage_QueryResult:
			g.handleQueryResult(sourceName, m.QueryResult)
		case *pb.SourceMessage_HealthStatus:
			g.handleHealthStatus(sourceName, m.HealthStatus)
		case *pb.SourceMessage_Error:
			g.handleSourceError(sourceName, m.Error)
		}
	}
}

// ExecuteQuery handles query execution requests from exposers
func (g *GRPCServer) ExecuteQuery(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	startTime := time.Now()

	// Authenticate exposer
	if !g.server.authorizer.AuthenticateExposer(req.ExposerName, req.AuthToken) {
		if g.server.datadogClient != nil {
			g.server.datadogClient.Count("exposer.auth.failed", 1)
		}
		return nil, status.Errorf(codes.Unauthenticated, "invalid exposer token")
	}

	// Track successful exposer auth
	if g.server.datadogClient != nil {
		g.server.datadogClient.Count("exposer.auth.success", 1)
	}

	// Authorize operation
	if !g.server.authorizer.AuthorizeOperation(req.ExposerName, req.Label, req.Operation) {
		if g.server.datadogClient != nil {
			g.server.datadogClient.Count("exposer.authorization.denied", 1)
		}
		return nil, status.Errorf(codes.PermissionDenied, "operation not permitted")
	}

	// Select source
	source, err := g.server.router.SelectSource(req.Label, req.Operation)
	if err != nil {
		if g.server.datadogClient != nil {
			g.server.datadogClient.Count("routing.no_sources", 1)
		}
		return nil, status.Errorf(codes.Unavailable, "no sources available for label: %s", req.Label)
	}

	// Track routing success
	if g.server.datadogClient != nil {
		g.server.datadogClient.Count(fmt.Sprintf("routing.to_source.%s", metrics.SanitizeMetricName(source.Name)), 1)
		g.server.datadogClient.Count(fmt.Sprintf("routing.by_label.%s", metrics.SanitizeMetricName(req.Label)), 1)
	}

	// Get source stream
	g.streamsMu.RLock()
	stream, exists := g.activeStreams[source.Name]
	g.streamsMu.RUnlock()

	if !exists {
		return nil, status.Errorf(codes.Unavailable, "source stream not available")
	}

	// Generate query ID
	queryID := req.Metadata.QueryId
	if queryID == "" {
		queryID = nextQueryID()
	}

	// Create response channel
	respCh := make(chan *pb.QueryResult, 1)
	g.queriesMu.Lock()
	g.pendingQueries[queryID] = respCh
	g.queriesMu.Unlock()

	defer func() {
		g.queriesMu.Lock()
		delete(g.pendingQueries, queryID)
		g.queriesMu.Unlock()
		close(respCh)
	}()

	// Send query to source
	queryMsg := &pb.HubMessage{
		Message: &pb.HubMessage_QueryExecution{
			QueryExecution: &pb.QueryExecution{
				QueryId:    queryID,
				Operation:  req.Operation,
				Parameters: req.Parameters,
				Metadata:   req.Metadata,
			},
		},
	}

	if err := stream.Send(queryMsg); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to send query to source: %v", err)
	}

	// Wait for result with timeout
	select {
	case result := <-respCh:
		executionTime := time.Since(startTime).Milliseconds()

		// Track metrics
		g.server.metrics.QueriesTotal.Inc()
		g.server.metrics.QueryDuration.Observe(float64(result.ExecutionTimeMs))

		// Log query
		clientID := req.Metadata.ClientId
		if result.Success {
			g.server.metrics.QueriesSuccess.Inc()

			// Send detailed Datadog metrics
			if g.server.datadogClient != nil {
				g.server.datadogClient.Count(fmt.Sprintf("queries.success.%s", metrics.SanitizeMetricName(req.Operation)), 1)
				g.server.datadogClient.Timing(fmt.Sprintf("query.duration.%s", metrics.SanitizeMetricName(req.Operation)),
					time.Duration(result.ExecutionTimeMs)*time.Millisecond)
				g.server.datadogClient.Timing("query.total_duration",
					time.Duration(executionTime)*time.Millisecond)
			}

			g.server.queryLogger.LogQuery(
				queryID,
				req.ExposerName,
				clientID,
				req.Label,
				req.Operation,
				source.Name,
				"success",
				0,
				result.ExecutionTimeMs,
				executionTime,
				nil,
			)
		} else {
			g.server.metrics.QueriesFailed.Inc()

			// Track failures by operation
			if g.server.datadogClient != nil {
				g.server.datadogClient.Count(fmt.Sprintf("queries.failed.%s", metrics.SanitizeMetricName(req.Operation)), 1)
				g.server.datadogClient.Count("errors.query_execution", 1)
			}

			g.server.queryLogger.LogQuery(
				queryID,
				req.ExposerName,
				clientID,
				req.Label,
				req.Operation,
				source.Name,
				"error",
				0,
				result.ExecutionTimeMs,
				executionTime,
				fmt.Errorf("%s", result.ErrorMessage),
			)
		}

		// Return response
		return &pb.QueryResponse{
			QueryId:      queryID,
			Success:      result.Success,
			Data:         result.Data,
			ErrorMessage: result.ErrorMessage,
			Trace: &pb.QueryTrace{
				SourceName:        source.Name,
				HubProcessingMs:   0,
				SourceExecutionMs: result.ExecutionTimeMs,
				TotalTimeMs:       executionTime,
			},
		}, nil

	case <-time.After(30 * time.Second):
		if g.server.datadogClient != nil {
			g.server.datadogClient.Count("errors.query_timeout", 1)
		}
		return nil, status.Errorf(codes.DeadlineExceeded, "query timeout")
	case <-ctx.Done():
		if g.server.datadogClient != nil {
			g.server.datadogClient.Count("errors.query_canceled", 1)
		}
		return nil, status.Errorf(codes.Canceled, "query canceled")
	}
}

// handleQueryResult processes query results from sources
func (g *GRPCServer) handleQueryResult(sourceName string, result *pb.QueryResult) {
	g.queriesMu.RLock()
	ch, exists := g.pendingQueries[result.QueryId]
	g.queriesMu.RUnlock()

	if exists {
		select {
		case ch <- result:
		default:
			g.server.logger.Warn("query_result_dropped", "Query result channel full", map[string]interface{}{
				"query_id": result.QueryId,
				"source":   sourceName,
			})
		}
	}
}

// handleHealthStatus processes health status from sources
func (g *GRPCServer) handleHealthStatus(sourceName string, healthStatus *pb.HealthStatus) {
	if healthStatus.Healthy {
		g.server.router.MarkHealthy(sourceName)
		if g.server.datadogClient != nil {
			g.server.datadogClient.Count(fmt.Sprintf("health.%s.healthy", metrics.SanitizeMetricName(sourceName)), 1)
		}
	} else {
		g.server.router.MarkUnhealthy(sourceName)
		if g.server.datadogClient != nil {
			g.server.datadogClient.Count(fmt.Sprintf("health.%s.unhealthy", metrics.SanitizeMetricName(sourceName)), 1)
		}
	}

	g.server.logger.Debug("health_status_received", "Health status received from source", map[string]interface{}{
		"source":  sourceName,
		"healthy": healthStatus.Healthy,
		"message": healthStatus.StatusMessage,
	})
}

// handleSourceError processes errors from sources
func (g *GRPCServer) handleSourceError(sourceName string, sourceError *pb.SourceError) {
	g.server.logger.Error("source_error", "Source reported error", fmt.Errorf("%s", sourceError.ErrorMessage), map[string]interface{}{
		"source":     sourceName,
		"error_code": sourceError.ErrorCode,
	})

	// Track source errors
	if g.server.datadogClient != nil {
		g.server.datadogClient.Count(fmt.Sprintf("source.errors.%s", metrics.SanitizeMetricName(sourceName)), 1)
		g.server.datadogClient.Count("source.errors.total", 1)
	}

	g.server.router.MarkUnhealthy(sourceName)
}

// cleanup removes source from active connections
func (g *GRPCServer) cleanup(sourceName string) {
	g.streamsMu.Lock()
	delete(g.activeStreams, sourceName)
	g.streamsMu.Unlock()

	// Check if source was healthy before unregistering, to avoid counter drift
	source, exists := g.server.router.GetSource(sourceName)
	wasHealthy := exists && source.Healthy

	g.server.router.UnregisterSource(sourceName)
	g.server.metrics.SourcesConnected.Dec()
	if wasHealthy {
		g.server.metrics.SourcesHealthy.Dec()
	}

	// Track disconnection
	if g.server.datadogClient != nil {
		g.server.datadogClient.Count("sources.disconnected", 1)
		g.server.datadogClient.Gauge("sources.total", float64(g.server.metrics.SourcesConnected.Get()))
	}

	g.server.connLogger.LogSourceDisconnected(sourceName, "", "stream closed")
}

// SendHealthCheck sends health check to a source
func (g *GRPCServer) SendHealthCheck(sourceName string) error {
	g.streamsMu.RLock()
	stream, exists := g.activeStreams[sourceName]
	g.streamsMu.RUnlock()

	if !exists {
		return fmt.Errorf("source stream not found")
	}

	msg := &pb.HubMessage{
		Message: &pb.HubMessage_HealthCheck{
			HealthCheck: &pb.HealthCheckRequest{
				Timestamp: timestamppb.Now(),
			},
		},
	}

	return stream.Send(msg)
}

// ConvertMapToStruct converts a map to protobuf Struct
func ConvertMapToStruct(m map[string]interface{}) (*structpb.Struct, error) {
	return structpb.NewStruct(m)
}

// ConvertStructToMap converts a protobuf Struct to map
func ConvertStructToMap(s *structpb.Struct) map[string]interface{} {
	if s == nil {
		return nil
	}
	return s.AsMap()
}
