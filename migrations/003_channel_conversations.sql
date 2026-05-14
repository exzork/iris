-- Channel conversations table - per-channel conversation lock
CREATE TABLE IF NOT EXISTS channel_conversations (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id BIGINT NOT NULL,
    last_bot_reply_at TIMESTAMP NOT NULL,
    lock_until TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(guild_id, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_conversations_lock
    ON channel_conversations(guild_id, channel_id, lock_until);
