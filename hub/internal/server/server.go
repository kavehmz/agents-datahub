package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/regentmarkets/agents-datahub/common/logging"
	"github.com/regentmarkets/agents-datahub/common/metrics"
	"github.com/regentmarkets/agents-datahub/hub/internal/auth"
	"github.com/regentmarkets/agents-datahub/hub/internal/config"
	"github.com/regentmarkets/agents-datahub/hub/internal/health"
	"github.com/regentmarkets/agents-datahub/hub/internal/router"
	pb "github.com/regentmarkets/agents-datahub/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Server represents the hub server
type Server struct {
	config        *config.Config
	authorizer    *auth.Authorizer
	router        *router.Router
	healthMon     *health.Monitor
	logger        *logging.Logger
	queryLogger   *logging.QueryLogger
	connLogger    *logging.ConnectionLogger
	metrics       *metrics.Metrics
	datadogClient *metrics.DatadogClient
	grpcSvc       *GRPCServer
	grpcServer    *grpc.Server
	datadogStopCh chan struct{}

	httpServer    *http.Server
	metricsServer *http.Server

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer creates a new hub server
func NewServer(cfg *config.Config) (*Server, error) {
	// Create logger
	logger := logging.NewLogger("hub", logging.Level(cfg.Logging.Level))
	queryLogger := logging.NewQueryLogger(logger)
	connLogger := logging.NewConnectionLogger(logger)

	// Create authorizer
	authorizer, err := auth.NewAuthorizer(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create authorizer: %w", err)
	}

	// Create router
	routerInst := router.NewRouter()

	// Create health monitor
	healthMon := health.NewMonitor(
		routerInst,
		logger,
		cfg.Sources.HealthCheckInterval,
		cfg.Sources.UnhealthyThreshold,
		cfg.Sources.RecoveryThreshold,
	)

	// Create metrics
	metricsInst := metrics.NewMetrics()

	// Create Datadog client if endpoint is configured
	ddEndpoint := config.GetDatadogEndpoint()
	ddClient, err := metrics.NewDatadogClient(ddEndpoint, "hub", map[string]string{
		"env": "production",
	})
	if err != nil {
		logger.Warn("datadog_init_failed", "Failed to initialize Datadog client", map[string]interface{}{
			"error": err.Error(),
		})
	} else if ddClient != nil && ddEndpoint != "" {
		logger.Info("datadog_enabled", "Datadog metrics enabled", map[string]interface{}{
			"endpoint": ddEndpoint,
		})
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		config:        cfg,
		authorizer:    authorizer,
		router:        routerInst,
		healthMon:     healthMon,
		logger:        logger,
		queryLogger:   queryLogger,
		connLogger:    connLogger,
		metrics:       metricsInst,
		datadogClient: ddClient,
		datadogStopCh: make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}

	return s, nil
}

// Start starts all servers
func (s *Server) Start() error {
	s.logger.Info("hub_starting", "Starting hub server", map[string]interface{}{
		"grpc_port":    s.config.Server.GRPCPort,
		"http_port":    s.config.Server.HTTPPort,
		"metrics_port": s.config.Server.MetricsPort,
	})

	// Start health monitor
	s.healthMon.Start(s.ctx)

	// Start Datadog metrics sender if enabled
	if s.datadogClient != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.datadogClient.StartPeriodicSend(s.metrics, 10*time.Second, s.datadogStopCh)
		}()
		s.logger.Info("datadog_sender_started", "Datadog metrics sender started", map[string]interface{}{
			"interval": "10s",
		})
	}

	// Start gRPC server (for source connections)
	if err := s.startGRPCServer(); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	// Start HTTP server (for exposer queries)
	if err := s.startHTTPServer(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start metrics server
	if err := s.startMetricsServer(); err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}

	s.logger.Info("hub_started", "Hub server started successfully", nil)
	return nil
}

// Stop stops all servers
func (s *Server) Stop() error {
	s.logger.Info("hub_stopping", "Stopping hub server", nil)

	// Stop Datadog sender
	close(s.datadogStopCh)

	// Close Datadog connection
	if s.datadogClient != nil {
		s.datadogClient.Close()
	}

	// Stop health monitor
	s.healthMon.Stop()

	// Cancel context
	s.cancel()

	// Stop HTTP servers
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}

	if s.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.metricsServer.Shutdown(ctx)
	}

	// Gracefully stop gRPC server (drains active connections)
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	// Wait for goroutines
	s.wg.Wait()

	s.logger.Info("hub_stopped", "Hub server stopped", nil)
	return nil
}

