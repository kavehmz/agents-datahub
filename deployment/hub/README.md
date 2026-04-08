# Hub Deployment Guide

## Overview

Deploy the Hub service in its dedicated K8s cluster. Hub is the central control point that authenticates sources and exposers, routes queries, and maintains audit logs.

## Prerequisites

- K8s cluster with kubectl access
- Container registry access
- Hub configuration file ready
- Tokens generated for all sources and exposers

## Deployment Steps

### 1. Create Namespace

```bash
kubectl create namespace datahub-hub
```

### 2. Create Secrets

```bash
# Create secret with all tokens
kubectl create secret generic hub-tokens \
  --from-literal=TOKEN_SOURCE_cashier-source=<generate-secure-token> \
  --from-literal=TOKEN_SOURCE_analytics-source=<generate-secure-token> \
  --from-literal=TOKEN_EXPOSER_ai-agent-api=<generate-secure-token> \
  --from-literal=TOKEN_EXPOSER_xano-api=<generate-secure-token> \
  --namespace=datahub-hub
```

### 3. Create ConfigMap

```bash
kubectl create configmap hub-config \
  --from-file=hub-config.yaml=./hub-config.yaml \
  --namespace=datahub-hub
```

### 4. Deploy Hub

```bash
kubectl apply -f deployment.yaml -n datahub-hub
```

### 5. Create Service

```bash
kubectl apply -f service.yaml -n datahub-hub
```

### 6. Verify Deployment

```bash
# Check pods
kubectl get pods -n datahub-hub

# Check logs
kubectl logs -f deployment/hub -n datahub-hub

# Check health
kubectl port-forward svc/hub 8080:8080 -n datahub-hub
curl http://localhost:8080/health
```

## Configuration

Edit `hub-config.yaml` before creating ConfigMap:

```yaml
server:
  grpc_port: 50051
  http_port: 8080
  metrics_port: 9090

sources:
  health_check_interval: 30s
  unhealthy_threshold: 3

exposers:
  - name: ai-agent-api
    permissions:
      - label: user-data
        operations: ["*"]
      - label: payment-data
        operations: ["getUserPayments"]
        
  - name: xano-api
    permissions:
      - label: "*"
        operations: ["*"]
```

## Scaling

Hub is stateless and can be scaled horizontally:

```bash
kubectl scale deployment hub --replicas=3 -n datahub-hub
```

## Monitoring

Metrics available at:
- Prometheus: `http://hub-service:9090/metrics`
- Health: `http://hub-service:8080/health`
- Stats: `http://hub-service:8080/stats`

## Troubleshooting

### Sources Not Connecting

Check Hub logs:
```bash
kubectl logs -f deployment/hub -n datahub-hub | grep "source_connected"
```

Verify Source can reach Hub:
```bash
# From Source location
telnet hub-external-endpoint 50051
```

### Exposer Authentication Failing

Check Hub logs:
```bash
kubectl logs -f deployment/hub -n datahub-hub | grep "exposer_auth"
```

Verify token in secret:
```bash
kubectl get secret hub-tokens -n datahub-hub -o yaml
```

## External Access

If Sources need to connect from outside K8s:

### Option 1: LoadBalancer
```yaml
apiVersion: v1
kind: Service
metadata:
  name: hub-external
spec:
  type: LoadBalancer
  selector:
    app: hub
  ports:
    - name: grpc
      port: 50051
      targetPort: 50051
```

### Option 2: Ingress with TLS
See `ingress.yaml` for configuration.

## Updates

### Update Configuration

```bash
# Edit config
kubectl edit configmap hub-config -n datahub-hub

# Reload Hub (if SIGHUP supported)
kubectl exec deployment/hub -n datahub-hub -- kill -HUP 1
```

### Update Tokens

```bash
# During rotation, add new token
kubectl patch secret hub-tokens -n datahub-hub \
  --type=json \
  -p='[{"op":"add","path":"/data/TOKEN_SOURCE_cashier-source","value":"<base64-new-token>"}]'

# Reload Hub
kubectl rollout restart deployment/hub -n datahub-hub
```

### Update Image

```bash
kubectl set image deployment/hub \
  hub=your-registry.io/datahub/hub:v1.1.0 \
  -n datahub-hub