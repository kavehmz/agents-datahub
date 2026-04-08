package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/regentmarkets/agents-datahub/common/logging"
	"github.com/regentmarkets/agents-datahub/exposer/internal/config"
)

// HubClient represents a client for communicating with the hub
type HubClient struct {
	config     *config.Config
	httpClient *http.Client
	logger     *logging.Logger
	authToken  string
}

// NewHubClient creates a new hub client
func NewHubClient(cfg *config.Config) (*HubClient, error) {
	logger := logging.NewLogger("exposer-client", logging.INFO)
	authToken := config.GetExposerToken(cfg.Exposer.Name)
	if authToken == "" {
		return nil, fmt.Errorf("exposer token not configured: set TOKEN_EXPOSER_%s environment variable", cfg.Exposer.Name)
	}

	return &HubClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Hub.Timeout,
		},
		logger:    logger,
		authToken: authToken,
	}, nil
}

// QueryRequest represents a query request to the hub
type QueryRequest struct {
	ExposerName string                 `json:"exposer_name"`
	AuthToken   string                 `json:"auth_token"`
	Label       string                 `json:"label"`
	Operation   string                 `json:"operation"`
	Parameters  map[string]interface{} `json:"parameters"`
	Metadata    map[string]string      `json:"metadata"`
}

// QueryResponse represents a query response from the hub
type QueryResponse struct {
	QueryID      string                 `json:"query_id"`
	Success      bool                   `json:"success"`
	Data         map[string]interface{} `json:"data"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Trace        QueryTrace             `json:"trace"`
}

// QueryTrace represents execution trace information
type QueryTrace struct {
	SourceName        string `json:"source_name"`
	HubProcessingMs   int64  `json:"hub_processing_ms"`
	SourceExecutionMs int64  `json:"source_execution_ms"`
	TotalTimeMs       int64  `json:"total_time_ms"`
}

// ExecuteQuery executes a query through the hub
func (c *HubClient) ExecuteQuery(
	ctx context.Context,
	label string,
	operation string,
	parameters map[string]interface{},
	metadata map[string]string,
) (*QueryResponse, error) {

	// Create request
	req := QueryRequest{
		ExposerName: c.config.Exposer.Name,
		AuthToken:   c.authToken,
		Label:       label,
		Operation:   operation,
		Parameters:  parameters,
		Metadata:    metadata,
	}

	// Marshal request
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	scheme := "http"
	if c.config.Hub.TLS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/query", scheme, c.config.Hub.Endpoint)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	startTime := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	duration := time.Since(startTime).Milliseconds()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("query_failed", "Query request failed", map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(body),
			"duration":    duration,
		})
		return nil, fmt.Errorf("hub returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var queryResp QueryResponse
	if err := json.Unmarshal(body, &queryResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debug("query_executed", "Query executed successfully", map[string]interface{}{
		"query_id": queryResp.QueryID,
		"duration": duration,
	})

	return &queryResp, nil
}

// HealthCheck checks the hub's health
func (c *HubClient) HealthCheck(ctx context.Context) error {
	scheme := "http"
	if c.config.Hub.TLS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s/health", scheme, c.config.Hub.Endpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub returned status %d", resp.StatusCode)
	}

	return nil
}
