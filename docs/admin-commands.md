# Admin Commands

The `!iris` command family is reserved for guild administrators. Commands run in any text channel the bot can see. Unless noted, the bot replies in the same channel.

## Who can run admin commands

A user must have the Discord **Manage Guild** permission in the guild where the command is issued. Users without this permission get a neutral refusal: *"Perintah ini hanya untuk administrator guild."*

Direct messages are rejected. Admin commands are guild-scoped only.

## Commands

### `!iris help`

Show the list of admin commands and a short usage hint.

Syntax:

```
!iris help
```

Example:

```
!iris help
```

Error behavior: none. Always returns the help text.

### `!iris exception add <channelID>`

Add a channel to the exception list. In exception channels the bot will not auto-reply to the `iris` name trigger. Mentions and replies still work.

Syntax:

```
!iris exception add <channelID>
```

Example:

```
!iris exception add 1234567890
```

Error behavior:

- Missing or malformed channel ID: *"Channel ID tidak valid."*
- Channel not found or not in this guild: *"Channel tidak ditemukan di guild ini."*
- Channel already in the list: no-op, returns *"Channel sudah ada di daftar pengecualian."*

### `!iris exception remove <channelID>`

Remove a channel from the exception list.

Syntax:

```
!iris exception remove <channelID>
```

Example:

```
!iris exception remove 1234567890
```

Error behavior:

- Missing or malformed channel ID: *"Channel ID tidak valid."*
- Channel not in the list: *"Channel tidak ada di daftar pengecualian."*

### `!iris exception list`

List the exception channels for the current guild.

Syntax:

```
!iris exception list
```

Example:

```
!iris exception list
```

Error behavior: returns an empty list message if none exist.

### `!iris ratelimit set <scope> <limit>`

Set a rate limit for a given scope. Supported scopes are `guild` and `user`. The limit is the maximum number of bot-triggering messages per minute.

Syntax:

```
!iris ratelimit set <scope> <limit>
```

Example:

```
!iris ratelimit set guild 60
!iris ratelimit set user 10
```

Error behavior:

- Unknown scope: *"Scope harus `guild` atau `user`."*
- Non-numeric or negative limit: *"Limit harus angka positif."*
- Limit above the configured hard ceiling: clamped to the ceiling, response notes the adjustment.

### `!iris ratelimit get`

Show the current rate limit configuration for the guild.

Syntax:

```
!iris ratelimit get
```

Example:

```
!iris ratelimit get
```

Error behavior: none.

### `!iris config list`

List all per-guild config keys and their current values.

Syntax:

```
!iris config list
```

Example:

```
!iris config list
```

Error behavior: none. Returns an empty list if no keys are set.

### `!iris config get <key>`

Show the current value of a single config key.

Syntax:

```
!iris config get <key>
```

Example:

```
!iris config get language
```

Error behavior:

- Missing key argument: *"Key wajib diisi."*
- Unknown key: *"Key tidak dikenal."*

### `!iris config set <key> <value>`

Set a config key for the current guild. Values are validated against the key's type (boolean, integer, or string with a fixed enum).

Syntax:

```
!iris config set <key> <value>
```

Example:

```
!iris config set language id
!iris config set autoreply true
```

Error behavior:

- Missing arguments: *"Key dan value wajib diisi."*
- Unknown key: *"Key tidak dikenal."*
- Invalid value for the key's type: *"Value tidak valid untuk key ini."*
- Audit: every successful `config set` writes an `audit_events` row with the actor, key, old value, and new value.

## General error behavior

- Commands with unknown verbs return *"Perintah tidak dikenal. Coba `!iris help`."*
- Commands issued by a non-admin return the permission refusal above and write an audit row.
- Commands that fail due to a transient database error return *"Terjadi kesalahan, coba lagi nanti."* and log the underlying error at `error` level.
