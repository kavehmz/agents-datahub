package operations

import (
	"context"
	"fmt"

	"github.com/regentmarkets/agents-datahub/source/internal/postgres"
	"github.com/regentmarkets/agents-datahub/source/internal/restapi"
)

// Handler handles all operations across different data sources
type Handler struct {
	pgClient    *postgres.Client
	restClient  *restapi.Client
	pgEnabled   bool
	restEnabled bool
}

// NewHandler creates a new operations handler
func NewHandler(pgClient *postgres.Client, restClient *restapi.Client) *Handler {
	return &Handler{
		pgClient:    pgClient,
		restClient:  restClient,
		pgEnabled:   pgClient != nil,
		restEnabled: restClient != nil,
	}
}

// ExecuteOperation executes an operation by name, routing to the appropriate backend
func (h *Handler) ExecuteOperation(ctx context.Context, operation string, params map[string]interface{}) (map[string]interface{}, error) {
	switch operation {
	// PostgreSQL operations
	case "getUserById":
		if h.pgClient == nil {
			return nil, fmt.Errorf("PostgreSQL client not configured")
		}
		return h.pgClient.GetUserByID(ctx, params)

	case "getUsersByFilter":
		if h.pgClient == nil {
			return nil, fmt.Errorf("PostgreSQL client not configured")
		}
		return h.pgClient.GetUsersByFilter(ctx, params)

	// REST API operations
	case "getUserPayments":
		if h.restClient == nil {
			return nil, fmt.Errorf("REST API client not configured")
		}
		return h.restClient.GetUserPayments(ctx, params)

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// HealthCheck checks all configured backends
func (h *Handler) HealthCheck(ctx context.Context) error {
	// Check PostgreSQL if configured
	if h.pgClient != nil {
		if err := h.pgClient.HealthCheck(ctx); err != nil {
			return fmt.Errorf("PostgreSQL health check failed: %w", err)
		}
	}

	// Check REST API if configured
	if h.restClient != nil {
		if err := h.restClient.HealthCheck(ctx); err != nil {
			return fmt.Errorf("REST API health check failed: %w", err)
		}
	}

	return nil
}

// GetOperations returns all supported operations from configured backends
func (h *Handler) GetOperations() []map[string]interface{} {
	var operations []map[string]interface{}

	// Add PostgreSQL operations if enabled
	if h.pgEnabled {
		operations = append(operations, postgres.GetOperations()...)
	}

	// Add REST API operations if enabled
	if h.restEnabled {
		operations = append(operations, restapi.GetOperations()...)
	}

	return operations
}
