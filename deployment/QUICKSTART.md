# Deployment Quick Start

## Prerequisites

- [ ] Hub K8s cluster ready
- [ ] Docker registry accessible
- [ ] Hub external endpoint DNS configured
- [ ] Tokens generated (see `secrets/README.md`)

## Step-by-Step Deployment

### 1. Build and Push Images

```bash
# Build
docker build -f docker/Dockerfile.hub -t your-registry.io/datahub/hub:v1.0.0 .
docker build -f docker/Dockerfile.source -t your-registry.io/datahub/source:v1.0.0 .
docker build -f docker/Dockerfile.exposer -t your-registry.io/datahub/exposer:v1.0.0 .

# Push
docker push your-registry.io/datahub/hub:v1.0.0
docker push your-registry.io/datahub/source:v1.0.0
docker push your-registry.io/datahub/exposer:v1.0.0
```

### 2. Deploy Hub (Hub K8s Cluster)

```bash
cd deployment/hub

# Create namespace
kubectl create namespace datahub-hub

# Create secrets (update tokens!)
kubectl create secret generic hub-tokens \
  --from-literal=TOKEN_SOURCE_cashier-source-01=$(openssl rand -base64 32) \
  --from-literal=TOKEN_EXPOSER_ai-agent-api=$(openssl rand -base64 32) \
  --from-literal=TOKEN_EXPOSER_xano-api=$(openssl rand -base64 32) \
  -n datahub-hub

# Create config
kubectl create configmap hub-config \
  --from-file=hub-config.yaml \
  -n datahub-hub

# Deploy
kubectl apply -f deployment.yaml -n datahub-hub
kubectl apply -f service.yaml -n datahub-hub

# Verify
kubectl get pods -n datahub-hub
kubectl logs -f deployment/hub -n datahub-hub
```

### 3. Get Hub External Endpoint

```bash
# Wait for LoadBalancer IP
kubectl get svc hub-external -n datahub-hub

# Note the EXTERNAL-IP for Source and Exposer configuration
```

### 4. Deploy Source (Cashier AWS or Other Data Zone)

**Docker Deployment:**

```bash
# Copy config
cp deployment/source/source-cashier.yaml /deployment/location/

# Get Source token from Hub secrets
SOURCE_TOKEN=$(kubectl get secret hub-tokens -n datahub-hub \
  -o jsonpath='{.data.TOKEN_SOURCE_cashier-source-01}' | base64 -d)

# Run
docker run -d \
  --name datahub-source \
  --restart unless-stopped \
  -e SOURCE_NAME=cashier-source-01 \
  -e HUB_ENDPOINT=<hub-external-ip>:50051 \
  -e SOURCE_AUTH_TOKEN=$SOURCE_TOKEN \
  -e RESTAPI_AUTH_TOKEN=<payops-api-token> \
  -v /deployment/location/source-cashier.yaml:/app/config.yaml:ro \
  your-registry.io/datahub/source:v1.0.0

# Verify
docker logs -f datahub-source
```

### 5. Verify Source Registration

```bash
# Check Hub logs
kubectl logs -f deployment/hub -n datahub-hub | grep "source_connected"

# Should see:
# {"event":"source_connected","data":{"name":"cashier-source-01"}}

# Check Hub stats
kubectl port-forward svc/hub 8080:8080 -n datahub-hub
curl http://localhost:8080/stats
```

### 6. Deploy Exposer (AI Agent K8s Cluster)

```bash
cd deployment/exposer

# Create namespace
kubectl create namespace datahub-exposer

# Get Exposer token from Hub
EXPOSER_TOKEN=$(kubectl get secret hub-tokens -n datahub-hub \
  -o jsonpath='{.data.TOKEN_EXPOSER_ai-agent-api}' | base64 -d)

# Create secrets
kubectl create secret generic exposer-secrets \
  --from-literal=EXPOSER_AUTH_TOKEN=$EXPOSER_TOKEN \
  --from-literal=TOKEN_CLIENT_cs-ai-agent=$(openssl rand -base64 32) \
  --from-literal=TOKEN_CLIENT_compliance-agent=$(openssl rand -base64 32) \
  -n datahub-exposer

# Create config
kubectl create configmap exposer-config \
  --from-file=exposer-ai-agents.yaml \
  -n datahub-exposer

# Deploy
kubectl apply -f deployment.yaml -n datahub-exposer
kubectl apply -f service.yaml -n datahub-exposer

# Verify
kubectl get pods -n datahub-exposer
kubectl logs -f deployment/exposer -n datahub-exposer
```

### 7. Test End-to-End

```bash
# Get Exposer endpoint
EXPOSER_IP=$(kubectl get svc exposer-external -n datahub-exposer -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Get client token
CLIENT_TOKEN=$(kubectl get secret exposer-secrets -n datahub-exposer \
  -o jsonpath='{.data.TOKEN_CLIENT_cs-ai-agent}' | base64 -d)

# Test query
curl -X POST http://$EXPOSER_IP/data/payment-data/getUserPayments \
  -H "Authorization: Bearer $CLIENT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"userId": "test-user"}'

# Should return payment data from PayOps API via DataHub!
```

## Summary

After completion, you have:

**Hub Cluster:**
- Hub deployment (2 replicas)
- External LoadBalancer for Source connections

**Cashier AWS:**
- Source container calling PayOps APIs
- Outbound connection to Hub

**AI Agent Cluster:**
- Exposer deployment (2 replicas)
- REST API for AI agents
- External LoadBalancer (optional)

**Test:** AI agent can query payment data through secure, audited gateway.

## Next Steps

- Configure monitoring (Prometheus scraping, Datadog)
- Set up alerting (source disconnections, query failures)
- Deploy additional Exposers (for Xano, other services)
- Add more Sources (analytics, user data, etc.)
- Review audit logs