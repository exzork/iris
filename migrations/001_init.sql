-- Enable pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Guilds table - base table for multi-guild scoping
CREATE TABLE IF NOT EXISTS guilds (
    id BIGINT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Guild settings - per-guild configuration
CREATE TABLE IF NOT EXISTS guild_settings (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    setting_key VARCHAR(255) NOT NULL,
    setting_value TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(guild_id, setting_key)
);

-- Exception channels - channels where bot should not respond
CREATE TABLE IF NOT EXISTS exception_channels (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id BIGINT NOT NULL,
    reason VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(guild_id, channel_id)
);

-- Memory records - hybrid memory with vector embeddings
CREATE TABLE IF NOT EXISTS memory_records (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id BIGINT,
    content TEXT NOT NULL,
    embedding vector(1536),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_guild_memory ON memory_records(guild_id);
CREATE INDEX IF NOT EXISTS idx_user_memory ON memory_records(guild_id, user_id);

-- Lore documents - knowledge base documents
CREATE TABLE IF NOT EXISTS lore_documents (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_guild_lore ON lore_documents(guild_id);

-- Lore chunks - vector-indexed chunks of lore documents
CREATE TABLE IF NOT EXISTS lore_chunks (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    document_id INT NOT NULL REFERENCES lore_documents(id) ON DELETE CASCADE,
    chunk_text TEXT NOT NULL,
    embedding vector(1536),
    chunk_index INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_guild_chunks ON lore_chunks(guild_id);
CREATE INDEX IF NOT EXISTS idx_document_chunks ON lore_chunks(document_id);

-- Tool logs - audit trail for tool executions
CREATE TABLE IF NOT EXISTS tool_logs (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id BIGINT,
    tool_name VARCHAR(255) NOT NULL,
    input_data JSONB,
    output_data JSONB,
    status VARCHAR(50),
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_guild_tools ON tool_logs(guild_id);
CREATE INDEX IF NOT EXISTS idx_tool_name ON tool_logs(tool_name);

-- Reminders - scheduled reminders per guild
CREATE TABLE IF NOT EXISTS reminders (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id BIGINT,
    channel_id BIGINT NOT NULL,
    reminder_text TEXT NOT NULL,
    scheduled_for TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_guild_reminders ON reminders(guild_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_reminders ON reminders(scheduled_for);

-- Audit events - audit trail for all operations
CREATE TABLE IF NOT EXISTS audit_events (
    id SERIAL PRIMARY KEY,
    guild_id BIGINT NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id BIGINT,
    event_type VARCHAR(255) NOT NULL,
    entity_type VARCHAR(255),
    entity_id VARCHAR(255),
    changes JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_guild_audit ON audit_events(guild_id);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_events(event_type);
