# Canonical File Events

All file-event producers publish JSON messages with this shape to
`engram.ingest`:

| Field | Required | Description |
|---|---|---|
| `event` | yes | `create`, `delete`, or `rename` |
| `file_path` | yes | Absolute path for `fs`; object key for `s3`; storage identity |
| `filename` | yes | User-facing display path. For S3 folder uploads this is the sanitized relative upload path, such as `docs/myfile.pdf` |
| `size` | create/rename | Size in bytes |
| `hash` | create/rename | SHA-256 as `sha256:<hex>` |
| `mtime` | create/rename | UTC RFC3339 timestamp |
| `device_name` | yes | Producer or host identifier |
| `storage_type` | yes | `fs` or `s3` |
| `old_file_path` | rename | Previous path or object key |

Delivery is at least once. Consumers must identify a stored file by
`(storage_type, file_path)` and handle repeated messages idempotently.
Clients should use `filename` for display and folder grouping, and treat
`file_path` as storage identity.

The JSON files in this directory are contract fixtures shared by producer and
consumer tests.
