package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/regentmarkets/agents-datahub/common/logging"
	"github.com/regentmarkets/agents-datahub/common/metrics"
	"github.com/regentmarkets/agents-datahub/source/internal/config"
)

// OperationsHandler defines the interface for executing operations
type OperationsHandler interface {
	ExecuteOperation(ctx context.Context, operation string, params map[string]interface{}) (map[string]interface{}, error)
	HealthCheck(ctx context.Context) error
	GetOperations() []map[string]interface{}
}

// Client represents a source client that connects to the hub
type Client struct {
	config        *config.Config
	opsHandler    OperationsHandler
	logger        *logging.Logger
	metrics       *metrics.Metrics
	datadogClient *metrics.DatadogClient
	authToken     string
	datadogStopCh chan struct{}

	grpcClient *GRPCClient
	connected  bool
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewClient creates a new source client
func NewClient(cfg *config.Config, opsHandler OperationsHandler) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())

	logger := logging.NewLogger("source", logging.INFO)
	metricsInst := metrics.NewMetrics()

	// Create Datadog client if endpoint is configured
	ddEndpoint := config.GetDatadogEndpoint()
	ddClient, err := metrics.NewDatadogClient(ddEndpoint, "source", map[string]string{
		"source": cfg.Source.Name,
		"label":  cfg.Source.Label,
	})
	if err != nil {
		logger.Warn("datadog_init_failed", "Failed to initialize Datadog client", map[string]interface{}{
			"error": err.Error(),
		})
	}

	authToken := config.GetSourceToken(cfg.Source.Name)
	if authToken == "" {
		cancel()
		return nil, fmt.Errorf("source token not configured: set SOURCE_AUTH_TOKEN environment variable")
	}

	return &Client{
		config:        cfg,
		opsHandler:    opsHandler,
		logger:        logger,
		metrics:       metricsInst,
		datadogClient: ddClient,
		authToken:     authToken,
		datadogStopCh: make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// Start connects to the hub and starts processing
func (c *Client) Start() error {
	c.logger.Info("source_starting", "Starting source client", map[string]interface{}{
		"name":  c.config.Source.Name,
		"label": c.config.Source.Label,
		"hub":   c.config.Hub.Endpoint,
	})

	// Start connection loop
	c.wg.Add(1)
	go c.connectionLoop()

	return nil
}

// Stop stops the client
func (c *Client) Stop() error {
	c.logger.Info("source_stopping", "Stopping source client", nil)

	// Stop Datadog sender
	close(c.datadogStopCh)

	// Close Datadog connection
	if c.datadogClient != nil {
		c.datadogClient.Close()
	}

	// Close active gRPC connection to unblock processMessages
	c.mu.RLock()
	if c.grpcClient != nil {
		c.grpcClient.Close()
	}
	c.mu.RUnlock()

	c.cancel()
	c.wg.Wait()
	c.logger.Info("source_stopped", "Source client stopped", nil)
	return nil
}

// connectionLoop maintains connection to hub with reconnection logic
func (c *Client) connectionLoop() {
	defer c.wg.Done()

	attempts := 0
	maxAttempts := c.config.Hub.MaxReconnectAttempts

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if attempts >= maxAttempts && maxAttempts > 0 {
			c.logger.Error("max_reconnect_attempts", "Max reconnection attempts reached", nil, nil)
			return
		}

		// Attempt connection
		err := c.connect()
		if err != nil {
			attempts++
			c.logger.Warn("connection_failed", "Failed to connect to hub", map[string]interface{}{
				"error":   err.Error(),
				"attempt": attempts,
				"max":     maxAttempts,
			})

			// Wait before reconnecting
			select {
			case <-time.After(c.config.Hub.ReconnectInterval):
			case <-c.ctx.Done():
				return
			}
			continue
		}

		// Reset attempts on successful connection
		attempts = 0
		c.mu.Lock()
		c.connected = true
		c.mu.Unlock()

		// Process messages (blocks until disconnection)
		c.processMessages()

		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()

		c.logger.Warn("disconnected_from_hub", "Disconnected from hub", nil)
	}
}

// connect establishes connection to the hub
func (c *Client) connect() error {
	grpcClient := NewGRPCClient(c.config, c.opsHandler, c.logger, c.authToken)

	if err := grpcClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.mu.Lock()
	c.grpcClient = grpcClient
	c.mu.Unlock()

	c.logger.Info("connected_to_hub", "Successfully connected to hub via gRPC", nil)
	return nil
}

// processMessages blocks while processing messages from the hub.
// Returns when the stream disconnects or errors.
func (c *Client) processMessages() {
	c.mu.RLock()
	grpcClient := c.grpcClient
	c.mu.RUnlock()

	if grpcClient == nil {
		return
	}

	if err := grpcClient.ProcessMessages(); err != nil {
		c.logger.Warn("message_processing_stopped", "Message processing stopped", map[string]interface{}{
			"error": err.Error(),
		})
	}

	grpcClient.Close()

	c.mu.Lock()
	c.grpcClient = nil
	c.mu.Unlock()
}

// handleQueryExecution handles a query execution request from the hub
func (c *Client) handleQueryExecution(queryID, operation string, parameters map[string]interface{}) {
	startTime := time.Now()

	c.logger.Debug("query_received", "Received query from hub", map[string]interface{}{
		"query_id":  queryID,
		"operation": operation,
	})

	// Create query context with timeout
	timeout := 10 * time.Second
	for _, op := range c.config.Operations {
		if op.Name == operation {
			timeout = op.Timeout
			break
		}
	}

	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()

	// Execute operation
	c.metrics.QueriesTotal.Inc()
	result, err := c.opsHandler.ExecuteOperation(ctx, operation, parameters)

	executionTime := time.Since(startTime).Milliseconds()
	c.metrics.QueryDuration.Observe(float64(executionTime))

	if err != nil {
		c.metrics.QueriesFailed.Inc()
		c.logger.Error("query_failed", "Query execution failed", err, map[string]interface{}{
			"query_id":  queryID,
			"operation": operation,
			"duration":  executionTime,
		})

		// TODO: Send error response to hub via gRPC
		return
	}

	c.metrics.QueriesSuccess.Inc()
	c.logger.Info("query_executed", "Query executed successfully", map[string]interface{}{
		"query_id":  queryID,
		"operation": operation,
		"duration":  executionTime,
	})

	// TODO: Send success response to hub via gRPC with result
	_ = result
}

// handleHealthCheck handles a health check request from the hub
func (c *Client) handleHealthCheck() {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	err := c.opsHandler.HealthCheck(ctx)
	healthy := err == nil

	if healthy {
		c.logger.Debug("health_check", "Health check passed", nil)
	} else {
		c.logger.Warn("health_check", "Health check failed", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// TODO: Send health status to hub via gRPC
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// GetMetrics returns current metrics
func (c *Client) GetMetrics() *metrics.Metrics {
	return c.metrics
}
