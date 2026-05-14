# I.R.I.S Discord Bot

A Discord bot for Wuthering Waves communities, written in Go. It combines wiki-grounded lore answers, Discord memory, slash-command moderation, tool calling, and MCP integrations while replying in Bahasa Indonesia.

## Overview

I.R.I.S (Intelligent Retrieval & Indexing System) is a Discord bot built in Go. It answers Wuthering Waves questions by retrieving indexed wiki content, composing cited Indonesian responses, and enriching context with guild-scoped memory and recent conversation history.

The persona is inspired by the game's archival and retrieval AI concept. Dialogue is grounded in cited wiki content only. The bot does not invent character traits, relationships, or backstory beyond what the wiki supports.

The stack is Go 1.22+, PostgreSQL 16 with pgvector, ONNX-backed local embeddings, SearXNG for web search, and an OpenAI-compatible LLM provider. Docker Compose runs the database, migrations, SearXNG, and bot process for local development or deployment.

Current bot capabilities:

- Responds to direct mentions, replies to bot messages, and `iris` name triggers when Discord Message Content Intent is enabled.
- Routes requests between default and strong model tiers, with owner-gated runtime model switching.
- Uses RAG over indexed Wuthering Waves wiki pages with citations and canon-check tooling.
- Stores per-guild message memory in Postgres/pgvector and recalls relevant context through local embeddings.
- Streams long replies to Discord, applies safety filters, and redacts sensitive output before sending.
- Registers native Discord slash commands for admin configuration, allowed channels, exceptions, rate limits, and lore-thread settings.
- Can call built-in tools (`canon_check`, `meme_search`, web search, model switching, lore-thread controls) plus file-configured MCP servers.

## Quickstart

1. Clone the repository.
   ```
   git clone https://github.com/eko/iris-bot.git
   cd iris-bot
   ```
2. Copy the environment template and fill in real values.
   ```
   cp .env.example .env
   ```
3. Start the services.
   ```
   docker compose up -d
   ```
4. Run migrations (the `migrate` service runs automatically on first boot, or run it manually).
   ```
   docker compose run --rm migrate
   ```
