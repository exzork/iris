package lorethread

import "errors"

// ErrDMNotSupported is returned when attempting to create a thread in a DM channel.
var ErrDMNotSupported = errors.New("thread creation not supported in DM channels")

// ErrThreadAlreadyExists is returned when Discord rejects with code 160004; adapters fall back to a standalone thread.
var ErrThreadAlreadyExists = errors.New("thread already exists for parent message")

// ErrNoSessionsDue is returned when no lore sessions are currently due for summary processing.
var ErrNoSessionsDue = errors.New("no lore sessions due")
