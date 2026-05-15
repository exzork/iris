package memesearch

type Safety string

const (
	SafetySafe    Safety = "safe"
	SafetyNSFW    Safety = "nsfw"
	SafetyUnknown Safety = "unknown"
)

type Source string

const (
	SourceDiscordHistory Source = "discord_history"
	SourceGuildSticker   Source = "guild_sticker"
	SourceGiphy          Source = "giphy"
	SourceTenor          Source = "tenor"
	SourceX              Source = "x"
	SourceReddit         Source = "reddit"
	SourceFacebook       Source = "facebook"
)

type MediaItem struct {
	URL       string
	Source    Source
	MimeType  string
	Caption   string
	Safety    Safety
	StickerID string
}