5. Invite the bot to your guild (see [Invite URL](#invite-url)).

## Invite URL

Use the URL below, replacing `YOUR_CLIENT_ID` with the **Application ID** from the Discord Developer Portal (*General Information* tab of your app).

```
https://discord.com/oauth2/authorize?client_id=YOUR_CLIENT_ID&permissions=2147609664&scope=bot+applications.commands
```

Scopes:

- `bot` - core bot account in the guild.
- `applications.commands` - register and handle the `/iris` slash commands defined in [internal/slash/native.go](internal/slash/native.go).

Permissions (bitmask `2147609664`):

| Permission | Bit | Why it is needed |
|------------|-----|------------------|
| View Channels | `1024` | Receive `MESSAGE_CREATE` and fetch channel context. |
| Send Messages | `2048` | Reply to mentions, answer wiki questions, run admin commands. |
| Add Reactions | `64` | React to user or bot messages (acknowledgements, status signals). |
| Manage Messages | `8192` | Edit or delete bot messages, and remove stale replies when needed. |
| Embed Links | `16384` | Render wiki URL previews inside cited responses. |
| Attach Files | `32768` | Send images and file attachments (e.g. Pixiv / image-MCP responses). |
| Read Message History | `65536` | Resolve replied-to messages via `ChannelMessage` for reply context. |
| Use Application Commands | `2147483648` | Handle `/iris` slash command interactions. |

Thread creation and messaging in threads are covered by the existing "Send Messages" permission. No additional permissions are required.

The bot does not request Administrator, Manage Channels, or Mention Everyone. If you want to grant those later, update the bitmask via the [Discord Permissions Calculator](https://discordapi.com/permissions.html).

After the guild owner authorizes the URL, enable the Message Content Intent (see below) and restart the bot.

## Discord Message Content Intent

The `iris` name trigger (replying when a user mentions the bot by name in a regular message) requires the **Message Content Intent**. This is a privileged intent and must be enabled in the Discord Developer Portal.

To enable:

1. Open https://discord.com/developers/applications.
2. Select your application.
3. Open **Bot** in the sidebar.
4. Toggle **Message Content Intent** on under *Privileged Gateway Intents*.
5. Save changes and restart the bot.

Without this intent the bot still responds to direct mentions (`@iris`) and replies to its own messages. The name trigger on arbitrary messages will not work until the intent is granted.

## Environment Variables

All variables are loaded from `.env` (or the shell environment).

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DISCORD_TOKEN` | yes | | Bot token from the Discord Developer Portal. |
| `OPENAI_API_KEY` | yes | | API key for the OpenAI-compatible LLM provider. |
| `DATABASE_URL` | yes | | Full Postgres connection string used by the bot. |
| `POSTGRES_HOST` | yes | | Postgres hostname (inside Docker network this is `postgres`). |
| `POSTGRES_PORT` | yes | `5432` | Postgres port. |
| `POSTGRES_USER` | yes | | Postgres username. |
| `POSTGRES_PASSWORD` | yes | | Postgres password. |
| `POSTGRES_DB` | yes | | Postgres database name. |
| `LLM_MODEL` | no | `gpt-4` | Model name passed to the LLM provider. |
| `LLM_BASE_URL` | no | `https://api.openai.com` | Base URL for the provider (useful for self-hosted or proxy endpoints). |
| `LLM_TEMPERATURE` | no | `0.7` | Sampling temperature. |
| `LLM_MAX_TOKENS` | no | `2048` | Max tokens per completion. |
| `LLM_TIMEOUT` | no | `2m` | Legacy fallback timeout for LLM requests (Go duration). |
| `LLM_CHAT_TIMEOUT` | no | `2m` | Timeout for plain chat/reply completions. |
| `LLM_TOOL_TIMEOUT` | no | `10m` | Timeout for tool-call streaming completions. |
| `LLM_MAX_RETRIES` | no | `3` | Retry attempts on transient errors. |
| `LLM_RETRY_DELAY` | no | `1s` | Initial backoff delay between retries. |
| `LLM_MODEL_ROUTER` | no | `kr/claude-haiku-4.5` | Fast classifier model used to pick default vs strong tier. |
| `LLM_MODEL_DEFAULT` | no | `LLM_MODEL` | Standard reply model fallback. |
| `LLM_MODEL_STRONG` | no | `kr/claude-opus-4.7` | Heavy-reasoning model for complex lore/tool tasks. |
| `IRIS_OWNER_ID` | no | | Discord user ID allowed to mutate MCP config and runtime model overrides. |
| `MCP_CONFIG_PATH` | no | `mcps.json` | Path to MCP server config JSON. In Docker this resolves to `/app/mcps.json`. |
| `IRIS_CONV_LOCK_TTL` | no | `5m` | How long a conversation remains active after the bot replies. |
| `IRIS_STREAMING` | no | `true` | Enables streaming Discord responses unless set to `false` or `0`. |
| `DEBUG` | no | `false` | Enables debug logging and LLM audit metadata. |

Never commit real values. Keep `.env` out of version control.

## Docker Compose

`docker-compose.yml` defines four services:

- `postgres`: PostgreSQL 16 with pgvector, data persisted in the `postgres_data` volume. The port is bound to `127.0.0.1` only.
- `migrate`: one-shot job that applies SQL migrations from `./migrations` on boot. Depends on a healthy `postgres`.
- `searxng`: local SearXNG instance bound to `127.0.0.1:8888`, used by web-search tooling.
- `bot`: the Go binary built from the repository `Dockerfile`. Depends on `postgres` healthy and `migrate` completed.

Networking is on the private `iris-network` bridge. The bot container has a 1 CPU / 512MB limit by default.

Start everything:

```
docker compose up -d
```

Tail logs:

```
docker compose logs -f bot
```

Stop and remove containers (data volume is preserved):

```
docker compose down
```

## Running Migrations

Migrations live in `migrations/` as plain SQL files. They are applied automatically by the `migrate` compose service on first boot. To run them manually:

```
docker compose run --rm migrate
```

Or from the host against a running Postgres:

```
go run ./cmd/migrate up
```

The `cmd/migrate` tool accepts `up`, `down`, and `status` subcommands and reads `DATABASE_URL` from the environment.

## Slash Commands and Tools

The current command surface is native Discord slash commands registered from `internal/slash/native.go`:

- `/iris-exception` manages exception channels.
- `/iris-allowed` manages the allow-list of channels where the bot may answer.
- `/iris-config` reads and writes guild settings.
- `/iris-ratelimit` manages guild/user rate limits.
- `/iris-lore-settings` controls lore-thread behavior.
- `/iris-help` summarizes the command surface.

Admin-only slash commands require Discord administrator permissions. Older `!iris` command behavior is documented in [docs/admin-commands.md](docs/admin-commands.md) for historical/operator reference, but the active invite URL grants `applications.commands` for slash interactions.

The LLM can also call internal tools through `internal/tools`:

- `canon_check` verifies lore claims against indexed wiki sources and returns verdicts with citations.
- `meme_search` searches safe Discord/social media image results.
- Web search, patch notes, character/item lookup, conversation summary, escalation, model switching, and lore-thread tools are registered at startup when their dependencies are available.
- MCP tools are loaded from `mcps.json`; when `IRIS_OWNER_ID` is set, the owner can add, remove, and list MCP servers at runtime.

## Server Memory

Per-guild long-term memory, separate from the per-user selective memory. All rows are scoped by `guild_id` and recall refuses `GuildID=0` (DMs) at the service layer.

Architecture borrows the capture/recall shape from [stash](https://github.com/alash3al/stash) (Brain / Embedder / Reasoner / Store), mapped onto existing iris components:

- **Store**: `channel_messages` in Postgres, with `content_embedding vector(384)` and an IVFFLAT cosine index.
- **Embedder**: the in-process ONNX runtime in `internal/embedder` (dim 384). Async workers in `internal/memory/queue.go` and `embedding_worker.go` fill pending rows.
- **Brain / Reasoner**: `GuildRecallService`, `BehaviorProfileService`, and `ContextBuilder` in `internal/memory/`. They build prompt context that is then handed to the existing `internal/llm` client.

Boundaries (enforced, not aspirational):

- LLM calls go through `internal/llm` only. The memory package does not import provider SDKs (no `github.com/sashabaranov/go-openai`, no `github.com/openai/openai-go`). A static guard in `internal/memory/provider_boundary_test.go` fails the build if that ever regresses.
- Embeddings go through `internal/embedder`. No provider embedding APIs, no second provider layer, no extra keys.
- stash is inspiration only. It is not vendored, not a dependency, and there is no API, schema, or protocol parity with it.

Env vars (all optional, safe defaults):

| Variable | Default | Description |
|----------|---------|-------------|
| `MEMORY_SERVER_ENABLED` | `true` | Master switch for guild-scoped recall and async capture. |
| `MEMORY_SERVER_RECALL_THRESHOLD` | `0.72` | Cosine similarity floor for recall hits. Values outside `[0,1]` fall back to the default. |
| `MEMORY_SERVER_RECALL_TOP_K` | `5` | Max recall rows injected per query. |
| `MEMORY_SERVER_EMBED_BATCH_SIZE` | `32` | Rows per ONNX embedding batch. |
| `MEMORY_SERVER_EMBED_WORKERS` | `1` | Parallel embedding workers. |
| `MEMORY_SERVER_EMBED_BACKFILL_LIMIT` | `500` | Max pending rows scanned per backfill pass. |

## Lore Threads

Lore threads are optional worker-driven conversations for longer lore topics. They are disabled by default and controlled through environment variables plus `/iris-lore-settings`:

| Variable | Default | Description |
|----------|---------|-------------|
| `IRIS_LORE_THREADS_ENABLED` | `false` | Enables the lore-thread worker and thread capture. |
| `IRIS_LORE_IDLE_DURATION` | `5m` | Idle time before a lore thread is considered ready for summarization/compaction. |
| `IRIS_LORE_COMPACTION_TARGET` | `0.70` | Target compression ratio for lore-thread compaction. |
| `IRIS_LORE_THREAD_CAP_PER_HOUR` | `6` | Per-guild cap for created lore threads. |
| `IRIS_LORE_WORKER_POLL_INTERVAL` | `30s` | Background worker polling interval. |
| `IRIS_LORE_LLM_TIMEOUT` | `30s` | Timeout for lore-thread LLM work. |
| `IRIS_LORE_LLM_MODEL` | `LLM_MODEL_STRONG` | Model used for lore-thread compaction/capture. |
| `IRIS_LORE_CAPTURE_TIMEOUT` | `60s` | Timeout for capture operations. |

Invalid values fall back to defaults rather than failing startup, so a malformed env cannot silently disable guild isolation.

## Development

Run tests:

```
go test ./...
```

Build the bot binary:

```
go build ./cmd/iris-bot
```

Architecture overview: [docs/architecture.md](docs/architecture.md). Operator runbook: [docs/runbook.md](docs/runbook.md). Persona and wiki rules: [docs/persona-policy.md](docs/persona-policy.md) and [docs/wiki-compliance.md](docs/wiki-compliance.md).

## Contributing

1. Fork and branch from `main`.
2. Keep changes focused. One feature or fix per pull request.
3. Run `go test ./...` and `go vet ./...` before opening a PR.
4. Do not add persona claims that are not backed by the Wuthering Waves Wiki. See `docs/persona-policy.md`.
5. Open a PR against `main` with a clear summary of what changed and why.

## License

MIT. See `LICENSE` (placeholder).
