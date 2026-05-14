package patchnotes

// SourceLevel represents the authority level of a patch note or news source.
type SourceLevel string

const (
	LevelOfficial  SourceLevel = "official"
	LevelWiki      SourceLevel = "wiki"
	LevelCommunity SourceLevel = "community"
)

// String returns the string representation of the SourceLevel.
func (s SourceLevel) String() string {
	return string(s)
}

// Label returns the Indonesian label for the SourceLevel.
func (s SourceLevel) Label() string {
	switch s {
	case LevelOfficial:
		return "Resmi"
	case LevelWiki:
		return "Wiki"
	case LevelCommunity:
		return "Komunitas"
	default:
		return "Tidak Diketahui"
	}
}
