DROP INDEX IF EXISTS idx_files_filename_trgm;

DROP INDEX IF EXISTS idx_files_tsv;
ALTER TABLE files DROP COLUMN IF EXISTS tsv;

-- Restore the migration 003 definition.
ALTER TABLE files ADD COLUMN tsv tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple',
            coalesce(filename, '') || ' ' || coalesce(extracted_text, ''))
    ) STORED;

CREATE INDEX idx_files_tsv ON files USING GIN (tsv);
