package tools

import "errors"

var (
	ErrUnknownTool        = errors.New("unknown tool")
	ErrPermissionDenied   = errors.New("permission denied")
	ErrTimeout            = errors.New("tool execution timed out")
	ErrDuplicateTool      = errors.New("duplicate tool registration")
	ErrInvalidSchema      = errors.New("invalid schema")
	ErrMissingRequiredArg = errors.New("missing required arg")
	ErrInvalidArgType     = errors.New("invalid arg type")
	ErrUnknownArg         = errors.New("unknown arg")
)
