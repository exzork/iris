package wire

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/eko/iris-bot/internal/discord"
	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/memory"
	"github.com/eko/iris-bot/internal/orchestrator"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/eko/iris-bot/internal/router"
	"github.com/eko/iris-bot/internal/safety"
	"github.com/eko/iris-bot/internal/tools"
	"github.com/jackc/pgx/v5"
)

// MemoryStoreAdapter wraps MemoryRepo to satisfy memory.MemoryStore interface.
type MemoryStoreAdapter struct {
	Repo *repository.MemoryRepo
}

func (a *MemoryStoreAdapter) Save(ctx context.Context, guildID int64, content string, embedding []float32) error {
	return a.Repo.Save(ctx, guildID, content, embedding)
}

func (a *MemoryStoreAdapter) SearchSimilar(ctx context.Context, guildID int64, embedding []float32, limit int) ([]domain.MemoryRecord, error) {
	return a.Repo.SearchSimilar(ctx, guildID, embedding, limit)
}

// LoreStoreAdapter wraps LoreRepo to satisfy rag.ChunkStore interface.
type LoreStoreAdapter struct {
	Repo *repository.LoreRepo
}

func (a *LoreStoreAdapter) SearchSimilar(ctx context.Context, embedding []float32, topK int) ([]rag.ScoredChunk, error) {
	// TODO: LoreRepo.SearchChunks returns domain.LoreCitation, not rag.ScoredChunk.
	// For now, return empty to avoid blocking bot startup.
	// Wire real lore chunks once repository exposes the right shape.
	return nil, nil
}

// ExceptionChannelAdapter wraps ExceptionChannelRepo to satisfy router.ExceptionChannelQuerier interface.
type ExceptionChannelAdapter struct {
	Repo *repository.ExceptionChannelRepo
}

func (a *ExceptionChannelAdapter) IsException(ctx context.Context, guildID, channelID int64) (bool, error) {
	return a.Repo.IsException(ctx, guildID, channelID)
}

// GuildStoreAdapter wraps GuildRepo to satisfy bootstrap.GuildStore interface.
type GuildStoreAdapter struct {
	Repo *repository.GuildRepo
}

func (a *GuildStoreAdapter) Upsert(ctx context.Context, g *domain.Guild) error {
	if a.Repo == nil {
		return nil
	}
	existing, err := a.Repo.GetByID(ctx, g.ID)
	if err == nil && existing != nil {
		return nil
	}
	return a.Repo.Create(ctx, g)
}

func (a *GuildStoreAdapter) Get(ctx context.Context, id int64) (*domain.Guild, error) {
	if a.Repo == nil {
		return nil, nil
	}
	guild, err := a.Repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set") {
			return nil, nil
		}
		return nil, err
	}
	return guild, nil
}

// SettingsStoreAdapter wraps SettingsRepo to satisfy bootstrap.SettingsStore interface.
type SettingsStoreAdapter struct {
	Repo *repository.SettingsRepo
}

func (a *SettingsStoreAdapter) Get(ctx context.Context, guildID int64, key string) (string, bool, error) {
	if a.Repo == nil {
		return "", false, nil
	}
	cfg, err := a.Repo.GetByKey(ctx, guildID, key)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows in result set") {
			return "", false, nil
		}
		return "", false, err
	}
	if cfg == nil {
		return "", false, nil
	}
	return cfg.SettingValue, true, nil
}

func (a *SettingsStoreAdapter) Set(ctx context.Context, guildID int64, key, value string) error {
	cfg := &domain.GuildConfig{
		GuildID:      guildID,
		SettingKey:   key,
		SettingValue: value,
	}
	return a.Repo.Save(ctx, cfg)
}

func (a *SettingsStoreAdapter) List(ctx context.Context, guildID int64) (map[string]string, error) {
	configs, err := a.Repo.GetAllByGuild(ctx, guildID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, cfg := range configs {
		result[cfg.SettingKey] = cfg.SettingValue
	}
	return result, nil
}

// LLMAdapter wraps llm.Client to satisfy app.LLMPort interface.
type LLMAdapter struct {
	Client *llm.Client
}

func (a *LLMAdapter) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	return a.Client.Chat(ctx, guildID, messages)
}

func (a *LLMAdapter) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	return a.Client.ChatWithModel(ctx, model, guildID, messages)
}

// ImageAdapter wraps llm.ImageClient to satisfy app.ImagePort interface.
type ImageAdapter struct {
	Client *llm.ImageClient
}

