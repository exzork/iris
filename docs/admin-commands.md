# Admin Slash Commands

I.R.I.S uses native Discord slash commands. They are registered from `internal/slash/native.go` and wrap the same admin handlers used by the bot's internal command layer.

## Who can run admin commands

Admin-only commands require Discord administrator permissions in the guild where the command is issued. Non-admin users receive a neutral refusal in Indonesian. Direct messages are rejected because admin settings are guild-scoped.

The public help command is available to all users:

```
/iris-help
```

It lists the active slash command surface and reminds the bot owner that MCP servers can be managed through owner-gated natural-language tool calls.

## Channel exceptions

Exception channels disable the plain `iris` name trigger in specific channels. Direct mentions and replies to the bot still work.

### `/iris-exception add channel_id:<channelID>`

Adds a channel to the exception list.

Example:

```
/iris-exception add channel_id:1234567890
```

Error behavior:

- Missing or malformed channel ID: `Channel ID tidak valid.`
- Channel not found or not in this guild: `Channel tidak ditemukan di guild ini.`
- Channel already listed: no-op response.

### `/iris-exception remove channel_id:<channelID>`

Removes a channel from the exception list.

Example:

```
/iris-exception remove channel_id:1234567890
```

Error behavior:

- Missing or malformed channel ID: `Channel ID tidak valid.`
- Channel not listed: no-op or not-found response from the handler.

### `/iris-exception list`

Lists exception channels for the current guild.

## Allowed channels

Allowed channels define where I.R.I.S may answer. When configured, the router uses this allow-list before admitting normal chat traffic.

### `/iris-allowed add channel_id:<channelID>`

Adds a channel to the allow-list.

Example:

```
/iris-allowed add channel_id:1234567890
```

### `/iris-allowed remove channel_id:<channelID>`

Removes a channel from the allow-list.

Example:

```
/iris-allowed remove channel_id:1234567890
```

### `/iris-allowed list`

Lists allowed channels for the current guild.

## Guild configuration

### `/iris-config list`

Lists per-guild config keys and current values.

### `/iris-config get key:<key>`

Shows one config value.

Example:

```
/iris-config get key:language
```

### `/iris-config set key:<key> value:<value>`

Sets one config value. Values are validated by the underlying settings handler before they are persisted.

Example:

```
/iris-config set key:language value:id
/iris-config set key:autoreply value:true
```

Successful changes are recorded in `audit_events` with actor and before/after values when the handler has enough context.

## Rate limits

The current slash command manages per-channel rates.

### `/iris-ratelimit set channel_id:<channelID> rate:<rate>`

Sets the allowed rate for a channel. The `rate` option is an integer rate per second.

Example:

```
/iris-ratelimit set channel_id:1234567890 rate:1
```

### `/iris-ratelimit get channel_id:<channelID>`

Shows the configured rate limit for a channel.

Example:

```
/iris-ratelimit get channel_id:1234567890
```

## Lore-thread settings

Lore-thread settings are registered from `internal/slash/lore_settings.go` when the lore settings handler is wired at startup.

### `/iris-lore enable`

Enables lore threads for the guild.

### `/iris-lore disable`

Disables lore threads for the guild.

### `/iris-lore status`

Shows whether lore threads are enabled and the current thread cap.

### `/iris-lore cap value:<1-100>`

Sets the per-hour lore-thread creation cap.

Example:

```
/iris-lore cap value:6
```

## Owner-gated runtime tools

When `IRIS_OWNER_ID` is set, the matching Discord user can ask I.R.I.S in natural language to manage runtime-only owner tools:

- Add, remove, and list MCP servers backed by `mcps.json`.
- Set, reset, and inspect model overrides persisted in `global_settings`.

These are not public slash commands. They are LLM tool calls gated by caller identity.

## General error behavior

- Unknown or malformed slash options are handled by Discord before command execution when possible.
- Non-admin users receive the command's Indonesian admin refusal.
- Transient database errors return a short Indonesian retry message and are logged server-side.
