package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Client represents a PostgreSQL client
type Client struct {
	db *sql.DB
}

// NewClient creates a new PostgreSQL client
func NewClient(databaseURL string, maxConnections int) (*Client, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(maxConnections)
	db.SetMaxIdleConns(maxConnections / 2)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Client{db: db}, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	return c.db.Close()
}

// GetUserByID retrieves a user by ID
func (c *Client) GetUserByID(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	userID, ok := params["userId"].(string)
	if !ok {
		return nil, fmt.Errorf("userId parameter is required")
	}

	includeMetadata := false
	if meta, ok := params["includeMetadata"].(bool); ok {
		includeMetadata = meta
	}

	query := `
		SELECT id, name, email, created_at, updated_at, status
		FROM users
		WHERE id = $1
	`

	var user struct {
		ID        string
		Name      string
		Email     string
		CreatedAt time.Time
		UpdatedAt time.Time
		Status    string
	}

	err := c.db.QueryRowContext(ctx, query, userID).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.Status,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	result := map[string]interface{}{
		"id":        user.ID,
		"name":      user.Name,
		"email":     user.Email,
		"createdAt": user.CreatedAt.Format(time.RFC3339),
		"status":    user.Status,
	}

	if includeMetadata {
		result["updatedAt"] = user.UpdatedAt.Format(time.RFC3339)
	}

	return result, nil
}

// GetUsersByFilter retrieves users by filter
func (c *Client) GetUsersByFilter(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	filter, ok := params["filter"].(map[string]interface{})
	if !ok {
		filter = make(map[string]interface{})
	}

	sort, ok := params["sort"].(map[string]interface{})
	if !ok {
		sort = map[string]interface{}{
			"field": "createdAt",
			"order": "desc",
		}
	}

	const maxLimit = 1000

	limit := 100
	if l, ok := filter["limit"].(float64); ok {
		limit = int(l)
	}
	if limit <= 0 {
		limit = 100
	} else if limit > maxLimit {
		limit = maxLimit
	}

	offset := 0
	if o, ok := filter["offset"].(float64); ok {
		offset = int(o)
	}
	if offset < 0 {
		offset = 0
	}

	// Build query
	query := `SELECT id, name, email, created_at, status FROM users WHERE 1=1`
	args := []interface{}{}
	argNum := 1

	// Add filters
	if status, ok := filter["status"].(string); ok {
		query += fmt.Sprintf(" AND status = $%d", argNum)
		args = append(args, status)
		argNum++
	}

	if createdAfter, ok := filter["createdAfter"].(string); ok {
		query += fmt.Sprintf(" AND created_at > $%d", argNum)
		args = append(args, createdAfter)
		argNum++
	}

	// Add sorting (validated against allowlist to prevent SQL injection)
	allowedSortFields := map[string]string{
		"id":        "id",
		"name":      "name",
		"email":     "email",
		"createdAt": "created_at",
		"status":    "status",
	}
	allowedSortOrders := map[string]string{
		"asc":  "ASC",
		"desc": "DESC",
	}

	sortFieldRaw, _ := sort["field"].(string)
	sortOrderRaw, _ := sort["order"].(string)

	sortColumn, ok := allowedSortFields[sortFieldRaw]
	if !ok {
		sortColumn = "created_at"
	}
	sortDir, ok := allowedSortOrders[sortOrderRaw]
	if !ok {
		sortDir = "DESC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortColumn, sortDir)

	// Add pagination
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argNum, argNum+1)
	args = append(args, limit, offset)

	// Execute query
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	// Collect results
	users := []map[string]interface{}{}
	for rows.Next() {
		var user struct {
			ID        string
			Name      string
			Email     string
			CreatedAt time.Time
			Status    string
		}

		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt, &user.Status); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		users = append(users, map[string]interface{}{
			"id":        user.ID,
			"name":      user.Name,
			"email":     user.Email,
			"createdAt": user.CreatedAt.Format(time.RFC3339),
			"status":    user.Status,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration failed: %w", err)
	}

	// Get total count (using same filters as the main query, excluding pagination)
	countQuery := `SELECT COUNT(*) FROM users WHERE 1=1`
	countArgs := []interface{}{}
	countArgNum := 1

	if status, ok := filter["status"].(string); ok {
		countQuery += fmt.Sprintf(" AND status = $%d", countArgNum)
		countArgs = append(countArgs, status)
		countArgNum++
	}

	if createdAfter, ok := filter["createdAfter"].(string); ok {
		countQuery += fmt.Sprintf(" AND created_at > $%d", countArgNum)
		countArgs = append(countArgs, createdAfter)
		countArgNum++
	}

	var totalCount int
	err = c.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("count query failed: %w", err)
	}

	hasMore := offset+len(users) < totalCount

	return map[string]interface{}{
		"users":      users,
		"totalCount": totalCount,
		"hasMore":    hasMore,
	}, nil
}

// ExecuteOperation executes an operation by name
func (c *Client) ExecuteOperation(ctx context.Context, operation string, params map[string]interface{}) (map[string]interface{}, error) {
	switch operation {
	case "getUserById":
		return c.GetUserByID(ctx, params)
	case "getUsersByFilter":
		return c.GetUsersByFilter(ctx, params)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

// HealthCheck checks database health
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// GetOperations returns supported operations with their schemas
func GetOperations() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "getUserById",
			"description": "Get user by ID",
			"timeout":     5,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"userId": map[string]interface{}{
						"type": "string",
					},
					"includeMetadata": map[string]interface{}{
						"type": "boolean",
					},
				},
				"required": []string{"userId"},
			},
			"outputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id":        map[string]string{"type": "string"},
					"name":      map[string]string{"type": "string"},
					"email":     map[string]string{"type": "string"},
					"createdAt": map[string]string{"type": "string"},
					"status":    map[string]string{"type": "string"},
				},
			},
		},
		{
			"name":        "getUsersByFilter",
			"description": "Get users by filter",
			"timeout":     30,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filter": map[string]interface{}{
						"type": "object",
					},
					"sort": map[string]interface{}{
						"type": "object",
					},
				},
			},
			"outputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"users": map[string]interface{}{
						"type": "array",
					},
					"totalCount": map[string]string{"type": "integer"},
					"hasMore":    map[string]string{"type": "boolean"},
				},
			},
		},
	}
}

// MarshalParams converts parameters to JSON-compatible format
func MarshalParams(params map[string]interface{}) ([]byte, error) {
	return json.Marshal(params)
}

// UnmarshalParams converts JSON to parameters
func UnmarshalParams(data []byte) (map[string]interface{}, error) {
	var params map[string]interface{}
	err := json.Unmarshal(data, &params)
	return params, err
}