func (a *ImageAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	return a.Client.Generate(ctx, prompt)
}

// LoreAdapter wraps rag.Composer to satisfy app.LorePort interface.
type LoreAdapter struct {
	Composer *rag.Composer
}

func (a *LoreAdapter) Compose(ctx context.Context, query string) (*rag.PromptContext, *rag.UnsupportedResponse, error) {
	return a.Composer.Compose(ctx, query)
}

// TriggerAdapter wraps router.TriggerRouter to satisfy app.TriggerPort interface.
type TriggerAdapter struct {
	Router *router.TriggerRouter
}

func (a *TriggerAdapter) Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error) {
	return a.Router.Decide(ctx, event)
}

// DiscordSenderAdapter wraps discord.GatewayAdapter to satisfy app.SenderPort interface.
type DiscordSenderAdapter struct {
	Gateway *discord.GatewayAdapter
}

func (a *DiscordSenderAdapter) Send(ctx context.Context, guildID, channelID int64, content string) error {
	return a.Gateway.SendMessage(ctx, guildID, channelID, content)
}

func (a *DiscordSenderAdapter) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	return a.Gateway.SendMessage(ctx, guildID, channelID, content)
}

func (a *DiscordSenderAdapter) SendMessageReturningID(ctx context.Context, guildID, channelID int64, content string) (int64, error) {
	return a.Gateway.SendMessageReturningID(ctx, guildID, channelID, content)
}

func (a *DiscordSenderAdapter) EditMessage(ctx context.Context, guildID, channelID, messageID int64, content string) error {
	return a.Gateway.EditMessage(ctx, guildID, channelID, messageID, content)
}

func (a *DiscordSenderAdapter) SendTyping(ctx context.Context, guildID, channelID int64) error {
	return a.Gateway.SendTyping(ctx, guildID, channelID)
}

func (a *DiscordSenderAdapter) SendImageEmbeds(ctx context.Context, guildID, channelID int64, urls []string) error {
	return a.Gateway.SendImageEmbeds(ctx, guildID, channelID, urls)
}

func (a *DiscordSenderAdapter) SendFiles(ctx context.Context, guildID, channelID int64, files []orchestrator.FileAttachment) error {
	out := make([]discord.AttachmentFile, 0, len(files))
	for _, f := range files {
		out = append(out, discord.AttachmentFile{
			Name:        f.Name,
			ContentType: f.ContentType,
			Reader:      bytes.NewReader(f.Bytes),
		})
	}
	return a.Gateway.SendFiles(ctx, guildID, channelID, out)
}

// GuildEnsurer interface for ensuring guild row exists before writes.
type GuildEnsurer interface {
	Ensure(ctx context.Context, guildID int64) error
}

// GuildEnsurerAdapter wraps GuildRepo to satisfy GuildEnsurer interface.
type GuildEnsurerAdapter struct {
	Repo *repository.GuildRepo
}

func (a *GuildEnsurerAdapter) Ensure(ctx context.Context, guildID int64) error {
	if a.Repo == nil {
		return nil
	}
	return a.Repo.EnsureGuild(ctx, guildID)
}

// ChannelCaptureAdapter wraps ChannelMessageRepo to satisfy orchestrator.ChannelCapture interface.
type ChannelCaptureAdapter struct {
	Repo         *repository.ChannelMessageRepo
	GuildEnsurer GuildEnsurer
}

func (a *ChannelCaptureAdapter) Capture(ctx context.Context, msg *domain.ChannelMessage) error {
	if a.Repo == nil {
		return nil
	}

	// Ensure guild row exists before any per-guild write
	if a.GuildEnsurer != nil && msg.GuildID > 0 {
		if err := a.GuildEnsurer.Ensure(ctx, msg.GuildID); err != nil {
			slog.WarnContext(ctx, "failed to ensure guild row", "guild_id", msg.GuildID, "error", err)
			// Don't block capture on ensure failure
		}
	}

	// Capture message with NULL embedding; async worker will backfill embeddings
	return a.Repo.Upsert(ctx, msg)
}

// ContextStoreAdapter wraps ChannelMessageRepo to satisfy orchestrator.ContextStore interface.
type ContextStoreAdapter struct {
	Repo *repository.ChannelMessageRepo
}

func (a *ContextStoreAdapter) ListRecent(ctx context.Context, guildID, channelID int64, limit int) ([]*domain.ChannelMessage, error) {
	if a.Repo == nil {
		return nil, nil
	}
	return a.Repo.ListRecent(ctx, guildID, channelID, limit)
}

