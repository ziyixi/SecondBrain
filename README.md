# Second Brain — Cognitive Operating System

A microservices-based personal knowledge management system that transforms a passive "Second Brain" into an active, agentic entity capable of autonomous reasoning, context management, and continuous self-improvement.

## Architecture

The system is composed of four microservices, all written in Go, communicating via gRPC with Protocol Buffers:

| Service | Port | Description |
|---------|------|-------------|
| **Cortex** | 50051 | Orchestration, API Gateway, MCP Client, Session Management |
| **Frontal Lobe** | 50052 | Reasoning Engine, Clarify/Reflect Agents, LLM Integration |
| **Hippocampus** | 50053 | Vector Store, Knowledge Graph, Embeddings, RAG Pipeline |
| **Sensory Gateway** | 50054 (gRPC) / 8081 (HTTP) | Webhook Ingestion, Polling, Data Normalization |

```
┌─────────────┐      gRPC       ┌───────────────┐
│   Client     │◄──────────────►│    Cortex      │
└─────────────┘                 │  (Orchestrator)│
                                └───┬───────┬────┘
                          gRPC ▼           ▼ gRPC
                    ┌──────────────┐ ┌──────────────┐
                    │ Frontal Lobe │ │ Hippocampus  │
                    │  (Reasoning) │ │   (Memory)   │
                    └──────────────┘ └──────────────┘
                                          
┌─────────────┐  HTTP webhooks  ┌──────────────┐
│ Email/Slack/ │───────────────►│   Sensory    │
│ GitHub/etc  │                 │   Gateway    │
└─────────────┘                 └──────────────┘
```

## Quick Start

### Prerequisites

- Go 1.24+
- Docker & Docker Compose (for containerized deployment)
- `protoc` with Go plugins (for proto regeneration)

### Build

```bash
make build
```

### Test

```bash
make test
```

### Run with Docker Compose

```bash
docker compose up -d
```

### Run Locally

```bash
# Start each service in separate terminals:
cd services/hippocampus && go run ./cmd/server
cd services/frontal_lobe && go run ./cmd/server
cd services/gateway && go run ./cmd/server
cd services/cortex && go run ./cmd/server
```

## Project Structure

```
.
├── PRD.md                          # Product Requirements Document
├── Makefile                        # Build automation
├── docker-compose.yml              # Local development orchestration
├── proto/                          # Protocol Buffer definitions
│   ├── agent/v1/agent.proto        # Reasoning engine contract
│   ├── common/v1/common.proto      # Shared types
│   ├── ingestion/v1/ingestion.proto # Ingestion service contract
│   └── memory/v1/memory.proto      # Memory service contract
├── services/
│   ├── cortex/                     # Cortex (Orchestrator) service
│   │   ├── cmd/server/             # Entry point
│   │   ├── internal/               # Business logic
│   │   │   ├── config/             # Configuration
│   │   │   ├── mcp/                # MCP client
│   │   │   ├── middleware/         # gRPC interceptors
│   │   │   ├── server/             # gRPC server implementation
│   │   │   ├── session/            # Session management
│   │   │   └── tools/              # Notion tool wrappers
│   │   ├── pkg/gen/                # Generated protobuf code
│   │   └── tests/e2e/              # E2E integration tests
│   ├── gateway/                    # Sensory Gateway service
│   │   ├── cmd/server/
│   │   └── internal/
│   │       ├── config/
│   │       ├── normalizer/         # Payload normalization
│   │       ├── poller/             # Source polling
│   │       ├── server/
│   │       └── webhook/            # HTTP webhook handlers
│   ├── hippocampus/                # Hippocampus (Memory) service
│   │   ├── cmd/server/
│   │   └── internal/
│   │       ├── chunker/            # Text chunking strategies
│   │       ├── config/
│   │       ├── embedder/           # Vector embedding
│   │       ├── graph/              # Knowledge graph
│   │       ├── server/
│   │       └── vectorstore/        # Vector similarity search
│   └── frontal_lobe/               # Frontal Lobe (Reasoning) service
│       ├── cmd/server/
│       └── internal/
│           ├── agents/             # Clarify & Reflect agents
│           ├── config/
│           ├── reasoning/          # LLM provider interface
│           └── server/
├── deployments/k8s/                # Kubernetes manifests
└── .github/workflows/              # CI/CD pipelines
```

## Testing

Unit tests are provided for all services:

```bash
# All services
make test

# Individual service
cd services/cortex && go test -v ./...

# With coverage
make test-coverage

# With race detection
cd services/hippocampus && go test -race ./...
```

## CI/CD

- **CI Pipeline** (`.github/workflows/ci.yml`): Runs on every push/PR to `main`. Tests, lints, and builds all services.
- **Release Pipeline** (`.github/workflows/release.yml`): Builds and pushes Docker images to GHCR on version tags (`v*`).

## Kubernetes Deployment

```bash
kubectl apply -f deployments/k8s/hippocampus/
kubectl apply -f deployments/k8s/frontal-lobe/
kubectl apply -f deployments/k8s/gateway/
kubectl apply -f deployments/k8s/cortex/
```
