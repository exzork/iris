-- Wiki content store: global lore from registered external wikis.
-- Distinct from lore_documents/lore_chunks (per-guild authored content).
-- Source-scoped, no guild_id; one ingestion run serves all guilds.

CREATE TABLE IF NOT EXISTS wiki_pages (
    source_id   TEXT   NOT NULL,
    page_id     BIGINT NOT NULL,
    title       TEXT   NOT NULL,
    url         TEXT   NOT NULL,
    revision    BIGINT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_id, page_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_pages_source_title
    ON wiki_pages (source_id, title);

CREATE TABLE IF NOT EXISTS wiki_chunks (
    id            BIGSERIAL PRIMARY KEY,
    source_id     TEXT      NOT NULL,
    page_id       BIGINT    NOT NULL,
    chunk_index   INT       NOT NULL,
    content       TEXT      NOT NULL,
    content_hash  TEXT      NOT NULL,
    embedding     vector(384),
    title         TEXT      NOT NULL,
    url           TEXT      NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source_id, page_id, chunk_index),
    FOREIGN KEY (source_id, page_id)
        REFERENCES wiki_pages (source_id, page_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_wiki_chunks_source_page
    ON wiki_chunks (source_id, page_id);

CREATE INDEX IF NOT EXISTS idx_wiki_chunks_hash
    ON wiki_chunks (source_id, content_hash);

CREATE TABLE IF NOT EXISTS wiki_ingest_cursors (
    source_id   TEXT PRIMARY KEY,
    last_title  TEXT   NOT NULL DEFAULT '',
    last_page_id BIGINT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wiki_content_hashes (
    source_id    TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_id, content_hash)
);
