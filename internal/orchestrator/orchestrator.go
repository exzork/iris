package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/repository"
	"github.com/eko/iris-bot/internal/router"
)

var (
	ErrQueueFull      = errors.New("orchestrator queue full")
	ErrNotRunning     = errors.New("orchestrator not running")
	ErrAlreadyRunning = errors.New("orchestrator already running")
)

type Decider interface {
	Decide(ctx context.Context, event *domain.DiscordEvent) (*router.Decision, error)
}

type MessageSender interface {
	SendMessage(ctx context.Context, guildID, channelID int64, content string) error
}

type TypingSender interface {
	SendTyping(ctx context.Context, guildID, channelID int64) error
}

type LLMCaller interface {
	Chat(ctx context.Context, guildID int64, messages []map[string]string) (string, error)
	ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error)
}

type ToolCallingLLM interface {
	ChatWithTools(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsConfig) (string, error)
}

type StreamLLM interface {
	ChatStream(ctx context.Context, model string, guildID int64, messages []map[string]string, cb llm.StreamCallbacks) (string, error)
}

type StreamToolsLLM interface {
	ChatWithToolsStream(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsStreamConfig) (string, error)
}

type ChannelCapture interface {
	Capture(ctx context.Context, msg *domain.ChannelMessage) error
}

type BehaviorUpdater interface {
	UpdateFromMessage(ctx context.Context, guildID, userID int64, content string, createdAt time.Time) error
}

type LoreCapturer interface {
	OnMessage(ctx context.Context, event *domain.DiscordEvent)
}

type escalationAware interface {
	Reason() string
	Clear()
}

type Config struct {
	Router                 Decider
	LLM                    LLMCaller
	Discord                MessageSender
	ContextStore           ContextStore
	InWindowRelevance      InWindowRelevance
	CrossChannel           CrossChannelClassifier
	Promoter               MemoryPromoter
	ConversationRefresher  repository.ChannelConversationQuerier
	AllowedQuerier         repository.AllowedChannelQuerier
	Capture                ChannelCapture  // optional; if nil, skip persistence
	GuildMemory            GuildMemorySource // optional; if nil, skip guild recall
	UserBehavior           UserBehaviorSource // optional; if nil, skip user behavior hints
	BehaviorUpdater        BehaviorUpdater // optional; if nil, skip behavior learning
	LoreCapturer           LoreCapturer // optional; if nil, skip lore capture
	ConversationLockTTL    time.Duration
	QueueSize              int
	WorkerCount            int
	EnqueueLimit           time.Duration
	DedupeTTL              time.Duration
	TypingAfter            time.Duration
	TypingRepeat           time.Duration
	JobTimeout             time.Duration
	SystemPrompt           string
	ImmediateTyping        bool
	TierRouter             interface{}
	ToolsLLM               ToolCallingLLM           // optional; if nil, tool-calling path is off
	ToolExecutor           llm.ToolExecutor         // optional; required if ToolsLLM is set
	Tools                  []map[string]interface{} // optional; if empty, tool-calling path is off
	Streaming              bool                     // enable streaming responses
	StreamLLM              StreamLLM                // optional; if nil, streaming is off
	StreamToolsLLM         StreamToolsLLM           // optional; if nil, tool streaming is off
	RateLimiter            *ChannelRateLimiter      // optional; defaults to 5 sends/5s per channel
	StrongModel            string                   // optional; if set, enables escalation to stronger model

	AllowedChannelLister AllowedChannelLister // optional; if set, context builder pulls from every whitelisted channel
	ChannelNameResolver  ChannelNameResolver  // optional; resolves channel + thread names for the tagged message format
	Compactor            Compactor            // optional; LLM-backed summarizer used when context exceeds TotalCharBudget
	EpisodeArchiver      EpisodeArchiver      // optional; persists stash-style episodes before compaction squashes them
	LoreAnchorResolver   LoreAnchorResolver   // optional; resolves lore thread anchor metadata
	LoreCompactor        *LoreCompactor       // optional; applies 70% retention compaction for lore threads
	LoreCaptureTimeout   time.Duration        // optional; timeout for per-message lore capture. 0 = no deadline
}

type job struct {
	event     *domain.DiscordEvent
	dedupeKey string
}

