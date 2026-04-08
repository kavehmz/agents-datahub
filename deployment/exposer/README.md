# Exposer Deployment Guide

## Overview

Deploy Exposer services where applications need data access (AI agent clusters, integration points, etc.). Exposers provide REST API endpoints and connect to Hub.

## Deployment Locations

- **AI Agent K8s Cluster** - For direct AI agent access
- **Integration Cluster** - For Xano and external service access
- **On-premise** - For internal tools

## Prerequisites

- K8s cluster or Docker environment
- Hub endpoint accessible
- Exposer authentication token (from Hub team)
- Client tokens for applications that will use this Exposer

## Configuration

### Exposer Config File

Create `exposer-config.yaml`:

```yaml
exposer:
  name: ai-agent-api            # Must match TOKEN_EXPOSER_ai-agent-api in Hub

hub:
  endpoint: hub.example.com:8080  # Hub HTTP endpoint
  timeout: 30s

api:
  port: 3000
  cors:
    enabled: true
    origins: ["*"]               # Restrict in production
```

### Environment Variables

```bash
# Required
EXPOSER_NAME=ai-agent-api
HUB_ENDPOINT=hub.example.com:8080
EXPOSER_AUTH_TOKEN=<token-from-hub-team>

# Client tokens (who can use this Exposer)
TOKEN_CLIENT_cs-ai-agent=<client-token-1>
TOKEN_CLIENT_compliance-agent=<client-token-2>
TOKEN_CLIENT_xano=<client-token-3>

# Optional
API_PORT=3000
DATADOG_ENDPOINT=datadog:8125
```

## Deployment: Kubernetes

### 1. Create Namespace

```bash
kubectl create namespace datahub-exposer
```

### 2. Create Secrets

```bash
kubectl create secret generic exposer-secrets \
  --from-literal=EXPOSER_AUTH_TOKEN=<hub-token> \
  --from-literal=TOKEN_CLIENT_cs-ai-agent=<client-token-1> \
  --from-literal=TOKEN_CLIENT_compliance-agent=<client-token-2> \
  --from-literal=TOKEN_CLIENT_xano=<client-token-3> \
  --namespace=datahub-exposer
```

### 3. Create ConfigMap

```bash
kubectl create configmap exposer-config \
  --from-file=exposer-config.yaml \
  --namespace=datahub-exposer
```

### 4. Deploy

```bash
kubectl apply -f deployment.yaml -n datahub-exposer
kubectl apply -f service.yaml -n datahub-exposer
```

### 5. Verify

```bash
# Check pods
kubectl get pods -n datahub-exposer

# Check logs
kubectl logs -f deployment/exposer -n datahub-exposer

# Test health
kubectl port-forward svc/exposer 3000:3000 -n datahub-exposer
curl http://localhost:3000/health
```

## Deployment: Docker

```bash
docker run -d \
  --name datahub-exposer \
  --restart unless-stopped \
  -p 3000:3000 \
  -e EXPOSER_NAME=ai-agent-api \
  -e HUB_ENDPOINT=hub.example.com:8080 \
  -e EXPOSER_AUTH_TOKEN=<your-token> \
  -e TOKEN_CLIENT_cs-ai-agent=<client-token> \
  -v $(pwd)/exposer-config.yaml:/app/config.yaml:ro \
  datahub/exposer:v1.0.0

# Check logs
docker logs -f datahub-exposer
```

## Multiple Exposers

Deploy different Exposers for different purposes:

### Exposer 1: AI Agents (K8s Cluster)
```yaml
exposer:
  name: ai-agent-api
  
# Clients: CS agents, compliance agents
TOKEN_CLIENT_cs-ai-agent=token1
TOKEN_CLIENT_compliance-agent=token2
```

### Exposer 2: Xano Integration
```yaml
exposer:
  name: xano-api

# Client: Xano service
TOKEN_CLIENT_xano=token3
```

