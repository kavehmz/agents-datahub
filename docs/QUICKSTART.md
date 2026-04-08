# Quick Start Guide

Get DataHub running in 3 minutes!

## Step 1: Generate Protocol Buffers

```bash
chmod +x generate-proto-docker.sh
./generate-proto-docker.sh
```

✅ Expected output:
```
✅ Proto code generation complete!
Generated files:
  proto/datahub.pb.go
  proto/datahub_grpc.pb.go
```

## Step 2: Start Services

```bash
docker-compose up -d
```

Wait ~30 seconds for services to be ready.

## Step 3: Test

```bash
chmod +x examples/test-client.sh
./examples/test-client.sh
```

✅ You should see successful API responses with user data!

## Step 4: Explore

```bash
# View logs
docker-compose logs -f

# Check health
curl http://localhost:3000/health

# View metrics
curl http://localhost:3000/metrics

# Query a user
curl -X POST http://localhost:3000/data/user-data/getUserById \
  -H "Authorization: Bearer testtoken456" \
  -H "Content-Type: application/json" \
  -d '{"userId": "user-001"}'
```

## Stop

```bash
docker-compose down
```

## Troubleshooting

**Problem:** Proto generation fails

**Solution:** Make sure Docker is running:
```bash
docker ps
```

**Problem:** Services won't start

**Solution:** Check logs and rebuild:
```bash
docker-compose logs
docker-compose down -v
docker-compose up -d --build
```

## Next Steps

- Read [`SETUP.md`](SETUP.md) for detailed setup
- Read [`TESTING.md`](TESTING.md) for comprehensive testing
- Read [`README.md`](README.md) for full documentation