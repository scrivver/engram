ALTER TABLE files ADD COLUMN owner TEXT;
CREATE INDEX idx_files_owner ON files(owner);
DROP INDEX idx_files_hash;
CREATE UNIQUE INDEX idx_files_hash_owner ON files(hash, owner);
