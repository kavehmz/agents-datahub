# Data Access Layer Architecture - Complete Specification

## 1. Executive Summary

A three-tier data access architecture that provides controlled, secure, and auditable access to multiple data sources through a centralized hub. The system enables fine-grained permission control, automatic load balancing, and complete query audit trails.

## 2. Architecture Overview

### 2.1 Core Components

1. **Source Services**: Direct data access services deployed near data sources
2. **Hub Service**: Central orchestrator managing sources and exposers
3. **Exposer Services**: API gateways exposing controlled REST endpoints to client applications

### 2.2 Key Principles

- **Zero Trust**: Every connection requires authentication
- **Least Privilege**: Components only access what they explicitly need
- **Audit Everything**: Complete query trail from client to source
- **High Availability**: Automatic failover through label-based round-robin
- **Simplicity**: REST APIs for clients, efficient protocols internally
- **Secure by Default**: All tokens in environment variables, never in code

## 3. Architecture Diagram

```mermaid
graph TB
    subgraph "Data Sources Layer"
        DB1[(PostgreSQL<br/>Production)]
        DB2[(MongoDB<br/>Analytics)]
        DB3[(PostgreSQL<br/>Replica)]
    end
    
    subgraph "Source Services Layer"
        S1[Source: postgres-prod-01<br/>Label: user-data<br/>✓ getUserById<br/>✓ getUsersByFilter]
        S2[Source: mongo-analytics-01<br/>Label: analytics-data<br/>✓ getMetrics<br/>✓ getAggregates]
        S3[Source: postgres-replica-01<br/>Label: user-data<br/>✓ getUserById<br/>✓ getUsersByFilter]
    end
    
    subgraph "Hub Service"
        H[Hub<br/>• Source Registry<br/>• Permission Matrix<br/>• Round-Robin Router<br/>• Health Monitor]
    end
    
    subgraph "Exposer Services Layer"
        E1[Exposer: api-west-01<br/>Allowed:<br/>• user-data/*<br/>• analytics-data/getMetrics]
        E2[Exposer: api-east-01<br/>Allowed:<br/>• user-data/getUserById]
        E3[Exposer: internal-api-01<br/>Allowed:<br/>• all operations]
    end
    
    subgraph "Client Applications"
        C1[AI Agent CS]
        C2[AI Agent Compliance]
        C3[Internal Tools]
    end
    
    DB1 -.->|Direct Access| S1
    DB2 -.->|Direct Access| S2
    DB3 -.->|Direct Access| S3
    
    S1 ===>|gRPC Bidirectional<br/>Stream| H
    S2 ===>|gRPC Bidirectional<br/>Stream| H
    S3 ===>|gRPC Bidirectional<br/>Stream| H
    
    E1 -->|HTTP/2 Request<br/>Per Query| H
    E2 -->|HTTP/2 Request<br/>Per Query| H
    E3 -->|HTTP/2 Request<br/>Per Query| H
    
    C1 -->|REST API<br/>Bearer Token| E1
    C2 -->|REST API<br/>Bearer Token| E2
    C3 -->|REST API<br/>Bearer Token| E3
    
    style H fill:#f96,stroke:#333,stroke-width:4px
    style S1 fill:#9f9,stroke:#333,stroke-width:2px
    style S2 fill:#9f9,stroke:#333,stroke-width:2px
    style S3 fill:#9f9,stroke:#333,stroke-width:2px
    style E1 fill:#99f,stroke:#333,stroke-width:2px
    style E2 fill:#99f,stroke:#333,stroke-width:2px
    style E3 fill:#99f,stroke:#333,stroke-width:2px
```

## 4. Connection Flow Diagram

