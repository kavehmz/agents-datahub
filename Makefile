.PHONY: help proto build test clean docker-build docker-up docker-down docker-logs

help:
	@echo "DataHub Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  proto        - Generate protobuf code"
	@echo "  build        - Build all services"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts"
	@echo "  docker-build - Build Docker images"
	@echo "  docker-up    - Start services with Docker Compose"
	@echo "  docker-down  - Stop Docker Compose services"
	@echo "  docker-logs  - View Docker Compose logs"
	@echo "  run-test     - Run test client"

proto:
	@echo "Generating protobuf code using Docker..."
	@chmod +x generate-proto-docker.sh
	@./generate-proto-docker.sh

build: proto
	@echo "Building services..."
	@mkdir -p bin
	@go build -o bin/hub ./hub
	@go build -o bin/source ./source
	@go build -o bin/exposer ./exposer
	@echo "Build complete! Binaries in ./bin/"

test:
	@echo "Running tests..."
	@go test ./...

clean:
	@echo "Cleaning..."
	@rm -rf bin/
	@rm -f proto/*.pb.go
	@echo "Clean complete!"

docker-build:
	@echo "Building Docker images..."
	@docker-compose build

docker-up:
	@echo "Starting services..."
	@docker-compose up -d
	@echo ""
	@echo "Services started! Waiting for health checks..."
	@sleep 5
	@echo ""
	@echo "Service URLs:"
	@echo "  - Exposer API: http://localhost:3000"
	@echo "  - Hub HTTP:    http://localhost:8080"
	@echo "  - Hub gRPC:    localhost:50051"
	@echo "  - Metrics:     http://localhost:9090/metrics"
	@echo ""
	@echo "Run 'make run-test' to test the system"

docker-down:
	@echo "Stopping services..."
	@docker-compose down

docker-logs:
	@docker-compose logs -f

run-test:
	@echo "Running test client..."
	@chmod +x examples/test-client.sh
	@./examples/test-client.sh

# Development shortcuts
dev-hub:
	@go run ./hub -config config/hub-config.yaml

dev-source:
	@go run ./source -config config/source-config.yaml

dev-exposer:
	@go run ./exposer -config config/exposer-config.yaml