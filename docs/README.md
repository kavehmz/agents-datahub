# DataHub - Three-Tier Data Access Architecture

A complete implementation of a secure, scalable data access layer with Hub, Source, and Exposer services.

## Architecture Overview

```
Client Applications
       ↓ (REST + Bearer Token)
Exposer Services (API Gateway)
       ↓ (HTTP/2)
Hub Service (Central Orchestrator)
       ↓ (gRPC Bidirectional Stream)
Source Services (Data Access)
       ↓ (Direct Connection)
Data Sources (PostgreSQL, MongoDB, etc.)
```

## Features

- ✅ **Zero Trust Security**: Every connection requires authentication
- ✅ **Fine-grained Permissions**: Control exactly what each exposer can access
- ✅ **High Availability**: Automatic round-robin load balancing across sources
- ✅ **Complete Audit Trail**: Full query logging from client to source
- ✅ **Health Monitoring**: Automatic health checks and failover
- ✅ **Token Rotation**: Zero-downtime token rotation with SIGHUP
- ✅ **Prometheus Metrics**: Built-in metrics endpoints
- ✅ **Structured Logging**: JSON-formatted logs for easy parsing

## Quick Start

### Prerequisites

- Docker and Docker Compose (required)
- Go 1.21+ (optional, only for local development)

> **Note:** The system is designed to run entirely with Docker. No local Go installation needed!

### Quick Setup

See **[`SETUP.md`](SETUP.md)** for detailed setup instructions.

**TL;DR:**

```bash
# 1. Generate proto code (Docker-based, no local setup needed)
chmod +x generate-proto-docker.sh
./generate-proto-docker.sh

# 2. Start all services
docker-compose up -d

# 3. Test the system
chmod +x examples/test-client.sh
./examples/test-client.sh
```

### Running with Docker Compose (Detailed)

1. **Start all services:**

```bash
docker-compose up -d
```

2. **Check service health:**

```bash
# Check exposer health
curl http://localhost:3000/health

# Check hub health
curl http://localhost:8080/health
```

3. **Run test client:**

```bash
chmod +x examples/test-client.sh
./examples/test-client.sh
```

4. **View logs:**

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f hub
docker-compose logs -f source-postgres
docker-compose logs -f exposer
```

5. **Stop services:**

```bash
docker-compose down
```

## Manual Build and Run

### 1. Install Dependencies

```bash
# Install protobuf compiler
# macOS
brew install protobuf

# Linux
apt-get install -y protobuf-compiler

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 2. Generate Protocol Buffers

```bash
./generate-proto.sh
```

### 3. Build Services

```bash
# Build all services
go build -o bin/hub ./hub
go build -o bin/source ./source
go build -o bin/exposer ./exposer
```

### 4. Set Environment Variables

```bash
# Copy example environment file
cp config/.env.example .env

# Edit with your values
vi .env

# Source the environment
source .env
```

### 5. Run Services

**Terminal 1 - Hub:**
```bash
./bin/hub -config config/hub-config.yaml
```

**Terminal 2 - Source:**
```bash
export DATABASE_URL="postgresql://user:pass@localhost:5432/db"
./bin/source -config config/source-postgres.yaml
```

**Terminal 3 - Exposer:**
```bash
./bin/exposer -config config/exposer-config.yaml
```

## API Usage

### Authentication

All client requests require a Bearer token:

```bash
curl -H "Authorization: Bearer YOUR_TOKEN" \
  http://localhost:3000/data/user-data/getUserById
```

### Get User by ID

```bash
curl -X POST http://localhost:3000/data/user-data/getUserById \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user-001",
    "includeMetadata": false
  }'
```

Response:
```json
{
  "query_id": "q-1234567890",
  "success": true,
  "data": {
    "id": "user-001",
    "name": "John Doe",
    "email": "john@example.com",
    "createdAt": "2024-01-15T10:00:00Z",
    "status": "active"
  },
  "trace": {
    "source_name": "postgres-prod-01",
    "hub_processing_ms": 2,
    "source_execution_ms": 43,
    "total_time_ms": 45
  }
}
```

### Get Users by Filter