```mermaid
sequenceDiagram
    participant Client
    participant Exposer
    participant Hub
    participant Source1
    participant Source2
    
    Note over Source1, Hub: Persistent gRPC Stream
    Source1->>Hub: Connect + Register(name: postgres-prod-01, label: user-data)
    Hub-->>Source1: Acknowledge Registration
    
    Note over Source2, Hub: Persistent gRPC Stream
    Source2->>Hub: Connect + Register(name: postgres-replica-01, label: user-data)
    Hub-->>Source2: Acknowledge Registration
    
    Note over Hub: Round-Robin Setup for label: user-data
    
    Note over Client, Exposer: REST API Call
    Client->>Exposer: POST /data/user-data/getUserById<br/>Bearer: xxx<br/>{"userId": "123"}
    
    Note over Exposer, Hub: HTTP/2 Request
    Exposer->>Hub: Query Request<br/>(exposer: api-west-01, operation: getUserById)
    
    Note over Hub: Select Source (Round-Robin)
    Hub->>Source1: Query via gRPC stream<br/>(getUserById, userId: 123)
    Source1-->>Hub: Query Response
    
    Hub-->>Exposer: HTTP/2 Response
    Exposer-->>Client: JSON Response
    
    Note over Hub: Next query uses Source2
    Client->>Exposer: POST /data/user-data/getUserById<br/>Bearer: yyy<br/>{"userId": "456"}
    Exposer->>Hub: Query Request
    Hub->>Source2: Query via gRPC stream
    Source2-->>Hub: Query Response
    Hub-->>Exposer: HTTP/2 Response
    Exposer-->>Client: JSON Response
```

## 5. Authentication & Token Management

### 5.1 Token Environment Variable Patterns

```bash
# Hub validates these tokens from Sources
TOKEN_SOURCE_<source-name>=token
TOKEN_SOURCE_<source-name>=[token1,token2]  # Multiple for rotation

# Hub validates these tokens from Exposers  
TOKEN_EXPOSER_<exposer-name>=token
TOKEN_EXPOSER_<exposer-name>=[token1,token2]  # Multiple for rotation

# Exposers validate these tokens from Clients
TOKEN_CLIENT_<client-name>=token
TOKEN_CLIENT_<client-name>=[token1,token2]  # Multiple for rotation
```

### 5.2 Environment Variable Examples

```bash
# ============ HUB ENVIRONMENT ============
# Source Tokens (sources connecting to hub)
TOKEN_SOURCE_postgres-prod-01=abc123xyz
TOKEN_SOURCE_postgres-replica-01=[abc123xyz,def456uvw]  # Rotating
TOKEN_SOURCE_mongo-analytics-01=ghi789rst

# Exposer Tokens (exposers connecting to hub)  
TOKEN_EXPOSER_api-west-01=west123abc
TOKEN_EXPOSER_api-east-01=[east456def,east789ghi]  # Rotating
TOKEN_EXPOSER_internal-api-01=internal123xyz

# Hub Configuration
GRPC_PORT=50051
HTTP_PORT=8080
METRICS_PORT=9090
DATADOG_ENDPOINT=http://datadog-agent:8125

# ============ SOURCE ENVIRONMENT ============
# Source Identity
SOURCE_NAME=postgres-prod-01
DATABASE_URL=postgresql://user:pass@localhost:5432/db
HUB_ENDPOINT=hub.internal:50051
DATADOG_ENDPOINT=http://datadog-agent:8125

# ============ EXPOSER ENVIRONMENT ============
# Exposer Identity
EXPOSER_NAME=api-west-01
HUB_ENDPOINT=hub.internal:8080
API_PORT=3000

# Client Tokens (clients connecting to this exposer)
TOKEN_CLIENT_web-app=webtoken123
TOKEN_CLIENT_mobile-app=[mobiletoken456,mobiletoken789]  # Rotating
TOKEN_CLIENT_internal-dashboard=dashtoken123

DATADOG_ENDPOINT=http://datadog-agent:8125
```

### 5.3 Token Rotation Process