type Orchestrator struct {
	cfg Config

	contextBuilder *ContextBuilder

	queue    chan job
	stopCh   chan struct{}
	wg       sync.WaitGroup
	refreshWG sync.WaitGroup
	started  atomic.Bool
	stopping atomic.Bool

	inFlight  atomic.Int64
	processed atomic.Int64
	dropped   atomic.Int64

	rootCtx    context.Context
	rootCancel context.CancelFunc

	dedupeMu sync.Mutex
	dedupe   map[string]time.Time
}

func New(cfg Config) *Orchestrator {
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 128
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4
	}
	if cfg.EnqueueLimit <= 0 {
		cfg.EnqueueLimit = 10 * time.Millisecond
	}
	if cfg.DedupeTTL <= 0 {
		cfg.DedupeTTL = 30 * time.Second
	}
	if cfg.TypingAfter <= 0 {
		cfg.TypingAfter = 500 * time.Millisecond
	}
	if cfg.TypingRepeat <= 0 {
		cfg.TypingRepeat = 5 * time.Second
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 5 * time.Minute
	}
	if cfg.ConversationLockTTL <= 0 {
		cfg.ConversationLockTTL = 5 * time.Minute
	}
	if !cfg.ImmediateTyping {
		cfg.ImmediateTyping = true
	}
	if cfg.RateLimiter == nil {
		cfg.RateLimiter = NewChannelRateLimiter(5, 5*time.Second)
	}

	contextBuilder := NewContextBuilder(ContextBuilderConfig{
		MinContext:                10,
		CurrentChannelMax:         10,
		ReplyDepthLimit:           5,
		PerMessageCharCap:         2000,
		IncludeAllAllowedChannels: cfg.AllowedChannelLister != nil,
		PerChannelLimit:           30,
		TotalCharBudget:           40000,
		CompactionKeepRecent:      40,
	})
	if cfg.GuildMemory != nil {
		contextBuilder.WithGuildMemory(cfg.GuildMemory)
	}
	if cfg.UserBehavior != nil {
		contextBuilder.WithUserBehavior(cfg.UserBehavior)
	}
	if cfg.AllowedChannelLister != nil {
		contextBuilder.WithAllowedChannels(cfg.AllowedChannelLister)
	}
	if cfg.ChannelNameResolver != nil {
		contextBuilder.WithChannelNames(cfg.ChannelNameResolver)
	}
	if cfg.Compactor != nil {
		contextBuilder.WithCompactor(cfg.Compactor)
	}
	if cfg.EpisodeArchiver != nil {
		contextBuilder.WithEpisodeArchiver(cfg.EpisodeArchiver)
	}
	if cfg.LoreAnchorResolver != nil {
		contextBuilder.WithLoreAnchorResolver(cfg.LoreAnchorResolver)
	}
	if cfg.LoreCompactor != nil {
		contextBuilder.WithLoreCompactor(cfg.LoreCompactor)
	}

	return &Orchestrator{
		cfg:             cfg,
		contextBuilder:  contextBuilder,
		queue:           make(chan job, cfg.QueueSize),
		stopCh:          make(chan struct{}),
		dedupe:          make(map[string]time.Time),
	}
}

func (o *Orchestrator) Start() {
	if !o.started.CompareAndSwap(false, true) {
		return
	}
	o.rootCtx, o.rootCancel = context.WithCancel(context.Background())
	for i := 0; i < o.cfg.WorkerCount; i++ {
		o.wg.Add(1)
		go o.worker()
	}
	if o.cfg.LoreCapturer != nil {
		slog.Info("lore_capture_armed")
	} else {
		slog.Info("lore_capture_disabled")
	}
}

func (o *Orchestrator) Stop() {
	if !o.stopping.CompareAndSwap(false, true) {
		o.wg.Wait()
		o.refreshWG.Wait()
		return
	}
	close(o.stopCh)
	if o.rootCancel != nil {
		o.rootCancel()
	}
	o.wg.Wait()
	o.refreshWG.Wait()
}

