-- User behavior profiles per (guild_id, user_id). Profiles never cross guilds;
-- a separate row exists for the same Discord user in each server they talk in.
CREATE TABLE IF NOT EXISTS user_behavior_profiles (
    id BIGSERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    communication_style TEXT NOT NULL DEFAULT '',
    formality TEXT NOT NULL DEFAULT '',
    response_length_preference TEXT NOT NULL DEFAULT '',
    formatting_preference TEXT NOT NULL DEFAULT '',
    recurring_topics TEXT[] NOT NULL DEFAULT '{}',
    evidence_count INTEGER NOT NULL DEFAULT 0,
    last_observed_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (guild_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_behavior_profiles_lookup
    ON user_behavior_profiles (guild_id, user_id);