// ReloadTokens reloads authentication tokens from environment variables
func (s *Server) ReloadTokens() error {
	return s.authorizer.ReloadTokens()
}

// startGRPCServer starts the gRPC server for source connections
func (s *Server) startGRPCServer() error {
	addr := fmt.Sprintf(":%d", s.config.Server.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Create gRPC server with optional TLS
	var grpcOpts []grpc.ServerOption
	if s.config.Server.TLS.Enabled {
		creds, err := credentials.NewServerTLSFromFile(
			s.config.Server.TLS.CertFile,
			s.config.Server.TLS.KeyFile,
		)
		if err != nil {
			return fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
	}
	s.grpcServer = grpc.NewServer(grpcOpts...)

	// Create and register our gRPC service
	s.grpcSvc = NewGRPCServer(s)
	pb.RegisterDataHubServer(s.grpcServer, s.grpcSvc)

	// Wire health monitor to use the gRPC server for health checks
	s.healthMon.SetChecker(s.grpcSvc)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("grpc_server_listening", "gRPC server listening", map[string]interface{}{
			"address": addr,
		})

		if err := s.grpcServer.Serve(lis); err != nil {
			s.logger.Error("grpc_server_error", "gRPC server error", err, nil)
		}
	}()

	return nil
}

// startHTTPServer starts the HTTP/2 server for exposer queries
func (s *Server) startHTTPServer() error {
	mux := http.NewServeMux()

	// Query execution endpoint
	mux.HandleFunc("/query", s.handleQuery)

	// Health check endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Stats endpoint
	mux.HandleFunc("/stats", s.handleStats)

	addr := fmt.Sprintf(":%d", s.config.Server.HTTPPort)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("http_server_listening", "HTTP server listening", map[string]interface{}{
			"address": addr,
		})

		var err error
		if s.config.Server.TLS.Enabled {
			err = s.httpServer.ListenAndServeTLS(s.config.Server.TLS.CertFile, s.config.Server.TLS.KeyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("http_server_error", "HTTP server error", err, nil)
		}
	}()

	return nil
}

// startMetricsServer starts the Prometheus metrics server
func (s *Server) startMetricsServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", s.handleMetrics)

	addr := fmt.Sprintf(":%d", s.config.Server.MetricsPort)
	s.metricsServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("metrics_server_listening", "Metrics server listening", map[string]interface{}{
			"address": addr,
		})

		if err := s.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("metrics_server_error", "Metrics server error", err, nil)
		}
	}()

	return nil
}

// handleQuery handles query execution requests from exposers
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size to 1MB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	// Parse request
	var req struct {
		ExposerName string                 `json:"exposer_name"`
		AuthToken   string                 `json:"auth_token"`
		Label       string                 `json:"label"`
		Operation   string                 `json:"operation"`
		Parameters  map[string]interface{} `json:"parameters"`
		Metadata    map[string]string      `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Create proto request
	params, _ := ConvertMapToStruct(req.Parameters)
	metadata := &pb.QueryMetadata{
		QueryId:   nextQueryID(),
		ClientId:  req.Metadata["client_id"],
		Timestamp: nil,
		Headers:   req.Metadata,
	}

	protoReq := &pb.QueryRequest{
		ExposerName: req.ExposerName,
		AuthToken:   req.AuthToken,
		Label:       req.Label,
		Operation:   req.Operation,
		Parameters:  params,
		Metadata:    metadata,
	}

	// Execute via gRPC
	resp, err := s.grpcSvc.ExecuteQuery(r.Context(), protoReq)
	if err != nil {
		s.logger.Error("query_execution_error", "Query execution failed", err, nil)
		http.Error(w, "query execution failed", http.StatusServiceUnavailable)
		return
	}

	// Convert response
	response := map[string]interface{}{
		"query_id": resp.QueryId,
		"success":  resp.Success,
		"data":     ConvertStructToMap(resp.Data),
		"trace": map[string]interface{}{
			"source_name":         resp.Trace.SourceName,
			"hub_processing_ms":   resp.Trace.HubProcessingMs,
			"source_execution_ms": resp.Trace.SourceExecutionMs,
			"total_time_ms":       resp.Trace.TotalTimeMs,
		},
	}

	if !resp.Success {
		response["error"] = resp.ErrorMessage
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	stats := s.healthMon.GetHealthStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleStats handles stats requests
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := s.router.GetStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleMetrics handles Prometheus metrics requests
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	exporter := metrics.NewPrometheusExporter("hub", s.metrics, nil)
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(exporter.Export()))
}
