# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Engram is the server-side metadata extraction layer for the Mind Palace archival system. Files arrive via canonical storage events, metadata is extracted by a background worker, and a read-only API provides search and query access to the indexed metadata.

## Architecture

```
Path 1 (fs):   Go watcher (fsnotify) → RabbitMQ → Python worker → PostgreSQL
Path 2 (S3):   Storage backend → RabbitMQ → Python worker → PostgreSQL

API:            PostgreSQL ← read-only queries ← clients
```

Three separate components:

- **Go backend API** (`backend/`): Read-only metadata queries using stdlib `net/http`. Only depends on PostgreSQL — no storage, no RabbitMQ.
- **Go filesystem watcher** (`watcher/`): Separate binary. Watches directories via fsnotify, publishes file events to RabbitMQ. Deployed independently.
- **Python ingestion worker** (`ingestion/`): Consumes events from RabbitMQ, reads files (from filesystem or S3), extracts metadata (MIME, text, page count), auto-tags, inserts records into PostgreSQL.

**No file upload or download through the API.** Files are accessed directly from storage.

Database migrations are embedded in the Go backend binary via `//go:embed` and run at startup using `golang-migrate`.

File lifecycle: event received → worker inserts record as `pending` → sets `processing` → extraction → `ready` (or `failed`).

## Development

### Prerequisites

Nix with flakes enabled.

### Quick Start

```bash
nix develop        # Enter full dev shell
bin/dev            # Launches tmux: infra, backend, ingestion, watcher
```

Drop files into `.data/watch/` to trigger ingestion.

### Build & Run

```bash
# Backend (read-only API)
cd backend && go build ./...
cd backend && air                    # Hot reload (needs PGHOST)

# Watcher
cd watcher && go build ./...
cd watcher && WATCH_DIRS=.data/watch go run .   # Needs RABBITMQ_AMQP_PORT

# Ingestion worker
cd ingestion && uv run main.py       # Needs PGHOST + RABBITMQ_AMQP_PORT

# Integration test (requires all services running)
bin/test-ingest
```

## Packaged Outputs

Engram owns its deployment package and image outputs:

- `.#backend`: Go read-only metadata API package.
- `.#api-container`: `engram-api:latest`, running the API on port `8081` with
  `engram-api-healthcheck` probing `/api/health`.
- `.#ingestion`: packaged Python ingestion runtime.
- `.#ingestion-container`: `engram-ingestion:latest`, running the ingestion
  worker with CA certificates and extraction tools (`file`, `ffmpeg`,
  `poppler-utils`, `tesseract`) included.

Mind Palace root deployment builds these child outputs with
`nix build path:$PROJECT_ROOT/engram#api-container` and
`nix build path:$PROJECT_ROOT/engram#ingestion-container`, then tags the loaded
images to the root Compose names. Do not duplicate Engram Go vendor hashes,
Python dependency materialization, entrypoints, or healthcheck helpers in the
Mind Palace root repo.

### Dev Shells

| Shell | Command | Contents |
|-------|---------|----------|
| `infra` | `nix develop .#infra` | PostgreSQL, RabbitMQ, MinIO, process-compose |
| `backend` | `nix develop .#backend` | infra + Go, gopls, air |
| `watcher` | `nix develop .#watcher` | same as backend |
| `ingestion` | `nix develop .#ingestion` | infra + Python 3.13, uv, ruff, libmagic |
| `full` (default) | `nix develop` | backend + ingestion combined |

### Infrastructure Notes

- **PostgreSQL** uses unix socket only (no TCP). Socket at `.data/postgres/`. `PGHOST` points there.
- **RabbitMQ** uses dynamically assigned ports written to `.data/rabbitmq/{amqp_port,mgmt_port}`. Queue `engram.ingest` and its binding to `amq.direct` are declared via `load_definitions` in the RabbitMQ config (see `infra/rabbitmq.nix`).
- **MinIO** provides S3-compatible storage in development. Ports are written to `.data/minio/{api_port,console_port}`.
- All runtime data lives in `.data/` (gitignored). Delete it to reset state: `rm -rf .data/`