```bash
curl -X POST http://localhost:3000/data/user-data/getUsersByFilter \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -d '{
    "filter": {
      "status": "active",
      "limit": 10,
      "offset": 0
    },
    "sort": {
      "field": "createdAt",
      "order": "desc"
    }
  }'
```

## Configuration

### Hub Configuration

Located in `config/hub-config.yaml`:

```yaml
server:
  grpc_port: 50051
  http_port: 8080
  metrics_port: 9090

sources:
  health_check_interval: 30s
  unhealthy_threshold: 3

exposers:
  - name: api-west-01
    permissions:
      - label: user-data
        operations: ["*"]
```

### Environment Variables

Tokens are managed through environment variables:

```bash
# Hub validates these
TOKEN_SOURCE_postgres-prod-01=abc123xyz
TOKEN_EXPOSER_api-west-01=west123abc

# Exposer validates these
TOKEN_CLIENT_web-app=webtoken123
```

### Token Rotation

Zero-downtime token rotation:

1. **Add new token:**
```bash
TOKEN_EXPOSER_api-west-01=[old-token,new-token]
```

2. **Reload configuration:**
```bash
kill -HUP <pid>
```

3. **Remove old token after transition:**
```bash
TOKEN_EXPOSER_api-west-01=new-token
kill -HUP <pid>
```

## Monitoring

### Prometheus Metrics

Available at:
- Hub: `http://localhost:9090/metrics`
- Exposer: `http://localhost:3000/metrics`

Key metrics:
```
hub_queries_total
hub_query_duration_seconds
hub_sources_connected
hub_sources_healthy
exposer_client_requests
exposer_queries_success
```

### Health Endpoints

- Hub: `http://localhost:8080/health`
- Exposer: `http://localhost:3000/health`

## Development

### Project Structure

```
datahub/
├── proto/              # Protocol Buffer definitions
├── hub/                # Hub service
│   ├── internal/
│   │   ├── server/     # gRPC and HTTP servers
│   │   ├── router/     # Round-robin routing
│   │   ├── auth/       # Authentication
│   │   └── health/     # Health monitoring
│   └── main.go
├── source/             # Source service
│   ├── internal/
│   │   ├── client/     # Hub client
│   │   ├── postgres/   # PostgreSQL operations
│   │   └── mongodb/    # MongoDB operations
│   └── main.go
├── exposer/            # Exposer service
│   ├── internal/
│   │   ├── server/     # REST API server
│   │   └── client/     # Hub client
│   └── main.go
├── common/             # Shared utilities
│   ├── token/          # Token management
│   ├── logging/        # Structured logging
│   └── metrics/        # Prometheus metrics
├── config/             # Configuration files
├── docker/             # Dockerfiles
└── examples/           # Example code and tests
```

### Adding a New Operation

1. **Implement in Source** (`source/internal/postgres/postgres.go`):
```go
func (c *Client) GetUserProfile(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
    // Implementation
}
```

2. **Add to GetOperations()**:
```go
{
    "name": "getUserProfile",
    "description": "Get user profile with details",
    "timeout": 5,
    // schemas...
}
```

3. **Update Hub permissions** (`config/hub-config.yaml`):
```yaml
- name: api-west-01
  permissions:
    - label: user-data
      operations: ["getUserById", "getUserProfile"]
```

## Security

- All tokens stored in environment variables only
- No tokens in configuration files or code
- TLS recommended for production
- Fine-grained permission control
- Complete audit logging

## Troubleshooting

### Services won't start

Check logs:
```bash
docker-compose logs hub
docker-compose logs source-postgres
```

### Database connection issues

Verify DATABASE_URL:
```bash
psql "$DATABASE_URL" -c "SELECT 1;"
```

### Token authentication failing

Check environment variables are set:
```bash
env | grep TOKEN_
```

## Performance

Expected latencies (p99):
- Simple queries: < 100ms
- Complex queries: < 500ms
- Health checks: < 10ms

Throughput targets:
- Hub: 10,000 queries/second
- Source: 1,000 queries/second  
- Exposer: 5,000 requests/second

## License

MIT License - See LICENSE file for details

## Contributing

Contributions welcome! Please read CONTRIBUTING.md first.

## Support

For issues and questions:
- GitHub Issues: https://github.com/datahub/datahub/issues
- Documentation: https://docs.datahub.io