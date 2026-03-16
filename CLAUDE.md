# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Engram is the server-side metadata extraction layer for the Mind Palace archival system. It receives file uploads from client daemons, extracts rich metadata via a background worker, and provides an API to query indexed file metadata.

## Architecture

```
Client Upload → Go Backend API → RabbitMQ (engram.ingest queue) → Python Ingestion Worker → PostgreSQL
                     ↓
              Storage (fs or S3)
```

- **Go backend** (`backend/`): HTTP API server using stdlib `net/http` with Go 1.22+ routing patterns. Handles uploads, stores files, publishes jobs to RabbitMQ, serves metadata queries.
- **Python ingestion worker** (`ingestion/`): Consumes from RabbitMQ, downloads files from storage, extracts metadata (MIME, text, page count), auto-tags, writes results to PostgreSQL.
- **Storage interface** (`backend/internal/storage/storage.go`): `Store` interface with filesystem (`fs.go`) and S3 (`s3.go`) implementations. Selected via `STORAGE_BACKEND` env var (`fs` default).
- **Database migrations** are embedded in the Go binary via `//go:embed` and run at startup using `golang-migrate`.

File lifecycle: upload → status `pending` → worker sets `processing` → extraction → status `ready` (or `failed`).

## Development

### Prerequisites

Nix with flakes enabled.

### Quick Start

```bash
nix develop        # Enter full dev shell (Go + Python + infra tools)
bin/dev            # Launches tmux session: infra (PG + RabbitMQ), backend (air), ingestion (uv run)
```

### Individual Components

```bash
bin/start-infra           # Start PostgreSQL + RabbitMQ via process-compose
source bin/load-infra-env # Export PGHOST, RABBITMQ_AMQP_PORT, RABBITMQ_MGMT_PORT
bin/shutdown-infra        # Stop services
```

### Build & Run

```bash
# Backend
cd backend && go build ./...
cd backend && air                    # Hot reload (needs infra env vars)

# Ingestion worker
cd ingestion && uv run main.py       # Needs infra env vars

# Integration test (requires all services running)
bin/test-upload
```

### Dev Shells

| Shell | Command | Contents |
|-------|---------|----------|
| `infra` | `nix develop .#infra` | PostgreSQL, RabbitMQ, process-compose |
| `backend` | `nix develop .#backend` | infra + Go, gopls, air |
| `ingestion` | `nix develop .#ingestion` | infra + Python 3.13, uv, ruff, libmagic |
| `full` (default) | `nix develop` | backend + ingestion combined |

### Infrastructure Notes

- **PostgreSQL** uses unix socket only (no TCP). Socket at `.data/postgres/`. `PGHOST` points there.
- **RabbitMQ** uses dynamically assigned ports written to `.data/rabbitmq/{amqp_port,mgmt_port}`.
- All runtime data lives in `.data/` (gitignored). Delete it to reset state: `rm -rf .data/`

## Key Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PGHOST` | (required) | PostgreSQL unix socket directory |
| `RABBITMQ_AMQP_PORT` | `5672` | RabbitMQ AMQP port |
| `STORAGE_BACKEND` | `fs` | `fs` or `s3` |
| `STORAGE_FS_ROOT` | `.data/storage` | Filesystem storage root |
| `STORAGE_S3_ENDPOINT` | — | S3 endpoint (required when `s3`) |
| `STORAGE_S3_ACCESS_KEY` | — | S3 access key (required when `s3`) |
| `STORAGE_S3_SECRET_KEY` | — | S3 secret key (required when `s3`) |
| `STORAGE_S3_BUCKET` | `engram` | S3 bucket name |

## API Endpoints

- `POST /api/files` — Multipart upload (`file` + `metadata` JSON with filename, size, path, device_name, hash, mtime)
- `GET /api/files` — List/search (params: `q`, `tag`, `device`, `status`, `limit`, `offset`)
- `GET /api/files/{id}` — Full file detail with extracted_text and tags
- `GET /api/files/{id}/download` — Download file (S3: 302 redirect to presigned URL; fs: stream)
- `GET /api/tags` — All tags with file counts
- `GET /api/devices` — All devices
- `GET /api/health` — Health check

## Conventions

- Go backend uses no web framework — stdlib `net/http` only.
- Python dependencies managed by `uv` (not pip). Add deps with `uv add <pkg>` from `ingestion/`.
- Nix infra processes are defined in `infra/*.nix` and composed into `process-compose.yaml` via `flake.nix`.
- PostgreSQL connections always use unix sockets for local dev. Do not configure TCP listeners.
- RabbitMQ queue `engram.ingest` is declared as durable by both the Go publisher and Python consumer.
