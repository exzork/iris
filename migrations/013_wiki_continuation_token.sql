-- Add continuation_token to wiki_ingest_cursors so MediaWiki apcontinue
-- pagination tokens survive restarts. Pre-existing rows default to ''.

ALTER TABLE wiki_ingest_cursors
    ADD COLUMN IF NOT EXISTS continuation_token TEXT NOT NULL DEFAULT '';
