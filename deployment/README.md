# DataHub Deployment Guide

## Overview

This directory contains all deployment configurations and instructions for deploying DataHub components across different environments.

## Deployment Architecture

```
Hub K8s Cluster
    └─ Hub Service (central control)

Cashier AWS / Other Secure Zones
    └─ Source Services (data connectors)

AI Agent K8s Cluster / Other Locations
    └─ Exposer Services (API endpoints)
```

## Quick Navigation

- **[Hub Deployment](hub/)** - Deploy Hub in dedicated K8s cluster
- **[Source Deployment](source/)** - Deploy Sources in data zones (Cashier AWS, etc.)
- **[Exposer Deployment](exposer/)** - Deploy Exposers in application clusters
- **[Secrets Management](secrets/)** - Token and credential management

## Deployment Order

1. **Deploy Hub** first (in Hub K8s cluster)
2. **Deploy Sources** in their respective data zones
3. **Deploy Exposers** where applications need access
4. **Verify** connections and test queries

## Prerequisites

- Kubernetes 1.24+ (for Hub and Exposer clusters)
- Docker (for Source deployments)
- kubectl configured for target clusters
- Helm 3+ (optional, for Hub deployment)
- Access to create secrets in each environment

## Building Images

All services use the same codebase but different configurations.

```bash
# Build all images
docker build -f docker/Dockerfile.hub -t datahub/hub:v1.0.0 .
docker build -f docker/Dockerfile.source -t datahub/source:v1.0.0 .
docker build -f docker/Dockerfile.exposer -t datahub/exposer:v1.0.0 .

# Tag for your registry
docker tag datahub/hub:v1.0.0 your-registry.io/datahub/hub:v1.0.0
docker tag datahub/source:v1.0.0 your-registry.io/datahub/source:v1.0.0
docker tag datahub/exposer:v1.0.0 your-registry.io/datahub/exposer:v1.0.0

# Push to registry
docker push your-registry.io/datahub/hub:v1.0.0
docker push your-registry.io/datahub/source:v1.0.0
docker push your-registry.io/datahub/exposer:v1.0.0
```

## Network Requirements

### Hub
- **Inbound:** gRPC (50051), HTTP (8080), Metrics (9090)
- **Outbound:** None required

### Source  
- **Inbound:** None required
- **Outbound:** gRPC to Hub (50051), access to data sources

### Exposer
- **Inbound:** REST API (3000)
- **Outbound:** HTTP/2 to Hub (8080)

## Security Checklist

- [ ] Unique tokens generated for each Source
- [ ] Unique tokens generated for each Exposer
- [ ] Unique tokens generated for each Client
- [ ] Tokens stored in Kubernetes Secrets (not ConfigMaps)
- [ ] Hub permissions configured in hub-config.yaml
- [ ] Network policies restrict inter-pod communication
- [ ] TLS enabled for all external connections
- [ ] Datadog endpoint configured (if using monitoring)

## Support

For deployment issues, check:
1. Logs: `kubectl logs -f deployment/hub`
2. Health: `curl http://hub-service:8080/health`
3. Metrics: `curl http://hub-service:9090/metrics`