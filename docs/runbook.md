# I.R.I.S Bot Runbook

Operator guide for deploying, operating, and recovering the I.R.I.S Discord bot.

## Prerequisites

- Docker 24+ and Docker Compose plugin v2.
- Outbound HTTPS to `discord.com`, the configured OpenAI-compatible provider, and `wutheringwaves.fandom.com` for lore ingestion. SearXNG runs locally through Docker Compose for web-search tooling.
- A Discord application with a bot user, invited with the `bot` and `applications.commands` scopes and permissions for Send Messages, Embed Links, Read Message History, Attach Files, Add Reactions, Manage Messages, and Use Application Commands.
- An OpenAI-compatible API key with budget for chat completions.
- Roughly 1 GB disk for the Postgres data volume under normal load.

## Initial Deployment

1. Provision the host (any Linux box with Docker installed).
2. Clone the repo to `/opt/iris-bot` (or any path).
   ```
   git clone https://github.com/eko/iris-bot.git /opt/iris-bot
   cd /opt/iris-bot
   ```
3. Copy the environment template and fill values.
   ```
   cp .env.example .env
   chmod 600 .env
   ```
4. Review `docker-compose.yml` resource limits. Adjust CPU and memory if your host is small.
5. Start services.
   ```
   docker compose up -d
   ```
6. Confirm health.
   ```
   docker compose ps
   docker compose logs -f bot
   ```
7. Verify the bot appears online in at least one guild.

## Bootstrap / Seed

On a fresh database the `migrate` compose service applies `migrations/001_init.sql` automatically. To re-run or check status:

```
docker compose run --rm migrate
```

Lore content is read from the indexed database tables used by the RAG pipeline. If you refresh lore data, run the repository's ingestion/backfill workflow for your deployment and then verify `lore_chunks` contains current rows. Ingestion is an offline batch; it does not need to run on every boot.

## Operating the Bot

Day-to-day operator tasks.

### Check status

```
docker compose ps
docker compose logs --tail=200 bot
```

### Restart the bot without touching the database

```
docker compose restart bot
```

### Apply new code

```
git pull
docker compose build bot
docker compose up -d bot
```

### Tail Discord gateway events

```
docker compose logs -f bot | grep -i gateway
```

### Change configuration

Edit `.env` on the host, then restart the bot container:

```
docker compose up -d bot
```

Guild-scoped settings are changed through slash commands such as `/iris-config`, `/iris-ratelimit`, `/iris-allowed`, `/iris-exception`, and `/iris-lore`. Owner-gated runtime model and MCP changes are requested in natural language by the Discord user whose ID matches `IRIS_OWNER_ID`.

## Backup & Restore

### Backup

Postgres data lives in the `postgres_data` volume. Snapshot it with `pg_dump`:

```
docker compose exec postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" > backup-$(date +%F).sql
```

Store backups off-host. Rotate daily, keep at least 7 days.

### Restore

1. Stop the bot.
   ```
   docker compose stop bot
   ```
2. Reset the database volume (destroys current data).
   ```
   docker compose down -v
   ```
3. Start Postgres only.
   ```
   docker compose up -d postgres
   ```
4. Wait for health, then restore.
   ```
   cat backup-YYYY-MM-DD.sql | docker compose exec -T postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"
   ```
5. Start the rest.
   ```
   docker compose up -d
   ```

## Troubleshooting

### Discord Gateway disconnect

Symptoms: log lines like `gateway: websocket closed` or the bot going offline repeatedly.

Checks:

1. `docker compose logs --tail=500 bot | grep -i gateway` to get the close code.
2. Close code 4004 means invalid token. Rotate `DISCORD_TOKEN`.
3. Close code 4013 / 4014 means a privileged intent is requested but not enabled in the Developer Portal. Enable Message Content Intent or remove the trigger.
4. Network flaps: confirm the host has outbound 443 to `gateway.discord.gg`.
5. If the bot loops reconnecting, restart it and confirm the exponential backoff kicks in. Persistent loops indicate a bad token or a rate-limited identify.

### Postgres connection error

Symptoms: `failed to connect to postgres`, `dial tcp: connect: connection refused`, bot crash-loops on boot.

Checks:

1. `docker compose ps` - is `iris-postgres` healthy?
2. `docker compose logs postgres` for startup errors (bad password, volume corruption).
3. Confirm `DATABASE_URL`, `POSTGRES_USER`, and `POSTGRES_PASSWORD` match between `.env` and the Postgres container (these were set on first boot and are not changed by editing `.env` alone; see *Rotating Secrets*).
4. From the bot container: `docker compose exec bot sh -c 'pg_isready -h postgres -U "$POSTGRES_USER"'`.
5. If the volume is corrupted and you have a backup, restore per the *Backup & Restore* section.

### LLM provider failure

Symptoms: replies fail with `llm: request failed`, 5xx from the provider, or empty completions.

Checks:

1. `docker compose logs bot | grep -i llm` for the exact error.
2. Confirm `OPENAI_API_KEY` is valid and has budget.
3. If using a self-hosted or proxy provider, confirm `LLM_BASE_URL` is reachable from the bot container.
4. Bump `LLM_CHAT_TIMEOUT`, `LLM_TOOL_TIMEOUT`, and `LLM_MAX_RETRIES` temporarily if the upstream is slow. `LLM_TIMEOUT` remains a legacy fallback.
5. For repeated 429s, see *rate-limit exhaustion* below.

### Rate-limit exhaustion

Symptoms: `429` from the LLM provider or Discord, responses get dropped, or `/iris-ratelimit` reports a tight channel setting.

Checks:

1. Inspect current limits: `/iris-ratelimit get channel_id:<channelID>` in an admin channel.
2. Lower a noisy channel's rate if it is burning the budget: `/iris-ratelimit set channel_id:<channelID> rate:<rate>`.
3. For provider 429s, reduce concurrent requests by lowering `LLM_MAX_TOKENS` or slow the worker pool.
4. If the Discord rate limit is hit, confirm the bot is not fan-out posting to many channels at once. Batch admin announcements.

### Memory injection attempts in logs

Symptoms: audit log entries tagged `memory_injection_attempt` or `persona_override_attempt`.

Checks:

1. These are expected and should be neutralized by the safety layer. Confirm the bot's response did not change language or persona in the affected thread.
2. Review scoped memory directly in Postgres (`memory_records` for per-user facts and `channel_messages` for guild recall inputs).
3. If malicious content leaked through, open an incident, redact the row, and tighten the injection filter in `internal/safety`.
4. Consider banning the user at the guild level if repeated.

### Image or attachment response failures

Symptoms: an image-capable response returns the Indonesian image fallback, logs show `image: provider error` or `image: safety block`, or Discord rejects an attachment.

Checks:

1. Confirm the image provider key (if distinct from `OPENAI_API_KEY`) is valid.
2. Safety blocks are expected for disallowed prompts. The bot will return a neutral refusal.
3. For timeouts, bump the per-request timeout in the image client config and verify the provider status page.
4. For unsupported Discord attachment types, confirm the output is PNG or JPEG before posting.

## Rotating Secrets

Rotate `DISCORD_TOKEN`, `OPENAI_API_KEY`, and Postgres credentials at least yearly, or immediately on suspected compromise.

### Discord token

1. Regenerate the token in the Discord Developer Portal under **Bot**.
2. Update `.env` with the new value.
3. `docker compose up -d bot` to apply.
4. Revoke the old token in the portal.

### OpenAI key

1. Create a new key in the provider dashboard.
2. Update `.env`. Restart the bot.
3. Revoke the old key.

### Postgres password

The Postgres password is set on first init and stored inside the data volume. Changing `.env` alone is not enough.

1. Connect with the old password: `docker compose exec postgres psql -U "$POSTGRES_USER"`.
2. Run `ALTER USER "<user>" WITH PASSWORD '<new>';`.
3. Update `.env` with the new value.
4. Restart the bot: `docker compose up -d bot`.

## Upgrading

1. Read the release notes for breaking changes.
2. Back up the database.
3. Pull the new code.
   ```
   git fetch --tags
   git checkout v<new-version>
   ```
4. Rebuild and restart.
   ```
   docker compose build
   docker compose up -d
   ```
5. Watch logs for a few minutes. Roll back by checking out the previous tag and rebuilding if needed.