```mermaid
sequenceDiagram
    participant Ops as Operations
    participant Env as Environment
    participant Service as Service
    
    Note over Ops, Service: Initial State
    Ops->>Env: TOKEN_EXPOSER_api-west-01=token1
    Service->>Service: Validates with token1
    
    Note over Ops, Service: Start Rotation
    Ops->>Env: TOKEN_EXPOSER_api-west-01=[token1,token2]
    Service->>Service: Reloads config (SIGHUP)
    Service->>Service: Now accepts token1 OR token2
    
    Note over Ops, Service: Transition Period
    Note over Service: Clients gradually update to token2
    
    Note over Ops, Service: Complete Rotation
    Ops->>Env: TOKEN_EXPOSER_api-west-01=token2
    Service->>Service: Reloads config (SIGHUP)
    Service->>Service: Now only accepts token2
```

## 6. Protocol Definitions

### 6.1 Proto Definition (datahub.proto)

```protobuf
syntax = "proto3";
package datahub;

import "google/protobuf/struct.proto";
import "google/protobuf/timestamp.proto";

// ============ Source to Hub Communication ============

service DataHub {
  // Bidirectional streaming for source connection
  rpc ConnectSource(stream SourceMessage) returns (stream HubMessage);
  
  // HTTP/2 endpoint for exposer queries
  rpc ExecuteQuery(QueryRequest) returns (QueryResponse);
}

// Messages from Source to Hub
message SourceMessage {
  oneof message {
    SourceRegistration registration = 1;
    QueryResult query_result = 2;
    HealthStatus health_status = 3;
    SourceError error = 4;
  }
}

// Messages from Hub to Source
message HubMessage {
  oneof message {
    RegistrationAck registration_ack = 1;
    QueryExecution query_execution = 2;
    HealthCheckRequest health_check = 3;
  }
}

// Source Registration
message SourceRegistration {
  string name = 1;                    // Unique source identifier
  string label = 2;                   // Category for round-robin
  string version = 3;                 // API version
  string auth_token = 4;              // Authentication token
  repeated Operation operations = 5;   // Supported operations
  SourceCapabilities capabilities = 6; // Optional capabilities
}

message Operation {
  string name = 1;                           // Operation identifier
  string description = 2;                    // Human-readable description
  google.protobuf.Struct input_schema = 3;   // JSON Schema for input
  google.protobuf.Struct output_schema = 4;  // JSON Schema for output
  int32 timeout_seconds = 5;                 // Operation timeout
}

message SourceCapabilities {
  int32 max_concurrent_queries = 1;
  bool supports_transactions = 2;
  bool supports_batch_queries = 3;
}

message RegistrationAck {
  bool success = 1;
  string message = 2;
  string assigned_id = 3;  // Hub-assigned connection ID
}

// Query Execution
message QueryExecution {
  string query_id = 1;                        // Unique query identifier
  string operation = 2;                       // Operation to execute
  google.protobuf.Struct parameters = 3;      // Query parameters
  QueryMetadata metadata = 4;                 // Query metadata
}

message QueryResult {
  string query_id = 1;
  bool success = 2;
  google.protobuf.Struct data = 3;           // Result data
  string error_message = 4;                   // Error if success=false
  int64 execution_time_ms = 5;               // Query execution time
}

// Health Check
message HealthCheckRequest {
  google.protobuf.Timestamp timestamp = 1;
}

message HealthStatus {
  bool healthy = 1;
  string status_message = 2;
  SourceMetrics metrics = 3;
}

message SourceMetrics {
  int32 active_connections = 1;
  int32 queries_processed = 2;
  double avg_response_time_ms = 3;
  int64 uptime_seconds = 4;
}

message SourceError {
  string error_code = 1;
  string error_message = 2;
  google.protobuf.Timestamp timestamp = 3;
}

// ============ Exposer to Hub Communication ============

message QueryRequest {
  string exposer_name = 1;                    // Exposer identifier
  string auth_token = 2;                      // Exposer auth token
  string label = 3;                          // Target data label
  string operation = 4;                       // Operation to execute
  google.protobuf.Struct parameters = 5;      // Query parameters
  QueryMetadata metadata = 6;                 // Query metadata
}

message QueryResponse {
  string query_id = 1;
  bool success = 2;
  google.protobuf.Struct data = 3;           // Response data
  string error_message = 4;
  QueryTrace trace = 5;                      // Execution trace
}

message QueryMetadata {
  string query_id = 1;                       // Unique query ID
  string client_id = 2;                      // Optional client identifier
  google.protobuf.Timestamp timestamp = 3;
  map<string, string> headers = 4;          // Additional headers
}

message QueryTrace {
  string source_name = 1;                    // Which source handled it
  int64 hub_processing_ms = 2;
  int64 source_execution_ms = 3;
  int64 total_time_ms = 4;
  repeated string trace_points = 5;         // Debug trace points
}
```

