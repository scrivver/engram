# Engram Architecture

## Overview

Engram is the server-side metadata extraction layer for the Mind Palace archival system. It indexes files from external storage (local filesystem or S3-compatible object storage), extracts rich metadata, and provides a read-only API for querying the index.

No files are uploaded through or downloaded from the API. Files live in their original storage location — engram only tracks metadata about them.

## Components

```
┌─────────────────────┐     ┌──────────────────────────┐
│  Filesystem Storage  │     │  S3-compatible Storage   │
│  (local directories) │     │  (MinIO, AWS S3, etc.)   │
└────────┬────────────┘     └────────────┬─────────────┘
         │                               │
         ▼                               ▼
┌─────────────────────┐     ┌──────────────────────────┐
│  Go Watcher          │     │  Storage-owning Backend   │
│  (fsnotify)          │     │  (for example Reliquary)  │
└────────┬────────────┘     └────────────┬─────────────┘
         │                               │
         └───────────┐   ┌──────────────┘
                     ▼   ▼
              ┌──────────────────┐
              │    RabbitMQ      │
              │  engram.ingest   │
              └────────┬─────────┘
                       │
                       ▼
              ┌──────────────────┐
              │  Python Worker   │
              │  (ingestion)     │
              │                  │
              │  - read file     │
              │  - detect MIME   │
              │  - extract text  │
              │  - auto-tag      │
              └────────┬─────────┘
                       │
                       ▼
              ┌──────────────────┐
              │   PostgreSQL     │
              │                  │
              │  files, devices  │
              │  tags, file_tags │
              └────────┬─────────┘
                       │
                       ▼
              ┌──────────────────┐
              │  Go Backend API  │
              │  (read-only)     │
              │                  │
              │  /api/files      │
              │  /api/tags       │
              │  /api/devices    │
              └──────────────────┘
```

## Component Details

### Go Backend API (`backend/`)

A read-only HTTP server that queries PostgreSQL and returns file metadata. It has no storage or queue dependencies.

At startup it connects to PostgreSQL (via unix socket), runs embedded database migrations, and starts serving HTTP.

**Endpoints:**
- `GET /api/files` — list and search files by name, tag, device, or status; `scope=folder&path=<dir>` narrows it to one directory's direct children
- `GET /api/files/{id}` — single file with full metadata including extracted text and tags
- `GET /api/folders` — immediate subfolders of `path`, with recursive file counts
- `GET /api/tags` — all tags with file counts
- `GET /api/devices` — all registered devices
- `GET /api/health` — liveness check

Directory structure is derived from `filename`, which the file-event contract
defines as the user-facing display path (`docs/myfile.pdf`), not from
`file_path`, which is storage identity (`files/alice/2026/07/docs/myfile.pdf`).
Prefix matching escapes LIKE metacharacters and is served by
`idx_files_owner_filename`.

Uses Go stdlib `net/http` with 1.22+ routing patterns. No external web framework.

### Go Filesystem Watcher (`watcher/`)

A separate binary that monitors local directories for new or modified files. When a file is detected:

1. Wait for writes to stabilize (1 second debounce)
2. Compute SHA256 hash
3. Publish an event to the `engram.ingest` RabbitMQ queue

Watches directories recursively using `fsnotify`. Automatically picks up new subdirectories as they are created.

This is deployed independently from the backend — it runs wherever the files are.

### S3-Backed Events

For S3-compatible storage, the backend that performs the object mutation publishes
the same canonical file event after the operation succeeds. Engram does not require
the S3 provider to support native notifications.

The Python worker temporarily handles both canonical events and legacy MinIO/S3
notifications during migration. New producers must publish the canonical format.

### Python Ingestion Worker (`ingestion/`)

Consumes events from the `engram.ingest` RabbitMQ queue. For each event:

