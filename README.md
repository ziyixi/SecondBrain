# Second Brain — Cognitive Operating System

A microservices-based personal knowledge management system that transforms a passive "Second Brain" into an active, agentic entity capable of autonomous reasoning, context management, and continuous self-improvement.

## Architecture

The system is composed of four microservices, all written in Go, communicating via gRPC with Protocol Buffers:

| Service | Port | Description |
|---------|------|-------------|
| **Cortex** | 50051 (gRPC) / 8080 (HTTP) | Orchestration, API Gateway, MCP Client & Server, Session Management |
| **Frontal Lobe** | 50052 | Reasoning Engine, Clarify/Reflect Agents, LLM Integration |
| **Hippocampus** | 50053 | Vector Store, Full-Text Index, Hybrid Search, Knowledge Graph, RAG Pipeline |
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

## Usage Examples

Once the services are running (via Docker Compose or locally), the Cortex service
exposes an **OpenAI-compatible REST API** on port `8080` and an **MCP server** at
`POST /mcp`. All examples below use `curl` and assume the default `localhost:8080`.

> **Note:** Every example below is verified by automated integration tests
> (`services/cortex/tests/e2e/doc_examples_test.go`) that run in CI on every push.

### Chat Completion (non-streaming)

Ask a question and receive a single JSON response. This is the same format as the
OpenAI Chat Completions API, so any client library (Python `openai`, TypeScript,
etc.) works out of the box.

**Request:**

```bash
curl -s http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
  "model": "secondbrain",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "What do my notes say about machine learning?"}
  ]
}'
```

**Response:**

```json
{
  "id": "chatcmpl-1749537607884793061",
  "object": "chat.completion",
  "created": 1749537607,
  "model": "secondbrain",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Based on your knowledge base, machine learning is a subset of AI that enables systems to learn from data..."
      },
      "finish_reason": "stop"
    }
  ]
}
```

### Chat Completion (streaming / SSE)

Stream tokens as they are generated, in the same Server-Sent Events format used by
the OpenAI API.

**Request:**

```bash
curl -s -N http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
  "model": "secondbrain",
  "stream": true,
  "messages": [
    {"role": "user", "content": "Summarize my knowledge base"}
  ]
}'
```

**Response (SSE stream):**

```
data: {"id":"chatcmpl-1749537607","object":"chat.completion.chunk","created":1749537607,"model":"secondbrain","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}

data: {"id":"chatcmpl-1749537607","object":"chat.completion.chunk","created":1749537607,"model":"secondbrain","choices":[{"index":0,"delta":{"content":"Your knowledge base covers..."},"finish_reason":null}]}

data: {"id":"chatcmpl-1749537607","object":"chat.completion.chunk","created":1749537607,"model":"secondbrain","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

### List Available Models

**Request:**

```bash
curl -s http://localhost:8080/v1/models
```

**Response:**

```json
{
  "object": "list",
  "data": [
    {
      "id": "secondbrain",
      "object": "model",
      "created": 1704067200,
      "owned_by": "secondbrain"
    },
    {
      "id": "mock",
      "object": "model",
      "created": 1704067200,
      "owned_by": "secondbrain"
    }
  ]
}
```

### System Metrics

Monitor interaction quality, satisfaction, and knowledge coverage.

**Request:**

```bash
curl -s http://localhost:8080/v1/metrics
```

**Response:**

```json
{
  "total_interactions": 42,
  "avg_response_quality": 0.82,
  "avg_context_relevance": 0.78,
  "user_satisfaction_rate": 0.90,
  "knowledge_coverage": 0.85,
  "feedback_counts": {
    "positive": 9,
    "negative": 1
  },
  "topic_coverage": {
    "machine_learning": 15,
    "go_programming": 12,
    "architecture": 10,
    "databases": 5
  }
}
```

### Error Handling

Errors follow the OpenAI error response format.

**Request (invalid — empty messages):**

```bash
curl -s http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "secondbrain", "messages": []}'
```

**Response (400):**

```json
{
  "error": {
    "message": "messages is required",
    "type": "invalid_request_error",
    "code": "400"
  }
}
```

### Using with the OpenAI Python SDK

Because the API is OpenAI-compatible, you can use the official Python client by
pointing `base_url` at your Second Brain instance:

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="not-needed",        # auth not required for local
)

response = client.chat.completions.create(
    model="secondbrain",
    messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "What are my open action items?"},
    ],
)

print(response.choices[0].message.content)
```

