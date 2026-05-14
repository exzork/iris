package itemlookup

type Category string

const (
	CategoryEcho     Category = "echo"
	CategoryWeapon   Category = "weapon"
	CategoryMaterial Category = "material"
	CategoryUnknown  Category = "unknown"
)

type Item struct {
	ID       int64
	Name     string
	Aliases  []string
	Category Category
	Rarity   string
	PageURL  string
	Summary  string
}