### Exposer 3: Internal Tools
```yaml
exposer:
  name: internal-api

# Clients: Backoffice, dashboards
TOKEN_CLIENT_backoffice=token4
TOKEN_CLIENT_dashboard=token5
```

Each Exposer has:
- Unique authentication token to Hub
- Own set of client tokens
- Own permissions configured in Hub

## API Access

Applications access Exposer via REST API:

```bash
POST /data/{label}/{operation}
Authorization: Bearer <client-token>
Content-Type: application/json

{"param1": "value1"}
```

Example:
```bash
curl -X POST http://exposer:3000/data/payment-data/getUserPayments \
  -H "Authorization: Bearer cs-ai-agent-token" \
  -H "Content-Type: application/json" \
  -d '{"userId": "12345"}'
```

## Network Requirements

### Inbound
- TCP 3000 (REST API) - from applications

### Outbound
- TCP 8080 - to Hub HTTP endpoint
- UDP 8125 - to Datadog (optional)

## Monitoring

### Health Check

```bash
curl http://exposer:3000/health

# Response:
{
  "status": "healthy",
  "hub": "connected"
}
```

### Metrics

```bash
curl http://exposer:3000/metrics

# Prometheus format metrics
exposer_queries_total{exposer="ai-agent-api"} 1234
exposer_query_duration_seconds{quantile="0.99"} 0.045
```

## Scaling

Exposers are stateless and can be scaled:

```bash
# K8s
kubectl scale deployment exposer --replicas=5 -n datahub-exposer

# Behind load balancer
```

## Security

### CORS Configuration

For web applications:

```yaml
api:
  cors:
    enabled: true
    origins:
      - "https://app.example.com"
      - "https://dashboard.example.com"
```

### Client Token Management

Each client (AI agent, Xano, etc.) gets unique token:

```bash
# Add new client
kubectl patch secret exposer-secrets -n datahub-exposer \
  --type=json \
  -p='[{"op":"add","path":"/data/TOKEN_CLIENT_new-agent","value":"<base64-token>"}]'

# Restart Exposer to load
kubectl rollout restart deployment/exposer -n datahub-exposer
```

## Troubleshooting

### Can't Connect to Hub

```bash
# Test connectivity
curl http://hub.example.com:8080/health

# Check logs
kubectl logs -f deployment/exposer -n datahub-exposer | grep "hub"
```

### Client Authentication Failing

```bash
# Check logs
kubectl logs -f deployment/exposer -n datahub-exposer | grep "auth"

# Verify token in secret
kubectl get secret exposer-secrets -n datahub-exposer -o yaml
```

### Queries Return "Forbidden"

Hub permissions need update. Contact Hub team to add operations to this Exposer's permission list.

## Example: AI Agent Cluster Deployment

Complete example for deploying in AI agent K8s cluster:

```bash
# 1. Create namespace
kubectl create namespace ai-agents

# 2. Create secrets
kubectl create secret generic datahub-exposer-secrets \
  --from-literal=EXPOSER_AUTH_TOKEN=<from-hub-team> \
  --from-literal=TOKEN_CLIENT_cs-agent=<generate> \
  --from-literal=TOKEN_CLIENT_compliance-agent=<generate> \
  -n ai-agents

# 3. Create config
kubectl create configmap datahub-exposer-config \
  --from-file=exposer-config.yaml \
  -n ai-agents

# 4. Deploy
kubectl apply -f deployment.yaml -n ai-agents

# 5. Verify
kubectl get pods -n ai-agents
kubectl logs -f deployment/exposer -n ai-agents
```

## Production Checklist

- [ ] Exposer deployed in correct cluster
- [ ] Unique authentication token configured
- [ ] All client tokens generated and stored
- [ ] Hub endpoint reachable
- [ ] CORS properly configured
- [ ] TLS enabled for external access
- [ ] Monitoring configured
- [ ] Health checks passing
- [ ] Test query successful