## 7. REST API Specification

### 7.1 Exposer REST API Endpoints

#### Base URL Pattern
```
https://{exposer-host}/data/{label}/{operation}
```

#### Authentication
All requests require Bearer token authentication:
```http
Authorization: Bearer {client-token}
```

### 7.2 API Endpoints

#### User Data Operations

##### Get User by ID
```http
POST /data/user-data/getUserById
Content-Type: application/json
Authorization: Bearer {token}

{
  "userId": "string",
  "includeMetadata": false  // optional
}

Response 200:
{
  "success": true,
  "data": {
    "id": "123",
    "name": "John Doe",
    "email": "john@example.com",
    "createdAt": "2024-01-15T10:00:00Z"
  },
  "trace": {
    "queryId": "q-123-456",
    "source": "postgres-prod-01",
    "executionTime": 45
  }
}

Response 404:
{
  "success": false,
  "error": {
    "code": "USER_NOT_FOUND",
    "message": "User with ID 123 not found"
  },
  "trace": {
    "queryId": "q-123-456",
    "source": "postgres-prod-01",
    "executionTime": 12
  }
}

Response 403:
{
  "success": false,
  "error": {
    "code": "FORBIDDEN",
    "message": "Operation not permitted for this exposer"
  }
}

Response 503:
{
  "success": false,
  "error": {
    "code": "NO_SOURCES_AVAILABLE",
    "message": "No healthy sources available for label: user-data"
  }
}
```

##### Get Users by Filter
```http
POST /data/user-data/getUsersByFilter
Content-Type: application/json
Authorization: Bearer {token}

{
  "filter": {
    "status": "active",
    "createdAfter": "2024-01-01",
    "limit": 100,
    "offset": 0
  },
  "sort": {
    "field": "createdAt",
    "order": "desc"
  }
}

Response 200:
{
  "success": true,
  "data": {
    "users": [
      {
        "id": "123",
        "name": "John Doe",
        "email": "john@example.com",
        "status": "active",
        "createdAt": "2024-01-15T10:00:00Z"
      }
    ],
    "totalCount": 150,
    "hasMore": true
  },
  "trace": {
    "queryId": "q-789-012",
    "source": "postgres-replica-01",
    "executionTime": 123
  }
}
```

#### Analytics Data Operations

##### Get Metrics
```http
POST /data/analytics-data/getMetrics
Content-Type: application/json
Authorization: Bearer {token}

{
  "metricNames": ["revenue", "users", "transactions"],
  "startDate": "2024-01-01",
  "endDate": "2024-01-31",
  "groupBy": "day",
  "filters": {
    "region": "US"
  }
}

Response 200:
{
  "success": true,
  "data": {
    "metrics": [
      {
        "date": "2024-01-01",
        "revenue": 45000.00,
        "users": 1250,
        "transactions": 340
      }
    ],
    "summary": {
      "totalRevenue": 1350000.00,
      "totalUsers": 38500,
      "totalTransactions": 10200
    }
  },
  "trace": {
    "queryId": "q-345-678",
    "source": "mongo-analytics-01",
    "executionTime": 234
  }
}
```

##### Get Aggregates
```http
POST /data/analytics-data/getAggregates
Content-Type: application/json
Authorization: Bearer {token}

{
  "aggregationType": "sum|avg|count|min|max",
  "field": "revenue",
  "groupBy": ["region", "product"],
  "dateRange": {
    "start": "2024-01-01",
    "end": "2024-01-31"
  }
}

Response 200:
{
  "success": true,
  "data": {
    "aggregates": [
      {
        "region": "US",
        "product": "Premium",
        "value": 450000.00,
        "count": 1234
      }
    ]
  },
  "trace": {
    "queryId": "q-567-890",
    "source": "mongo-analytics-01",
    "executionTime": 456
  }
}
```

