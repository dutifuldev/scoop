# SCOOP

SCOOP is a news intelligence platform (newsint) built for agent-driven news collection.

It is designed to be used by AI agents (e.g. OpenClaw): agents browse the web, build canonical news item JSON, and call SCOOP CLI commands to ingest items.

SCOOP then:
- stores raw arrivals safely
- normalizes them into canonical documents
- generates embeddings
- auto-deduplicates documents into canonical stories
- serves an API + modern web UI to explore stories and their merged source items

## Core Model

- `news item`: one ingested article/document from a source
- `story`: a deduplicated cluster of related items
- `collection`: hard dedup boundary (for example `ai_news`, `world_news`, `china_news`)

In plain terms: many items can collapse into one story, while still keeping every original source item traceable.

## What Works Today

- CLI ingestion (`scoop ingest`) with JSON schema validation
- Pipeline stages: `normalize`, `embed`, `dedup`, `process`
- Semantic + lexical + exact-match dedup with audit signals
- Echo API (`/api/v1/...`) with JSend responses
- React + TanStack + Tailwind story explorer UI
- Story/item deep-link routes (`/c/<collection>/s/<story>/i/<item>`)

## Architecture

- `backend/` (Go, GORM, Echo): CLI + API + pipeline logic
- `frontend/` (Vite, TypeScript, React, TanStack): viewer UX
- `embedding-service/` (Python): embedding HTTP service used by backend pipeline

## Quick Start (Local)

1. Bootstrap repo dependencies and hooks:

```bash
make bootstrap
```

```bash
# alternate
./scripts/bootstrap.sh
```

2. Start backend API:

```bash
cd backend
```

```bash
go run ./cmd/scoop serve --env .env --host 0.0.0.0 --port 8090
```

3. Start frontend dev server:

```bash
cd frontend
```

```bash
pnpm dev
```

4. Open the UI:

- `http://127.0.0.1:5173`

Vite proxies `/api/*` to `http://127.0.0.1:8090` by default.

## OpenClaw -> SCOOP Flow

1. OpenClaw scrapes pages and builds canonical `v1` news item JSON.
2. OpenClaw calls:

```bash
cd backend
```

```bash
go run ./cmd/scoop ingest --env .env --payload-file /path/to/item.json --triggered-by-topic ai_news
```

3. SCOOP processes pending items:

```bash
cd backend
```

```bash
go run ./cmd/scoop process --env .env
```

4. UI/API shows deduplicated stories and all merged source items.

## Ranking and Weighting Customization

SCOOP is intentionally designed so feed ranking is customizable per use case.

- Put source-specific signals (scores, tags, trust hints) into `source_metadata`.
- Keep collection-specific behavior by using collection labels as dedup boundaries.
- Customize story ordering/weighting in backend query logic (default is recency).
- Frontend feed renders whatever ranked order the API returns.

This lets teams define their own news feed behavior for any source mix without changing the ingestion contract.

## Key Docs

- Backend details and full pipeline: `backend/README.md`
- Canonical payload schema decision: `backend/NEWS_ITEM_SCHEMA.md`
- Embedding service usage: `embedding-service/README.md`

## License

MIT (`LICENSE`)
