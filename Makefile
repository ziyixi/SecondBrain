.PHONY: all build test lint clean proto docker

all: lint test build

# Build all services
build:
	cd services/cortex && go build -o ../../bin/cortex ./cmd/server
	cd services/gateway && go build -o ../../bin/gateway ./cmd/server
	cd services/hippocampus && go build -o ../../bin/hippocampus ./cmd/server
	cd services/frontal_lobe && go build -o ../../bin/frontal_lobe ./cmd/server

# Run all tests
test:
	cd services/cortex && go test -race ./...
	cd services/gateway && go test -race ./...
	cd services/hippocampus && go test -race ./...
	cd services/frontal_lobe && go test -race ./...

# Run tests with coverage
test-coverage:
	cd services/cortex && go test -race -coverprofile=coverage.out ./...
	cd services/gateway && go test -race -coverprofile=coverage.out ./...
	cd services/hippocampus && go test -race -coverprofile=coverage.out ./...
	cd services/frontal_lobe && go test -race -coverprofile=coverage.out ./...

# Run linting
lint:
	cd services/cortex && go vet ./...
	cd services/gateway && go vet ./...
	cd services/hippocampus && go vet ./...
	cd services/frontal_lobe && go vet ./...

# Generate protobuf code (requires protoc + plugins installed)
proto:
	@echo "Generating Go protobuf code..."
	@export PATH="$$PATH:$$(go env GOPATH)/bin" && \
	for svc in cortex gateway hippocampus frontal_lobe; do \
		rm -rf services/$$svc/pkg/gen/*; \
	done && \
	protoc --proto_path=proto \
		--go_out=services/cortex/pkg/gen --go_opt=paths=source_relative \
		--go_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/common/v1 \
		--go_opt=Magent/v1/agent.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1 \
		--go_opt=Mmemory/v1/memory.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/memory/v1 \
		--go_opt=Mingestion/v1/ingestion.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/ingestion/v1 \
		--go-grpc_out=services/cortex/pkg/gen --go-grpc_opt=paths=source_relative \
		--go-grpc_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/common/v1 \
		--go-grpc_opt=Magent/v1/agent.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/agent/v1 \
		--go-grpc_opt=Mmemory/v1/memory.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/memory/v1 \
		--go-grpc_opt=Mingestion/v1/ingestion.proto=github.com/ziyixi/SecondBrain/services/cortex/pkg/gen/ingestion/v1 \
		common/v1/common.proto agent/v1/agent.proto memory/v1/memory.proto ingestion/v1/ingestion.proto && \
	protoc --proto_path=proto \
		--go_out=services/gateway/pkg/gen --go_opt=paths=source_relative \
		--go_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/common/v1 \
		--go_opt=Mingestion/v1/ingestion.proto=github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/ingestion/v1 \
		--go-grpc_out=services/gateway/pkg/gen --go-grpc_opt=paths=source_relative \
		--go-grpc_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/common/v1 \
		--go-grpc_opt=Mingestion/v1/ingestion.proto=github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/ingestion/v1 \
		common/v1/common.proto ingestion/v1/ingestion.proto && \
	protoc --proto_path=proto \
		--go_out=services/hippocampus/pkg/gen --go_opt=paths=source_relative \
		--go_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/common/v1 \
		--go_opt=Mmemory/v1/memory.proto=github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/memory/v1 \
		--go-grpc_out=services/hippocampus/pkg/gen --go-grpc_opt=paths=source_relative \
		--go-grpc_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/common/v1 \
		--go-grpc_opt=Mmemory/v1/memory.proto=github.com/ziyixi/SecondBrain/services/hippocampus/pkg/gen/memory/v1 \
		common/v1/common.proto memory/v1/memory.proto && \
	protoc --proto_path=proto \
		--go_out=services/frontal_lobe/pkg/gen --go_opt=paths=source_relative \
		--go_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/common/v1 \
		--go_opt=Magent/v1/agent.proto=github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/agent/v1 \
		--go-grpc_out=services/frontal_lobe/pkg/gen --go-grpc_opt=paths=source_relative \
		--go-grpc_opt=Mcommon/v1/common.proto=github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/common/v1 \
		--go-grpc_opt=Magent/v1/agent.proto=github.com/ziyixi/SecondBrain/services/frontal_lobe/pkg/gen/agent/v1 \
		common/v1/common.proto agent/v1/agent.proto

# Build Docker images
docker:
	docker compose build

# Run all services locally
up:
	docker compose up -d

# Stop all services
down:
	docker compose down

# Clean build artifacts
clean:
	rm -rf bin/
	cd services/cortex && go clean
	cd services/gateway && go clean
	cd services/hippocampus && go clean
	cd services/frontal_lobe && go clean
