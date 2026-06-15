WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY storage_type, file_path
               ORDER BY updated_at DESC, created_at DESC, id DESC
           ) AS position
    FROM files
)
DELETE FROM files
WHERE id IN (SELECT id FROM ranked WHERE position > 1);

DROP INDEX idx_files_hash_owner;
CREATE INDEX idx_files_hash_owner ON files(hash, owner);
CREATE UNIQUE INDEX idx_files_storage_path ON files(storage_type, file_path);