func (a *ContextStoreAdapter) GetByID(ctx context.Context, guildID, messageID int64) (*domain.ChannelMessage, error) {
	if a.Repo == nil {
		return nil, nil
	}
	return a.Repo.GetByID(ctx, guildID, messageID)
}

func (a *ContextStoreAdapter) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	if a.Repo == nil {
		return nil, nil
	}
	return a.Repo.ListByUserAcrossChannels(ctx, guildID, userID, sinceMinutes, limit)
}

// CrossChannelLLMAdapter wraps llm.Client for classifier interfaces.
type CrossChannelLLMAdapter struct {
	Client *llm.Client
}

func (a *CrossChannelLLMAdapter) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	if a.Client == nil {
		return "", nil
	}
	return a.Client.ChatWithModel(ctx, model, guildID, messages)
}

// CandidateStoreAdapter wraps ChannelMessageRepo for cross-channel candidate queries.
type CandidateStoreAdapter struct {
	Repo *repository.ChannelMessageRepo
}

func (a *CandidateStoreAdapter) ListByUserAcrossChannels(ctx context.Context, guildID int64, userID int64, sinceMinutes int, limit int) ([]*domain.ChannelMessage, error) {
	if a.Repo == nil {
		return nil, nil
	}
	return a.Repo.ListByUserAcrossChannels(ctx, guildID, userID, sinceMinutes, limit)
}

// ChannelAllowAdapter wraps AllowedChannelRepo for include-list filtering.
type ChannelAllowAdapter struct {
	Repo *repository.AllowedChannelRepo
}

func (a *ChannelAllowAdapter) HasAny(ctx context.Context, guildID int64) (bool, error) {
	if a.Repo == nil {
		return false, nil
	}
	return a.Repo.HasAny(ctx, guildID)
}

func (a *ChannelAllowAdapter) IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	if a.Repo == nil {
		return false, nil
	}
	return a.Repo.IsAllowed(ctx, guildID, channelID)
}

// MemoryWriterAdapter wraps existing memory service API for async promotion writes.
type MemoryWriterAdapter struct {
	Service interface {
		Consider(ctx context.Context, guildID, userID int64, text string) (bool, error)
	}
}

func (a *MemoryWriterAdapter) Save(ctx context.Context, guildID int64, userID int64, content string) error {
	if a.Service == nil {
		return nil
	}
	_, err := a.Service.Consider(ctx, guildID, userID, content)
	return err
}

// SafetyCheckerAdapter wraps safety helpers for memory-summary admission checks.
type SafetyCheckerAdapter struct {
	Injection *safety.InjectionFilter
	Output    *safety.OutputFilter
}

func (a *SafetyCheckerAdapter) IsSafeForMemory(content string) bool {
	if a.Injection == nil {
		a.Injection = safety.NewInjectionFilter()
	}
	if a.Output == nil {
		a.Output = safety.NewOutputFilter()
	}
	if len(a.Injection.Detect(content)) > 0 {
		return false
	}
	filtered := a.Output.Apply(content)
	if filtered.Blocked {
		return false
	}
	return strings.TrimSpace(filtered.Content) != ""
}

// DeciderAdapter wraps router.TriggerRouter to satisfy orchestrator.Decider interface.
type DeciderAdapter struct {
	Router *router.TriggerRouter
}

func (a *DeciderAdapter) Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error) {
	return a.Router.Decide(ctx, event)
}

// LLMCallerAdapter wraps llm.Client to satisfy orchestrator.LLMCaller interface.
type LLMCallerAdapter struct {
	Client *llm.Client
}

func (a *LLMCallerAdapter) Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error) {
	if a.Client == nil {
		return "", nil
	}
	return a.Client.Chat(ctx, guildID, messages)
}

func (a *LLMCallerAdapter) ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error) {
	if a.Client == nil {
		return "", nil
	}
	return a.Client.ChatWithModel(ctx, model, guildID, messages)
}

// ToolCallingLLMAdapter wraps llm.Client to satisfy orchestrator.ToolCallingLLM interface.
type ToolCallingLLMAdapter struct {
	Client *llm.Client
}

func (a *ToolCallingLLMAdapter) ChatWithTools(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsConfig) (string, error) {
	if a.Client == nil {
		return "", nil
	}
	return a.Client.ChatWithTools(ctx, messages, cfg)
}

// RegistryExecutor wraps tools.Registry to satisfy llm.ToolExecutor interface.
type RegistryExecutor struct {
	Reg *tools.Registry
}

