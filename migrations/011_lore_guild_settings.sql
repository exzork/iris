-- Lore guild settings table - per-guild lore feature configuration
CREATE TABLE IF NOT EXISTS lore_guild_settings (
    guild_id BIGINT PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    thread_cap_per_hour INT NOT NULL DEFAULT 6,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Thread counter table for rate limiting lore threads per hour
CREATE TABLE IF NOT EXISTS lore_thread_counters (
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    hour_bucket TIMESTAMPTZ NOT NULL,
    count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (guild_id, hour_bucket)
);

-- Index for efficient hourly count queries
CREATE INDEX IF NOT EXISTS idx_lore_thread_counters_guild_hour
    ON lore_thread_counters (guild_id, hour_bucket DESC);
