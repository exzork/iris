-- global_settings stores bot-wide overrides that are not scoped to any guild.
-- Used for owner-controlled runtime configuration (e.g., active default/strong
-- LLM models) that must survive restarts. Rows are authoritative when
-- present; when absent, the in-process fallback (env-var config) wins.
--
-- Design notes:
--   - Single-row-per-key model, trivially keyed by setting_key.
--   - No guild_id column: these settings are global by definition. Per-guild
--     overrides live in guild_settings (migration 001).
--   - updated_by records the Discord user ID that performed the last mutation
--     so audit reconstruction does not need to cross-reference audit_events.

CREATE TABLE IF NOT EXISTS global_settings (
    setting_key   VARCHAR(255) PRIMARY KEY,
    setting_value TEXT NOT NULL,
    updated_by    BIGINT,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
