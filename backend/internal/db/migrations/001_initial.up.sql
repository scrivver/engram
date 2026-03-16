CREATE TABLE IF NOT EXISTS devices (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS files (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename            TEXT NOT NULL,
    size                BIGINT NOT NULL,
    hash                TEXT NOT NULL,
    original_path       TEXT NOT NULL,
    device_id           UUID NOT NULL REFERENCES devices(id),
    status              TEXT NOT NULL DEFAULT 'pending',
    object_key          TEXT,
    storage_bucket      TEXT,
    mime_type           TEXT,
    page_count          INTEGER,
    extracted_text      TEXT,
    mtime               TIMESTAMPTZ NOT NULL,
    archival_timestamp  TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_files_hash ON files(hash);
CREATE INDEX idx_files_device ON files(device_id);
CREATE INDEX idx_files_status ON files(status);
CREATE INDEX idx_files_filename ON files(filename);

CREATE TABLE IF NOT EXISTS tags (
    id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name    TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS file_tags (
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    tag_id  UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (file_id, tag_id)
);
