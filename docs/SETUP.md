# Setup Guide

This guide explains how to set up and run the DataHub system.

## Prerequisites

**Required:**
- Docker and Docker Compose
- Git

**Optional (for development):**
- Go 1.21+ (only if building locally without Docker)
- `curl` and `jq` (for testing)

## Setup Options

You have two options to set up the system:

### Option 1: Docker Only (Recommended)

This is the **easiest and most reliable** approach. It requires only Docker.

### Option 2: Local Development

For Go development with local builds. Requires Go toolchain.

---

## Option 1: Docker Only Setup (Recommended)

This approach uses Docker for everything, including proto generation.

### Step 1: Generate Protocol Buffer Code

```bash
# Make the script executable and run it
chmod +x generate-proto-docker.sh
./generate-proto-docker.sh
```

Or using Make:
```bash
make proto
```

This will:
1. Build a Docker container with `protoc` and Go plugins
2. Generate `proto/datahub.pb.go` and `proto/datahub_grpc.pb.go`
3. Clean up the container

**Expected output:**
```
Generating Protocol Buffer code using Docker...
✅ Proto code generation complete!
Generated files:
  proto/datahub.pb.go
  proto/datahub_grpc.pb.go
```

### Step 2: Start the System

```bash
make docker-up
```

Or without Make:
```bash
docker-compose up -d
```

This will:
1. Pull the PostgreSQL image
2. Build Hub, Source, and Exposer containers
3. Start all services with health checks
4. Initialize the database with sample data

**Wait for services to be ready** (about 30 seconds):
```bash
# Check status
docker-compose ps

# All services should show "healthy"
```

### Step 3: Verify Everything Works

```bash
# Check exposer health
curl http://localhost:3000/health

# Check hub health
curl http://localhost:8080/health
```

Expected responses should show `"status": "healthy"`.

### Step 4: Run Tests

```bash
make run-test
```

Or:
```bash
chmod +x examples/test-client.sh
./examples/test-client.sh
```

**Expected output:**
- ✅ Successful user queries with data
- ✅ Trace information showing source and timing
- ✅ Authentication rejection for invalid tokens
- ✅ Health checks passing
- ✅ Metrics data

### Step 5: View Logs

```bash
# All services
docker-compose logs -f

# Specific service
docker-compose logs -f hub
docker-compose logs -f source-postgres
docker-compose logs -f exposer
```

You should see JSON-formatted logs with query execution details.

### Step 6: Stop the System

```bash
make docker-down
```

Or:
```bash
docker-compose down
```

To completely remove all data:
```bash
docker-compose down -v
```

---

## Option 2: Local Development Setup

For local Go development without Docker (except for database).

### Prerequisites

Install required tools:

**macOS:**
```bash
brew install go protobuf
```

**Linux (Ubuntu/Debian):**
```bash
sudo apt-get update
sudo apt-get install -y golang-go protobuf-compiler
```

**Install Go plugins:**
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Step 1: Generate Proto Code

```bash
chmod +x generate-proto.sh
./generate-proto.sh
```

### Step 2: Install Dependencies

```bash
go mod tidy
```

### Step 3: Build Services

```bash
make build
```

Or manually:
```bash
mkdir -p bin
go build -o bin/hub ./hub
go build -o bin/source ./source
go build -o bin/exposer ./exposer
```

### Step 4: Start PostgreSQL

```bash
docker-compose up -d postgres
```

### Step 5: Set Environment Variables

```bash
# Copy example
cp config/.env.example .env

# Edit if needed
vi .env

# Load into shell
export $(cat .env | xargs)
```

Or manually:
```bash
# Hub
export TOKEN_SOURCE_postgres-prod-01=abc123xyz
export TOKEN_EXPOSER_api-west-01=west123abc
export GRPC_PORT=50051
export HTTP_PORT=8080

# Source
export SOURCE_NAME=postgres-prod-01
export DATABASE_URL="postgresql://testuser:testpass@localhost:5432/testdb"
export HUB_ENDPOINT=localhost:50051
export SOURCE_AUTH_TOKEN=abc123xyz

# Exposer  
export EXPOSER_NAME=api-west-01
export HUB_ENDPOINT=localhost:8080
export EXPOSER_AUTH_TOKEN=west123abc
export TOKEN_CLIENT_test-client=testtoken456
export API_PORT=3000
```

### Step 6: Run Services

**Terminal 1 - Hub:**
```bash
./bin/hub -config config/hub-config.yaml
```

**Terminal 2 - Source:**
```bash
./bin/source -config config/source-postgres.yaml
```

**Terminal 3 - Exposer:**
```bash
./bin/exposer -config config/exposer-config.yaml
```

### Step 7: Test

In Terminal 4:
```bash
./examples/test-client.sh
```

---

## Troubleshooting

### Proto Generation Fails

**Problem:** Docker proto generation fails

**Solution:**
```bash
# Check Docker is running
docker ps

# Try rebuilding
docker build -f docker/Dockerfile.proto -t datahub-proto-gen .
```

### Services Won't Start

**Problem:** Docker Compose services fail to start

**Solution:**
```bash
# Check logs
docker-compose logs

# Remove old containers
docker-compose down -v

# Rebuild
docker-compose build --no-cache

# Try again
docker-compose up -d
```

### Database Connection Issues

**Problem:** Source can't connect to database

**Solution:**
```bash
# Check PostgreSQL is running
docker-compose ps postgres

# Check it's healthy
docker-compose exec postgres pg_isready -U testuser

# Restart if needed
docker-compose restart postgres
```

### Port Already in Use

**Problem:** Port conflict (3000, 8080, 50051, 5432)

**Solution:**
```bash
# Find what's using the port
lsof -i :3000
lsof -i :8080

# Stop conflicting service or change port in config
```

### Permission Denied

**Problem:** Script permission errors

**Solution:**
```bash
chmod +x generate-proto-docker.sh
chmod +x examples/test-client.sh
```

---

## Next Steps

After successful setup:

1. **Explore the API** - See [`README.md`](README.md) for API examples
2. **Run tests** - See [`TESTING.md`](TESTING.md) for comprehensive testing
3. **View metrics** - Visit `http://localhost:3000/metrics`
4. **Check logs** - `docker-compose logs -f`
5. **Customize** - Modify configurations in `config/`

## Quick Reference

```bash
# Essential commands
make proto         # Generate proto code (Docker)
make docker-up     # Start all services
make run-test      # Run test client
make docker-logs   # View logs
make docker-down   # Stop services

# Without Make
./generate-proto-docker.sh
docker-compose up -d
./examples/test-client.sh
docker-compose logs -f
docker-compose down
```

## Support

If you encounter issues:
1. Check this guide
2. Check [`TESTING.md`](TESTING.md) troubleshooting section
3. Review logs: `docker-compose logs`
4. Verify Docker is running: `docker ps`