func (o *Orchestrator) Enqueue(ctx context.Context, event *domain.DiscordEvent) error {
	if !o.started.Load() {
		return ErrNotRunning
	}
	if o.stopping.Load() {
		return ErrNotRunning
	}
	if event == nil {
		return errors.New("nil event")
	}

	key := dedupeKey(event)
	if o.seenRecently(key) {
		return nil
	}

	j := job{event: event, dedupeKey: key}

	select {
	case o.queue <- j:
		return nil
	default:
	}

	timer := time.NewTimer(o.cfg.EnqueueLimit)
	defer timer.Stop()

	select {
	case o.queue <- j:
		return nil
	case <-timer.C:
		o.dropped.Add(1)
		return ErrQueueFull
	case <-ctx.Done():
		return ctx.Err()
	case <-o.stopCh:
		return ErrNotRunning
	}
}

func (o *Orchestrator) seenRecently(key string) bool {
	if key == "" {
		return false
	}
	now := time.Now()
	o.dedupeMu.Lock()
	defer o.dedupeMu.Unlock()

	o.evictExpiredLocked(now)

	if _, ok := o.dedupe[key]; ok {
		return true
	}
	o.dedupe[key] = now
	return false
}

func (o *Orchestrator) evictExpiredLocked(now time.Time) {
	ttl := o.cfg.DedupeTTL
	for k, t := range o.dedupe {
		if now.Sub(t) > ttl {
			delete(o.dedupe, k)
		}
	}
}

func (o *Orchestrator) worker() {
	defer o.wg.Done()
	for {
		select {
		case j := <-o.queue:
			o.handle(j)
		case <-o.stopCh:
			return
		}
	}
}

