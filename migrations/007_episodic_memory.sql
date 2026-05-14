-- episodic_memory stores pre-compaction message snapshots as append-only
-- episodes inspired by alash3al/stash's Episode model. Each row captures
-- one Discord message at the moment it was about to be squashed by the
-- context-builder compactor, so long-term recall survives even after the
-- live context window drops it.
--
-- Design notes:
-- - guild_id isolation is mandatory: every query MUST filter by guild_id.
-- - embedding dim 384 matches the internal ONNX embedder (same as
--   channel_messages.content_embedding). Never mix dims here.
-- - (guild_id, channel_id, message_id) UNIQUE prevents duplicate rows if
--   compaction fires twice over the same window.
-- - occurred_at mirrors the source message's CreatedAt so time-based
--   recall ranks by when the thing happened, not when it was archived.

CREATE TABLE IF NOT EXISTS episodic_memory (
    id                BIGSERIAL PRIMARY KEY,
    guild_id          BIGINT NOT NULL,
    channel_id        BIGINT NOT NULL,
    thread_id         BIGINT,
    channel_name      TEXT,
    thread_name       TEXT,
    user_id           BIGINT NOT NULL,
    author_name       TEXT,
    message_id        BIGINT NOT NULL,
    content           TEXT NOT NULL,
    tagged_line       TEXT NOT NULL,
    embedding         vector(384),
    embedding_model   TEXT,
    occurred_at       TIMESTAMPTZ NOT NULL,
    archived_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_episodic_memory_guild_message
    ON episodic_memory (guild_id, message_id);

CREATE INDEX IF NOT EXISTS idx_episodic_memory_guild_time
    ON episodic_memory (guild_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_episodic_memory_embedding
    ON episodic_memory
    USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX IF NOT EXISTS idx_episodic_memory_pending_embedding
    ON episodic_memory (guild_id, id)
    WHERE embedding IS NULL;
