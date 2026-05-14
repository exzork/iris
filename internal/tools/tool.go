package tools

import (
	"context"
	"time"
)

type Tool interface {
	Schema() *Schema
	Run(ctx context.Context, args map[string]interface{}) (string, error)
}

type ToolDefinition struct {
	Tool      Tool
	Timeout   time.Duration
	MaxOutput int
	AdminOnly bool
}

func (td *ToolDefinition) GetTimeout() time.Duration {
	if td.Timeout == 0 {
		return 10 * time.Second
	}
	return td.Timeout
}

func (td *ToolDefinition) GetMaxOutput() int {
	if td.MaxOutput == 0 {
		return 16384
	}
	return td.MaxOutput
}
