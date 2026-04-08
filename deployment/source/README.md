# Source Deployment Guide

## Overview

Deploy Source services in data zones (Cashier AWS, analytics servers, etc.). Sources connect to local data sources and initiate outbound connections to Hub.

## Deployment Options

- **Docker container** (recommended for AWS EC2, on-premise servers)
- **Kubernetes** (for K8s-enabled data zones)
- **ECS/Fargate** (for AWS environments)

## Prerequisites

- Hub endpoint accessible from source location
- Source authentication token (from Hub team)
- Access to local data sources (database or APIs)
- Docker or K8s in deployment zone

## Configuration

### Source Config File

Create `source-config.yaml`:

```yaml
source:
  name: cashier-source-01        # Must match TOKEN_SOURCE_cashier-source-01
  label: payment-data             # Data category
  version: 1.0.0

hub:
  endpoint: hub.example.com:50051  # External Hub endpoint
  reconnect_interval: 5s
  max_reconnect_attempts: -1       # Reconnect forever

database:
  type: postgresql                 # If using database
  max_connections: 20
  query_timeout: 10s

restapi:
  enabled: true                    # If using REST APIs
  base_url: http://localhost:8080  # PayOps API endpoint
  timeout: 10s

operations:
  - name: getUserPayments
    timeout: 10s
  - name: getTransactionHistory
    timeout: 15s
```

### Environment Variables

```bash
# Required
SOURCE_NAME=cashier-source-01
HUB_ENDPOINT=hub.example.com:50051
SOURCE_AUTH_TOKEN=<token-from-hub-team>

# If using database
DATABASE_URL=postgresql://user:pass@localhost:5432/cashier

# If using REST API
RESTAPI_AUTH_TOKEN=<payops-api-token>

# Optional
DATADOG_ENDPOINT=datadog:8125
```

## Deployment: Docker (Cashier AWS)

### 1. Build Image

```bash
docker build -f docker/Dockerfile.source -t datahub/source:v1.0.0 .
```

### 2. Create Config

Place `source-config.yaml` in deployment location.

### 3. Run Container

```bash
docker run -d \
  --name datahub-source \
  --restart unless-stopped \
  -e SOURCE_NAME=cashier-source-01 \
  -e HUB_ENDPOINT=hub-external-lb.example.com:50051 \
  -e SOURCE_AUTH_TOKEN=<your-token> \
  -e DATABASE_URL=postgresql://user:pass@cashier-db:5432/db \
  -e RESTAPI_AUTH_TOKEN=<payops-token> \
  -v $(pwd)/source-config.yaml:/app/config.yaml:ro \
  datahub/source:v1.0.0
```

### 4. Verify Connection

```bash
# Check logs
docker logs -f datahub-source

# Should see:
# {"event":"source_starting","message":"Starting source client"}
# {"event":"registered_with_hub","message":"Successfully registered with hub"}
# {"event":"connected_to_hub","message":"Successfully connected to hub via gRPC"}
```

## Deployment: Kubernetes

### 1. Create Namespace

```bash
kubectl create namespace datahub-source
```

### 2. Create Secrets

```bash
kubectl create secret generic source-secrets \
  --from-literal=SOURCE_AUTH_TOKEN=<your-token> \
  --from-literal=DATABASE_URL=<database-url> \
  --from-literal=RESTAPI_AUTH_TOKEN=<api-token> \
  --namespace=datahub-source
```

### 3. Create ConfigMap

```bash
kubectl create configmap source-config \
  --from-file=source-config.yaml \
  --namespace=datahub-source
```

### 4. Deploy

```bash
kubectl apply -f deployment.yaml -n datahub-source
```

See `deployment.yaml` for full manifest.

## Deployment: AWS ECS

### 1. Create Task Definition

```json
{
  "family": "datahub-source",
  "containerDefinitions": [{
    "name": "source",
    "image": "your-registry.io/datahub/source:v1.0.0",
    "environment": [
      {"name": "SOURCE_NAME", "value": "cashier-source-01"},
      {"name": "HUB_ENDPOINT", "value": "hub.example.com:50051"}
    ],
    "secrets": [
      {"name": "SOURCE_AUTH_TOKEN", "valueFrom": "arn:aws:secretsmanager:..."},
      {"name": "DATABASE_URL", "valueFrom": "arn:aws:secretsmanager:..."}
    ],
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-group": "/ecs/datahub-source",
        "awslogs-region": "us-east-1"
      }
    }
  }]
}
```

### 2. Create Service

```bash
aws ecs create-service \
  --cluster your-cluster \
  --service-name datahub-source \
  --task-definition datahub-source \
  --desired-count 1
```

## Network Configuration

### Outbound Requirements

Source needs outbound access to:
- **Hub gRPC endpoint** (port 50051)
- **Data sources** (database ports or API endpoints)
- **Datadog** (port 8125, optional)

### Inbound Requirements

**None.** Source initiates all connections.

### Security Groups (AWS)

```
Outbound:
  - Hub: TCP 50051 to <hub-endpoint>
  - Database: TCP 5432 to <db-endpoint>
  - APIs: TCP 80/443 to <api-endpoints>
  - Datadog: UDP 8125 to <datadog-endpoint>

Inbound:
  - None required
```

## Multiple Sources

Deploy multiple sources for:
- High availability (label-based load balancing)
- Different data sources (payment data, user data, analytics)
- Geographic distribution

Example:
```bash
# Source 1 in Cashier AWS (payment data)
SOURCE_NAME=cashier-source-01
SOURCE_AUTH_TOKEN=<unique-token-1>

# Source 2 in Analytics AWS (analytics data)
SOURCE_NAME=analytics-source-01
SOURCE_AUTH_TOKEN=<unique-token-2>
```

Each gets its own unique token in Hub configuration.

## Monitoring

### Health Check

Source doesn't expose HTTP endpoints. Check via logs:

```bash
# Docker
docker logs datahub-source | grep "connected_to_hub"

# K8s
kubectl logs -f deployment/source -n datahub-source | grep "connected_to_hub"
```

### Hub Connection Status

Check from Hub:

```bash
curl http://hub:8080/stats
# Should show your source in connected list
```

## Troubleshooting

### Source Can't Connect to Hub

```bash
# Test network connectivity
telnet hub.example.com 50051

# Check token
echo $SOURCE_AUTH_TOKEN

# Check logs
docker logs datahub-source | grep "error"
```

### Source Connects But Queries Fail

Check Hub logs for permission or routing issues:

```bash
kubectl logs -f deployment/hub -n datahub-hub | grep <source-name>
```

### Database Connection Issues

```bash
# Test database from source container
docker exec datahub-source psql $DATABASE_URL -c "SELECT 1"

# Check logs
docker logs datahub-source | grep "database"
```

## Updates

### Update Configuration

```bash
# Docker: Update config file and restart
docker restart datahub-source

# K8s: Update ConfigMap and rollout
kubectl edit configmap source-config -n datahub-source
kubectl rollout restart deployment/source -n datahub-source
```

### Update Image

```bash
# Docker
docker pull datahub/source:v1.1.0
docker stop datahub-source
docker rm datahub-source
# Run with new image

# K8s
kubectl set image deployment/source \
  source=your-registry.io/datahub/source:v1.1.0 \
  -n datahub-source