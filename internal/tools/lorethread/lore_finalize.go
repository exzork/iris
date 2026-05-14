package lorethread

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/eko/iris-bot/internal/lorethread"
	"github.com/eko/iris-bot/internal/tools"
)

const Name = "lore_finalize_now"

// Tool implements the lore_finalize_now tool for on-demand session finalization.
type Tool struct {
	Finalizer *lorethread.Finalizer
}

func New(finalizer *lorethread.Finalizer) *Tool {
	return &Tool{Finalizer: finalizer}
}

func (t *Tool) Schema() *tools.Schema {
	return &tools.Schema{
		Name:        Name,
		Description: "Closes the current channel's open lore session and posts a summary thread NOW. Only callable by the user who started the lore conversation. Returns thread_id and title on success.",
		Fields:      []tools.FieldSpec{}, // No args needed; guild/channel/requester come from context
	}
}

// ErrorResponse is the structured error response for the tool.
type ErrorResponse struct {
	Error       string `json:"error"`
	StarterID   int64  `json:"starter_user_id,omitempty"`
	Description string `json:"description,omitempty"`
}

// SuccessResponse is the structured success response for the tool.
type SuccessResponse struct {
	ThreadID       int64  `json:"thread_id"`
	MessageID      int64  `json:"message_id"`
	Title          string `json:"title"`
	SummaryPreview string `json:"summary_preview"`
}

// Run executes the lore finalization. Expects context to have caller user ID and guild/channel info.
func (t *Tool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	if t.Finalizer == nil {
		errResp := ErrorResponse{
			Error:       "internal_error",
			Description: "Finalizer not configured",
		}
		return marshalResponse(errResp), nil
	}

	// Extract context from the invocation context
	// The caller user ID, guild ID, and channel ID should be set by the tool framework
	callerUserID, ok := ctx.Value("caller_user_id").(int64)
	if !ok {
		errResp := ErrorResponse{
			Error:       "internal_error",
			Description: "Caller user ID not available in context",
		}
		return marshalResponse(errResp), nil
	}

	guildID, ok := ctx.Value("guild_id").(int64)
	if !ok {
		errResp := ErrorResponse{
			Error:       "internal_error",
			Description: "Guild ID not available in context",
		}
		return marshalResponse(errResp), nil
	}

	channelID, ok := ctx.Value("channel_id").(int64)
	if !ok {
		errResp := ErrorResponse{
			Error:       "internal_error",
			Description: "Channel ID not available in context",
		}
		return marshalResponse(errResp), nil
	}

	// Call the finalizer
	result, err := t.Finalizer.ForceFinalize(ctx, guildID, channelID, callerUserID)
	if err != nil {
		slog.WarnContext(ctx, "lore_finalize_now failed", "error", err, "guild", guildID, "channel", channelID, "caller", callerUserID)

		// Map specific errors to structured responses
		if errors.Is(err, lorethread.ErrNoOpenSession) {
			errResp := ErrorResponse{
				Error:       "no_open_session",
				Description: "No open lore session in this channel",
			}
			return marshalResponse(errResp), nil
		}
		if errors.Is(err, lorethread.ErrNotConversationStarter) {
			errResp := ErrorResponse{
				Error:       "not_starter",
				Description: "Only the conversation starter can finalize the session",
			}
			return marshalResponse(errResp), nil
		}

		// Generic error
		errResp := ErrorResponse{
			Error:       "finalization_failed",
			Description: err.Error(),
		}
		return marshalResponse(errResp), nil
	}

	if result == nil {
		errResp := ErrorResponse{
			Error:       "internal_error",
			Description: "Finalization returned nil result",
		}
		return marshalResponse(errResp), nil
	}

	successResp := SuccessResponse{
		ThreadID:       result.ThreadID,
		MessageID:      result.MessageID,
		Title:          result.Title,
		SummaryPreview: result.SummaryPreview,
	}
	return marshalResponse(successResp), nil
}

func marshalResponse(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"error":"json_marshal_failed"}`
	}
	return string(data)
}
