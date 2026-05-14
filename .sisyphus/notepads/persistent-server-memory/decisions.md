# Decisions - persistent-server-memory

## 2026-05-12 Server memory config schema
- Decision: represent all `MEMORY_SERVER_*` env vars as a nested `MemoryServerConfig` struct on `Config.MemoryServer`.
- Rationale: keeps the top-level `Config` flat section unchanged and lets downstream packages depend on just the nested struct.

## 2026-05-12 Invalid env fallback policy
- Decision: invalid values for threshold/top-k/batch/workers fall back to defaults instead of returning error from `Load()`.
- Rationale: a malformed operator env should never cause guild isolation to be silently disabled by refusing to start; defaults are safe and visible in logs when the service wires memory.

## 2026-05-12 Vector index choice
- Decision: use IVFFLAT with `vector_cosine_ops` and `lists=100`.
- Rationale: IVFFLAT is broadly supported on pgvector builds shipped in stock Postgres images and is fine for expected per-guild row counts; lists=100 is the documented default starting point.

## 2026-05-12 Threshold validation behavior
- Decision: keep `Load()` non-fatal for invalid `MEMORY_SERVER_RECALL_THRESHOLD`, but emit warning text and retain default `0.72`.
- Rationale: preserves startup resilience while making operator misconfiguration visible.

## 2026-05-12 Pending-index key choice
- Decision: use `idx_channel_messages_pending_embedding ON channel_messages (guild_id, message_id) WHERE content_embedding IS NULL`.
- Rationale: preserves guild-local scan locality and stable ordering via message identity while staying fully idempotent.