### 7.3 Standard Error Responses

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": {
      // Optional additional error context
    }
  },
  "trace": {
    "queryId": "q-xxx-xxx",
    "timestamp": "2024-01-15T10:00:00Z"
  }
}
```

#### Error Codes
- `INVALID_PARAMETERS`: Request parameters failed validation
- `OPERATION_NOT_FOUND`: Requested operation doesn't exist
- `FORBIDDEN`: Exposer lacks permission for this operation
- `UNAUTHORIZED`: Invalid or missing authentication token
- `NO_SOURCES_AVAILABLE`: No healthy sources for the requested label
- `SOURCE_ERROR`: Source returned an error
- `TIMEOUT`: Query exceeded timeout limit
- `INTERNAL_ERROR`: Unexpected error in hub or exposer

## 8. Configuration Files

### 8.1 Hub Configuration
```yaml
# hub-config.yaml
server:
  grpc_port: 50051          # For source connections
  http_port: 8080           # For exposer requests
  metrics_port: 9090        # Prometheus metrics

sources:
  connection_timeout: 30s
  health_check_interval: 30s
  unhealthy_threshold: 3    # Failed checks before marking unhealthy
  recovery_threshold: 2     # Successful checks before marking healthy
  # Tokens loaded from TOKEN_SOURCE_* env vars

exposers:
  # Define permissions only, tokens from TOKEN_EXPOSER_* env vars
  - name: api-west-01
    permissions:
      - label: user-data
        operations: ["*"]   # All operations
      - label: analytics-data
        operations: ["getMetrics"]
        
  - name: api-east-01
    permissions:
      - label: user-data
        operations: ["getUserById"]
        
  - name: internal-api-01
    permissions:
      - label: "*"          # All labels
        operations: ["*"]   # All operations

logging:
  level: INFO
  format: json
  # Datadog endpoint from DATADOG_ENDPOINT env var
```

### 8.2 Source Configuration
```yaml
# source-config.yaml
source:
  name: postgres-prod-01  # Must match TOKEN_SOURCE_postgres-prod-01
  label: user-data
  version: 1.0.0
  
hub:
  endpoint: hub.internal:50051
  # Token from TOKEN_SOURCE_postgres-prod-01 env var
  reconnect_interval: 5s
  max_reconnect_attempts: 10
  
database:
  type: postgresql
  # Connection from DATABASE_URL env var
  max_connections: 20
  query_timeout: 10s
  
operations:
  - name: getUserById
    timeout: 5s
    cache_ttl: 0     # No caching
    
  - name: getUsersByFilter
    timeout: 30s
    max_limit: 1000  # Maximum records to return
    
monitoring:
  # Datadog endpoint from DATADOG_ENDPOINT env var
```

### 8.3 Exposer Configuration
```yaml
# exposer-config.yaml
exposer:
  name: api-west-01  # Must match TOKEN_EXPOSER_api-west-01
  
hub:
  endpoint: hub.internal:8080
  # Token from TOKEN_EXPOSER_api-west-01 env var
  timeout: 30s
  
api:
  port: 3000
  rate_limit:
    enabled: false  # As per requirements
  cors:
    enabled: true
    origins: ["*"]
    
# Client tokens loaded from TOKEN_CLIENT_* env vars
# No client definitions needed in yaml

monitoring:
  # Datadog endpoint from DATADOG_ENDPOINT env var
