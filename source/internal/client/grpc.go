package client

import (
	"context"
	"fmt"
	"time"

	"github.com/regentmarkets/agents-datahub/common/logging"
	pb "github.com/regentmarkets/agents-datahub/proto"
	"github.com/regentmarkets/agents-datahub/source/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

// GRPCClient handles gRPC communication with the hub
type GRPCClient struct {
	config     *config.Config
	opsHandler OperationsHandler
	logger     *logging.Logger
	authToken  string
	startTime  time.Time

	conn   *grpc.ClientConn
	client pb.DataHubClient
	stream pb.DataHub_ConnectSourceClient
	ctx    context.Context
	cancel context.CancelFunc
}

// NewGRPCClient creates a new gRPC client
func NewGRPCClient(cfg *config.Config, opsHandler OperationsHandler, logger *logging.Logger, authToken string) *GRPCClient {
	ctx, cancel := context.WithCancel(context.Background())

	return &GRPCClient{
		config:     cfg,
		opsHandler: opsHandler,
		logger:     logger,
		authToken:  authToken,
		startTime:  time.Now(),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Connect establishes gRPC connection to the hub
func (g *GRPCClient) Connect() error {
	// Create gRPC connection with optional TLS
	var transportCreds grpc.DialOption
	if g.config.Hub.TLS.Enabled {
		if g.config.Hub.TLS.CAFile != "" {
			creds, err := credentials.NewClientTLSFromFile(g.config.Hub.TLS.CAFile, "")
			if err != nil {
				return fmt.Errorf("failed to load TLS CA: %w", err)
			}
			transportCreds = grpc.WithTransportCredentials(creds)
		} else {
			transportCreds = grpc.WithTransportCredentials(credentials.NewTLS(nil))
		}
	} else {
		transportCreds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}
	conn, err := grpc.NewClient(
		g.config.Hub.Endpoint,
		transportCreds,
	)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	g.conn = conn
	g.client = pb.NewDataHubClient(conn)

	// Create bidirectional stream
	stream, err := g.client.ConnectSource(g.ctx)
	if err != nil {
		g.conn.Close()
		return fmt.Errorf("failed to create stream: %w", err)
	}

	g.stream = stream

	// Send registration
	if err := g.sendRegistration(); err != nil {
		g.conn.Close()
		return fmt.Errorf("failed to send registration: %w", err)
	}

	// Wait for registration ack
	msg, err := stream.Recv()
	if err != nil {
		g.conn.Close()
		return fmt.Errorf("failed to receive ack: %w", err)
	}

	ack := msg.GetRegistrationAck()
	if ack == nil || !ack.Success {
		g.conn.Close()
		errorMsg := "registration failed"
		if ack != nil {
			errorMsg = ack.Message
		}
		return fmt.Errorf("%s", errorMsg)
	}

	g.logger.Info("registered_with_hub", "Successfully registered with hub", map[string]interface{}{
		"assigned_id": ack.AssignedId,
		"message":     ack.Message,
	})

	return nil
}

// sendRegistration sends the source registration message
func (g *GRPCClient) sendRegistration() error {
	// Get operations from the handler
	allOps := g.opsHandler.GetOperations()

	pbOperations := make([]*pb.Operation, len(allOps))

	for i, op := range allOps {
		inputSchema, _ := structpb.NewStruct(op["inputSchema"].(map[string]interface{}))
		outputSchema, _ := structpb.NewStruct(op["outputSchema"].(map[string]interface{}))

		pbOperations[i] = &pb.Operation{
			Name:           op["name"].(string),
			Description:    op["description"].(string),
			InputSchema:    inputSchema,
			OutputSchema:   outputSchema,
			TimeoutSeconds: int32(op["timeout"].(int)),
		}
	}

	reg := &pb.SourceMessage{
		Message: &pb.SourceMessage_Registration{
			Registration: &pb.SourceRegistration{
				Name:       g.config.Source.Name,
				Label:      g.config.Source.Label,
				Version:    g.config.Source.Version,
				AuthToken:  g.authToken,
				Operations: pbOperations,
				Capabilities: &pb.SourceCapabilities{
					MaxConcurrentQueries: 100,
					SupportsTransactions: true,
					SupportsBatchQueries: false,
				},
			},
		},
	}

	return g.stream.Send(reg)
}

// ProcessMessages processes incoming messages from the hub
func (g *GRPCClient) ProcessMessages() error {
	for {
		msg, err := g.stream.Recv()
		if err != nil {
			return fmt.Errorf("stream receive error: %w", err)
		}

		switch m := msg.Message.(type) {
		case *pb.HubMessage_QueryExecution:
			g.handleQueryExecution(m.QueryExecution)
		case *pb.HubMessage_HealthCheck:
			g.handleHealthCheck(m.HealthCheck)
		case *pb.HubMessage_RegistrationAck:
			// Already handled during connection
		}
	}
}

// handleQueryExecution handles a query execution request
func (g *GRPCClient) handleQueryExecution(query *pb.QueryExecution) {
	startTime := time.Now()

	g.logger.Debug("query_received", "Received query from hub", map[string]interface{}{
		"query_id":  query.QueryId,
		"operation": query.Operation,
	})

	// Convert parameters
	params := query.Parameters.AsMap()

	// Execute operation
	ctx, cancel := context.WithTimeout(g.ctx, 30*time.Second)
	defer cancel()

	result, err := g.opsHandler.ExecuteOperation(ctx, query.Operation, params)

	executionTime := time.Since(startTime).Milliseconds()

	// Create result message
	var resultMsg *pb.SourceMessage
	if err != nil {
		g.logger.Error("query_failed", "Query execution failed", err, map[string]interface{}{
			"query_id":  query.QueryId,
			"operation": query.Operation,
		})

		// Track query failure metrics (these would be sent by parent Client)

		resultMsg = &pb.SourceMessage{
			Message: &pb.SourceMessage_QueryResult{
				QueryResult: &pb.QueryResult{
					QueryId:         query.QueryId,
					Success:         false,
					ErrorMessage:    err.Error(),
					ExecutionTimeMs: executionTime,
				},
			},
		}
	} else {
		g.logger.Info("query_executed", "Query executed successfully", map[string]interface{}{
			"query_id":  query.QueryId,
			"operation": query.Operation,
			"duration":  executionTime,
		})

		// Track query success metrics (these would be sent by parent Client)

		dataStruct, _ := structpb.NewStruct(result)

		resultMsg = &pb.SourceMessage{
			Message: &pb.SourceMessage_QueryResult{
				QueryResult: &pb.QueryResult{
					QueryId:         query.QueryId,
					Success:         true,
					Data:            dataStruct,
					ExecutionTimeMs: executionTime,
				},
			},
		}
	}

	// Send result
	if err := g.stream.Send(resultMsg); err != nil {
		g.logger.Error("send_result_failed", "Failed to send query result", err, map[string]interface{}{
			"query_id": query.QueryId,
		})
	}
}

// handleHealthCheck handles a health check request
func (g *GRPCClient) handleHealthCheck(healthCheck *pb.HealthCheckRequest) {
	ctx, cancel := context.WithTimeout(g.ctx, 5*time.Second)
	defer cancel()

	err := g.opsHandler.HealthCheck(ctx)
	healthy := err == nil

	statusMsg := "healthy"
	if !healthy {
		statusMsg = err.Error()
	}

	msg := &pb.SourceMessage{
		Message: &pb.SourceMessage_HealthStatus{
			HealthStatus: &pb.HealthStatus{
				Healthy:       healthy,
				StatusMessage: statusMsg,
				Metrics: &pb.SourceMetrics{
					ActiveConnections: 1,
					QueriesProcessed:  0,
					AvgResponseTimeMs: 0,
					UptimeSeconds:     int64(time.Since(g.startTime).Seconds()),
				},
			},
		},
	}

	if err := g.stream.Send(msg); err != nil {
		g.logger.Error("send_health_failed", "Failed to send health status", err, nil)
	}
}

// Close closes the gRPC connection
func (g *GRPCClient) Close() error {
	g.cancel()
	if g.conn != nil {
		return g.conn.Close()
	}
	return nil
}
