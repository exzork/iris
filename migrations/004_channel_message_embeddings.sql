-- Add content_embedding column to channel_messages table
ALTER TABLE channel_messages ADD COLUMN IF NOT EXISTS content_embedding vector(384);