func (o *Orchestrator) handle(j job) {
	o.inFlight.Add(1)
	defer o.inFlight.Add(-1)
	defer o.processed.Add(1)

	ctx, cancel := context.WithTimeout(o.rootCtx, o.cfg.JobTimeout)
	defer cancel()

	event := j.event

	if event != nil && event.UserID != 0 {
		ctx = WithCallerUserID(ctx, event.UserID)
	}

	// Capture incoming user message for context window (before router decision)
	if o.cfg.Capture != nil && event.Message != nil && event.GuildID > 0 && event.ChannelID > 0 {
		msg := &domain.ChannelMessage{
			GuildID:   event.GuildID,
			ChannelID: event.ChannelID,
			MessageID: event.Message.ID,
			UserID:    event.UserID,
			Content:   event.Message.Content,
			IsBot:     false,
			CreatedAt: time.Now(),
		}
		if captureErr := o.cfg.Capture.Capture(ctx, msg); captureErr != nil {
			slog.DebugContext(ctx, "channel_capture", "guild", event.GuildID, "channel", event.ChannelID, "is_bot", false, "err", captureErr)
		}
	}

	// Update user behavior profile from this message (async, non-blocking)
	if o.cfg.BehaviorUpdater != nil && event.Message != nil && event.GuildID > 0 && event.UserID > 0 && !event.IsBot {
		go func() {
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := o.cfg.BehaviorUpdater.UpdateFromMessage(updateCtx, event.GuildID, event.UserID, event.Message.Content, event.Message.CreatedAt); err != nil {
				slog.DebugContext(updateCtx, "behavior_update_failed", "guild", event.GuildID, "user", event.UserID, "error", err)
			}
		}()
	}

	// Capture lore messages (async, non-blocking) BEFORE router decision.
	// The adapter handles allow-list filtering independently; capture should fire for all
	// allowed-channel messages from real users, regardless of whether Iris will reply.
	if o.cfg.LoreCapturer != nil && event.Message != nil && event.GuildID > 0 && event.ChannelID > 0 && event.UserID > 0 && !event.IsBot {
		go func() {
			var captureCtx context.Context
			var cancel context.CancelFunc
			if o.cfg.LoreCaptureTimeout > 0 {
				captureCtx, cancel = context.WithTimeout(context.Background(), o.cfg.LoreCaptureTimeout)
			} else {
				captureCtx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			o.cfg.LoreCapturer.OnMessage(captureCtx, event)
		}()
	}

	decision, err := o.cfg.Router.Decide(ctx, event)
	slog.InfoContext(ctx, "router_decision", "reason", func() string {
		if decision != nil {
			return string(decision.Reason)
		}
		return "nil"
	}(), "should", decision != nil && decision.Should)
	if err != nil || decision == nil || !decision.Should {
		return
	}

	// Check in-window relevance if configured (for active_conversation reason)
	if decision.Reason == router.ReasonActiveConversation && o.cfg.InWindowRelevance != nil && o.cfg.ContextStore != nil {
		contextMessages, err := o.cfg.ContextStore.ListRecent(ctx, event.GuildID, event.ChannelID, 10)
		if err != nil {
			slog.DebugContext(ctx, "relevance_check_failed", "error", err)
		} else {
			relevant, sim, threshold, err := o.cfg.InWindowRelevance.IsRelevant(ctx, event, contextMessages)
			if err != nil {
				slog.DebugContext(ctx, "relevance_check_error", "error", err)
			} else {
				slog.InfoContext(ctx, "conv_lock_similarity", "guild", event.GuildID, "channel", event.ChannelID, "sim", sim, "threshold", threshold, "decision", relevant)
				if !relevant {
					return
				}
			}
		}
	}

	// Fetch context for cross-channel and LLM
	var contextMessages []*domain.ChannelMessage
	if o.cfg.ContextStore != nil {
		var err error
		contextMessages, err = o.cfg.ContextStore.ListRecent(ctx, event.GuildID, event.ChannelID, 10)
		if err != nil {
			slog.DebugContext(ctx, "context_fetch_failed", "error", err)
		}
	}

	content := ""
	if event.Message != nil {
		content = event.Message.Content
	}

	// Build context using ContextBuilder with optional guild memory and user behavior
	var messages []map[string]string
	var buildErr error

	// Check for cross-channel candidates
	var crossChannelCandidates []*domain.ChannelMessage
	if o.cfg.CrossChannel != nil {
		candidates, classErr := o.cfg.CrossChannel.Classify(ctx, event)
		if classErr != nil {
			slog.DebugContext(ctx, "cross_channel_classify_error", "error", classErr)
		} else if len(candidates) > 0 {
			slog.DebugContext(ctx, "cross_channel_classified", "guild", event.GuildID, "current", event.ChannelID, "kept", len(candidates))
			crossChannelCandidates = candidates
		}
	}

	// Use BuildWithCrossChannel if we have cross-channel candidates, otherwise Build
	if len(crossChannelCandidates) > 0 {
		messages, buildErr = o.contextBuilder.BuildWithCrossChannel(ctx, event, o.cfg.ContextStore, o.cfg.SystemPrompt, crossChannelCandidates)
	} else {
		messages, buildErr = o.contextBuilder.Build(ctx, event, o.cfg.ContextStore, o.cfg.SystemPrompt)
	}

	if buildErr != nil {
		slog.DebugContext(ctx, "context_builder_error", "error", buildErr)
		return
	}

	slog.DebugContext(ctx, "context_builder", "messages", len(messages))
	
	modelToUse := "unknown"
	if o.cfg.TierRouter != nil {
		if tr, ok := o.cfg.TierRouter.(*llm.TierRouter); ok {
			tier, tierErr := tr.Classify(ctx, event.GuildID, content)
			if tierErr == nil {
				modelToUse = tr.ModelFor(tier)
			}
		}
	}
	
	stopTyping := o.startTyping(ctx, event.GuildID, event.ChannelID)
	defer stopTyping()

	slog.InfoContext(ctx, "llm_request", "model", modelToUse)
	
	var resp string
	var llmErr error
	streamingUsed := false
	
	if o.cfg.Streaming && o.cfg.StreamLLM != nil && len(o.cfg.Tools) == 0 {
		streamingUsed = true
		sender := NewStreamingSender(o.cfg.Discord, o.cfg.RateLimiter, event.GuildID, event.ChannelID)
		callbacks := llm.StreamCallbacks{
			OnDelta: func(text string) {
				if err := sender.Push(ctx, text); err != nil {
					slog.WarnContext(ctx, "stream_sender_push_error", "guild", event.GuildID, "channel", event.ChannelID, "err", err)
				}
			},
			OnDone: func() {
				_ = sender.Flush(ctx)
			},
			OnError: func(err error) {
				slog.WarnContext(ctx, "stream_error", "guild", event.GuildID, "channel", event.ChannelID, "err", err)
			},
		}
		resp, llmErr = o.cfg.StreamLLM.ChatStream(ctx, modelToUse, event.GuildID, messages, callbacks)
		if llmErr != nil {
			slog.WarnContext(ctx, "stream_llm_error", "guild", event.GuildID, "channel", event.ChannelID, "err", llmErr)
			if sender.SentCount() == 0 {
				return
			}
			streamingUsed = true
		}
	} else if o.cfg.Streaming && o.cfg.StreamToolsLLM != nil && len(o.cfg.Tools) > 0 {
		streamingUsed = true
		sender := NewStreamingSender(o.cfg.Discord, o.cfg.RateLimiter, event.GuildID, event.ChannelID)
		resp, llmErr = o.cfg.StreamToolsLLM.ChatWithToolsStream(ctx, messages, llm.ChatWithToolsStreamConfig{
			Model:   modelToUse,
			GuildID: event.GuildID,
			Tools:   o.cfg.Tools,
			Exec:    o.cfg.ToolExecutor,
			Max:     3,
			OnDelta: func(text string) {
				if err := sender.Push(ctx, text); err != nil {
					slog.WarnContext(ctx, "stream_sender_push_error", "guild", event.GuildID, "channel", event.ChannelID, "err", err)
				}
			},
		})
		if llmErr != nil {
			slog.WarnContext(ctx, "stream_tools_llm_error", "guild", event.GuildID, "channel", event.ChannelID, "err", llmErr)
			if sender.SentCount() == 0 {
				return
			}
			streamingUsed = true
		}
		
		// Check for escalation after first round
		alreadyEscalated := false
		if ex, ok := o.cfg.ToolExecutor.(escalationAware); ok {
			strongModel := o.resolveStrongModel()
			if reason := ex.Reason(); reason != "" && strongModel != "" && modelToUse != strongModel {
				alreadyEscalated = true
				_ = sender.Discard()
				
				secondSender := NewStreamingSender(o.cfg.Discord, o.cfg.RateLimiter, event.GuildID, event.ChannelID)
				
				filteredTools := make([]map[string]interface{}, 0, len(o.cfg.Tools))
				for _, tool := range o.cfg.Tools {
					if fn, ok := tool["function"].(map[string]interface{}); ok {
						if name, ok := fn["name"].(string); ok && name == "escalate_to_strong_model" {
							continue
						}
					}
					filteredTools = append(filteredTools, tool)
				}
				
				resp, llmErr = o.cfg.StreamToolsLLM.ChatWithToolsStream(ctx, messages, llm.ChatWithToolsStreamConfig{
					Model:   strongModel,
					GuildID: event.GuildID,
					Tools:   filteredTools,
					Exec:    o.cfg.ToolExecutor,
					Max:     3,
					OnDelta: func(text string) {
						if err := secondSender.Push(ctx, text); err != nil {
							slog.WarnContext(ctx, "stream_sender_push_error", "guild", event.GuildID, "channel", event.ChannelID, "err", err)
						}
					},
				})
				_ = secondSender.Flush(ctx)
				
				slog.InfoContext(ctx, "llm_escalated", "reason", reason, "from", modelToUse, "to", strongModel)
				
				ex.Clear()
			} else if reason := ex.Reason(); reason != "" {
				// Log that escalation was suppressed so operators see why no re-run happened
				slog.InfoContext(ctx, "llm_escalate_skipped", "reason", reason, "model", modelToUse, "strong_model", strongModel)
				ex.Clear()
			}
		}
		
		// Only flush original sender if we didn't escalate
		if !alreadyEscalated {
			_ = sender.Flush(ctx)
		}
	} else if o.cfg.ToolsLLM != nil && len(o.cfg.Tools) > 0 {
		resp, llmErr = o.cfg.ToolsLLM.ChatWithTools(ctx, messages, llm.ChatWithToolsConfig{
			Model:   modelToUse,
			GuildID: event.GuildID,
			Tools:   o.cfg.Tools,
			Exec:    o.cfg.ToolExecutor,
			Max:     3,
		})
	} else {
		if modelToUse != "unknown" {
			resp, llmErr = o.cfg.LLM.ChatWithModel(ctx, modelToUse, event.GuildID, messages)
		} else {
			resp, llmErr = o.cfg.LLM.Chat(ctx, event.GuildID, messages)
		}
	}
	
	if llmErr != nil || resp == "" {
		if streamingUsed {
			slog.WarnContext(ctx, "streaming_response_incomplete", "guild", event.GuildID, "channel", event.ChannelID, "err", llmErr)
		}
		return
	}

	if !streamingUsed {
		chunks := SplitMessage(resp, DiscordMessageLimit)
		slog.InfoContext(ctx, "response_chunks", "n", len(chunks))
		for _, chunk := range chunks {
			if ctx.Err() != nil {
				return
			}
			_ = o.cfg.Discord.SendMessage(ctx, event.GuildID, event.ChannelID, chunk)
		}
	}

	if o.cfg.Capture != nil && event.GuildID > 0 && event.ChannelID > 0 {
		syntheticMsgID := -(time.Now().UnixNano() % 9223372036854775807)
		botMsg := &domain.ChannelMessage{
			GuildID:   event.GuildID,
			ChannelID: event.ChannelID,
			MessageID: syntheticMsgID,
			UserID:    0,
			Content:   resp,
			IsBot:     true,
			CreatedAt: time.Now(),
		}
		if captureErr := o.cfg.Capture.Capture(ctx, botMsg); captureErr != nil {
			slog.DebugContext(ctx, "channel_capture", "guild", event.GuildID, "channel", event.ChannelID, "is_bot", true, "err", captureErr)
		}
	}

	if o.cfg.ConversationRefresher != nil {
		o.refreshWG.Add(1)
		go func() {
			defer o.refreshWG.Done()
			refreshCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := o.cfg.ConversationRefresher.Refresh(refreshCtx, event.GuildID, event.ChannelID, time.Now(), o.cfg.ConversationLockTTL)
			if err != nil {
				slog.DebugContext(ctx, "lock_refresh_error", "guild", event.GuildID, "channel", event.ChannelID, "error", err)
			} else {
				slog.InfoContext(ctx, "lock_refresh", "guild", event.GuildID, "channel", event.ChannelID, "ttl", o.cfg.ConversationLockTTL)
			}
		}()
	}

	if o.cfg.Promoter != nil {
		o.cfg.Promoter.Consider(context.Background(), event, contextMessages, resp)
	}

	slog.InfoContext(ctx, "typing_stopped", "guild", event.GuildID, "channel", event.ChannelID)
}

func (o *Orchestrator) startTyping(ctx context.Context, guildID, channelID int64) func() {
	ts, ok := o.cfg.Discord.(TypingSender)
	if !ok {
		return func() {}
	}

	stopCh := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(stopCh) }) }

	slog.InfoContext(ctx, "typing_started", "guild", guildID, "channel", channelID, "reason", "llm_request")
	_ = ts.SendTyping(ctx, guildID, channelID)

	o.refreshWG.Add(1)
	go func() {
		defer o.refreshWG.Done()
		ticker := time.NewTicker(o.cfg.TypingRepeat)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				slog.DebugContext(ctx, "typing_refresh", "guild", guildID, "channel", channelID)
				_ = ts.SendTyping(ctx, guildID, channelID)
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return stop
}

func dedupeKey(event *domain.DiscordEvent) string {
	if event.Message == nil {
		return ""
	}
	return fmtID(event.GuildID) + ":" + fmtID(event.ChannelID) + ":" + fmtID(event.Message.ID)
}

func fmtID(id int64) string {
	if id == 0 {
		return "0"
	}
	neg := id < 0
	if neg {
		id = -id
	}
	buf := [20]byte{}
	pos := len(buf)
	for id > 0 {
		pos--
		buf[pos] = byte('0' + id%10)
		id /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func (o *Orchestrator) QueueDepth() int    { return len(o.queue) }
func (o *Orchestrator) InFlight() int64    { return o.inFlight.Load() }
func (o *Orchestrator) ProcessedCount() int64 { return o.processed.Load() }
func (o *Orchestrator) DroppedCount() int64 { return o.dropped.Load() }

func (o *Orchestrator) resolveStrongModel() string {
	if o.cfg.TierRouter != nil {
		if tr, ok := o.cfg.TierRouter.(*llm.TierRouter); ok && tr != nil {
			if m := tr.ModelFor(llm.TierStrong); m != "" {
				return m
			}
		}
	}
	return o.cfg.StrongModel
}
