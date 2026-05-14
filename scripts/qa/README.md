# Lore Thread Protocol QA Harness

Binary-pass/fail QA procedure for verifying the lore thread protocol end-to-end against a live Discord staging channel.

## Quick Start

```bash
# Dry run (no network calls, prints planned steps)
./scripts/qa/lore_thread_qa.sh --dry-run

# Run with default 5-minute idle duration
DISCORD_BOT_TOKEN=your_token QA_GUILD_ID=123 QA_CHANNEL_ID=456 ./scripts/qa/lore_thread_qa.sh

# Run with custom idle duration (e.g., 30 seconds for faster testing)
DISCORD_BOT_TOKEN=your_token QA_GUILD_ID=123 QA_CHANNEL_ID=456 ./scripts/qa/lore_thread_qa.sh --idle-duration 30s
```

## Required Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `DISCORD_BOT_TOKEN` | Discord bot token for API calls | `MTA4MjM4NzA4NzA4NzA4NzA4.GxYzAb.abc123...` |
| `QA_GUILD_ID` | Guild ID for QA testing | `761163966030151701` |
| `QA_CHANNEL_ID` | Channel ID for QA testing | `1504020311496986715` |

## Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `IRIS_LORE_IDLE_DURATION` | Idle duration before lore summary | `5m` |
| `IRIS_DB_HOST` | Database host | `localhost` |
| `IRIS_DB_PORT` | Database port | `5432` |
| `IRIS_DB_USER` | Database user | `iris_user` |
| `IRIS_DB_PASSWORD` | Database password | `iris_password` |
| `IRIS_DB_NAME` | Database name | `iris` |

## Flags

### `--dry-run`
Print planned steps and exit without making any network calls or database changes.

```bash
./scripts/qa/lore_thread_qa.sh --dry-run
```

### `--idle-duration DURATION`
Override the idle duration for testing. Accepts Go duration strings: `30s`, `5m`, `1h`, etc.

```bash
./scripts/qa/lore_thread_qa.sh --idle-duration 30s
```

## Test Flow

The harness executes the following phases:

1. **Validate Environment** - Check required env vars are set
2. **Enable Lore Threads** - Enable lore feature for QA guild via DB
3. **Post Test Messages** - Post 2 lore messages + 1 non-lore message to QA channel
4. **Wait Idle Duration** - Wait for configured idle period (default 5m)
5. **Verify Thread Creation** - Assert exactly 1 thread was created
6. **Verify Thread Properties** - Check title ≤80 chars, non-empty
7. **Verify Thread Anchor** - Check DB `lore_thread_anchors` row exists
8. **Verify Summary Content** - Assert non-lore content excluded, Indonesian-dominant
9. **Disable Lore Threads** - Disable lore feature for QA guild
10. **Post More Lore** - Post 3 more lore messages with feature disabled
11. **Wait Idle Duration** - Wait again
12. **Verify No New Thread** - Assert no additional thread was created
13. **Write Report** - Output binary pass/fail report to `.sisyphus/evidence/task-12-manual-qa.txt`

## Assertions

Each phase produces binary PASS/FAIL assertions:

- Enable lore for guild
- Post lore message 1
- Post lore message 2
- Post non-lore message
- Exactly 1 thread created
- Thread title ≤80 chars and non-empty
- Thread anchor exists in DB
- Non-lore content excluded from summary
- Summary is Bahasa Indonesia-dominant
- Disable lore for guild
- Post lore message 3 (disabled)
- Post lore message 4 (disabled)
- No new thread created when disabled

## Output

Evidence report is written to: `.sisyphus/evidence/task-12-manual-qa.txt`

Report format:
```
=== Lore Thread Protocol QA Report ===
Timestamp: 2026-05-13T14:31:23Z
Guild ID: 761163966030151701
Channel ID: 1504020311496986715
Idle Duration: 5m

=== Test Execution ===
  [PASS] Enable lore for guild
  [PASS] Post lore message 1
  ...
  [PASS] No new thread created when disabled

=== Assertions ===
PASS: 13
FAIL: 0

=== Thread IDs Observed ===
  1234567890123456789

=== Final Verdict ===
PASS
```

## Test Data

Test messages are defined in `lore_thread_qa_fixtures.json`:

- **Lore messages**: Indonesian text about Wuthering Waves lore
- **Non-lore message**: Indonesian text about gaming/social activity
- **Expected keywords**: Used for summary validation
- **Non-lore keywords**: Used to verify exclusion from summary

## Database Requirements

The harness requires direct database access to:
- Enable/disable lore settings via `lore_guild_settings` table
- Verify thread anchors via `lore_thread_anchors` table

Ensure PostgreSQL is accessible at the configured host/port with the provided credentials.

## Staging Setup

For a complete staging environment:

1. Start Docker Compose services:
   ```bash
   docker compose up -d
   ```

2. Run migrations:
   ```bash
   docker compose run --rm migrate
   ```

3. Invite bot to staging guild and get channel ID

4. Run QA harness:
   ```bash
   DISCORD_BOT_TOKEN=<token> QA_GUILD_ID=<id> QA_CHANNEL_ID=<id> ./scripts/qa/lore_thread_qa.sh --idle-duration 30s
   ```

## Troubleshooting

### Missing env vars
```
ERROR: Missing required env var: DISCORD_BOT_TOKEN
```
Set all three required variables before running.

### Database connection failed
```
ERROR: Failed to connect to database
```
Verify `IRIS_DB_*` variables match your database configuration. Check PostgreSQL is running.

### Discord API errors
```
ERROR: Discord API returned error
```
Verify `DISCORD_BOT_TOKEN` is valid and bot has permissions in the guild/channel.

### Timeout waiting for thread
The bot may not have processed the lore messages yet. Increase `--idle-duration` or check bot logs.

## Exit Codes

- `0` - All assertions passed (PASS verdict)
- `1` - One or more assertions failed (FAIL verdict) or validation error

## Notes

- The harness is deterministic: same inputs produce same assertions
- No human judgment required: all assertions are binary PASS/FAIL
- Dry-run mode allows verification of planned steps without side effects
- Evidence artifacts are preserved for audit trail