Streaming works too:

```python
stream = client.chat.completions.create(
    model="secondbrain",
    stream=True,
    messages=[{"role": "user", "content": "Summarize this week's notes"}],
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")
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
│   │   │   ├── mcp/                # MCP client (Notion)
│   │   │   ├── mcpserver/          # MCP server (search tools)
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
│   │       ├── hybrid/             # Hybrid search with RRF
│   │       ├── server/
│   │       ├── textindex/          # BM25 full-text search
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

### Documentation Example Tests

The integration test suite includes `doc_examples_test.go` which programmatically
exercises every API example shown in this README (chat completions, streaming, model
listing, metrics, MCP tools) and verifies the response shapes. These tests run on
every push and PR via the `integration.yml` GitHub Actions workflow.

```bash
# Run doc example tests locally
cd services/cortex && go test -v -race -run TestDocExamples ./tests/e2e/
```

## Search Capabilities

The Hippocampus service provides three search modes, inspired by the hybrid search architecture of [qmd](https://github.com/tobi/qmd):

| Mode | RPC | Description |
|------|-----|-------------|
| **Semantic Search** | `SemanticSearch` | Vector similarity using cosine distance. Finds conceptually related content even without exact keyword matches. |
| **Full-Text Search** | `FullTextSearch` | BM25-ranked keyword search. Fast, no embedding required. Best for exact words or phrases. |
| **Hybrid Search** | `HybridSearch` | Combines BM25 + vector search with Reciprocal Rank Fusion (RRF). Highest quality results. |

### Hybrid Search Pipeline

```
Query ──► BM25 Full-Text Search (×2 weight)
  │
  └────► Vector Semantic Search
              │
              └──► Reciprocal Rank Fusion (k=60)
                       │
                       └──► Top-Rank Bonus (+0.05 for #1, +0.02 for #2-3)
                                │
                                └──► Normalized Results [0.0 - 1.0]
```

The cortex automatically uses hybrid search when enriching context for LLM reasoning, falling back to semantic-only if unavailable.

## MCP Server

The Cortex exposes an MCP (Model Context Protocol) server at `POST /mcp` for agentic workflows. AI agents can search and retrieve knowledge from the Second Brain.

**Tools exposed:**

| Tool | Description |
|------|-------------|
| `search` | Semantic vector search using embeddings |
| `fts` | Fast BM25 keyword-based full-text search |
| `hybrid` | Highest quality search combining BM25 + vector + RRF |
| `status` | Index health: document counts, chunks, graph triples |

### MCP Initialize

```bash
curl -s http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize"}'
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": { "tools": {} },
    "serverInfo": { "name": "secondbrain", "version": "0.1.0" }
  }
}
```

### MCP List Tools

```bash
curl -s http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      { "name": "search",  "description": "Semantic vector search using embeddings..." },
      { "name": "fts",     "description": "Fast BM25 keyword-based full-text search..." },
      { "name": "hybrid",  "description": "Highest quality search combining BM25 + vector + RRF..." },
      { "name": "status",  "description": "Show index health: document counts, chunk counts..." }
    ]
  }
}
```

### MCP Search Example

```bash
curl -s http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -d '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "hybrid",
    "arguments": {
      "query": "seismic signal detection",
      "limit": 10,
      "min_score": 0.3
    }
  }
}'
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Found 3 result(s) for \"seismic signal detection\":\n\n  [92%] doc-phasenet\n  PhaseNet-TF is a deep-learning model for seismic phase picking...\n"
      }
    ]
  }
}
```

### Claude Desktop Configuration

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "secondbrain": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

Once configured, Claude can call `search`, `fts`, `hybrid`, and `status` tools
to query your knowledge base directly during a conversation.

## Deployment

### Option 1: Docker Compose (Local / Single Server)

The simplest way to run all services. No external dependencies required — the
Hippocampus uses an in-memory vector store and BM25 index by default.

```bash
# Start all services
docker compose up -d

