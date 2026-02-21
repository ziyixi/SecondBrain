# Second Brain — Tech details

This document covers layout, configuration, build, run, testing, and CI. For **why this project** and **how to get started**, see [README.md](README.md).

---

## Layout

| Path | Purpose |
|------|--------|
| `cmd/server` | Main entrypoint; starts Gin on `PORT` (default 8080), wires config, memory, and tools. |
| `cmd/integration-server` | Test-only server (faked LLM/Notion, real Qdrant) for integration tests; no Memorizer. |
| `internal/api` | OpenAI request/response types, `POST /v1/chat/completions` handler, router, health, tool execution. |
| `internal/config` | Central env-based config (tunables, defaults); see Environment table below. |
| `internal/llm` | LLM interface and Gemini client (genai SDK, function calling, env-based model config). |
| `internal/memory` | Working memory, Notion user-fact store, Notion knowledge store, Qdrant KB (hybrid search), memorizer (dynamic categorization). |
| `llm/Dockerfile` | Multi-stage build; use from repo root: `docker build -f ./llm/Dockerfile .` |
| `test/fakes` | Scripted LLM, in-memory fact store, Notion page fetcher, FakeNotionKnowledgeStore for tests. |
| `test/integration` | Docker Compose (Qdrant gRPC + REST) and integration test helpers. |

---

## Environment

