-- Switch memory_records.embedding from 1536 (OpenAI text-embedding-3-small)
-- to 384 (local ONNX paraphrase-MiniLM-L3-v2).
--
-- IDEMPOTENT: this migration only acts when the column dimension is wrong.
-- The migrate runner already tracks applied migrations in schema_migrations,
-- but this guard is defense in depth so accidentally re-running 014 against
-- a fresh DB (or one where the column was already correct) is a no-op
-- instead of wiping every embedding.
DO $$
DECLARE
    current_dim integer;
BEGIN
    SELECT a.atttypmod
      INTO current_dim
      FROM pg_attribute a
      JOIN pg_class c   ON c.oid = a.attrelid
      JOIN pg_type t    ON t.oid = a.atttypid
     WHERE c.relname = 'memory_records'
       AND a.attname = 'embedding'
       AND a.attnum > 0
       AND NOT a.attisdropped
       AND t.typname = 'vector';

    IF current_dim IS NULL THEN
        ALTER TABLE memory_records ADD COLUMN embedding vector(384);
    ELSIF current_dim <> 384 THEN
        ALTER TABLE memory_records DROP COLUMN embedding;
        ALTER TABLE memory_records ADD COLUMN embedding vector(384);
    END IF;
END
$$;
