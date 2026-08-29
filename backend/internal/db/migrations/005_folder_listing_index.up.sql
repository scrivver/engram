-- Prefix-matching support for folder listing.
--
-- Folder browsing derives directories from `filename`, the user-facing display
-- path, using `filename LIKE 'docs/%'`. Two properties of that query drive this
-- index:
--
--   * Every folder query is owner-scoped before it matches a prefix, so the
--     composite leads with `owner`. The existing single-column
--     `idx_files_filename` cannot serve both halves.
--   * `text_pattern_ops` makes prefix matching index-usable under any
--     collation. This deployment initialises PostgreSQL with `initdb
--     --no-locale` (infra/postgresql.nix), so its collation is C and a plain
--     btree would already work; the opclass keeps the index correct if Engram
--     is ever pointed at a database created with a non-C collation, where a
--     plain btree is skipped for LIKE.
--
-- No row data is modified by this migration.
CREATE INDEX IF NOT EXISTS idx_files_owner_filename
    ON files (owner, filename text_pattern_ops);
