DROP INDEX idx_files_storage_path;
DROP INDEX idx_files_hash_owner;
CREATE UNIQUE INDEX idx_files_hash_owner ON files(hash, owner);
