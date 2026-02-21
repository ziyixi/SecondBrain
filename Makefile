# Second Brain – local dev and CI helpers
# Run from repo root.

BINARY_NAME   ?= server
IMAGE_NAME    ?= secondbrain
IMAGE_TAG     ?= test
DOCKERFILE    := ./llm/Dockerfile
COMPOSE_FILE  := test/integration/docker-compose.yml
COMPOSE_FULL  := docker-compose.yml

.PHONY: help build run test test-unit test-integration docker-build docker-run up down compose-up compose-down env

help:
	@echo "Targets:"
	@echo "  make build            - build Go binary (./server)"
	@echo "  make run              - run binary (loads .env if present)"
	@echo "  make test             - run all tests (unit + integration, needs Docker)"
	@echo "  make test-unit        - run unit tests only (-short)"
	@echo "  make test-integration - run integration tests only (Docker Compose)"
	@echo "  make docker-build     - build Docker image ($(IMAGE_NAME):$(IMAGE_TAG))"
	@echo "  make docker-run       - run Docker image (loads .env)"
	@echo "  make up               - start full stack (server + Qdrant) via docker-compose"
	@echo "  make down             - stop full stack"
	@echo "  make compose-up       - start Qdrant only (test stack)"
	@echo "  make compose-down     - stop Qdrant test stack"
	@echo "  make env              - copy .env.example to .env if .env missing"

# Build Go binary
build:
	go build -o $(BINARY_NAME) ./cmd/server

# Run binary; export vars from .env if file exists (use 'set -a' in shell or dotenv)
run: build
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi; ./$(BINARY_NAME)

# All tests (unit + integration)
test:
	go test -v ./...

# Unit tests only (no Docker)
test-unit:
	go test -short ./...

# Integration tests only (uses Docker Compose)
test-integration:
	go test -v -run Integration ./internal/api/... ./internal/memory/...

# Build Docker image (context = repo root, file = llm/Dockerfile)
docker-build:
	docker build -f $(DOCKERFILE) -t $(IMAGE_NAME):$(IMAGE_TAG) .

# Run Docker image; load .env into container env (create with: make env)
docker-run: docker-build
	@port=$${PORT:-8080}; \
	if [ -f .env ]; then \
		docker run --rm -p $$port:$$port --env-file .env $(IMAGE_NAME):$(IMAGE_TAG); \
	else \
		echo "No .env found. Run: make env"; exit 1; \
	fi

# Start full stack (server + Qdrant) from root docker-compose.yml
up:
	docker compose -f $(COMPOSE_FULL) up -d --build

# Stop full stack
down:
	docker compose -f $(COMPOSE_FULL) down

# Start integration stack (Qdrant only) for tests
compose-up:
	docker compose -f $(COMPOSE_FILE) up -d

# Stop integration stack
compose-down:
	docker compose -f $(COMPOSE_FILE) down -v

# Ensure .env exists from .env.example
env:
	@if [ ! -f .env ]; then cp .env.example .env && echo "Created .env from .env.example"; else echo ".env already exists"; fi
