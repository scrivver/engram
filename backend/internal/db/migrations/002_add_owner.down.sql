DROP INDEX idx_files_hash_owner;
CREATE UNIQUE INDEX idx_files_hash ON files(hash);
DROP INDEX idx_files_owner;
ALTER TABLE files DROP COLUMN owner;