# Verify health
curl -s http://localhost:8080/v1/models

# View logs
docker compose logs -f cortex

# Stop
docker compose down
```

**Environment variables** (configured in `docker-compose.yml`):

| Variable | Default | Description |
|----------|---------|-------------|
| `CORTEX_GRPC_PORT` | `50051` | Cortex gRPC listen port |
| `CORTEX_HTTP_PORT` | `8080` | Cortex REST API listen port |
| `FRONTAL_LOBE_ADDR` | `frontal-lobe:50052` | Frontal Lobe gRPC address |
| `HIPPOCAMPUS_ADDR` | `hippocampus:50053` | Hippocampus gRPC address |
| `GATEWAY_ADDR` | `gateway:50054` | Gateway gRPC address |
| `LLM_PROVIDER` | `mock` | LLM backend (`mock`, `openai`, `google`) |
| `OPENAI_API_KEY` | — | Required when `LLM_PROVIDER=openai` |
| `GOOGLE_API_KEY` | — | Required when `LLM_PROVIDER=google` |

### Option 2: Kubernetes

Production-grade deployment with health checks, resource limits, and rolling
updates. Manifests are in `deployments/k8s/`.

```bash
# Apply all services
kubectl apply -f deployments/k8s/hippocampus/
kubectl apply -f deployments/k8s/frontal-lobe/
kubectl apply -f deployments/k8s/gateway/
kubectl apply -f deployments/k8s/cortex/

# Verify pods
kubectl get pods -l component

# Port-forward to test locally
kubectl port-forward svc/cortex 8080:8080
curl -s http://localhost:8080/v1/models
```

Each service deployment includes:
- Liveness and readiness probes (gRPC or HTTP)
- Resource requests and limits
- Non-root container user
- Configurable replicas

### Option 3: Run from Source

For development, run each service directly:

```bash
# Terminal 1 — Hippocampus (Memory)
cd services/hippocampus && go run ./cmd/server

# Terminal 2 — Frontal Lobe (Reasoning)
cd services/frontal_lobe && LLM_PROVIDER=mock go run ./cmd/server

# Terminal 3 — Gateway (Ingestion)
cd services/gateway && go run ./cmd/server

# Terminal 4 — Cortex (Orchestrator + HTTP API)
cd services/cortex && go run ./cmd/server
```

### Deployment Cost Estimates

All services are lightweight Go binaries with low memory footprints. Below are
approximate costs for common cloud providers.

| Method | Configuration | Estimated Monthly Cost |
|--------|--------------|------------------------|
| **Docker Compose on a VPS** | 1 × 2 vCPU / 4 GB VM (e.g., DigitalOcean, Hetzner) | **$12 – $24** |
| **Kubernetes (self-managed)** | 2-node cluster, 2 vCPU / 4 GB each | **$24 – $48** |
| **GKE Autopilot** | ~0.5 vCPU / 1 GB total across 4 pods | **$30 – $50** |
| **AWS ECS Fargate** | 4 tasks × 0.25 vCPU / 0.5 GB | **$25 – $45** |
| **Fly.io** | 4 machines × shared-cpu-1x / 256 MB | **$0 – $10** (free tier covers most) |

> **LLM API costs are separate.** Using `LLM_PROVIDER=mock` incurs no LLM cost.
> With OpenAI `gpt-4o-mini`, expect ~$0.15–$0.60 per 1M tokens. With Google
> Gemini Flash, cost is even lower. Self-hosted models (Ollama, vLLM) eliminate
> per-token costs but require GPU resources.

## CI/CD

- **CI Pipeline** (`.github/workflows/ci.yml`): Runs on every push/PR to `main`. Tests, lints, and builds all services.
- **Integration Tests** (`.github/workflows/integration.yml`): Runs the full end-to-end integration tests and doc example validation on every push/PR.
- **Release Pipeline** (`.github/workflows/release.yml`): Builds and pushes Docker images to GHCR on version tags (`v*`).