```

## 9. Logging Specification

### 9.1 Query Log Format

```json
{
  "timestamp": "2024-01-15T10:00:00.123Z",
  "level": "INFO",
  "service": "hub|source|exposer",
  "event": "query_executed",
  "query": {
    "id": "q-123-456-789",
    "exposer": "api-west-01",
    "client": "web-app",         // optional
    "label": "user-data",
    "operation": "getUserById",
    "source": "postgres-prod-01",
    "status": "success|error",
    "duration": {
      "hub_ms": 5,
      "source_ms": 40,
      "total_ms": 45
    },
    "error": "error message if failed"
  }
}
```

### 9.2 Connection Log Format

```json
{
  "timestamp": "2024-01-15T10:00:00.123Z",
  "level": "INFO",
  "service": "hub",
  "event": "source_connected|source_disconnected|exposer_authenticated",
  "connection": {
    "type": "source|exposer",
    "name": "postgres-prod-01",
    "label": "user-data",        // for sources
    "operations": ["getUserById", "getUsersByFilter"],
    "remote_addr": "10.0.1.5:45234"
  }
}
```

### 9.3 Audit Trail

Every query generates audit entries at three points:
1. **Exposer Entry**: Client request received
2. **Hub Routing**: Source selected and query forwarded
3. **Source Execution**: Query executed against data source

## 10. Message Format Decision

### Recommendation: Protocol Buffers with JSON Parameters

**Internal Communication (Source ↔ Hub)**
- Use Protocol Buffers for efficiency and type safety
- Parameters as `google.protobuf.Struct` for flexibility

**External Communication (Hub ↔ Exposer)**
- HTTP/2 with Protocol Buffers
- Automatic JSON conversion for REST endpoints

**Benefits:**
- 3-10x smaller than JSON
- Faster parsing
- Strong typing in Go
- Backward compatibility
- Native gRPC support

## 11. Health Check & Monitoring

### 11.1 Health Check Flow

```mermaid
graph LR
    subgraph "Health Monitoring"
        H[Hub] -->|Every 30s| S1[Source 1]
        H -->|Every 30s| S2[Source 2]
        H -->|Every 30s| S3[Source 3]
        
        S1 -->|Healthy/Unhealthy| H
        S2 -->|Healthy/Unhealthy| H
        S3 -->|Healthy/Unhealthy| H
    end
    
    H -->|Update Round-Robin| RR[Router]
```

### 11.2 Metrics Exposed

```yaml
# Prometheus metrics endpoint
hub_sources_connected{label="user-data"} 2
hub_sources_healthy{label="user-data"} 2
hub_queries_total{exposer="api-west-01",label="user-data",operation="getUserById"} 12345
hub_query_duration_seconds{quantile="0.99"} 0.045
hub_errors_total{type="source_timeout"} 23
```

## 12. Deployment Architecture

### 12.1 Docker Compose Example

```yaml
version: '3.8'

services:
  hub:
    image: datahub/hub:latest
    environment:
      - TOKEN_SOURCE_postgres-prod-01=${TOKEN_SOURCE_POSTGRES_PROD_01}
      - TOKEN_SOURCE_mongo-analytics-01=${TOKEN_SOURCE_MONGO_ANALYTICS_01}
      - TOKEN_EXPOSER_api-west-01=${TOKEN_EXPOSER_API_WEST_01}
      - GRPC_PORT=50051
      - HTTP_PORT=8080
      - DATADOG_ENDPOINT=http://datadog-agent:8125
    ports:
      - "50051:50051"  # gRPC
      - "8080:8080"    # HTTP
    volumes:
      - ./config/hub-config.yaml:/app/config.yaml
    
  source-postgres:
    image: datahub/source-postgres:latest
    environment:
      - SOURCE_NAME=postgres-prod-01
      - DATABASE_URL=${DATABASE_URL}
      - HUB_ENDPOINT=hub:50051
      - DATADOG_ENDPOINT=http://datadog-agent:8125
    volumes:
      - ./config/source-postgres.yaml:/app/config.yaml
    depends_on:
      - hub
    
  exposer-api:
    image: datahub/exposer:latest
    environment:
      - EXPOSER_NAME=api-west-01
      - HUB_ENDPOINT=hub:8080
      - TOKEN_CLIENT_web-app=${TOKEN_CLIENT_WEB_APP}
      - TOKEN_CLIENT_mobile-app=${TOKEN_CLIENT_MOBILE_APP}
      - API_PORT=3000
      - DATADOG_ENDPOINT=http://datadog-agent:8125
    ports:
      - "3000:3000"
    volumes:
      - ./config/exposer-config.yaml:/app/config.yaml
    depends_on:
      - hub
