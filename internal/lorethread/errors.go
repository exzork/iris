package lorethread

import "errors"

// ErrDMNotSupported is returned when attempting to create a thread in a DM channel.
var ErrDMNotSupported = errors.New("thread creation not supported in DM channels")

// ErrFirstMessageTooLong is returned when the first message exceeds Discord's 2000 character limit.
var ErrFirstMessageTooLong = errors.New("first message exceeds 2000 character limit")

// ErrNoSessionsDue is returned when no lore sessions are currently due for summary processing.
var ErrNoSessionsDue = errors.New("no lore sessions due")
