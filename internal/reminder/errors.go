package reminder

import (
	"errors"
)

var (
	ErrInvalidTimezone  = errors.New("invalid timezone")
	ErrInvalidHourMin   = errors.New("invalid hour:min format")
	ErrInvalidChannel   = errors.New("invalid channel id")
	ErrInvalidMessage   = errors.New("invalid message")
	ErrNotFound         = errors.New("reminder not found")
)