| Variable | Purpose |
|----------|---------|
| `PORT` | HTTP port (default `8080`). |
| `GOOGLE_API_KEY` | Gemini API key (required for chat/embed). |
| `GEMINI_CHAT_MODEL` | Chat model (default `gemini-2.5-flash`). |
| `GEMINI_EMBEDDING_MODEL` | Embedding model (default `text-embedding-004`). |
| `GEMINI_MAX_OUTPUT_TOKENS` | Max tokens (default 2048). |
| `GEMINI_TEMPERATURE` | Temperature (0–2). |
| `NOTION_TOKEN` | Notion API token (optional). From [Notion Integrations](https://www.notion.so/my-integrations) → Internal Integration Secret. |
| `NOTION_USER_PROFILE_PAGE_ID` | Notion **page** ID for user profile/facts (optional). See [README: Setting up Notion](README.md#setting-up-notion) for how to get page ID from URL. |
| `NOTION_KNOWLEDGE_DATABASE_ID` | Notion **database** ID for knowledge pages (required for MemorizeInformation). Database must have a Title property (default name `Name`). See README for setup. |
| `NOTION_KNOWLEDGE_TITLE_PROPERTY` | Title property name in that database (default `Name`). |
| `QDRANT_HOST`, `QDRANT_PORT` | Qdrant host/port (optional). |
| `QDRANT_COLLECTION` | Collection name (default `knowledge`). |
| **Tunables** (see `internal/config`) | |
| `MEMORIZE_TOPIC_SIMILARITY_THRESHOLD` | Cosine similarity to match existing topic (default `0.85`). |
| `WORKING_MEMORY_SIZE` | Last N messages in conversation (default `20`). |
| `CHAT_TOOL_LOOP_MAX_ITER` | Max tool-call rounds per request (default `10`). |
| `CHAT_DEFAULT_MAX_TOKENS` | Default when request omits (else `GEMINI_MAX_OUTPUT_TOKENS`). |
| `CHAT_DEFAULT_TEMPERATURE` | Default when request omits (else `GEMINI_TEMPERATURE`). |
| `KB_MAX_TEXT_CHARS` | Truncate fetched Notion text (default `6000`). |
| `KB_RRF_K` | RRF constant for hybrid scoring (default `60`). |
| `KB_SEARCH_PREFETCH_MIN` | Min points to prefetch for re-rank (default `20`). |

Copy `.env.example` to `.env` and fill in values.

---

## Memory & tools

- **Working memory** — Last N messages of the current conversation (configurable). Used as in-context history for the LLM.
- **User facts (Notion)** — Tool `UpsertUserFact(fact)`. Profile page content is injected into the system prompt; the model can append facts via the tool.
- **Knowledge base (Notion + Qdrant)** — The **knowledge graph lives in a Notion database**; storing full content in Qdrant would be too large and not scale. So:
  - **Qdrant** stores only **vectors and Notion page ID** per point (no document text). It is a lightweight vector index for retrieval.
  - **Notion** is the source of truth for all knowledge content. At index time you embed Notion pages and upsert (vector, page_id) into Qdrant. At query time the server runs vector search in Qdrant, gets back page IDs, then **fetches full text from Notion** via the Notion page fetcher. Retrieved text is truncated (see `KB_MAX_TEXT_CHARS`, default 6000) before being sent to the model. Optional keyword re-rank (RRF, see `KB_RRF_K`) is applied over the fetched text for hybrid scoring.
- **MemorizeInformation (dynamic categorization)** — Tool `MemorizeInformation(topic, content)`. The backend: (1) embeds the topic and runs a similarity search in Qdrant for existing **topic** points (cosine similarity ≥ `MEMORIZE_TOPIC_SIMILARITY_THRESHOLD`, default 0.85). (2) If a match is found: appends `content` to that Notion page and inserts a new **content** vector in Qdrant with the same `notion_page_id`. (3) If no match: creates a new page in the Notion knowledge database (title = topic, body = content), then inserts both a **topic** vector and a **content** vector in Qdrant. Payload `type` is `"topic"` or `"content"` so topic search can filter correctly. This avoids Notion page fragmentation by clustering under similar categories.

---

## Build & run

**Make (from repo root):**

```bash
make env              # copy .env.example → .env (once)
make build            # build Go binary (./server)
make run               # run binary (loads .env)
make up                # full stack: server + Qdrant (docker-compose.yml)
make down              # stop full stack
make docker-build      # build Docker image
make docker-run        # run image with .env
make compose-up       # Qdrant only (test stack)
make compose-down     # stop Qdrant
make test-unit        # unit tests only (-short)
make test-integration # integration tests (Docker Compose)
make test             # all tests
```

**Full stack (Docker Compose):** `docker-compose.yml` at repo root runs the server and Qdrant. The server gets `QDRANT_HOST=qdrant` and `QDRANT_PORT=6334`; other vars from `.env`. Run `make up` or `docker compose up -d --build`, then `http://localhost:8080/health`.

**Manual:**

```bash
cp .env.example .env   # then edit .env
go build -o server ./cmd/server
./server   # or: set -a && . ./.env && set +a && ./server

docker build -f ./llm/Dockerfile -t secondbrain .
docker run --rm -p 8080:8080 --env-file .env secondbrain
```

---

## Tests

- **Config** — `internal/config/config_test.go`: default values and env overrides.
- **Fakes** — `test/fakes/`: scripted LLM, in-memory fact store, Notion page fetcher, `FakeNotionKnowledgeStore` (implements `NotionKnowledgeWriter` + `NotionPageFetcher` for MemorizeInformation tests).
- **Fake Go server** — Integration tests start a real HTTP server (`cmd/integration-server`) with faked LLM/Notion and real Qdrant, then send HTTP requests to it (memory search flow).
- **Memorizer integration** — `internal/memory`: `TestMemorizer_Integration` uses compose (Qdrant) + in-memory fake Notion to test MemorizeInformation create-path and Search.
- **Docker Compose** — `test/integration/docker-compose.yml` starts Qdrant (gRPC + REST ports). Tests use free host ports and wait for Qdrant `/readyz` before running.
- **Unit:** `go test ./... -short` (skips integration).
- **Integration:** `go test ./...` or `make test-integration` (requires Docker). Runs api + memory integration tests (memory search, Memorizer, Qdrant KB).

CI runs `go test -v ./...` (unit + integration) and builds/pushes the image to GHCR (see `.github/workflows/deploy.yml`).

---

## Docker and compose

- **Production-style stack:** Root `docker-compose.yml` — services `server` (build from `llm/Dockerfile`) and `qdrant`; server depends on Qdrant health.
- **Integration tests:** `test/integration/docker-compose.yml` — Qdrant only; env `QDRANT_GRPC_PORT` and `QDRANT_HTTP_PORT` for host ports (gRPC and REST for `/readyz`).
