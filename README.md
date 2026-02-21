# Second Brain – LLM Proxy & Agentic Memory Engine

Go/Gin server that exposes an **OpenAI-compatible** chat API and an **agentic memory** layer: the LLM decides when to read/write via tools (Notion user facts, Qdrant hybrid search).

## Layout

- **`/cmd/server`** – `main.go`, starts Gin on `PORT` (default 8080).
- **`/internal/api`** – OpenAI request/response types, `POST /v1/chat/completions` handler, router.
- **`/internal/llm`** – LLM interface and Gemini client (genai SDK, function calling, env-based config).
- **`/internal/memory`** – Working memory (last N messages), Notion user-fact store, Qdrant knowledge base (hybrid search), Notion page fetcher.
- **`/llm/Dockerfile`** – Multi-stage build; run with build context at repo root: `docker build -f ./llm/Dockerfile .`

## Environment

| Variable | Purpose |
|----------|---------|
| `PORT` | HTTP port (default `8080`). App reads this; do not substitute in Docker `CMD`. |
| `GOOGLE_API_KEY` | Gemini API key. |
| `GEMINI_CHAT_MODEL` | Chat model (default `gemini-2.5-flash`). |
| `GEMINI_EMBEDDING_MODEL` | Embedding model (default `text-embedding-004`). |
| `GEMINI_MAX_OUTPUT_TOKENS` | Max tokens (default 2048). |
| `GEMINI_TEMPERATURE` | Temperature (0–2). |
| `NOTION_TOKEN` | Notion API token (optional). |
| `NOTION_USER_PROFILE_PAGE_ID` | Notion page ID for user profile/facts (optional). |
| `QDRANT_HOST`, `QDRANT_PORT` | Qdrant host/port (optional). |
| `QDRANT_COLLECTION` | Collection name (default `knowledge`). |

## Memory & Tools

- **Working memory:** Last N messages of the current conversation (configurable).
- **User facts (Notion):** Tool `UpsertUserFact(fact)`. Profile page content is injected into the system prompt.
- **Knowledge base (Qdrant):** Tool `SearchKnowledgeBase(query)`. Lazy embedding + hybrid (dense + keyword) search; retrieved text is truncated to ~1500 tokens. Optional Notion fetcher for full page text.

## Build & Run

**Using Make (from repo root):**

```bash
make env          # copy .env.example → .env (once)
# Edit .env and set GOOGLE_API_KEY (and optionally Notion/Qdrant vars)
make build        # build Go binary
make run          # run binary (loads .env)
make up           # start full stack (server + Qdrant) via Docker Compose
make down         # stop full stack
make docker-build # build image
make docker-run   # run image with .env
make compose-up   # start Qdrant only (for local dev / tests)
make compose-down # stop Qdrant
make test-unit    # unit tests only
make test-integration  # integration tests (Docker Compose)
make test         # all tests
```

**Full stack with Docker Compose:** From repo root, `docker-compose.yml` runs the server and Qdrant. The server gets `QDRANT_HOST=qdrant` and `QDRANT_PORT=6334`; other vars come from `.env`. Run `make up` (or `docker compose up -d --build`) then open `http://localhost:8080/health`.

**Manual:**

```bash
cp .env.example .env   # then edit .env
go build -o server ./cmd/server
./server   # or: set -a && . ./.env && set +a && ./server

docker build -f ./llm/Dockerfile -t secondbrain .
docker run --rm -p 8080:8080 --env-file .env secondbrain
```

## Tests

- **Fakes:** Mock services (LLM, Notion fact store, Notion page fetcher) live in **`test/fakes/`** for integration tests without external APIs.
- **Fake Go server:** Integration tests use a **real HTTP server process** (`cmd/integration-server`), not just in-process mocks. That binary wires the same API with faked LLM/Notion and real Qdrant; tests start it, send HTTP requests to it, then assert on responses.
- **Docker Compose:** Integration tests use **`test/integration/docker-compose.yml`** to start Qdrant. The test builds and runs the integration-server, seeds Qdrant, then hits the server over HTTP.
- **Unit:** `go test ./... -short` (skips integration).
- **Integration:** `go test ./...` (requires Docker and `docker compose`). Starts Qdrant via compose, starts the fake Go server, runs tests, then tears down. Each test uses a free host port to avoid conflicts.

CI runs `go test -v ./...` and then builds/pushes the image to GHCR (see `.github/workflows/deploy.yml`).
