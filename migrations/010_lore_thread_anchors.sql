-- Lore thread anchors table - maps Discord threads to lore sessions
CREATE TABLE IF NOT EXISTS lore_thread_anchors (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id BIGINT NOT NULL,
    thread_id BIGINT NOT NULL UNIQUE,
    summary_message_id BIGINT,
    summary_text TEXT,
    title TEXT,
    source_session_id BIGINT REFERENCES lore_sessions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for thread lookup by guild and thread_id
CREATE INDEX IF NOT EXISTS idx_lore_thread_anchors_guild_thread
    ON lore_thread_anchors (guild_id, thread_id);
