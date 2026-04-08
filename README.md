# DataHub

> A secure, scalable three-tier data access architecture for controlled access to multiple data sources

[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](https://golang.org)
[![Protocol Buffers](https://img.shields.io/badge/Protocol_Buffers-3-green.svg)](https://protobuf.dev)
[![gRPC](https://img.shields.io/badge/gRPC-1.76-brightgreen.svg)](https://grpc.io)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

## Overview

DataHub provides a centralized, secure gateway for accessing multiple data sources through a unified API. Built with Go, gRPC, and Protocol Buffers, it offers fine-grained permission control, automatic load balancing, and complete audit trails.

**Perfect for:**
- Exposing sensitive databases through controlled APIs
- Consolidating access to multiple data sources
- Providing AI agents with secure, limited data access
- Building a data mesh architecture
- Implementing zero-trust data access patterns

## Architecture

```
┌─────────────────┐
│  Client Apps    │  REST API + Bearer Token
└────────┬────────┘
         ↓
┌─────────────────┐
│   Exposer       │  API Gateway Layer
│  (REST API)     │  - Client authentication
└────────┬────────┘  - Request forwarding
         ↓ HTTP/2
┌─────────────────┐
│      Hub        │  Central Orchestrator
│ (gRPC + HTTP/2) │  - Permission enforcement
└────────┬────────┘  - Round-robin routing
         ↑ gRPC (Source connects TO Hub)
┌─────────────────┐
│     Source      │  Data Access Layer
│  (Multi-backend)│  - PostgreSQL
└────────┬────────┘  - REST APIs
         ↓           - Future: MongoDB, Redis, etc.
┌─────────────────┐
│  Data Sources   │  Actual Databases/APIs
└─────────────────┘
```

> **Note:** Source initiates gRPC connection to Hub. Hub uses the bidirectional stream to send queries.

## Key Features

### 🔐 Security First
- **Zero Trust**: Multi-layer authentication at every boundary
- **Unique Tokens**: Each source and exposer has isolated credentials
- **Fine-Grained Permissions**: Control exactly what each exposer can access
- **Token Rotation**: Zero-downtime token updates with SIGHUP
- **Audit Trail**: Complete query logging from client to database

### 🚀 High Performance
- **gRPC Bidirectional Streaming**: Persistent source connections
- **Round-Robin Load Balancing**: Automatic query distribution
- **HTTP/2**: Efficient exposer-to-hub communication
- **Connection Pooling**: Optimized database connections
- **Sub-100ms Latency**: Fast query execution (p99 < 100ms)

### 📊 Enterprise Observability
- **Prometheus Metrics**: Pull-based metrics endpoints
- **Datadog Integration**: Push-based metrics (DogStatsD)
- **Structured Logging**: JSON logs with complete context
- **Query Tracing**: End-to-end execution tracking
- **Health Checks**: Automatic failover support

### 🔧 Flexible Architecture
- **Multi-Backend Sources**: PostgreSQL + REST API + more
- **Single Deployment**: One source handles multiple backend types
- **Backend Transparency**: Clients don't know data source types
- **Easy Extension**: Add new backends without config changes

## Quick Start

### Prerequisites
- Docker and Docker Compose
- Go 1.25+ (optional, for local development)

### Run the System

```bash
# 1. Generate Protocol Buffer code
./generate-proto-docker.sh

# 2. Start all services
docker compose up -d

# 3. Test the system
curl -X POST http://localhost:3000/data/user-data/getUserById \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-001"}'
```

**Expected Response:**
```json
{
  "success": true,
  "data": {
    "id": "user-001",
    "name": "John Doe",
    "email": "john@example.com"
  },
  "trace": {
    "source_name": "hybrid-source-01",
    "total_time_ms": 12
  }
}
```

## Example Use Cases

### Use Case 1: Secure Database Access
```yaml
# Expose PostgreSQL through DataHub
operations:
  - name: getUserById       # SELECT * FROM users WHERE id = ?
  - name: getUsersByFilter  # SELECT with filters
```

### Use Case 2: REST API Gateway
```yaml
# Proxy external REST APIs with auth and monitoring
operations:
  - name: getUserPayments   # → POST https://payments:8080/api/history
```

### Use Case 3: Hybrid Data Access
```yaml
# Mix databases and APIs in ONE source
operations:
  - name: getUserData      # → PostgreSQL
  - name: getUserPayments  # → REST API
  - name: getUserAnalytics # → MongoDB (easily added!)
```

## Configuration Example

**Source Configuration** - Clean and declarative:

```yaml
source:
  name: my-source
  label: user-data

database:
  type: postgresql
  max_connections: 20

restapi:
  enabled: true
  base_url: https://api.example.com
  timeout: 10s

operations:  # Backend type is transparent!
  - name: getUserById
  - name: getUserPayments
```

**Implementation** decides routing:

```go
switch operation {
case "getUserById":
    return postgresClient.Query(...)
case "getUserPayments":
    return restAPIClient.Call(...)
}
```

## Documentation

- **[Quick Start](docs/QUICKSTART.md)** - Get running in 3 minutes
- **[Setup Guide](docs/SETUP.md)** - Detailed installation
- **[Testing Guide](docs/TESTING.md)** - How to test the system
- **[REST API Sources](docs/REST_API_SOURCE.md)** - Adding REST API backends
- **[Datadog Integration](docs/DATADOG_INTEGRATION.md)** - Metrics monitoring
- **[Metrics Guide](docs/METRICS_GUIDE.md)** - All tracked metrics
- **[Architecture Spec](spec.md)** - Original specification

## Project Structure

```
datahub/
├── hub/                    # Central orchestrator
│   └── internal/
│       ├── auth/          # Authentication & authorization
│       ├── router/        # Round-robin load balancing
│       ├── health/        # Health monitoring
│       └── server/        # gRPC & HTTP/2 servers
├── source/                # Data access layer
│   └── internal/
│       ├── postgres/      # PostgreSQL client
│       ├── restapi/       # REST API client
│       └── operations/    # Unified operation handler
├── exposer/               # API gateway
│   └── internal/
│       ├── server/        # REST API server
│       └── client/        # Hub HTTP client
├── common/                # Shared utilities
│   ├── token/            # Token management
│   ├── logging/          # Structured logging
│   └── metrics/          # Prometheus + Datadog
├── proto/                 # Protocol Buffer definitions
├── config/                # Configuration files
├── docker/                # Dockerfiles
└── examples/              # Test clients & mock services
```

## Key Technologies

- **Go 1.25** - Modern, concurrent, performant
- **Protocol Buffers** - Efficient serialization
- **gRPC** - High-performance RPC framework
- **PostgreSQL** - Reliable database backend
- **Docker** - Containerized deployment
- **DogStatsD** - Datadog metrics protocol

## Deployment

### Docker Compose (Recommended)
```bash
docker compose up -d
```

### Kubernetes
Helm charts and manifests available in `deploy/` (see deployment guide)

### Standalone
```bash
# Build
make build

# Run services
./bin/hub -config config/hub-config.yaml
./bin/source -config config/source-hybrid.yaml
./bin/exposer -config config/exposer-config.yaml
```

## Monitoring

### Prometheus Metrics
```bash
# Hub metrics
curl http://localhost:9090/metrics

# Exposer metrics  
curl http://localhost:3000/metrics
```

### Datadog (Optional)
Set `DATADOG_ENDPOINT` environment variable to enable automatic metrics pushing.

```bash
export DATADOG_ENDPOINT=localhost:8125
```

## Security Model

**Three Authentication Layers:**

1. **Client → Exposer**: Bearer token validation
2. **Exposer → Hub**: Exposer-specific token + permission check
3. **Source → Hub**: Source-specific token validation

**Each entity has unique, isolated credentials.**

## Performance

- **Latency**: < 100ms p99 for simple queries
- **Throughput**: 10,000+ queries/second (Hub)
- **Connections**: Persistent gRPC streams (no connection overhead)
- **Load Balancing**: Automatic round-robin across sources

## Development

```bash
# Install dependencies
go mod tidy

# Generate proto code
make proto

# Build all services
make build

# Run tests
make test

# Build Docker images
make docker-build
```

## Contributing

Contributions welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Authors

Built following enterprise-grade distributed systems patterns with security, observability, and scalability as core principles.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Issue Tracker](https://github.com/regentmarkets/agents-datahub/issues)
- 💬 [Discussions](https://github.com/regentmarkets/agents-datahub/discussions)

---

**DataHub** - Secure, scalable, observable data access for modern applications.