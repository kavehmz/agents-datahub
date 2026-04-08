package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents a REST API client for external services
type Client struct {
	httpClient *http.Client
	baseURL    string
	authToken  string
}

// NewClient creates a new REST API client
func NewClient(baseURL, authToken string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		baseURL:   baseURL,
		authToken: authToken,
	}
}

// GetUserPayments retrieves payment history for a user from external REST API
func (c *Client) GetUserPayments(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	userID, ok := params["userId"].(string)
	if !ok {
		return nil, fmt.Errorf("userId parameter is required")
	}

	// Prepare request to external payment service
	reqBody := map[string]interface{}{
		"user": userID,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build full URL
	url := fmt.Sprintf("%s/api/payment/history", c.baseURL)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response (limit to 10MB to prevent memory exhaustion)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("payment API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result, nil
}

// ExecuteOperation executes an operation by name
func (c *Client) ExecuteOperation(ctx context.Context, operation string, params map[string]interface{}) (map[string]interface{}, error) {
	switch operation {
	case "getUserPayments":
		return c.GetUserPayments(ctx, params)
	default:
		return nil, fmt.Errorf("unknown REST API operation: %s", operation)
	}
}

// HealthCheck checks the REST API health
func (c *Client) HealthCheck(ctx context.Context) error {
	// Try to reach the base URL health endpoint
	url := fmt.Sprintf("%s/health", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// GetOperations returns REST API operation definitions
func GetOperations() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "getUserPayments",
			"description": "Get user payment history from external payment service",
			"timeout":     10,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"userId": map[string]interface{}{
						"type": "string",
					},
					"startDate": map[string]interface{}{
						"type": "string",
					},
					"endDate": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"userId"},
			},
			"outputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"payments": map[string]interface{}{
						"type": "array",
					},
				},
			},
		},
	}
}
