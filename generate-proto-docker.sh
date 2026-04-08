#!/bin/bash

set -e

echo "Generating Protocol Buffer code using Docker..."

# Build the proto generation container
docker build -f docker/Dockerfile.proto -t datahub-proto-gen .

# Run proto generation in container
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  datahub-proto-gen \
  sh -c "protoc --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    proto/datahub.proto"

echo "✅ Proto code generation complete!"
echo "Generated files:"
ls -la proto/*.pb.go 2>/dev/null || echo "  proto/datahub.pb.go"
ls -la proto/*_grpc.pb.go 2>/dev/null || echo "  proto/datahub_grpc.pb.go"