func (e *RegistryExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	if e.Reg == nil {
		return "", nil
	}
	logger := slog.Default()
	argsForLog := summarizeArgs(args)
	logger.InfoContext(ctx, "tool_call_start", "tool", name, "args", argsForLog)
	start := time.Now()
	res := e.Reg.Execute(ctx, tools.ExecuteRequest{
		Tool:    name,
		Args:    args,
		GuildID: 0,
		UserID:  0,
		Caller:  tools.CallerContext{IsAdmin: false},
	})
	elapsedMS := time.Since(start).Milliseconds()
	if res.Err != nil {
		logger.WarnContext(ctx, "tool_call_error",
			"tool", name,
			"args", argsForLog,
			"elapsed_ms", elapsedMS,
			"err", res.Err.Error(),
		)
		return "", res.Err
	}
	outputLen := len(res.Output)
	preview := res.Output
	if outputLen > 600 {
		preview = res.Output[:600] + "...[truncated]"
	}
	logger.InfoContext(ctx, "tool_call_result",
		"tool", name,
		"args", argsForLog,
		"elapsed_ms", elapsedMS,
		"output_len", outputLen,
		"output_preview", preview,
	)
	return res.Output, nil
}

func summarizeArgs(args map[string]interface{}) map[string]interface{} {
	if len(args) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(args))
	for k, v := range args {
		switch val := v.(type) {
		case string:
			if len(val) > 200 {
				out[k] = val[:200] + "...[truncated]"
				continue
			}
			out[k] = val
		default:
			out[k] = val
		}
	}
	return out
}

type StreamLLMAdapter struct {
	Client *llm.Client
}

func (a *StreamLLMAdapter) ChatStream(ctx context.Context, model string, guildID int64, messages []map[string]string, cb llm.StreamCallbacks) (string, error) {
	return a.Client.ChatStream(ctx, model, guildID, messages, cb)
}

type StreamToolsLLMAdapter struct {
	Client *llm.Client
}

func (a *StreamToolsLLMAdapter) ChatWithToolsStream(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsStreamConfig) (string, error) {
	return a.Client.ChatWithToolsStream(ctx, messages, cfg)
}

// EscalationAwareExecutor wraps a ToolExecutor and tracks when the escalate_to_strong_model tool fires.
// It captures the escalation reason for the orchestrator to detect and handle escalation.
type EscalationAwareExecutor struct {
	wrapped llm.ToolExecutor
	reason  string
}

func NewEscalationAwareExecutor(wrapped llm.ToolExecutor) *EscalationAwareExecutor {
	return &EscalationAwareExecutor{wrapped: wrapped}
}

func (e *EscalationAwareExecutor) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	result, err := e.wrapped.Execute(ctx, name, args)
	// Capture escalation marker if the escalate tool fired
	if name == "escalate_to_strong_model" && err == nil && strings.HasPrefix(result, "ESCALATED:") {
		e.reason = strings.TrimPrefix(result, "ESCALATED:")
	}
	return result, err
}

func (e *EscalationAwareExecutor) Reason() string {
	return e.reason
}

func (e *EscalationAwareExecutor) Clear() {
	e.reason = ""
}

// BehaviorProfileUpdater wraps BehaviorProfileService to satisfy orchestrator.BehaviorUpdater interface.
// It triggers profile synthesis from recent user messages.
type BehaviorProfileUpdater struct {
	Service any
}

func (a *BehaviorProfileUpdater) UpdateFromMessage(ctx context.Context, guildID, userID int64, content string, createdAt time.Time) error {
	if a.Service == nil {
		return nil
	}
	if guildID == 0 || userID == 0 {
		return nil
	}

	svc, ok := a.Service.(interface {
		UpdateFromSamples(ctx context.Context, guildID, userID int64, samples interface{}) error
	})
	if !ok {
		return nil
	}

	return svc.UpdateFromSamples(ctx, guildID, userID, nil)
}

// BehaviorProfileServiceAdapter adapts BehaviorProfileService to the BehaviorUpdater interface.
type BehaviorProfileServiceAdapter struct {
	svc interface {
		UpdateFromSamples(ctx context.Context, guildID, userID int64, samples interface{}) (interface{}, error)
	}
}

func (a *BehaviorProfileServiceAdapter) UpdateFromSamples(ctx context.Context, guildID, userID int64, samples interface{}) (interface{}, error) {
	return a.svc.UpdateFromSamples(ctx, guildID, userID, samples)
}