```

### 12.2 Kubernetes Deployment

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hub-tokens
type: Opaque
stringData:
  TOKEN_SOURCE_postgres-prod-01: "changeme"
  TOKEN_SOURCE_mongo-analytics-01: "changeme"
  TOKEN_EXPOSER_api-west-01: "changeme"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hub
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: hub
        image: datahub/hub:latest
        envFrom:
        - secretRef:
            name: hub-tokens
        env:
        - name: GRPC_PORT
          value: "50051"
        - name: HTTP_PORT
          value: "8080"
        volumeMounts:
        - name: config
          mountPath: /app/config.yaml
          subPath: hub-config.yaml
      volumes:
      - name: config
        configMap:
          name: hub-config
```

## 13. Performance Specifications

### 13.1 Expected Latencies
- Simple queries (getUserById): < 100ms p99
- Complex queries (with filters): < 500ms p99
- Health checks: < 10ms
- Connection establishment: < 1s

### 13.2 Throughput Targets
- Hub: 10,000 queries/second per instance
- Source: 1,000 queries/second per instance
- Exposer: 5,000 requests/second per instance

### 13.3 Resource Requirements
- Hub: 2 CPU cores, 4GB RAM
- Source: 1 CPU core, 2GB RAM (plus database connection pool)
- Exposer: 1 CPU core, 1GB RAM

## 14. Security Model

### 14.1 Authentication Layers

```mermaid
graph LR
    subgraph "Layer 1: Client Auth"
        C[Client] -->|Bearer Token<br/>TOKEN_CLIENT_*| E[Exposer]
    end
    
    subgraph "Layer 2: Exposer Auth"
        E -->|Exposer Token<br/>TOKEN_EXPOSER_*| H[Hub]
    end
    
    subgraph "Layer 3: Source Auth"
        S[Source] -->|Source Token<br/>TOKEN_SOURCE_*| H
    end
```

### 14.2 Network Security
- All internal communication uses TLS 1.3
- Source↔Hub: mTLS optional for additional security
- Exposer→Hub: TLS with token authentication
- Client→Exposer: HTTPS with bearer token

### 14.3 Token Reload Without Downtime
- Services listen for SIGHUP signal
- Reload tokens from environment
- Support multiple tokens during rotation
- Zero-downtime token rotation

## 15. Development Guidelines

### 15.1 Directory Structure
```
datahub/
├── proto/
│   └── datahub.proto
├── hub/
│   ├── main.go
│   ├── server.go
│   ├── router.go
│   ├── auth.go
│   └── health.go
├── source/
│   ├── main.go
│   ├── client.go
│   ├── postgres/
│   └── mongodb/
├── exposer/
│   ├── main.go
│   ├── server.go
│   └── client.go
├── common/
│   ├── token.go
│   ├── logging.go
│   └── metrics.go
├── config/
│   ├── hub-config.yaml
│   ├── source-postgres.yaml
│   └── exposer-config.yaml
└── docker/
    ├── Dockerfile.hub
    ├── Dockerfile.source
    └── Dockerfile.exposer
```

### 15.2 Build Commands
```bash
# Generate protobuf code
protoc --go_out=. --go-grpc_out=. proto/datahub.proto

# Build services
go build -o bin/hub ./hub
go build -o bin/source ./source
go build -o bin/exposer ./exposer

# Docker builds
docker build -f docker/Dockerfile.hub -t datahub/hub:latest .
docker build -f docker/Dockerfile.source -t datahub/source:latest .
docker build -f docker/Dockerfile.exposer -t datahub/exposer:latest .
```

---

## Summary

This architecture provides:
- **Complete control** over data access
- **Secure by default** with environment-based tokens
- **High availability** through round-robin and health checks
- **Full auditability** with comprehensive logging
- **Easy scaling** with stateless components
- **Simple token rotation** without downtime
- **Clear separation** of concerns across three layers

Ready for implementation with Go, gRPC, and Protocol Buffers.