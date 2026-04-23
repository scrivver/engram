ALTER TABLE files ADD COLUMN tsv tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple',
            coalesce(filename, '') || ' ' || coalesce(extracted_text, ''))
    ) STORED;

CREATE INDEX idx_files_tsv ON files USING GIN (tsv);
