# REST API as Data Source

This guide explains how to use external REST APIs as data sources in DataHub.

## Overview

DataHub sources can now connect to **multiple backend types** in a single source instance:
- **PostgreSQL** - Traditional database queries
- **REST API** - External HTTP/HTTPS services
- **Future**: MongoDB, Redis, gRPC services, etc.

## Key Design Principle

The source configuration remains clean and **backend-agnostic**:

```yaml
operations:
  - name: getUserById       # → PostgreSQL
  - name: getUsersByFilter  # → PostgreSQL  
  - name: getUserPayments   # → REST API
```

The **implementation** decides which backend to use, not the configuration!

## How It Works

### 1. Operations Handler Pattern

The [`source/internal/operations/operations.go`](source/internal/operations/operations.go) provides a unified interface:

```go
func (h *Handler) ExecuteOperation(ctx context.Context, operation string, params map[string]interface{}) (map[string]interface{}, error) {
    switch operation {
    case "getUserById":
        return h.pgClient.GetUserByID(ctx, params)  // PostgreSQL
    case "getUserPayments":
        return h.restClient.GetUserPayments(ctx, params)  // REST API
    default:
        return nil, fmt.Errorf("unknown operation: %s", operation)
    }
}
```

### 2. Configuration

**Source Configuration** ([`config/source-hybrid.yaml`](config/source-hybrid.yaml)):

```yaml
source:
  name: hybrid-source-01
  label: payment-data
  version: 1.0.0

database:
  type: postgresql
  max_connections: 20
  query_timeout: 10s

restapi:
  enabled: true
  base_url: http://payments-service:8080
  timeout: 10s

operations:
  - name: getUserById
    timeout: 5s
  - name: getUserPayments
    timeout: 10s
```

**Environment Variables**:

```bash
# PostgreSQL connection
DATABASE_URL=postgresql://user:pass@host:5432/db

# REST API authentication (optional)
RESTAPI_AUTH_TOKEN=your-api-token

# Hub connection
HUB_ENDPOINT=hub:50051
SOURCE_AUTH_TOKEN=source-token
```

### 3. REST API Client

The REST API client ([`source/internal/restapi/restapi.go`](source/internal/restapi/restapi.go)) handles:
- HTTP request/response
- Authentication (Bearer token)
- Timeout handling
- Error handling
- Health checks

## Example: Payment Service

### External Payment API

Your existing payment service:

```bash
POST https://payments:8080/api/payment/history
Content-Type: application/json
Authorization: Bearer token

{
  "user": "123456789"
}

Response:
{
  "payments": [
    {
      "id": "111",
      "remark": "Monthly subscription",
      "status": "completed"
    }
  ]
}
```

### DataHub Operation

Add to [`source/internal/restapi/restapi.go`](source/internal/restapi/restapi.go:31):

```go
func (c *Client) GetUserPayments(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
    userID := params["userId"].(string)
    
    // Call external API
    reqBody := map[string]interface{}{"user": userID}
    resp := c.post("/api/payment/history", reqBody)
    
    return resp, nil
}
```

### Client Call

```bash
curl -X POST http://localhost:3000/data/payment-data/getUserPayments \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-001"}'
```

**Response:**
```json
{
  "query_id": "q-xxx",
  "success": true,
  "data": {
    "payments": [
      {"id": "111", "remark": "Monthly subscription", "status": "completed"}
    ]
  },
  "trace": {
    "source_name": "hybrid-source-01",
    "source_execution_ms": 45
  }
}
```

## Benefits

### 1. Unified Access Pattern
```
All your data sources → Single API endpoint
```

### 2. Consistent Security
- Same token authentication
- Same permission control
- Same audit logging

### 3. Single Deployment
One source service can handle:
- Multiple PostgreSQL databases
- Multiple REST APIs
- Mixed workloads

### 4. Backend Transparency
Clients don't know (or care) whether data comes from:
- Database
- REST API
- Cache
- File system
- Another service

## Adding New Backend Types

### Step 1: Create Backend Client

Create [`source/internal/{backend}/client.go`](source/internal):

```go
type Client struct {
    // backend-specific fields
}

func (c *Client) ExecuteOperation(ctx context.Context, operation string, params map[string]interface{}) (map[string]interface{}, error) {
    // implementation
}

func (c *Client) HealthCheck(ctx context.Context) error {
    // implementation
}

func GetOperations() []map[string]interface{} {
    // return operation definitions
}
```

### Step 2: Add to Operations Handler

Update [`source/internal/operations/operations.go`](source/internal/operations/operations.go):

```go
type Handler struct {
    pgClient    *postgres.Client
    restClient  *restapi.Client
    redisClient *redis.Client  // NEW
}

func (h *Handler) ExecuteOperation(...) {
    switch operation {
    case "getUserById":
        return h.pgClient.GetUserByID(...)
    case "getUserPayments":
        return h.restClient.GetUserPayments(...)
    case "getCachedProfile":
        return h.redisClient.Get(...)  // NEW
    }
}
```

### Step 3: Update Configuration

Add to [`source/internal/config/config.go`](source/internal/config/config.go):

```go
type Config struct {
    // existing fields...
    Redis RedisConfig `yaml:"redis"`
}
```

### Step 4: Initialize in main.go

Update [`source/main.go`](source/main.go):

```go
var redisClient *redis.Client
if cfg.Redis.Enabled {
    redisClient = redis.NewClient(cfg.Redis.Endpoint)
}

opsHandler := operations.NewHandler(dbClient, restClient, redisClient)
```

## Testing REST API Source

### Start Mock Payment Service

```bash
# Included in docker-compose.yaml
docker-compose up -d mock-payment
```

### Test Payment Query

```bash
chmod +x examples/test-rest-api-source.sh
./examples/test-rest-api-source.sh
```

### Manual Test

```bash
curl -X POST http://localhost:3000/data/payment-data/getUserPayments \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-001"}'
```

## Deployment Scenarios

### Scenario 1: PostgreSQL Only

```yaml
# config/source-postgres.yaml
database:
  type: postgresql
  
restapi:
  enabled: false  # Not using REST API
```

### Scenario 2: REST API Only

```yaml
# config/source-restapi.yaml
database:
  type: ""  # Not using database

restapi:
  enabled: true
  base_url: https://api.example.com
```

### Scenario 3: Hybrid (Both)

```yaml
# config/source-hybrid.yaml
database:
  type: postgresql
  
restapi:
  enabled: true
  base_url: https://api.example.com
```

## Security Considerations

### REST API Authentication

The REST API client supports Bearer token authentication:

```bash
# Set in environment
RESTAPI_AUTH_TOKEN=your-secure-token
```

The client automatically adds:
```http
Authorization: Bearer your-secure-token
```

### TLS/HTTPS

For production REST APIs, use HTTPS:

```yaml
restapi:
  base_url: https://payments.example.com
```

### Timeouts

Always set appropriate timeouts:

```yaml
operations:
  - name: getUserPayments
    timeout: 10s  # Prevent hanging
```

## Monitoring

REST API calls are logged just like database queries:

```json
{
  "event": "query_executed",
  "query": {
    "operation": "getUserPayments",
    "source": "hybrid-source-01",
    "status": "success",
    "duration": {
      "source_ms": 45
    }
  }
}
```

## Conclusion

This design allows you to:
- ✅ Mix different backend types in one source
- ✅ Keep configuration simple and declarative
- ✅ Hide implementation details from Hub and Exposers
- ✅ Add new backend types easily
- ✅ Maintain consistent security and monitoring

**One source service, unlimited data sources!**