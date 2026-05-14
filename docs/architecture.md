# Architecture

High-level view of how the I.R.I.S bot processes messages, where data lives, and how safety is enforced.

## Event Flow

A Discord event travels through this path before a reply is posted.

```
+-----------+      +---------+      +----------------+      +---------------+
|  Discord  | ---> | Gateway | ---> | Trigger Router | ---> | Orchestrator  |
+-----------+      +---------+      +----------------+      +------+--------+
                                                                   |
                                    +------------------------------+
                                    v
                             +-------------+     +------------+
                             | Lore & Mem  | --> |    LLM     |
                             | Retrieval   |     |  Provider  |
                             +-------------+     +-----+------+
                                                       |
                                                       v
                                                 +----------+
                                                 |  Safety  |
                                                 |  Filter  |
                                                 +-----+----+
                                                       |
                                                       v
                                                 +----------+
                                                 | Discord  |
                                                 |  Reply   |
                                                 +----------+
```

Stages:

1. **Gateway**: `discordgo` connects over websocket, receives `MESSAGE_CREATE` and interaction events, and forwards them to internal handlers.
2. **Trigger Router**: decides whether an event should reach the bot's main path. Triggers: direct mention, reply to the bot, or the `iris` name match (requires Message Content Intent). Exception-channel list is consulted here.
3. **Orchestrator**: builds the request context (guild ID, user ID, channel, message). Kicks off lore retrieval and memory lookup in parallel.
4. **Lore & Memory Retrieval**: see *Lore Retrieval* and *Data Layer* below.
5. **LLM Provider**: receives the assembled prompt (persona, lore policy, memory, user message) and returns a draft reply.
6. **Safety Filter**: runs injection neutralization, secret redaction, and output moderation before the reply leaves the process.
7. **Discord Reply**: posted as a regular message, reply, or embed depending on the response shape.

## Data Layer

PostgreSQL 16 with the pgvector extension. All state is stored here. No external cache or object store is required for day-to-day operation.

Tables:

- `guilds`: one row per guild the bot has joined. Tracks join time and soft-delete state.
- `guild_settings`: per-guild configuration (language, autoreply toggle, rate-limit overrides). Backs `!iris config set`.
- `exception_channels`: channels where the `iris` name trigger is disabled.
- `memory_records`: lightweight per-user memory facts, scoped by guild and user. Treated as reference data, never as instructions.
- `lore_chunks`: ingested chunks of Wuthering Waves wiki content with a pgvector embedding column for cosine retrieval.
- `reminders`: scheduled reminders a user has set.
- `audit_events`: append-only log of admin actions and safety events (injection attempts, rate-limit hits, config changes).

Migrations live in `migrations/*.sql` and are applied by the `cmd/migrate` binary or the `migrate` compose service.

## Lore Retrieval

Lore answering is grounded exclusively in the Wuthering Waves Wiki (`wutheringwaves.fandom.com`).

Pipeline:

1. **Ingestion**: a MediaWiki API client pages through the wiki, fetches article wikitext, and normalizes it to plain text.
2. **Chunking**: content is split into section-aware chunks of roughly 800 tokens, preserving page title and section heading for citation.
3. **Embedding**: each chunk is embedded using the configured LLM provider's embedding endpoint and stored in `lore_chunks.embedding`.
4. **Retrieval**: at query time, the user question is embedded and matched with cosine similarity via pgvector's `<=>` operator. The top N chunks are returned with their source URLs.
5. **Citation composer**: builds a sitasi block (page title plus URL) that accompanies every lore reply.
6. **Canon check**: before posting, the composed answer is compared against the retrieved chunks. If the draft contains claims not traceable to a cited chunk, the bot either refuses or downgrades to a neutral "not enough evidence" response.

See `docs/wiki-compliance.md` for the full citation rules.

## Safety

Safety runs on both ends of the LLM call.

- **Injection neutralization**: inbound text (including memory records) is scanned for common prompt-injection patterns (*"ignore previous instructions"*, persona override attempts, language switch demands). Matches are neutralized by wrapping or dropping the offending segment and an `audit_events` row is written.
- **Secret redaction**: outbound text is scanned for patterns that look like API keys, Discord tokens, and Postgres URLs. Matches are replaced with `[redacted]` before posting.
- **Output filter**: a light content filter rejects disallowed outputs (slurs, CSAM hints, doxxing). Rejections return a neutral refusal in Indonesian.
- **Rate limits**: per-guild and per-user counters enforce the limits configured via `!iris ratelimit set`. Over-limit events are logged and optionally surfaced in a short cooldown reply.

## Per-Guild Configuration

Each guild has an isolated config row in `guild_settings`. Admins manage it via `!iris config` and `!iris ratelimit` (see `docs/admin-commands.md`). The orchestrator reads the guild config on every request, with a short in-process cache to avoid hot-path database hits.

Config keys are type-checked. Unknown keys are rejected at the command layer. Every change is recorded in `audit_events` with the actor's user ID, the key, the old value, and the new value.

## Persona

The I.R.I.S persona is inspired by the game's archival and retrieval AI concept; dialogue is grounded in cited wiki content only. Persona rules and the exact prompt structure are documented in `docs/persona-policy.md`. This document does not restate personality details beyond that scope.
