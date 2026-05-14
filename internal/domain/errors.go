package domain

import "errors"

var (
	ErrToolRequestMissingID = errors.New("tool request missing ID")
	ErrToolRequestMissingName = errors.New("tool request missing tool name")
	ErrToolResultMissingID = errors.New("tool result missing ID")
)
