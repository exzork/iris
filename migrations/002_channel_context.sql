-- Allowed channels table - include-list for channel routing
CREATE TABLE IF NOT EXISTS allowed_channels (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id BIGINT NOT NULL,
    reason VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(guild_id, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_allowed_channels_guild ON allowed_channels(guild_id);

-- Channel messages table - rolling context storage per guild/channel
CREATE TABLE IF NOT EXISTS channel_messages (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    author_name VARCHAR(255),
    content TEXT NOT NULL DEFAULT '',
    attachment_count INT NOT NULL DEFAULT 0,
    reply_to_message_id BIGINT,
    reply_to_channel_id BIGINT,
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    trigger_source VARCHAR(64) NOT NULL DEFAULT 'observe',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    edited_at TIMESTAMP,
    deleted_at TIMESTAMP,
    UNIQUE(guild_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_messages_recent
    ON channel_messages(guild_id, channel_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_messages_user
    ON channel_messages(guild_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_messages_reply
    ON channel_messages(guild_id, reply_to_message_id);