## Key Environment Variables

### Backend API
| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP listen port |
| `PGHOST` | (required) | PostgreSQL unix socket directory |
| `JWT_SECRET` | — | Shared HMAC secret for validating Reliquary-issued JWTs. Enables JWT auth when set. |
| `OIDC_ISSUER_URL` | — | OIDC issuer URL. Enables OIDC token validation via userinfo when set. |
| `OIDC_USERNAME_CLAIM` | `preferred_username` | Userinfo claim used as the owner identity |

If **both** `JWT_SECRET` and `OIDC_ISSUER_URL` are set, the backend runs in
mixed mode: it tries JWT validation first, then falls back to OIDC. If neither
is set, requests are unauthenticated.

### Watcher
| Variable | Default | Description |
|----------|---------|-------------|
| `WATCH_DIRS` | (required) | Comma-separated directories to watch |
| `DEVICE_NAME` | hostname | Device identifier |
| `RABBITMQ_AMQP_PORT` | `5672` | RabbitMQ AMQP port |
| `WATCH_IGNORE` | — | Extra comma-separated ignore patterns |

### Ingestion Worker
| Variable | Default | Description |
|----------|---------|-------------|
| `PGHOST` | (required) | PostgreSQL unix socket directory |
| `RABBITMQ_AMQP_PORT` | `5672` | RabbitMQ AMQP port |

S3 env vars (`STORAGE_S3_ENDPOINT`, `STORAGE_S3_ACCESS_KEY`, `STORAGE_S3_SECRET_KEY`, `STORAGE_S3_BUCKET`) only needed when processing S3 events. In dev, `bin/load-infra-env` exports these automatically when MinIO is running.

## API Endpoints (read-only)

- `GET /api/health` — Health check
- `GET /api/files` — List/search (params: `q`, `tag`, `device`, `status`, `type`, `from`, `to`, `sort`, `limit`, `offset`, `scope`, `path`)
- `GET /api/files/{id}` — Full file detail with extracted_text and tags
- `GET /api/folders` — Immediate subfolders of `path` with recursive file counts
- `GET /api/tags` — All tags with file counts
- `GET /api/devices` — All devices

### Folder browsing

`scope=folder` restricts `/api/files` to the direct children of `path`; `scope=all`
(the default) ignores `path` and lists everything. `/api/folders` returns the
complete, unpaginated set of subfolders for that directory — clients must not
derive the folder tree from a page of files, which is what made the tree depend
on scroll position.

Both endpoints apply the same filters through one shared builder, so a folder's
count can never disagree with the files it contains. Folders are derived from
`filename` (the user-facing display path from the file-event contract), never
from `file_path`, which is storage identity.

## Conventions

- Go backend uses no web framework — stdlib `net/http` only.
- Backend has no storage or queue dependencies — only PostgreSQL.
- The watcher is a separate Go module (`watcher/`) with its own `go.mod`.
- Python dependencies managed by `uv` (not pip). Add deps with `uv add <pkg>` from `ingestion/`.
- Nix infra processes are defined in `infra/*.nix` and composed into `process-compose.yaml` via `flake.nix`.
- PostgreSQL connections always use unix sockets for local dev. Do not configure TCP listeners.
- RabbitMQ queue `engram.ingest` is declared declaratively via `load_definitions` in `infra/rabbitmq.nix`. Do not rely on application code to create queues.
- New producers publish the canonical event contract in `contracts/file-events/`.
- The Python worker temporarily accepts legacy S3 bucket notifications during migration.
- File event identity is `(storage_type, file_path)`; delivery is at least once.
- The Python worker auto-reconnects to RabbitMQ with exponential backoff on connection loss.
- The watcher publishes `create`, `delete`, and `rename` events. It ignores dotfiles, `.git`, `node_modules`, `__pycache__`, and other common patterns by default.