// BehaviorProfileUpdateAdapter buffers user messages and flushes them to the behavior profile service
// when the buffer reaches a threshold or a time limit is exceeded. This prevents excessive database
// writes while still learning from user behavior in near-real-time.
type BehaviorProfileUpdateAdapter struct {
	svc           interface {
		UpdateFromSamples(ctx context.Context, guildID, userID int64, samples []memory.MessageSample) (*domain.UserBehaviorProfile, error)
	}
	bufferThreshold int
	flushInterval   time.Duration
	mu              sync.Mutex
	buf             map[[2]int64][]memory.MessageSample
	lastFlush       map[[2]int64]time.Time
}

// NewBehaviorProfileUpdateAdapter creates a buffering adapter for behavior profile updates.
func NewBehaviorProfileUpdateAdapter(
	svc interface {
		UpdateFromSamples(ctx context.Context, guildID, userID int64, samples []memory.MessageSample) (*domain.UserBehaviorProfile, error)
	},
	bufferThreshold int,
	flushInterval time.Duration,
) *BehaviorProfileUpdateAdapter {
	return &BehaviorProfileUpdateAdapter{
		svc:             svc,
		bufferThreshold: bufferThreshold,
		flushInterval:   flushInterval,
		buf:             make(map[[2]int64][]memory.MessageSample),
		lastFlush:       make(map[[2]int64]time.Time),
	}
}

// UpdateFromMessage appends a message sample to the buffer for the given (guildID, userID) pair.
// If the buffer reaches the threshold or the flush interval has elapsed, it flushes to the service.
func (a *BehaviorProfileUpdateAdapter) UpdateFromMessage(ctx context.Context, guildID, userID int64, content string, createdAt time.Time) error {
	if a.svc == nil || guildID == 0 || userID == 0 {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	key := [2]int64{guildID, userID}
	a.buf[key] = append(a.buf[key], memory.MessageSample{
		Content:   content,
		CreatedAt: createdAt,
		IsBot:     false,
	})

	shouldFlush := false
	if len(a.buf[key]) >= a.bufferThreshold {
		shouldFlush = true
	} else if lastFlush, ok := a.lastFlush[key]; ok && time.Since(lastFlush) > a.flushInterval {
		shouldFlush = true
	}

	if !shouldFlush {
		return nil
	}

	samples := a.buf[key]
	delete(a.buf, key)
	a.lastFlush[key] = time.Now()

	go func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := a.svc.UpdateFromSamples(flushCtx, guildID, userID, samples); err != nil {
			slog.DebugContext(flushCtx, "behavior_profile_flush_failed", "guild", guildID, "user", userID, "error", err)
		}
	}()

	return nil
}

var _ orchestrator.BehaviorUpdater = (*BehaviorProfileUpdateAdapter)(nil)

// LoreThreadLLMCallerAdapter wraps llm.Client to satisfy lorethread.LLMCaller interface.
type LoreThreadLLMCallerAdapter struct {
	Client *llm.Client
	Model  string
}

func (a *LoreThreadLLMCallerAdapter) Call(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if a.Client == nil {
		return "", nil
	}

	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userPrompt},
	}

	model := a.Model
	if model == "" {
		return a.Client.Chat(ctx, 0, messages)
	}
	return a.Client.ChatWithModel(ctx, model, 0, messages)
}

var _ orchestrator.CandidateStore = (*CandidateStoreAdapter)(nil)
var _ orchestrator.ChannelAllowQuerier = (*ChannelAllowAdapter)(nil)
var _ orchestrator.CrossChannelLLM = (*CrossChannelLLMAdapter)(nil)
var _ orchestrator.MemoryWriter = (*MemoryWriterAdapter)(nil)
var _ orchestrator.SafetyChecker = (*SafetyCheckerAdapter)(nil)
var _ orchestrator.Decider = (*DeciderAdapter)(nil)
var _ orchestrator.LLMCaller = (*LLMCallerAdapter)(nil)
var _ orchestrator.ToolCallingLLM = (*ToolCallingLLMAdapter)(nil)
var _ orchestrator.StreamLLM = (*StreamLLMAdapter)(nil)
var _ orchestrator.StreamToolsLLM = (*StreamToolsLLMAdapter)(nil)
var _ orchestrator.BehaviorUpdater = (*BehaviorProfileUpdater)(nil)
var _ llm.ToolExecutor = (*RegistryExecutor)(nil)
var _ llm.ToolExecutor = (*EscalationAwareExecutor)(nil)