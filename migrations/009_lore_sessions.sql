-- Lore sessions table - tracks idle lore discussion per channel
CREATE TABLE IF NOT EXISTS lore_sessions (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id BIGINT NOT NULL,
    first_lore_message_id BIGINT NOT NULL,
    last_lore_message_id BIGINT NOT NULL,
    last_lore_message_at TIMESTAMPTZ NOT NULL,
    idle_deadline TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'summarizing', 'thread_created', 'skipped', 'failed')),
    title TEXT,
    summary TEXT,
    thread_id BIGINT,
    summary_message_id BIGINT,
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique partial index: only one open session per channel
CREATE UNIQUE INDEX IF NOT EXISTS uq_lore_sessions_open_per_channel
    ON lore_sessions (guild_id, channel_id)
    WHERE status = 'open';

-- Index for claiming sessions due for summarization
CREATE INDEX IF NOT EXISTS idx_lore_sessions_due_for_summary
    ON lore_sessions (status, idle_deadline)
    WHERE status IN ('open', 'summarizing');

-- Index for thread lookup by guild and thread_id
CREATE INDEX IF NOT EXISTS idx_lore_sessions_thread
    ON lore_sessions (guild_id, thread_id)
    WHERE thread_id IS NOT NULL;
