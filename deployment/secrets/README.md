# Secrets and Token Management

## Overview

DataHub uses environment-based token authentication. Each component has unique tokens that must be securely managed.

## Token Types

### 1. Source Tokens
Format: `TOKEN_SOURCE_<source-name>`

Each Source needs a unique token validated by Hub.

```bash
TOKEN_SOURCE_cashier-source-01=<generate-32-char-random>
TOKEN_SOURCE_analytics-source-01=<generate-32-char-random>
```

### 2. Exposer Tokens
Format: `TOKEN_EXPOSER_<exposer-name>`

Each Exposer needs a unique token validated by Hub.

```bash
TOKEN_EXPOSER_ai-agent-api=<generate-32-char-random>
TOKEN_EXPOSER_xano-api=<generate-32-char-random>
```

### 3. Client Tokens
Format: `TOKEN_CLIENT_<client-name>`

Each client (AI agent, Xano, etc.) needs a unique token validated by Exposer.

```bash
TOKEN_CLIENT_cs-ai-agent=<generate-32-char-random>
TOKEN_CLIENT_compliance-agent=<generate-32-char-random>
TOKEN_CLIENT_xano=<generate-32-char-random>
```

## Generating Tokens

### Secure Random Generation

```bash
# Generate a secure 32-character token
openssl rand -base64 32

# Or using /dev/urandom
head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32
```

### Token Requirements

- Minimum 32 characters
- Random (use cryptographic random generator)
- Unique per component
- Stored securely (never in code or logs)

## Token Distribution

### Hub Needs
```bash
# All Source tokens
TOKEN_SOURCE_cashier-source-01=xxx
TOKEN_SOURCE_analytics-source-01=yyy

# All Exposer tokens
TOKEN_EXPOSER_ai-agent-api=aaa
TOKEN_EXPOSER_xano-api=bbb
```

### Each Source Needs
```bash
# Its own token (matches Hub's TOKEN_SOURCE_<name>)
SOURCE_AUTH_TOKEN=xxx

# Plus database/API credentials
DATABASE_URL=postgresql://...
RESTAPI_AUTH_TOKEN=zzz
```

### Each Exposer Needs
```bash
# Its own token (matches Hub's TOKEN_EXPOSER_<name>)
EXPOSER_AUTH_TOKEN=aaa

# All client tokens it should accept
TOKEN_CLIENT_cs-ai-agent=111
TOKEN_CLIENT_compliance-agent=222
```

## Storage Methods

### Kubernetes Secrets

```bash
# Hub (in Hub cluster)
kubectl create secret generic hub-tokens \
  --from-literal=TOKEN_SOURCE_cashier-source-01=xxx \
  --from-literal=TOKEN_SOURCE_analytics-source-01=yyy \
  --from-literal=TOKEN_EXPOSER_ai-agent-api=aaa \
  --from-literal=TOKEN_EXPOSER_xano-api=bbb \
  -n datahub-hub

# Source (in data zone cluster, if using K8s)
kubectl create secret generic source-secrets \
  --from-literal=SOURCE_AUTH_TOKEN=xxx \
  --from-literal=DATABASE_URL=postgresql://... \
  --from-literal=RESTAPI_AUTH_TOKEN=zzz \
  -n datahub-source

# Exposer (in application cluster)
kubectl create secret generic exposer-secrets \
  --from-literal=EXPOSER_AUTH_TOKEN=aaa \
  --from-literal=TOKEN_CLIENT_cs-ai-agent=111 \
  --from-literal=TOKEN_CLIENT_compliance-agent=222 \
  -n datahub-exposer
```

### AWS Secrets Manager

For Source in AWS:

```bash
# Create secret
aws secretsmanager create-secret \
  --name datahub/source/cashier-source-01 \
  --secret-string '{
    "SOURCE_AUTH_TOKEN":"xxx",
    "DATABASE_URL":"postgresql://...",
    "RESTAPI_AUTH_TOKEN":"zzz"
  }'

# Reference in ECS task definition
"secrets": [
  {"name": "SOURCE_AUTH_TOKEN", "valueFrom": "arn:aws:secretsmanager:..."}
]
```

### Docker Environment File

For Docker deployments:

```bash
# .env file (never commit to git!)
SOURCE_AUTH_TOKEN=xxx
DATABASE_URL=postgresql://...
RESTAPI_AUTH_TOKEN=zzz
```

## Token Rotation

### Zero-Downtime Rotation Process

**Step 1: Add new token alongside old**

