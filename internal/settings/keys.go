package settings

import "strings"

type Key string

const (
	KeyAdminRoleIDs          Key = "admin_role_ids"          // comma-separated int64
	KeyDefaultLocale         Key = "default_locale"          // "id-ID"
	KeyMemoryEnabled         Key = "memory_enabled"          // bool
	KeyLoreCitationsRequired Key = "lore_citations_required" // bool
	KeyMaxResponseChars      Key = "max_response_chars"      // int
	KeyImageCooldownSec      Key = "image_cooldown_sec"      // int (60 default)
	KeyMemesEnabled          Key = "memes_enabled"           // bool (true default)
	KeyRemindersEnabled      Key = "reminders_enabled"       // bool (true default)
	KeyRateLimitUserPerMin   Key = "ratelimit_user_per_min"
	KeyRateLimitGuildPerMin  Key = "ratelimit_guild_per_min"
	KeyReminderChannelID     Key = "reminder_channel_id"     // int64
)

// DefaultValue returns the global default for a key.
func DefaultValue(k Key) (string, bool) {
	defaults := map[Key]string{
		KeyDefaultLocale:         "id-ID",
		KeyMemoryEnabled:         "true",
		KeyLoreCitationsRequired: "true",
		KeyMaxResponseChars:      "2000",
		KeyImageCooldownSec:      "60",
		KeyMemesEnabled:          "true",
		KeyRemindersEnabled:      "true",
		KeyRateLimitUserPerMin:   "30",
		KeyRateLimitGuildPerMin:  "120",
	}
	v, ok := defaults[k]
	return v, ok
}

// IsKnown reports whether a key is registered.
func IsKnown(k Key) bool {
	_, ok := DefaultValue(k)
	if ok {
		return true
	}
	switch k {
	case KeyAdminRoleIDs, KeyReminderChannelID:
		return true
	}
	return false
}

// ParseKey accepts an untyped input (trims + lowers) and returns Key + ok.
func ParseKey(s string) (Key, bool) {
	k := Key(strings.TrimSpace(strings.ToLower(s)))
	return k, IsKnown(k)
}
