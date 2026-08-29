-- Filenames were unsearchable by word. Postgres' default text-search parser
-- classifies a name like `statement.pdf` as a single `host` token and a path
-- like `photos/2025/Pokemon_Emerald.sav` as a single `file` token, so the tsv
-- built in migration 003 held one lexeme for the whole name. Searching
-- `statement`, `pokemon`, or `fsb0017` matched nothing; only a word that
-- happened to be followed by a space (`bank` in `bank statement.pdf`) worked.
--
-- Indexing the filename twice fixes it. The raw form keeps whole-name queries
-- working, because a query of `statement.pdf` parses to that same single
-- lexeme. The form with every non-alphanumeric run collapsed to a space turns
-- each word inside a name, path segment, or extension into its own lexeme, so
-- `Pokemon_Emerald.sav` becomes {pokemon, emerald, sav}.

DROP INDEX IF EXISTS idx_files_tsv;
ALTER TABLE files DROP COLUMN IF EXISTS tsv;

ALTER TABLE files ADD COLUMN tsv tsvector
    GENERATED ALWAYS AS (
        to_tsvector('simple',
            coalesce(filename, '') || ' ' ||
            coalesce(regexp_replace(filename, '[^[:alnum:]]+', ' ', 'g'), '') || ' ' ||
            coalesce(extracted_text, ''))
    ) STORED;

CREATE INDEX idx_files_tsv ON files USING GIN (tsv);

-- Full-text search only matches whole lexemes, but the vault search box filters
-- as the user types, so a half-typed word has to match something. The API pairs
-- the tsv predicate with a substring match on the filename; this trigram index
-- keeps that half from degrading into a sequential scan.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_files_filename_trgm ON files USING GIN (lower(filename) gin_trgm_ops);
