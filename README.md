# Engram

An engram is a unit of cognitive information imprinted in the mind palace. It is a memory trace that represents a specific experience, event, or piece of information.

> You never forget anything, you just have to find the right engram.

Engram is the server-side metadata extraction layer for the mind palace archival system. Files arrive via storage events (filesystem changes or S3 bucket notifications), metadata is extracted in the background, and a read-only API provides search and query access to the indexed metadata.

## How It Works

```
Path 1 (filesystem):  Go watcher (fsnotify) → RabbitMQ → Python worker → PostgreSQL
Path 2 (S3):          MinIO/S3 bucket notification → RabbitMQ → Python worker → PostgreSQL

API:                  PostgreSQL ← read-only queries ← clients
```

1. A file appears in storage (written to a watched directory, or uploaded to an S3 bucket)
2. An event is published to RabbitMQ (by the Go watcher for filesystem, or by MinIO natively for S3)
3. The Python ingestion worker picks up the event, reads the file, extracts metadata (MIME type, text content, page count), generates tags, and writes everything to PostgreSQL
4. The API serves read-only queries against the indexed metadata — search by filename, filter by tags or device

## Stack

- **Go** — Backend API server (read-only metadata queries) and filesystem watcher (separate binary)
- **Python** — Ingestion worker for metadata extraction
- **PostgreSQL** — Metadata database (unix socket, no TCP)
- **RabbitMQ** — Event queue between storage events and worker
- **Nix flakes** — Development environment and service orchestration

## Prerequisites

- [Nix](https://nixos.org/download/) with flakes enabled

That's it. Nix provides Go, Python, PostgreSQL, RabbitMQ, and all other tools.

## Setup

```bash
git clone <repo-url> && cd engram
nix develop    # Enter the development shell
```

## Development

### Quick Start (recommended)

```bash
bin/dev
```

This launches a tmux session with four windows:
- **infra** — PostgreSQL + RabbitMQ (via process-compose)
- **backend** — Go API with hot reload (air)
- **ingestion** — Python worker (uv run)
- **watcher** — Go filesystem watcher (watches `.data/watch/`)

Switch between windows with `Ctrl+b 0/1/2/3`. Detach with `Ctrl+b d`.

To test, drop a file into `.data/watch/` and query the API:

```bash
cp some-file.pdf .data/watch/
curl http://localhost:8080/api/files?status=ready
```

### Manual Setup

If you prefer separate terminals:

```bash
# Terminal 1: Start infrastructure
bin/start-infra

# Terminal 2: Start Go backend
source bin/load-infra-env
cd backend && air

# Terminal 3: Start ingestion worker
source bin/load-infra-env
cd ingestion && uv run main.py

# Terminal 4: Start filesystem watcher
source bin/load-infra-env
cd watcher && WATCH_DIRS=.data/watch go run .
```

### Shell Commands

| Command | Description |
|---------|-------------|
| `bin/dev` | Launch full dev environment in tmux |
| `bin/start-infra` | Start PostgreSQL + RabbitMQ |
| `bin/shutdown-infra` | Stop infrastructure services |
| `source bin/load-infra-env` | Export `PGHOST`, `RABBITMQ_AMQP_PORT` into current shell |
| `bin/start-backend` | Start Go API in a tmux window |
| `bin/start-ingestion` | Start Python worker in a tmux window |
| `bin/start-watcher` | Start filesystem watcher in a tmux window |
| `bin/test-ingest` | Run end-to-end integration test |

### Dev Shells

```bash
nix develop              # Full shell (Go + Python + infra)
nix develop .#backend    # Go backend only
nix develop .#watcher    # Go watcher (same as backend)
nix develop .#ingestion  # Python worker only
nix develop .#infra      # Infrastructure tools only
```

### Building

```bash
# Go backend
cd backend && go build -o engram-backend

# Go watcher
cd watcher && go build -o engram-watcher

# Python worker (dependencies managed by uv)
cd ingestion && uv sync
```

### Adding Dependencies

```bash
# Go (backend or watcher)
cd backend && go get <package>
cd watcher && go get <package>

# Python
cd ingestion && uv add <package>
```

### Resetting State

All runtime data (database, queues, watched files) lives in `.data/`. To start fresh:

```bash
rm -rf .data/
```

## API

The backend API is read-only — it queries metadata from PostgreSQL. No file upload or download.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/files` | List/search files (`?q=`, `?tag=`, `?device=`, `?status=`) |
| `GET` | `/api/files/{id}` | Get file detail with extracted metadata and tags |
| `GET` | `/api/tags` | List all tags with file counts |
| `GET` | `/api/devices` | List all devices |

## Configuration

### Backend API

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | API server port |
| `PGHOST` | — | PostgreSQL unix socket directory |

### Filesystem Watcher

| Variable | Default | Description |
|----------|---------|-------------|
| `WATCH_DIRS` | — | Comma-separated directories to watch |
| `DEVICE_NAME` | hostname | Device identifier |
| `RABBITMQ_AMQP_PORT` | `5672` | RabbitMQ AMQP port |

### Ingestion Worker

| Variable | Default | Description |
|----------|---------|-------------|
| `PGHOST` | — | PostgreSQL unix socket directory |
| `RABBITMQ_AMQP_PORT` | `5672` | RabbitMQ AMQP port |
| `STORAGE_S3_ENDPOINT` | — | S3 endpoint (only for S3 storage type) |
| `STORAGE_S3_ACCESS_KEY` | — | S3 access key |
| `STORAGE_S3_SECRET_KEY` | — | S3 secret key |
| `STORAGE_S3_BUCKET` | `engram` | S3 bucket name |

In development, `PGHOST` and `RABBITMQ_AMQP_PORT` are set automatically by the Nix shell and `bin/load-infra-env`.
