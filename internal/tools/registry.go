package tools

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type CallerContext struct {
	IsAdmin bool
}

type ExecuteRequest struct {
	GuildID int64
	UserID  int64
	Tool    string
	Args    map[string]interface{}
	Caller  CallerContext
}

type ExecuteResult struct {
	Output    string
	Err       error
	Truncated bool
	Duration  time.Duration
}

type Registry struct {
	mu    sync.RWMutex
	defs  map[string]*ToolDefinition
	audit AuditLogger
}

func NewRegistry(audit AuditLogger) *Registry {
	return &Registry{
		defs:  make(map[string]*ToolDefinition),
		audit: audit,
	}
}

func (r *Registry) Register(def *ToolDefinition) error {
	if def.Tool == nil {
		return fmt.Errorf("%w: tool is nil", ErrInvalidSchema)
	}

	schema := def.Tool.Schema()
	if schema == nil {
		return fmt.Errorf("%w: schema is nil", ErrInvalidSchema)
	}

	if err := schema.Validate(); err != nil {
		return fmt.Errorf("%w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.defs[schema.Name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateTool, schema.Name)
	}

	r.defs[schema.Name] = def
	return nil
}

func (r *Registry) Get(name string) (*ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.defs[name]
	return def, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.defs))
	for name := range r.defs {
		names = append(names, name)
	}
	return names
}

// UnregisterPrefix removes every tool whose name starts with any of the
// provided prefixes. Used during MCP hot-reload to evict stale adapters
// before a fresh manager spawns new ones.
func (r *Registry) UnregisterPrefix(prefixes []string) int {
	if len(prefixes) == 0 {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for name := range r.defs {
		for _, p := range prefixes {
			if p != "" && len(name) >= len(p) && name[:len(p)] == p {
				delete(r.defs, name)
				removed++
				break
			}
		}
	}
	return removed
}

func (r *Registry) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	start := time.Now()

	def, exists := r.Get(req.Tool)
	if !exists {
		r.recordAudit(ctx, req, "", "unknown_tool", "", time.Since(start))
		return ExecuteResult{
			Err:      fmt.Errorf("%w: %q", ErrUnknownTool, req.Tool),
			Duration: time.Since(start),
		}
	}

	if def.AdminOnly && !req.Caller.IsAdmin {
		r.recordAudit(ctx, req, "", "permission_denied", "", time.Since(start))
		return ExecuteResult{
			Err:      ErrPermissionDenied,
			Duration: time.Since(start),
		}
	}

	if err := def.Tool.Schema().ValidateArgs(req.Args); err != nil {
		r.recordAudit(ctx, req, "", "invalid_args", err.Error(), time.Since(start))
		return ExecuteResult{
			Err:      err,
			Duration: time.Since(start),
		}
	}

	timeout := def.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := def.Tool.Run(execCtx, req.Args)
	duration := time.Since(start)

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			r.recordAudit(ctx, req, "", "timeout", "", duration)
			return ExecuteResult{
				Err:      ErrTimeout,
				Duration: duration,
			}
		}
		r.recordAudit(ctx, req, "", "error", err.Error(), duration)
		return ExecuteResult{
			Err:      err,
			Duration: duration,
		}
	}

	maxOutput := def.MaxOutput
	if maxOutput <= 0 {
		maxOutput = 16 * 1024
	}
	truncated := false
	if len(output) > maxOutput {
		output = output[:maxOutput]
		truncated = true
		r.recordAudit(ctx, req, output, "truncated", "", duration)
	} else {
		r.recordAudit(ctx, req, output, "ok", "", duration)
	}

	return ExecuteResult{
		Output:    output,
		Truncated: truncated,
		Duration:  duration,
	}
}

func (r *Registry) recordAudit(ctx context.Context, req ExecuteRequest, output string, status string, errMsg string, duration time.Duration) {
	if r.audit == nil {
		return
	}
	evt := AuditEvent{
		GuildID:  req.GuildID,
		UserID:   req.UserID,
		Tool:     req.Tool,
		Args:     req.Args,
		Output:   output,
		Status:   status,
		Duration: duration,
		Error:    errMsg,
		At:       time.Now(),
	}
	_ = r.audit.Record(ctx, evt)
}

// OpenAIFunctions returns all non-admin tools in OpenAI function-calling format.
func (r *Registry) OpenAIFunctions() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]map[string]interface{}, 0)
	for _, def := range r.defs {
		if def.AdminOnly {
			continue
		}
		schema := def.Tool.Schema()
		result = append(result, schema.ToOpenAIFunction())
	}
	return result
}