1. Parse the message (detect if it's a filesystem watcher event or S3 notification)
2. Insert a file record into PostgreSQL with status `pending`
3. Read the file from its storage location (filesystem path or S3 download)
4. Detect MIME type using `libmagic`
5. Extract metadata based on file type:
   - PDF: text content and page count (pymupdf)
   - Images: dimensions (Pillow)
   - Text files: raw content (first 100KB)
   - Other: MIME type only
6. Generate tags using rule-based matching (MIME category, file extension, filename patterns)
7. Update the database record with extracted metadata and tags, set status to `ready`

If processing fails, the file status is set to `failed` and the message is not requeued.

### PostgreSQL

Stores all metadata. Connected via unix socket in development (no TCP).

**Tables:**
- `devices` — source devices identified by name
- `files` — file metadata, extraction results, and processing status
- `tags` — distinct tag names
- `file_tags` — many-to-many relationship between files and tags

Storage identity is enforced by `(storage_type, file_path)`. Hash and owner remain
indexed content metadata and may be used by higher-level deduplication workflows.

**Status lifecycle:** `pending` → `processing` → `ready` | `failed`

### RabbitMQ

Single durable queue `engram.ingest` used by all event producers and consumed by the
Python worker. Messages are persistent and delivered at least once, so processing is
idempotent by `(storage_type, file_path)`.

## Message Formats

### Canonical File Event

Published by the Go watcher or a storage-owning backend:

```json
{
  "event": "create",
  "file_path": "/srv/files/documents/report.pdf",
  "filename": "report.pdf",
  "size": 204800,
  "hash": "sha256:abcdef123456",
  "mtime": "2026-03-16T12:00:00Z",
  "device_name": "server1",
  "storage_type": "fs"
}
```

### Legacy S3 Bucket Notification

Accepted temporarily from existing MinIO/S3 integrations:

```json
{
  "EventName": "s3:ObjectCreated:Put",
  "Key": "engram/documents/report.pdf",
  "Records": [{
    "s3": {
      "bucket": { "name": "engram" },
      "object": { "key": "documents/report.pdf", "size": 204800, "eTag": "abcdef" }
    },
    "eventTime": "2026-03-16T12:00:00Z"
  }]
}
```

The Python worker detects the legacy format by checking for `"EventName"` or
`"Records"`. This compatibility path will be removed after all producers publish
canonical events. The contract fixtures live in `contracts/file-events/`.

## Two Ingestion Paths

### Path 1: Local Filesystem

```
File written to disk → Go watcher detects via fsnotify → publishes event → Python worker reads file directly → metadata stored
```

The watcher and worker must run on the same machine (or have access to the same filesystem).

### Path 2: S3-Compatible Storage

```
Backend uploads file to S3 → backend publishes canonical event → Python worker downloads file via boto3 → metadata stored → temp file cleaned up
```

The worker needs S3 credentials (`STORAGE_S3_ENDPOINT`, `STORAGE_S3_ACCESS_KEY`, `STORAGE_S3_SECRET_KEY`) to download files.

## Database Schema

```sql
devices (id, name, created_at)
files   (id, filename, size, hash, file_path, device_id, status,
         storage_type, mime_type, page_count, extracted_text,
         mtime, created_at, updated_at)
tags    (id, name)
file_tags (file_id, tag_id)
```

Key indexes:
- `idx_files_storage_path` (unique) — event identity and idempotency
- `idx_files_hash_owner` — content lookup by hash and owner
- `idx_files_device` — query by device
- `idx_files_status` — filter by processing status
- `idx_files_filename` — filename search

## Deployment

Each component can be deployed independently:

| Component | Binary | Requires |
|-----------|--------|----------|
| Backend API | `backend/` | PostgreSQL |
| Filesystem Watcher | `watcher/` | RabbitMQ, access to watched directories |
| Ingestion Worker | `ingestion/` | PostgreSQL, RabbitMQ, access to files (fs or S3) |

The backend only needs PostgreSQL. The watcher only needs RabbitMQ. The worker needs both PostgreSQL and RabbitMQ, plus access to the files being indexed.

Multiple watcher instances can run on different machines, each monitoring different directories with different device names, all feeding into the same RabbitMQ queue.
