# Engram

An engram is a unit of cognitive information imprinted in the mind palace. It is a memory trace that represents a specific experience, event, or piece of information.

> You never forget anything, you just have to find the right engram.

Engram is the server-side metadata extraction layer for the mind palace archival system. It receives file uploads from client daemons, extracts rich metadata in the background, and provides an API to search and query indexed files.

## How It Works

```
Client Upload → Go API → Storage (fs/S3) + RabbitMQ → Python Worker → PostgreSQL
                                                            ↓
                                                   MIME detection, text extraction,
                                                   page count, auto-tagging
```

1. A client uploads a file to the Go backend API
2. The backend stores the file and publishes a job to RabbitMQ
3. The Python ingestion worker picks up the job, extracts metadata (MIME type, text content, page count), generates tags, and writes everything to PostgreSQL
4. The API serves queries against the indexed metadata — search by filename, filter by tags or device, download files

## Stack

- **Go** — Backend API server (stdlib `net/http`, no framework)
- **Python** — Ingestion worker for metadata extraction
- **PostgreSQL** — Metadata database (unix socket, no TCP)
- **RabbitMQ** — Job queue between API and worker
- **S3-compatible storage** — File storage (filesystem by default, S3/MinIO optional)
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

This launches a tmux session with three windows:
- **infra** — PostgreSQL + RabbitMQ (via process-compose)
- **backend** — Go API with hot reload (air)
- **ingestion** — Python worker (uv run)

Switch between windows with `Ctrl+b 0/1/2`. Detach with `Ctrl+b d`.

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
```

### Shell Commands

| Command | Description |
|---------|-------------|
| `bin/dev` | Launch full dev environment in tmux |
| `bin/start-infra` | Start PostgreSQL + RabbitMQ |
| `bin/shutdown-infra` | Stop infrastructure services |
| `source bin/load-infra-env` | Export `PGHOST`, `RABBITMQ_AMQP_PORT` into current shell |
| `bin/start-backend` | Start Go backend in a tmux window |
| `bin/start-ingestion` | Start Python worker in a tmux window |
| `bin/test-upload` | Run end-to-end integration test |

### Dev Shells

Multiple Nix shells are available depending on what you're working on:

```bash
nix develop           # Full shell (Go + Python + infra)
nix develop .#backend    # Go backend only
nix develop .#ingestion  # Python worker only
nix develop .#infra      # Infrastructure tools only
```

### Building

```bash
# Go backend
cd backend && go build -o engram-backend

# Python worker (dependencies managed by uv)
cd ingestion && uv sync
```

### Adding Dependencies

```bash
# Go
cd backend && go get <package>

# Python
cd ingestion && uv add <package>
```

### Resetting State

All runtime data (database, queues, stored files) lives in `.data/`. To start fresh:

```bash
rm -rf .data/
```

## API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/health` | Health check |
| `POST` | `/api/files` | Upload file (multipart: `file` + `metadata` JSON) |
| `GET` | `/api/files` | List/search files (`?q=`, `?tag=`, `?device=`, `?status=`) |
| `GET` | `/api/files/{id}` | Get file detail with extracted metadata |
| `GET` | `/api/files/{id}/download` | Download file |
| `GET` | `/api/tags` | List all tags with file counts |
| `GET` | `/api/devices` | List all devices |

### Upload Example

```bash
curl -X POST http://localhost:8080/api/files \
  -F "file=@document.pdf" \
  -F 'metadata={"filename":"document.pdf","size":204800,"path":"/home/user/Documents/","device_name":"laptop","hash":"sha256:abc123","mtime":"2026-03-16T12:00:00Z"}'
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | API server port |
| `PGHOST` | — | PostgreSQL unix socket directory |
| `RABBITMQ_AMQP_PORT` | `5672` | RabbitMQ AMQP port |
| `STORAGE_BACKEND` | `fs` | Storage backend: `fs` or `s3` |
| `STORAGE_FS_ROOT` | `.data/storage` | Filesystem storage root |
| `STORAGE_S3_ENDPOINT` | — | S3 endpoint URL |
| `STORAGE_S3_ACCESS_KEY` | — | S3 access key |
| `STORAGE_S3_SECRET_KEY` | — | S3 secret key |
| `STORAGE_S3_BUCKET` | `engram` | S3 bucket name |

In development, `PGHOST` and `RABBITMQ_AMQP_PORT` are set automatically by the Nix shell and `bin/load-infra-env`.
