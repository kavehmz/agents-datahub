# Testing Guide

This guide explains how to test the DataHub system locally.

## Prerequisites

- Docker and Docker Compose installed
- `curl` and `jq` installed for API testing
- Internet connection (for downloading Docker images)

## Quick Test (Recommended)

The easiest way to test the system is using Docker Compose:

### 1. Start the System

```bash
# Start all services
make docker-up

# Or without make:
docker-compose up -d
```

This will start:
- PostgreSQL database with sample data
- Hub service (orchestrator)
- Source service (PostgreSQL connector)
- Exposer service (REST API gateway)

### 2. Wait for Services to be Ready

```bash
# Check service health
curl http://localhost:3000/health
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "hub": "connected"
}
```

### 3. Run Test Client

```bash
make run-test

# Or without make:
chmod +x examples/test-client.sh
./examples/test-client.sh
```

This will execute several test scenarios:
- ✅ Get user by ID
- ✅ Get users by filter
- ✅ Test invalid authentication
- ✅ Check health endpoints
- ✅ View metrics

### 4. View Logs

```bash
# All services
make docker-logs

# Specific service
docker-compose logs -f hub
docker-compose logs -f source-postgres
docker-compose logs -f exposer
```

### 5. Stop Services

```bash
make docker-down

# Or:
docker-compose down
```

## Manual API Testing

### Test 1: Get User by ID

```bash
curl -X POST http://localhost:3000/data/user-data/getUserById \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user-001"
  }' | jq .
```

Expected response:
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

### Test 2: Get Users with Filter

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
  }' | jq .
```

### Test 3: Invalid Authentication

```bash
curl -X POST http://localhost:3000/data/user-data/getUserById \
  -H "Authorization: Bearer wrong-token" \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-001"}' | jq .
```

Expected response:
```json
{
  "success": false,
  "error": {
    "code": "UNAUTHORIZED",
    "message": "Invalid token"
  }
}
```

### Test 4: Check Metrics

```bash
# Exposer metrics
curl http://localhost:3000/metrics

# Hub metrics  
curl http://localhost:9090/metrics
```

## Testing Without Docker

If you want to test without Docker (requires Go and PostgreSQL):

### 1. Set Up Database

```bash
# Create database and user
createdb testdb
psql testdb < examples/init.sql
```

### 2. Set Environment Variables

```bash
# Hub
export TOKEN_SOURCE_postgres-prod-01=abc123xyz
export TOKEN_EXPOSER_api-west-01=west123abc

# Source
export SOURCE_NAME=postgres-prod-01
export DATABASE_URL="postgresql://user:pass@localhost:5432/testdb"
export HUB_ENDPOINT=localhost:50051
export SOURCE_AUTH_TOKEN=abc123xyz

# Exposer
export EXPOSER_NAME=api-west-01
export HUB_ENDPOINT=localhost:8080
export EXPOSER_AUTH_TOKEN=west123abc
export TOKEN_CLIENT_test-client=testtoken456
```

### 3. Build and Run Services

```bash
# Terminal 1 - Build
make build

# Terminal 2 - Hub
./bin/hub -config config/hub-config.yaml

# Terminal 3 - Source
./bin/source -config config/source-postgres.yaml

# Terminal 4 - Exposer
./bin/exposer -config config/exposer-config.yaml
```

### 4. Run Tests

```bash
./examples/test-client.sh
```

## Troubleshooting

### Services Won't Start

**Problem:** Docker services fail to start

**Solution:**
```bash
# Check logs
docker-compose logs

# Remove old containers and try again
docker-compose down -v
docker-compose up -d
```

### Database Connection Failed

**Problem:** Source cannot connect to database

**Solution:**
```bash
# Check database is running
docker-compose ps postgres

# Check DATABASE_URL
docker-compose exec source-postgres env | grep DATABASE_URL

# Test connection manually
docker-compose exec postgres psql -U testuser -d testdb -c "SELECT 1;"
```

### Authentication Errors

**Problem:** Getting "UNAUTHORIZED" responses

**Solution:**
```bash
# Check token is set correctly
curl -H "Authorization: Bearer testtoken456" http://localhost:3000/health

# Verify token in environment
docker-compose exec exposer env | grep TOKEN_CLIENT
```

### Hub Not Responding

**Problem:** Exposer cannot reach hub

**Solution:**
```bash
# Check hub is running
docker-compose ps hub

# Check hub logs
docker-compose logs hub

# Test hub directly
curl http://localhost:8080/health
```

## Performance Testing

### Load Test with `ab` (Apache Bench)

```bash
# Install Apache Bench
# macOS: brew install httpd
# Linux: apt-get install apache2-utils

# Test with 1000 requests, 10 concurrent
ab -n 1000 -c 10 \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -p test-request.json \
  http://localhost:3000/data/user-data/getUserById
```

Create `test-request.json`:
```json
{"userId": "user-001"}
```

### Monitor Metrics During Load

```bash
# Watch metrics in real-time
watch -n 1 'curl -s http://localhost:3000/metrics | grep exposer_queries'
```

## Expected Results

When everything is working correctly, you should see:

1. **All health checks pass** (HTTP 200)
2. **Successful queries return data** with trace information
3. **Invalid tokens are rejected** (HTTP 401)
4. **Metrics are incrementing** during queries
5. **Logs show query execution** in JSON format

## Next Steps

After testing:

1. Review the code in each service directory
2. Modify configuration files to add new operations
3. Implement additional data sources (MongoDB, etc.)
4. Add more exposers with different permissions
5. Set up monitoring with Prometheus and Grafana

## Support

If you encounter issues:
- Check the README.md for configuration details
- Review logs with `docker-compose logs`
- Ensure all environment variables are set correctly