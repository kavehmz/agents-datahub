package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/regentmarkets/agents-datahub/common/logging"
	"github.com/regentmarkets/agents-datahub/common/metrics"
	"github.com/regentmarkets/agents-datahub/common/token"
	"github.com/regentmarkets/agents-datahub/exposer/internal/client"
	"github.com/regentmarkets/agents-datahub/exposer/internal/config"
)

var queryCounter uint64

func nextQueryID() string {
	return fmt.Sprintf("q-%d-%d", time.Now().Unix(), atomic.AddUint64(&queryCounter, 1))
}

// contextKey is a private type for context keys to avoid collisions.
type contextKey struct{ name string }

var clientIDKey = contextKey{"client_id"}

// Server represents the exposer server
type Server struct {
	config        *config.Config
	hubClient     *client.HubClient
	logger        *logging.Logger
	queryLogger   *logging.QueryLogger
	metrics       *metrics.Metrics
	datadogClient *metrics.DatadogClient
	tokenMgr      *token.Manager
	httpServer    *http.Server
	datadogStopCh chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer creates a new exposer server
func NewServer(cfg *config.Config) (*Server, error) {
	// Create logger
	logger := logging.NewLogger("exposer", logging.INFO)
	queryLogger := logging.NewQueryLogger(logger)

	// Create hub client
	hubClient, err := client.NewHubClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create hub client: %w", err)
	}

	// Create metrics
	metricsInst := metrics.NewMetrics()

	// Create Datadog client if endpoint is configured
	ddEndpoint := config.GetDatadogEndpoint()
	ddClient, err := metrics.NewDatadogClient(ddEndpoint, "exposer", map[string]string{
		"exposer": cfg.Exposer.Name,
	})
	if err != nil {
		logger.Warn("datadog_init_failed", "Failed to initialize Datadog client", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Create token manager for client authentication
	tokenMgr := token.NewManager()
	if err := tokenMgr.LoadFromEnv("TOKEN_CLIENT_"); err != nil {
		return nil, fmt.Errorf("failed to load client tokens: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		config:        cfg,
		hubClient:     hubClient,
		logger:        logger,
		queryLogger:   queryLogger,
		metrics:       metricsInst,
		datadogClient: ddClient,
		tokenMgr:      tokenMgr,
		datadogStopCh: make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// Start starts the server
func (s *Server) Start() error {
	s.logger.Info("exposer_starting", "Starting exposer server", map[string]interface{}{
		"name": s.config.Exposer.Name,
		"port": s.config.API.Port,
		"hub":  s.config.Hub.Endpoint,
	})

	// Start Datadog metrics sender if enabled
	if s.datadogClient != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.datadogClient.StartPeriodicSend(s.metrics, 10*time.Second, s.datadogStopCh)
		}()
		s.logger.Info("datadog_sender_started", "Datadog metrics sender started", nil)
	}

	// Create HTTP server
	mux := http.NewServeMux()

	// Data endpoints (pattern: /data/{label}/{operation})
	mux.HandleFunc("/data/", s.handleDataRequest)

	// Health endpoint
	mux.HandleFunc("/health", s.handleHealth)

	// Metrics endpoint
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Apply middleware
	handler := s.corsMiddleware(s.authMiddleware(mux))

	addr := fmt.Sprintf(":%d", s.config.API.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Info("api_server_listening", "API server listening", map[string]interface{}{
			"address": addr,
		})

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("api_server_error", "API server error", err, nil)
		}
	}()

	return nil
}

// Stop stops the server
func (s *Server) Stop() error {
	s.logger.Info("exposer_stopping", "Stopping exposer server", nil)

	// Stop Datadog sender
	close(s.datadogStopCh)

	// Close Datadog connection
	if s.datadogClient != nil {
		s.datadogClient.Close()
	}

	s.cancel()

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.httpServer.Shutdown(ctx)
	}

	s.wg.Wait()

	s.logger.Info("exposer_stopped", "Exposer server stopped", nil)
	return nil
}

// ReloadTokens reloads client authentication tokens from environment variables
func (s *Server) ReloadTokens() error {
	return s.tokenMgr.Reload("TOKEN_CLIENT_")
}

// authMiddleware authenticates client requests
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health and metrics endpoints
		if r.URL.Path == "/health" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// Get authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.sendError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing authorization header")
			return
		}

		// Parse bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.sendError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid authorization header")
			return
		}

		token := parts[1]

		// Validate token against any registered client
		authenticated := false
		clientID := ""
		for _, name := range s.tokenMgr.GetNames() {
			if s.tokenMgr.Validate(name, token) {
				authenticated = true
				clientID = name
				break
			}
		}

		if !authenticated {
			s.metrics.ClientRequestsFailed.Inc()
			s.sendError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token")
			return
		}

		// Store client ID in context
		ctx := context.WithValue(r.Context(), clientIDKey, clientID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// corsMiddleware handles CORS
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	// Build allowed origins set once
	allowedOrigins := make(map[string]bool)
	for _, o := range s.config.API.CORS.Origins {
		allowedOrigins[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.config.API.CORS.Enabled {
			requestOrigin := r.Header.Get("Origin")
			var allowOrigin string

			if allowedOrigins["*"] {
				allowOrigin = "*"
			} else if allowedOrigins[requestOrigin] {
				allowOrigin = requestOrigin
				w.Header().Set("Vary", "Origin")
			}

			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			}

			// Handle preflight
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// handleDataRequest handles data query requests
func (s *Server) handleDataRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.sendError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST is allowed")
		return
	}

	startTime := time.Now()
	s.metrics.ClientRequests.Inc()

	// Track request start
	if s.datadogClient != nil {
		s.datadogClient.Count("requests.total", 1)
	}

	// Extract client ID from context
	clientID := ""
	if id, ok := r.Context().Value(clientIDKey).(string); ok {
		clientID = id
	}

	// Parse URL path: /data/{label}/{operation}
	path := strings.TrimPrefix(r.URL.Path, "/data/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		if s.datadogClient != nil {
			s.datadogClient.Count("errors.invalid_path", 1)
		}
		s.sendError(w, http.StatusBadRequest, "INVALID_PATH", "Path must be /data/{label}/{operation}")
		return
	}

	label := parts[0]
	operation := parts[1]

	// Limit request body size to 1MB
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	// Parse request body
	var params map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		if s.datadogClient != nil {
			s.datadogClient.Count("errors.invalid_parameters", 1)
		}
		s.sendError(w, http.StatusBadRequest, "INVALID_PARAMETERS", "Failed to parse request body")
		return
	}

	// Create metadata
	metadata := map[string]string{
		"client_id": clientID,
	}

	// Execute query through hub
	queryID := nextQueryID()
	resp, err := s.hubClient.ExecuteQuery(r.Context(), label, operation, params, metadata)

	executionTime := time.Since(startTime).Milliseconds()
	s.metrics.QueryDuration.Observe(float64(executionTime))

	if err != nil {
		s.metrics.QueriesFailed.Inc()

		// Track query failures by operation
		if s.datadogClient != nil {
			s.datadogClient.Count(fmt.Sprintf("queries.failed.%s", metrics.SanitizeMetricName(operation)), 1)
			s.datadogClient.Timing(fmt.Sprintf("query.duration.%s", metrics.SanitizeMetricName(operation)), time.Duration(executionTime)*time.Millisecond)
		}

		s.queryLogger.LogQuery(
			queryID,
			s.config.Exposer.Name,
			clientID,
			label,
			operation,
			"",
			"error",
			0,
			executionTime,
			executionTime,
			err,
		)

		s.logger.Error("query_failed", "Query execution failed", err, map[string]interface{}{
			"label":     label,
			"operation": operation,
		})
		s.sendError(w, http.StatusServiceUnavailable, "QUERY_FAILED", "query execution failed")
		return
	}

	s.metrics.QueriesSuccess.Inc()

	// Track successful queries by operation and label
	if s.datadogClient != nil {
		s.datadogClient.Count(fmt.Sprintf("queries.success.%s", metrics.SanitizeMetricName(operation)), 1)
		s.datadogClient.Count(fmt.Sprintf("queries.by_label.%s", metrics.SanitizeMetricName(label)), 1)
		s.datadogClient.Timing(fmt.Sprintf("query.duration.%s", metrics.SanitizeMetricName(operation)), time.Duration(executionTime)*time.Millisecond)
	}
	s.queryLogger.LogQuery(
		resp.QueryID,
		s.config.Exposer.Name,
		clientID,
		label,
		operation,
		resp.Trace.SourceName,
		"success",
		resp.Trace.HubProcessingMs,
		resp.Trace.SourceExecutionMs,
		resp.Trace.TotalTimeMs,
		nil,
	)

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check hub connectivity
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := s.hubClient.HealthCheck(ctx)

	status := map[string]interface{}{
		"status": "healthy",
		"hub":    "connected",
	}

	if err != nil {
		s.logger.Warn("health_check_failed", "Hub health check failed", map[string]interface{}{
			"error": err.Error(),
		})
		status["status"] = "unhealthy"
		status["hub"] = "disconnected"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleMetrics handles metrics requests
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	exporter := metrics.NewPrometheusExporter("exposer", s.metrics, map[string]string{
		"exposer": s.config.Exposer.Name,
	})

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(exporter.Export()))
}

// sendError sends an error response
func (s *Server) sendError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	response := map[string]interface{}{
		"success": false,
		"error": map[string]interface{}{
			"code":    errorCode,
			"message": message,
		},
		"trace": map[string]interface{}{
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}
