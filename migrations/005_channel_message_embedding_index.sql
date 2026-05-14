-- Adds IVFFLAT cosine index on channel_messages.content_embedding for vector
-- recall plus a partial btree index to accelerate pending-embedding backfill
-- scans. Idempotent so re-applying is safe.
CREATE INDEX IF NOT EXISTS idx_channel_messages_content_embedding
    ON channel_messages
    USING ivfflat (content_embedding vector_cosine_ops)
    WITH (lists = 100);

CREATE INDEX IF NOT EXISTS idx_channel_messages_pending_embedding
    ON channel_messages (guild_id, message_id)
    WHERE content_embedding IS NULL;