```bash
# Hub: Support both tokens temporarily
TOKEN_SOURCE_cashier-source-01=[old-token,new-token]

# Update Hub secret
kubectl patch secret hub-tokens -n datahub-hub \
  --type=json \
  -p='[{"op":"replace","path":"/data/TOKEN_SOURCE_cashier-source-01","value":"<base64:[old,new]>"}]'

# Reload Hub
kubectl rollout restart deployment/hub -n datahub-hub
```

**Step 2: Update Source to use new token**

```bash
# Source: Start using new token
SOURCE_AUTH_TOKEN=new-token

# Update Source
kubectl patch secret source-secrets -n datahub-source \
  --type=json \
  -p='[{"op":"replace","path":"/data/SOURCE_AUTH_TOKEN","value":"<base64:new-token>"}]'

kubectl rollout restart deployment/source -n datahub-source
```

**Step 3: Remove old token from Hub**

```bash
# Hub: Only accept new token now
TOKEN_SOURCE_cashier-source-01=new-token

# Update and reload
kubectl patch secret hub-tokens -n datahub-hub \
  --type=json \
  -p='[{"op":"replace","path":"/data/TOKEN_SOURCE_cashier-source-01","value":"<base64:new-token>"}]'

kubectl rollout restart deployment/hub -n datahub-hub
```

## Token Mapping Table

Use this to track which tokens are where:

| Component | Location | Token Name | Stored In | Used For |
|-----------|----------|------------|-----------|----------|
| Hub | Hub K8s | TOKEN_SOURCE_cashier-source-01 | hub-tokens secret | Validate Source |
| Hub | Hub K8s | TOKEN_EXPOSER_ai-agent-api | hub-tokens secret | Validate Exposer |
| Source | Cashier AWS | SOURCE_AUTH_TOKEN | AWS Secrets Manager | Authenticate to Hub |
| Source | Cashier AWS | RESTAPI_AUTH_TOKEN | AWS Secrets Manager | Call PayOps APIs |
| Exposer | AI K8s | EXPOSER_AUTH_TOKEN | exposer-secrets secret | Authenticate to Hub |
| Exposer | AI K8s | TOKEN_CLIENT_cs-ai-agent | exposer-secrets secret | Validate AI agent |

## Security Best Practices

- Generate tokens using cryptographic random generators
- Never commit tokens to git
- Use K8s secrets or cloud secret managers
- Rotate tokens quarterly or when team members leave
- Limit token scope (one token per component, not shared)
- Monitor for authentication failures (may indicate compromised token)
- Use different tokens in dev/staging/production

## Emergency Token Revocation

If a token is compromised:

**1. Identify affected component** from token name

**2. Generate new token**
```bash
NEW_TOKEN=$(openssl rand -base64 32)
```

**3. Update immediately**

For Source:
```bash
# Update Source with new token
kubectl patch secret source-secrets -n datahub-source ...

# Update Hub to reject old token
kubectl patch secret hub-tokens -n datahub-hub ...
```

**4. Verify**
```bash
# Check Source reconnects with new token
kubectl logs -f deployment/source -n datahub-source | grep "registered"

# Old token should be rejected if tried
```

**5. Monitor**

Check Hub logs for any authentication attempts with old token:
```bash
kubectl logs -f deployment/hub -n datahub-hub | grep "auth_failed"
```

## Token Generation Script

```bash
#!/bin/bash
# generate-tokens.sh

generate_token() {
    openssl rand -base64 32 | tr -d '/+=' | head -c 32
}

echo "=== Hub Tokens ==="
echo "TOKEN_SOURCE_cashier-source-01=$(generate_token)"
echo "TOKEN_SOURCE_analytics-source-01=$(generate_token)"
echo ""
echo "TOKEN_EXPOSER_ai-agent-api=$(generate_token)"
echo "TOKEN_EXPOSER_xano-api=$(generate_token)"
echo ""
echo "=== Source Tokens ==="
echo "SOURCE_AUTH_TOKEN=(use corresponding token from Hub)"
echo "RESTAPI_AUTH_TOKEN=$(generate_token)"
echo ""
echo "=== Client Tokens ==="
echo "TOKEN_CLIENT_cs-ai-agent=$(generate_token)"
echo "TOKEN_CLIENT_compliance-agent=$(generate_token)"
echo "TOKEN_CLIENT_xano=$(generate_token)"
```

## Checklist

Before deployment:

- [ ] All tokens generated using secure method
- [ ] Token mapping table filled out
- [ ] Hub has all Source and Exposer tokens
- [ ] Each Source has correct authentication token
- [ ] Each Exposer has correct authentication token
- [ ] Each Exposer has all client tokens
- [ ] Tokens stored in appropriate secret manager (not plaintext)
- [ ] Test tokens work in non-production first
- [ ] Document token locations for rotation