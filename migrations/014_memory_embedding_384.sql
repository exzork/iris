-- Switch memory_records embedding from 1536 (OpenAI text-embedding-3-small)
-- to 384 (local ONNX paraphrase-MiniLM-L3-v2). The local LLM proxy does not
-- serve OpenAI embeddings, so production saves were failing at embed time;
-- the bot was already using the ONNX embedder elsewhere (channel_messages
-- content_embedding is vector(384) too).
--
-- Safe to drop+recreate because the table is empty in production and any
-- legacy 1536-dim rows would not be retrievable through the new ONNX path.

ALTER TABLE memory_records DROP COLUMN IF EXISTS embedding;
ALTER TABLE memory_records ADD COLUMN embedding vector(384);
