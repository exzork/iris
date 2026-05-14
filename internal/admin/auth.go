package admin

import "context"

const AdminPermissionBit = 0x8

func IsAdmin(cmd *CommandContext) bool {
	if cmd.IsOwner {
		return true
	}

	if cmd.Permissions&AdminPermissionBit != 0 {
		return true
	}

	return false
}

func IsAdminWithRoles(cmd *CommandContext, adminRoles map[int64]bool) bool {
	if IsAdmin(cmd) {
		return true
	}

	for _, roleID := range cmd.RoleIDs {
		if adminRoles[roleID] {
			return true
		}
	}

	return false
}

func RequireAdmin(h Handler) Handler {
	return HandlerFunc(func(ctx context.Context, cmd *CommandContext, args []string) (string, error) {
		if !IsAdmin(cmd) {
			return "Mohon maaf, hanya admin server yang dapat mengubah konfigurasi I.R.I.S.", nil
		}
		return h.Handle(ctx, cmd, args)
	})
}
