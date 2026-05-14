package slash

import (
	"fmt"
)

func formatLoreStatus(settings *LoreSettings) string {
	status := "❌ Dinonaktifkan"
	if settings.Enabled {
		status = "✅ Diaktifkan"
	}

	return fmt.Sprintf(
		"**Status Lore Threads**\n"+
			"Status: %s\n"+
			"Thread Cap: %d per jam\n"+
			"Terakhir diperbarui: <t:%d:R>",
		status,
		settings.ThreadCapPerHour,
		settings.UpdatedAt.Unix(),
	)
}
