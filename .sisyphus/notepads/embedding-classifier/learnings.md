## T1 LEARNINGS

### Backend Selection: Option B (onnxruntime_go + tokenizers) — REAL IMPLEMENTATION

**Decision**: Implemented Option B (`github.com/yalue/onnxruntime_go v1.16.0` + `github.com/daulet/tokenizers v1.27.0`) with full real ONNX inference.

**Why Option B Over Option A (hugot)**:
- More modular: separate ONNX runtime and tokenization concerns
- Lower-level control: explicit tensor management, clearer pooling strategy
- More reliable: fewer abstractions, easier to debug
- Better for production: explicit error handling and resource cleanup

**Implementation Details**:

**Library Versions**:
- `github.com/yalue/onnxruntime_go v1.16.0` (compatible with ORT 1.20.1 API version 20)
- `github.com/daulet/tokenizers v1.27.0` (Rust-built, static lib at /usr/local/lib/libtokenizers.a)

**Native Dependencies**:
- libonnxruntime.so.1.20.1 at /usr/local/lib/ (with symlinks .so and .so.1)
- libtokenizers.a at /usr/local/lib/ (static archive)
- Headers at /usr/local/include/onnxruntime/

**Pooling Strategy**:
- Manual mean pooling via `MeanPool()` helper (respects attention mask)
- Manual L2 normalization via `L2Normalize()` helper
- Gives full control over embedding post-processing

**Thread Safety**:
- sync.Mutex around session.Run() for concurrent safety
- sync.Once for ORT environment initialization (one-time setup)

**What Works Now**:
- `NewONNX()`: loads model + tokenizer, initializes ORT session
- `Embed()`: tokenizes text, runs inference, applies mean pooling + L2 norm
- `FakeEmbedder`: deterministic, 384-dim, L2-normalized (for testing)
- `MeanPool`, `L2Normalize`, `Cosine`: exported helpers for reuse
- All 11 unit tests PASS with race detector
- All 3 integration tests PASS (Dim=384, semantic similarity, concurrent safety)
- Build and vet clean

**Runtime Environment**:
- ORT library path: defaults to `/usr/local/lib/libonnxruntime.so`, overridable via `IRIS_ONNXRUNTIME_LIB_PATH` env var
- Model path: defaults to `/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx`, overridable via `ONNXConfig.ModelPath`
- Tokenizer path: defaults to `/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json`, overridable via `ONNXConfig.TokenizerPath`
- Max sequence length: defaults to 128, overridable via `ONNXConfig.MaxSeqLen`

**CGO Flags**:
- Automatically handled by Go build system (libraries in /usr/local/lib on ldconfig path)
- No manual CGO_LDFLAGS required for this setup

**Downstream Impact**:
- T2 can add pgvector column and backfill with real embeddings
- T3/T4 can implement classifiers with real embeddings, no fallback needed
- T5 can wire config + real embeddings
- Production ready: no fallback logic needed

## LIVE FIX STREAM HANG

### Problem Statement

Three bugs caused the streaming pipeline to hang silently:

1. **StreamingSender.Push() silently swallowed errors** including `ErrRateLimitExceeded`, so the orchestrator never aborted the stream loop when Discord rejected a paragraph.
2. **SSE scanner blocked forever** if the upstream proxy stalled mid-stream. `ctx.Done()` check was a non-blocking default, so it only fired when the scanner returned (which never happened on stall).
3. **HTTP client timeout (30s) was shorter than orchestrator JobTimeout (45s)**, and when the 30s did hit, there was no error propagation that stopped typing or refreshed the lock.

### Solution Architecture

#### FIX A: Error Propagation in StreamingSender

**Pattern**: Store first critical error, return it on subsequent calls, abort stream loop.

```go
type StreamingSender struct {
    // ... existing fields ...
    lastErr error  // stores first critical error
}

func (s *StreamingSender) Push(ctx context.Context, fragment string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    if s.lastErr != nil {
        return s.lastErr  // abort immediately
    }
    
    // ... buffer logic ...
    
    if err := s.emitChunk(ctx, chunk); err != nil {
        s.lastErr = err  // store error
        return err       // propagate to caller
    }
}
```

**Key insight**: Once an error occurs, all subsequent Push calls fail fast. The orchestrator's OnDelta callback receives the error and can cancel the stream context.

#### FIX B: Context-Aware SSE Scanner

**Pattern**: Goroutine + channel pattern to make scanner responsive to ctx.Done().

```go
lineCh := make(chan []byte, 16)
errCh := make(chan error, 1)

go func() {
    defer close(lineCh)
    for scanner.Scan() {
        b := make([]byte, len(scanner.Bytes()))
        copy(b, scanner.Bytes())
        select {
        case lineCh <- b:
        case <-ctx.Done():
            return  // exit goroutine on cancel
        }
    }
    if err := scanner.Err(); err != nil {
        errCh <- err
    }
}()

for {
    select {
    case line, ok := <-lineCh:
        if !ok { /* stream ended */ }
        // process line
    case err := <-errCh:
        return err
    case <-ctx.Done():
        return ctx.Err()  // respond to cancellation immediately
    }
}
```

**Key insight**: The main loop's select statement checks ctx.Done() on every iteration, not just when scanner.Scan() returns. If upstream stalls, the goroutine blocks on scanner.Scan(), but the main loop can still detect ctx cancellation.

#### FIX C: Timeout Governance

**Pattern**: Wrap streaming methods with 120s context timeout, rely on context for HTTP request cancellation.

```go
func (c *Client) ChatStream(ctx context.Context, ...) (string, error) {
    streamCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
    defer cancel()
    
    // Pass streamCtx to HTTP request and all downstream operations
    // HTTP client timeout (30s) still applies for non-streaming
    // But streaming requests governed by 120s deadline
}
```

**Key insight**: Context deadline is checked by Go's net/http transport. When streamCtx expires, the HTTP body read is cancelled, which unblocks scanner.Scan(), which allows the goroutine to exit, which closes lineCh, which unblocks the main loop's select.

#### FIX D: Orchestrator Error Handling

**Pattern**: Track Push errors in OnDelta callback, conditionally refresh lock based on SentCount.

```go
sender := NewStreamingSender(...)
resp, llmErr = o.cfg.StreamLLM.ChatStream(ctx, ..., llm.StreamCallbacks{
    OnDelta: func(text string) {
        if err := sender.Push(ctx, text); err != nil {
            slog.WarnContext(ctx, "stream_sender_push_error", "err", err)
        }
    },
})

if llmErr != nil {
    if sender.SentCount() == 0 {
        return  // no chunks sent, don't refresh lock
    }
    // at least one chunk sent, refresh lock for follow-ups
}
```

**Key insight**: Partial responses (some chunks sent, then error) should still refresh the lock so follow-ups work. Zero-chunk errors should not open the active-conversation window.

### Implementation Details

**Files Changed**:
- `internal/orchestrator/stream_sender.go`: Error propagation, SentCount()
- `internal/llm/client.go`: Context timeouts, goroutine-based scanners
- `internal/orchestrator/orchestrator.go`: Error tracking, conditional lock refresh

**Tests Added**:
- `TestStreamingSender_Push_ReturnsRateLimitError`: Verifies error propagation
- `TestStreamingSender_SentCount`: Verifies chunk counting
- `TestChatStream_CtxCancelled_AbortsScannerWithinOneSecond`: Verifies cancellation responsiveness

**Test Results**: All 30+ packages pass, no regressions.

### Key Learnings

1. **Error propagation is critical in streaming**: Silent error swallowing breaks the entire abort mechanism. Always return errors from callbacks.

2. **Context cancellation requires active checking**: Non-blocking select defaults don't work. Use goroutines + channels to make blocking operations responsive to ctx.Done().

3. **Timeout layering matters**: HTTP client timeout (30s) + context timeout (120s) + orchestrator timeout (45s) need to be coordinated. Context timeout should be the governing layer for streaming.

4. **Partial responses need special handling**: Don't treat "some chunks sent + error" the same as "zero chunks + error". Refresh lock for partial responses to enable follow-ups.

5. **Rate limit errors must be visible**: Log rate limit errors with context (guild, channel, buffered_len) so operators can diagnose Discord throttling issues.
G ADAPTER + SAME-MODEL ESCALATE FIX

### ROOT CAUSE A: Missing SendTyping Method on DiscordSenderAdapter

**Problem**: Typing indicator never fired because `DiscordSenderAdapter` was missing the `SendTyping` method. The orchestrator's `startTyping()` goroutine did a type assertion `ts, ok := o.cfg.Discord.(TypingSender)` which failed silently, causing the typing indicator to never be sent to Discord.

**Root Cause**: Interface mismatch — `DiscordSenderAdapter` only implemented `SendMessage`, not the full `TypingSender` interface which requires both `SendMessage` and `SendTyping`.

**Fix**: Added `SendTyping` method to `DiscordSenderAdapter` in `internal/app/wire/adapters.go`:
```go
func (a *DiscordSenderAdapter) SendTyping(ctx context.Context, guildID, channelID int64) error {
	return a.Gateway.SendTyping(ctx, guildID, channelID)
}
```

**Test**: `TestDiscordSenderAdapter_SatisfiesTypingSender` — compile-time interface check ensures the adapter satisfies `orchestrator.TypingSender`.

**Impact**: Typing indicator now fires correctly on `pipeline_start`, improving UX by showing the bot is processing.

### ROOT CAUSE B: Same-Model Escalation Wastes LLM Calls

**Problem**: Escalation logic triggered whenever `reason != "" && cfg.StrongModel != ""`, regardless of whether the currently-used model **already was** the strong model. Live logs showed `llm_escalated from=kr/claude-opus-4.7 to=kr/claude-opus-4.7`, wasting an LLM API call.

**Root Cause**: Missing guard — no check to prevent escalation when `modelToUse == StrongModel`.

**Fix**: Added guard in `internal/orchestrator/orchestrator.go` (line 428):
```go
if reason := ex.Reason(); reason != "" && o.cfg.StrongModel != "" && modelToUse != o.cfg.StrongModel {
    // escalate to strong model
} else if reason := ex.Reason(); reason != "" {
    slog.InfoContext(ctx, "llm_escalate_skipped", "reason", reason, "model", modelToUse, "strong_model", o.cfg.StrongModel)
    ex.Clear()
}
```

**Test**: `TestHandle_EscalateSkippedWhenSameModel` — verifies that when `StrongModel == modelToUse`, escalation is skipped and `llm_escalate_skipped` log is emitted.

**Impact**: Eliminates wasteful re-runs, reduces LLM API costs, operators can see why escalation was skipped via logs.

### ROOT CAUSE C: SendTyping Errors Don't Surface HTTP Status

**Problem**: When `SendTyping` failed (e.g., 403 permission denied), the error log was generic and didn't include the HTTP status code, making it hard to distinguish permission issues from network errors.

**Fix**: Enhanced error logging in `internal/discord/gateway.go` to extract and log HTTP status from `discordgo.RESTError`:
```go
if err := ga.session.ChannelTyping(channelIDStr); err != nil {
    var restErr *discordgo.RESTError
    if errors.As(err, &restErr) && restErr.Response != nil {
        ga.logger.Warn("send_typing_failed",
            "guild", guildID,
            "channel", channelID,
            "http_status", restErr.Response.StatusCode,
            "err", err)
    } else {
        ga.logger.Warn("send_typing_failed",
            "guild", guildID,
            "channel", channelID,
            "err", err)
    }
    return err
}
```

**Impact**: Operators can now distinguish 403 permission issues from network errors, enabling faster troubleshooting.

## T1 TIER HONORED

**Task**: Route tier-chosen model through non-tool chat path so casual greetings hit haiku.

**Implementation**:
- Extended `LLMCaller` interface with `ChatWithModel(ctx, model, guildID, messages) (string, error)`
- Updated `LLMCallerAdapter` in `wire/adapters.go` to implement `ChatWithModel` by forwarding to `llm.Client.ChatWithModel`
- Updated `FakeLLMClient` test utility to track `LastModelUsed` and implement `ChatWithModel`
- Modified `orchestrator.go` line 393-398: non-tools path now calls `ChatWithModel(modelToUse, ...)` when `modelToUse != "unknown"`, falls back to `Chat()` on classifier failure
- Changed `.env` and `.env.example`: `LLM_MODEL_DEFAULT=kr/claude-haiku-4.5` (was sonnet-4.5)
- Added `TestHandle_NonToolsPath_UsesTierModel` test: verifies tier-chosen model is passed to LLM in non-tools path

**Verification**:
- Test output shows `llm_request model=kr/claude-haiku-4.5` (tier-chosen model, not default)
- All 33 packages pass tests
- Build, vet, docker build all succeed
- Bot deploys and connects to Discord gateway

**Key Design Decision**:
- Preserved `Chat()` signature for classifiers (they still use it)
- Fallback to `Chat()` when tier classification fails (graceful degradation)
- Interface extension is minimal and backward-compatible

## SEARXNG PROVIDER

### Provider Contract

**SearXNGProvider** implements the `Provider` interface for the websearch tool:
- Endpoint: `GET <BaseURL>/search?q=<query>&format=json`
- Response: `{"results":[{"title","url","content","engine",...}],...}`
- Field mapping: `Title=title`, `URL=url`, `Snippet=content`, `Source="searxng"`, `Authoritative=IsCanonAuthoritative(url)`
- Limit enforcement: truncated in Go after fetch (respects `limit` parameter)
- Error handling: returns `ErrEmptyQuery`, `ErrTimeout`, `ErrProviderFailure`, `ErrInvalidResponse` consistent with HTTPProvider
- HTTP 4xx/5xx: treated as provider failure
- Context deadline: distinguished as `ErrTimeout`

### Docker Compose Integration

**SearXNG Service**:
- Image: `searxng/searxng:latest`
- Container: `iris-searxng`
- Port: `127.0.0.1:8888:8080` (local-only)
- Config: `./docker/searxng/settings.yml` (mounted at `/etc/searxng`)
- Network: `iris-network` (shared with bot)
- Key setting: `limiter: false` (disables rate limiting for programmatic JSON requests)

**Bot Integration**:
- Env var: `IRIS_SEARXNG_URL` (default `http://searxng:8080` in compose)
- Fallback: if unset, tries `SEARCH_BASE_URL + SEARCH_API_KEY` (HTTPProvider)
- Registration: logs `websearch tool registered provider=searxng base_url=...` on success
- Timeout: 10 seconds per request
- MaxOutput: 16 KB

**Settings File** (`docker/searxng/settings.yml`):
- `use_default_settings: true` (inherit SearXNG defaults)
- `limiter: false` (no 429 on rapid requests)
- `image_proxy: false` (disable image proxying)
- `formats: [html, json]` (enable JSON output)
- `safe_search: 0` (no filtering)
- `engines: [google, duckduckgo]` (minimal engine set)
- `outgoing.request_timeout: 6.0` (upstream timeout)

### Testing

All 6 SearXNG-specific tests PASS:
- `TestSearXNG_ReturnsParsedResults`: verifies field mapping and Authoritative flag
- `TestSearXNG_EmptyQuery`: returns ErrEmptyQuery without hitting server
- `TestSearXNG_Timeout`: server sleep past timeout → ErrTimeout
- `TestSearXNG_5xx_ReturnsProviderFailure`: HTTP 503 → ErrProviderFailure
- `TestSearXNG_BadJSON_ReturnsInvalidResponse`: malformed JSON → ErrInvalidResponse
- `TestSearXNG_RespectsLimit`: limit=2 against 5-result response yields 2 results

### Verification

- Unit tests: `go test ./internal/tools/websearch -count=1 -v` ✓ PASS
- Build: `go build ./...` ✓ clean
- Vet: `go vet ./...` ✓ clean
- Docker: `docker compose up -d searxng && curl http://127.0.0.1:8888/search?q=hello&format=json` ✓ returns JSON
- Bot startup: `docker compose up -d bot` ✓ logs `websearch tool registered provider=searxng base_url=http://searxng:8080`

## T3 WIRING

### Tool-Calling Integration into Orchestrator

**Architecture**:
- Tool-calling is an optional feature gate: when `Config.ToolsLLM != nil && len(Config.Tools) > 0`, the orchestrator uses `ChatWithTools` instead of plain `Chat`.
- When either condition is false, falls back to plain `Chat` (preserves all existing behavior).
- Classifiers (cross_channel, in_window_relevance, memory_promoter) remain on plain `ChatWithModel` — no tool-calling there.

**Wiring Hooks in main.go**:
1. **Registry Construction**: `registry := tools.NewRegistry(nil)` — creates empty registry.
2. **Websearch Registration**: Reads `SEARCH_BASE_URL` and `SEARCH_API_KEY` env vars. If both set, constructs `HTTPProvider` and registers websearch tool. If either unset, logs warn and continues (graceful degradation).
3. **Adapter Wiring**: 
   - `ToolCallingLLMAdapter{Client: chatClient}` wraps the LLM client for `ChatWithTools` calls.
   - `RegistryExecutor{Reg: registry}` wraps the registry to satisfy `llm.ToolExecutor` interface.
4. **Config Population**:
   - `ToolsLLM: adapter` — enables tool-calling path.
   - `ToolExecutor: executor` — provides tool execution.
   - `Tools: registry.OpenAIFunctions()` — populates OpenAI-shaped tool definitions (empty slice if no tools registered).

**Env-Based Gate**:
- `SEARCH_BASE_URL` and `SEARCH_API_KEY` control whether websearch is available.
- If unset, bot starts normally but tool-calling path is disabled (Tools slice is empty).
- Operator can enable by setting both env vars and restarting.

**Tests**:
- `TestHandle_UsesToolsLLMWhenConfigured`: Verifies ToolsLLM is called when Tools is non-empty.
- `TestHandle_FallsBackToPlainLLMWhenNoTools`: Verifies plain Chat is called when Tools is nil/empty.
- Both tests pass; all 48 orchestrator tests pass.

**Build & Verification**:
- `go build ./...` passes.
- `go vet ./...` clean.
- `go test -p 2 -count=1 ./...` all pass.
- `CGO_ENABLED=0 go build -o /tmp/iris-bot-test ./cmd/iris-bot` succeeds (static binary).
- Main.go grep shows all four required references (registry.Register, ToolsLLM, ToolExecutor, OpenAIFunctions).

## LIVE FIX CAPTURE

### Channel Message Capture Wired into Orchestrator Pipeline (2026-05-12)

**Problem**: `ChannelCaptureAdapter` existed but was never called in the live path. `channel_messages` table had zero rows for production guild 687347156524204067 (only test fixtures in guild 1111111111). Consequence: similarity gate always saw empty context, returned (false, 0, 0, nil), casual follow-ups were silently rejected, sliding-window conversation feature was dead in production.

**Solution**: Wire capture into orchestrator.handle() at two critical points:

**1. User Message Capture (orchestrator.go:237-256)**
- **Timing**: BEFORE router.Decide() call, at the very top of handle()
- **Behavior**: Captures every inbound event regardless of router decision
- **Reason**: Ensures rolling context window even for messages that end up ignored
- **Implementation**:
  ```go
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
  ```

**2. Bot Reply Capture (orchestrator.go:367-382)**
- **Timing**: AFTER SendMessage succeeds, before lock_refresh
- **Behavior**: Synthesizes ChannelMessage with IsBot=true, negative synthetic MessageID
- **Reason**: Allows similarity gate to score follow-ups against Iris's actual replies
- **Synthetic Message ID Scheme**: `-(time.Now().UnixNano() % 9223372036854775807)`
  - Negative values never collide with positive Discord snowflakes
  - Preserves uniqueness constraint (guild_id, message_id)
  - Deterministic and reproducible for testing
  - No int64 overflow issues
- **Implementation**:
  ```go
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
  ```

**3. Interface & Config Changes (orchestrator.go:39-48)**
- Added `ChannelCapture` interface:
  ```go
  type ChannelCapture interface {
      Capture(ctx context.Context, msg *domain.ChannelMessage) error
  }
  ```
- Added `Capture` field to Config (nil-safe; if nil, skip persistence)

**4. Wiring (cmd/iris-bot/main.go:311)**
- `Capture: &wireadapters.ChannelCaptureAdapter{Repo: channelMessageRepo, GuildEnsurer: &wireadapters.GuildEnsurerAdapter{Repo: guildRepo}, Embedder: emb}`
- Embedder may be nil when similarity disabled; adapter handles nil gracefully

**Tests Added**:
- `TestHandle_PersistsIncomingMessage`: Verifies user message captured before router decision
- `TestHandle_PersistsBotReply`: Verifies bot reply captured with IsBot=true, synthetic negative MessageID, full response content
- `TestHandle_CaptureNilSafe`: Verifies orchestrator processes messages without crashing when Capture is nil

**Verification**:
- go build ./... ✓
- go vet ./... ✓
- go test -p 2 -count=1 ./... ✓ (all 31 packages pass, including 3 new tests)
- docker compose build bot ✓
- docker compose up -d bot ✓ (startup clean, no errors)
- Startup logs show: `context classifier backend=similarity threshold=0.55` and `Discord gateway connected`

**Impact**:
- Similarity gate now sees non-empty context for follow-ups
- Sliding-window conversation feature enabled in production
- Both user and bot messages persisted with correct is_bot flag
- Casual follow-ups can now be scored against Iris's actual replies
- Rolling context window maintained even for ignored messages

## LIVE FIX SIM+PROMOTER+MODEL

### Three Precise Defects Fixed (2026-05-12)

**DEFECT A — Similarity score missing from logs**
- **Root cause**: InWindowRelevance interface returned only (bool, error). Orchestrator logged fallback with no sim/threshold values.
- **Fix**: Extended interface to return (bool, float64, float64, error) — added similarity score and threshold.
  - LLMInWindowRelevance: returns (relevant, 0, 0, nil) — no sim available for LLM classifier.
  - SimilarityInWindowRelevance: returns (decision, similarity, threshold, nil) — real values from embedder.
  - Orchestrator now logs: `conv_lock_similarity guild=... channel=... sim=0.XX threshold=0.55 decision=true|false`
  - Removed duplicate slog.InfoContext from similarity classifier (orchestrator is authoritative).
- **Files**: conversation_relevance.go (interface + LLM impl), conversation_relevance_sim.go (sim impl), orchestrator.go (logging), 16 test files (signature updates).

**DEFECT B — Memory promoter sees "context canceled"**
- **Root cause**: Orchestrator spawned goroutine with `context.WithTimeout(context.Background(), 15s)`, then `defer cancel()` fired immediately after Consider() returned (non-blocking). Promoter's internal goroutine inherited canceled context.
- **Fix**: Removed goroutine wrapper and timeout wrapper. Call Consider() directly with context.Background(). Consider() is fire-and-forget by contract; manages its own 15s timeout internally.
- **Files**: orchestrator.go (lines 360-367).

**DEFECT C — `llm_request model=unknown`**
- **Root cause**: Type assertion used inline interface{} signature that didn't match TierRouter.Classify return type (Tier, not interface{}). Assertion never matched.
- **Fix**: Import llm package, assert to concrete *llm.TierRouter type. Removed nil check on tier (Classify returns concrete Tier).
- **Files**: orchestrator.go (import + assertion block).

### Verification
- go build ./... ✓
- go vet ./... ✓
- go test -p 2 -count=1 ./... ✓ (28 packages, all pass)
- docker compose build bot ✓
- docker compose up -d bot ✓ (startup clean, 4 info lines)
- No "context canceled" warnings
- No "model=unknown" in logs
- Similarity scores now logged with real valuesFIX GUILD + PROMOTER + MODEL

### Three Defects Fixed in Live Logs

**FIX 1: Guild FK Constraint (channel_conversations)**

Problem: FK violation when guild never inserted into guilds table. All guild-FK writes fail silently except lock_refresh which logs error.

Solution: Added `EnsureGuild(ctx, guildID)` method to GuildRepo using `INSERT ... ON CONFLICT (id) DO NOTHING`. Integrated into:
- `ChannelCaptureAdapter.Capture()` before `repo.Upsert(msg)` (primary defense)
- Gateway callback in main.go BEFORE `orch.Enqueue(ctx, event)` (safety net)

Files: `internal/repository/guild.go` (EnsureGuild method), `internal/app/wire/adapters.go` (GuildEnsurer interface + ChannelCaptureAdapter update), `cmd/iris-bot/main.go` (gateway callback ensure call)

Test: `TestGuildRepo_EnsureIsIdempotent` verifies calling Ensure twice with same ID returns nil both times and does not duplicate rows.

**FIX 2: Memory Promoter Context Cancellation**

Problem: Memory promoter uses per-job context which is canceled on handle() return, causing "context canceled" WARN logs.

Solution: Spawned `promoter.Consider()` in its own goroutine with detached context (15s timeout from context.Background()). Tracked via existing refreshWG WaitGroup so Stop() drains promoter goroutines cleanly.

Files: `internal/orchestrator/orchestrator.go` (lines 347-355: wrapped promoter.Consider in detached goroutine)

Test: `TestPromoterRunsOnDetachedContext` verifies promoter context remains alive for 100ms after handle() returns, completing without "context canceled" error.

**FIX 3: LLM Model Logging**

Problem: llm_request log hardcodes "gpt-4o-mini" but bot uses kr/claude-opus-4.7 via tier routing.

Solution: Added TierRouter field to orchestrator.Config. Before Chat call, classify query tier and resolve actual model via TierRouter.ModelFor(). Log resolved model name instead of hardcoded value. Falls back to "unknown" if TierRouter not configured.

Files: `internal/orchestrator/orchestrator.go` (Config.TierRouter field, model determination logic before Chat), `cmd/iris-bot/main.go` (wire tierRouter into orchCfg)

Test: `TestLLMRequestLogsActualModel` verifies tier classification and model resolution work correctly.

### Implementation Details

**Guild Ensure Idempotency**:
- Uses PostgreSQL `ON CONFLICT (id) DO NOTHING` to safely insert or skip
- Called at two boundaries: gateway callback (early) and capture adapter (late)
- Redundancy is intentional: safety net in case capture is skipped
- Non-blocking: logs warn on error but does not block message processing

**Promoter Detachment**:
- Promoter already detaches internally (memory_promotion.go lines 68-80)
- Orchestrator now spawns promoter call in separate goroutine with detached context
- Uses refreshWG (existing WaitGroup) for clean shutdown
- 15s timeout ensures promoter completes even if LLM hangs

**Model Logging**:
- TierRouter.Classify() determines tier (default/strong) based on query complexity
- TierRouter.ModelFor(tier) resolves model string for that tier
- Logged before Chat call so we know which model was sent
- Type assertion used to handle interface{} TierRouter field (flexible wiring)

### Build & Test Results

All builds and tests pass:
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test -p 2 -count=1 ./...` ✅ (all 27 packages)

New tests added:
- `TestGuildRepo_EnsureIsIdempotent` (repository_test.go)
- `TestPromoterRunsOnDetachedContext` (orchestrator_test.go)
- `TestLLMRequestLogsActualModel` (orchestrator_test.go)

### Verification Steps for Operator

1. Start bot: `docker compose build bot && docker compose up -d bot && sleep 8`
2. Send mention in new guild (never seen before)
3. Wait 2 seconds, send casual follow-up
4. Check logs for:
   - NO FK error in lock_refresh
   - NO "context canceled" WARN in memory promoter
   - llm_request shows actual model (kr/claude-opus-4.7 or gpt-4o-mini), not hardcoded
5. Verify guild row created: `docker compose exec -T postgres psql -U iris_user -d iris -c "SELECT id FROM guilds ORDER BY id;"`

Expected: First mention triggers router_decision → typing_started → llm_request (real model) → response_chunks → lock_refresh (no FK error). Follow-up triggers router_decision (reason=active_conversation) → conv_lock_similarity → LLM or silence. No "context canceled" warnings.
WIRING FIX

### Restored Missing Orchestrator Integration (T4/T5/T6/T7)

**Problem**: Events reached gateway ("dispatching event" logs fired) but no router_decision, typing_started, llm_request, response_chunks, lock_refresh, or similarity score lines appeared. Earlier integration work (T4-T7) was missing from Config struct and main.go wiring.

**Root Causes**:
1. `internal/orchestrator/orchestrator.go` Config struct missing: CrossChannel, Promoter, ConversationRefresher, AllowedQuerier, ConversationLockTTL, ImmediateTyping
2. `cmd/iris-bot/main.go` orchCfg construction incomplete: no ContextStore, no classifiers, no promoter, no refresh wiring
3. Similarity logs at DEBUG level (invisible at INFO default)
4. Typing was delayed (TypingAfter=500ms) instead of immediate

**Fixes Applied**:

**A. Restored orchestrator.go Config struct** (lines 37-56):
- Added `CrossChannel CrossChannelClassifier`
- Added `Promoter MemoryPromoter`
- Added `ConversationRefresher repository.ChannelConversationQuerier`
- Added `AllowedQuerier repository.AllowedChannelQuerier`
- Added `ConversationLockTTL time.Duration` (default 5*time.Minute in New())
- Added `ImmediateTyping bool` (default true in New())
- Added `refreshWG sync.WaitGroup` to Orchestrator struct for tracking refresh goroutines

**B. Rewired orchestrator.go handle()** (lines 210-330):
- Changed router_decision log from DEBUG to INFO
- Added relevance check ONLY for ReasonActiveConversation (not all decisions)
- Log conv_lock_similarity at INFO with guild, channel, decision, reason
- Send typing IMMEDIATELY if ImmediateTyping=true (no delay)
- Fetch context once for both cross-channel and LLM
- Call CrossChannel.Classify() and prepend candidates to messages
- Log cross_channel_classified at DEBUG with count
- Log llm_request at INFO with model name
- Log response_chunks at INFO with count
- Spawn refresh goroutine after SendMessage succeeds, log lock_refresh at INFO
- Fire-and-forget Promoter.Consider() if configured

**C. Rewired cmd/iris-bot/main.go** (lines 51-53, 165-206, 294-313):
- Added slog import and DEBUG handler setup when cfg.Debug=true
- Captured inWindowRelevance, crossChannelClassifier, memoryPromoter into variables (not discarded)
- Populated orchCfg with all fields:
  - ContextStore: ContextStoreAdapter wrapping channelMessageRepo
  - AllowedQuerier: allowedRepo
  - ConversationRefresher: convRepo
  - ConversationLockTTL: cfg.ConversationLockTTL (5m default)
  - InWindowRelevance: similarity or LLM classifier
  - CrossChannel: similarity or LLM classifier
  - Promoter: memory promoter
  - ImmediateTyping: true

**D. Updated conversation_relevance_sim.go** (lines 70-82):
- Changed similarity log from DEBUG to INFO
- Added guild, channel, sim, threshold, decision fields
- Log always emitted (not conditional on decision)

**E. Updated cross_channel_sim.go** (lines 109-145):
- Added DEBUG logs for each kept/rejected candidate with sim score
- Added INFO summary log at end: cross_channel_classified with guild, current, kept count

### Observable Log Lines Now Emitted

**Startup**:
- `iris-bot starting`
- `context classifier backend=similarity threshold=0.55` (or backend=llm)
- `Discord gateway connected`
- `auto-derived bot ID from session botID=...`

**On Mention**:
- `dispatching event type=message_mention`
- `router_decision reason=mention should=true` (INFO)
- `typing_started guild=... channel=... reason=immediate` (INFO)
- `llm_request model=gpt-4o-mini` (INFO)
- `response_chunks n=...` (INFO)
- `lock_refresh guild=... channel=... ttl=5m0s` (INFO)

**On Casual (Active Conversation)**:
- `dispatching event type=message_casual`
- `router_decision reason=active_conversation should=true` (INFO)
- `conv_lock_similarity sim=0.72 threshold=0.55 decision=true reason=similarity_classifier` (INFO)
- `typing_started guild=... channel=... reason=immediate` (INFO)
- `cross_channel_sim_kept id=... sim=0.68 channel=...` (DEBUG, per candidate)
- `cross_channel_classified guild=... current=... kept=2` (INFO)
- `llm_request model=gpt-4o-mini` (INFO)
- `response_chunks n=...` (INFO)
- `lock_refresh guild=... channel=... ttl=5m0s` (INFO)

**On Off-Topic Casual**:
- `dispatching event type=message_casual`
- `router_decision reason=active_conversation should=true` (INFO)
- `conv_lock_similarity sim=0.22 threshold=0.55 decision=false reason=similarity_classifier` (INFO)
- (no typing, no llm_request, no response_chunks)

### Files Modified

- `internal/orchestrator/orchestrator.go`: Config struct, New(), Stop(), handle(), refreshWG
- `internal/orchestrator/conversation_relevance_sim.go`: IsRelevant() similarity logging
- `internal/orchestrator/cross_channel_sim.go`: Classify() candidate logging
- `cmd/iris-bot/main.go`: slog setup, classifier/promoter wiring, orchCfg population

### Verification

- `go build ./...`: ✓ clean
- `go vet ./...`: ✓ clean
- `go test -p 2 -count=1 ./...`: ✓ all 34 packages pass
- `CGO_ENABLED=0 go build -o /tmp/iris-bot-test ./cmd/iris-bot`: ✓ clean
- `docker compose build bot`: ✓ image built
- `docker compose up -d bot`: ✓ running
- Startup logs: ✓ all expected lines present

## LIVE DEBUG FIX

### Four Root Causes Fixed

**ROOT CAUSE 1 — Normalizer emits unrouted event type**
- Changed normalizer default type from "message_unknown" to "message_casual" (internal/discord/normalizer.go:135)
- Updated router to handle "message_casual" in active conversation branch (internal/router/router.go)
- Added two unit tests: `TestDecide_CasualMessage_ElevatesWhenActive` and `TestDecide_CasualMessage_IgnoresWhenInactive`
- Impact: Casual messages now properly feed into conversation-lock elevation logic

**ROOT CAUSE 2 — DISCORD_BOT_ID unset causes botID==0**
- Added `GatewayAdapter.BotID()` method to auto-derive bot ID from session.State.User.ID (internal/discord/gateway.go)
- Added `GatewayAdapter.SetNormalizerBotID(int64)` to update normalizer after connect
- Added `TriggerRouter.SetBotID(int64)` to update router after connect
- In main.go, after gateway.Connect(), auto-derive botID if env var not set and update both normalizer and router
- Kept DISCORD_BOT_ID env as override for backward compatibility
- Impact: Bot now correctly identifies its own messages even without env var

**ROOT CAUSE 3 — No observability on ignored events**
- Added DEBUG logs in gateway.handleMessage for ErrBotMessage, ErrNilMessage, and queue-full drops
- Added INFO log "dispatching event" in gateway.processWorkQueue before callback
- Added DEBUG logs in orchestrator.handle: router_decision, context_builder, llm_request, response_chunks, lock_refresh
- Added INFO logs in orchestrator.startTyping for typing_started with reason (immediate/delayed)
- Impact: Full visibility into event flow from Discord to LLM response

**ROOT CAUSE 4 — Verify immediate typing and increase timeout**
- Confirmed ImmediateTyping=true in main.go orchestrator config (line 311)
- Increased gateway.processWorkQueue timeout from 30s to 60s to handle slow LLM requests
- Impact: Typing indicator shows immediately, callback has enough time for LLM

### Observable Log Lines Added

1. `gateway.handleMessage`: `ignoring bot message channel=%s user=%s reason=bot_message`
2. `gateway.handleMessage`: `ignoring nil message reason=nil_message`
3. `gateway.handleMessage`: `work queue full, dropping event channel=%d user=%d type=%s`
4. `gateway.processWorkQueue`: `dispatching event type=%s guild=%d channel=%d user=%d`
5. `orchestrator.handle`: `router_decision reason=%s should=%v`
6. `orchestrator.handle`: `context_builder messages=%d`
7. `orchestrator.handle`: `llm_request model=%s`
8. `orchestrator.handle`: `response_chunks n=%d`
9. `orchestrator.handle`: `lock_refresh result=ok/error`
10. `orchestrator.startTyping`: `typing_started guild=%d channel=%d reason=%s`

### Files Modified

- internal/discord/normalizer.go: Changed default event type to "message_casual"
- internal/discord/gateway.go: Added BotID(), SetNormalizerBotID(), observability logs, increased timeout to 60s
- internal/router/router.go: Added message_casual case, SetBotID() method, two new unit tests
- cmd/iris-bot/main.go: Auto-derive bot ID after gateway.Connect()
- internal/orchestrator/orchestrator.go: Added observability logs throughout handle() and startTyping()

### Verification Steps

1. Build: `docker compose build bot && docker compose up -d`
2. Send one test message in Discord
3. Tail logs for 90 seconds: `docker compose logs --tail=120 bot`
4. Expected log sequence:
   - `dispatching event type=message_casual` (or mention/reply/content)
   - `router_decision should=true reason=...`
   - `typing_started ...`
   - `llm_request model=...`
   - `response_chunks n=...`
   - `lock_refresh result=ok`Y

### Production Docker Image & Compose Setup

**Image Strategy**: Multi-stage build with vendored native libraries.

**Build Stage**:
- Base: `golang:1.26-bookworm` (glibc, full build toolchain)
- Copies pre-built onnxruntime + tokenizers from `vendor/libs/` (build context)
- Builds with `CGO_ENABLED=1 -tags cgo -mod=mod` to link against native libs
- Output: `/out/iris-bot` binary

**Runtime Stage**:
- Base: `debian:bookworm-slim` (glibc, minimal)
- Copies libonnxruntime.so* from build stage
- Copies iris-bot binary
- Runs `ldconfig` to register dynamic libraries
- Non-root user `app` (UID 100)
- Model directory `/opt/iris-models` mounted read-only from host

**Model Mount Strategy**: Bind-mount via docker-compose (not COPY into image).
- Keeps image small (~100MB vs ~70MB with models)
- Allows model updates without rebuild
- Host path: `/opt/iris-models/paraphrase-MiniLM-L3-v2/`
- Container path: `/opt/iris-models/paraphrase-MiniLM-L3-v2/` (read-only)
- Operator must ensure host directory exists before `docker compose up`

**Migrations**: Updated migrate service to loop through all *.sql files alphabetically.
- Old: only applied 001_init.sql
- New: applies 001, 002, 003, 004 in order
- Command: `for f in /migrations/*.sql; do ... psql -f $$f; done`

**Environment Variables** (new IRIS_EMBED_* added to docker-compose.yml):
- `IRIS_EMBED_MODEL_PATH=/opt/iris-models/paraphrase-MiniLM-L3-v2/model.onnx`
- `IRIS_EMBED_TOKENIZER_PATH=/opt/iris-models/paraphrase-MiniLM-L3-v2/tokenizer.json`
- `IRIS_EMBED_SIM_THRESHOLD=0.55`
- `DEBUG=true`

**Memory Limit**: Raised from 512m to 1g (ONNX session + 384-dim tensors need headroom).

**Verification**:
- Bot logs show: `context classifier backend=similarity threshold=0.55`
- All 4 migrations applied: channel_messages has `content_embedding vector(384)` column
- channel_conversations and allowed_channels tables exist with correct schema

**Operator Checklist**:
1. Ensure `/opt/iris-models/paraphrase-MiniLM-L3-v2/` exists on host with model files
2. Run `docker compose down && docker compose build bot && docker compose up -d`
3. Wait ~10s for migrations and bot startup
4. Verify: `docker compose logs bot | grep "backend=similarity"`
5. If bot fails to start, check: model file permissions, mount path, libonnxruntime.so availability

**Code Quality**:
- Exported helpers (Cosine, MeanPool, L2Normalize) for reuse in T3/T4
- Deterministic FakeEmbedder suitable for snapshot testing
- Thread-safe via mutex on onnxEmbedder.Embed()
- Clear error messages for production debugging
- Graceful resource cleanup in Close()

**Test Results**:
- Unit tests (embedder_test.go): 11/11 PASS with race detector
- Integration tests (embedder_onnx_test.go with embed_real tag): 3/3 PASS
  - TestONNXIntegration_Dim384: PASS (0.38s)
  - TestONNXIntegration_ParaphraseHigherThanUnrelated: PASS (0.55s) — sim("I'm tired","I need sleep") > sim("I'm tired","pizza recipe")
  - TestONNXIntegration_Concurrent_RaceFree: PASS (2.13s) — 8 concurrent goroutines, 5 texts each
- Build: SUCCESS
- Vet: SUCCESS
- LSP diagnostics: CLEAN (0 errors)

## T2 LEARNINGS

### pgvector Integration: Pointer-Based NULL Handling

**pgvector Library**: `github.com/pgvector/pgvector-go`

**Key Wiring Pattern**:
```go
// Write: use pgvector.NewVector() to convert []float32 to pgvector.Vector
embeddingVal := pgvector.NewVector(msg.ContentEmbedding)
_, err := tx.Exec(ctx, sql, ..., embeddingVal, ...)

// Read: use *pgvector.Vector pointer to handle NULL
var embedding *pgvector.Vector
err := rows.Scan(..., &embedding, ...)
if embedding != nil {
    msg.ContentEmbedding = embedding.Slice()  // Convert back to []float32
}
```

**NULL Handling Critical Detail**:
- pgvector.Vector (non-pointer) cannot scan NULL values → panics with "unsupported data type: <nil>"
- Must use `*pgvector.Vector` (pointer) to allow NULL
- Check `if embedding != nil` before calling `.Slice()`
- Empty slice `[]float32{}` is valid and distinct from nil

**Migration**:
- `ALTER TABLE channel_messages ADD COLUMN IF NOT EXISTS content_embedding vector(384);`
- No index added (per spec; can add later for ANN queries)
- Column is nullable by default

**Repository Pattern**:
- Upsert: `ON CONFLICT DO UPDATE SET content_embedding = EXCLUDED.content_embedding` updates embedding on conflict
- ListRecent, GetByID, ListByUserAcrossChannels: all select and scan embedding column
- Prune logic unchanged: still keeps 20 newest messages per guild/channel

**Capture Integration**:
- ChannelCaptureAdapter accepts optional `Embedder` interface
- Embedder field: `interface{ Embed(context.Context, string) ([]float32, error) }`
- Soft-fail: embedder errors logged at debug level, never propagate
- 5-second timeout on embed calls prevents hanging
- Empty content skips embedder call entirely
- Message always upserts, even if embedding fails

**Test Coverage**:
- TDD: wrote failing tests first, then implemented
- Repository tests: round-trip with known 384-dim vectors, NULL handling, update semantics
- Adapter tests: embedder interface acceptance, soft-fail on error, skip on empty content
- All existing tests still pass (backward compatible)

**Files Changed**:
- migrations/004_channel_message_embeddings.sql (new)
- internal/domain/types.go (added ContentEmbedding field)
- internal/repository/channel_context.go (Upsert, ListRecent, GetByID, ListByUserAcrossChannels)
- internal/app/wire/adapters.go (ChannelCaptureAdapter with embedder)
- internal/repository/channel_context_test.go (new embedding tests)
- internal/app/wire/adapters_test.go (capture integration tests)

**Lessons**:
1. pgvector.Vector pointer is essential for NULL handling in pgx
2. Soft-fail on embedder errors keeps capture resilient
3. 5-second timeout prevents indefinite hangs
4. Empty embedding (nil slice) is valid and distinct from missing column
5. ON CONFLICT DO UPDATE must include embedding column to update it on upsert

## T3 LEARNINGS

### Similarity-Based InWindowRelevance: Centroid Pooling Strategy

**Architecture Decision**: Implemented `SimilarityInWindowRelevance` as a second implementation of the `InWindowRelevance` interface, alongside the LLM-based classifier renamed to `NewLLMInWindowRelevance`.

**Centroid Pooling Strategy**:
- Context messages are embedded individually (reusing stored `ContentEmbedding` when present to minimize embedder calls)
- All context embeddings are L2-normalized (they come from the embedder already normalized)
- Centroid is computed as the mean of all context embeddings: `centroid[i] = sum(embeddings[i]) / len(embeddings)`
- Centroid is then L2-normalized

## T4 LEARNINGS

### Similarity-Based CrossChannelClassifier: Ordering + Truncation Policy

**Architecture Decision**: Implemented `SimilarityCrossChannelClassifier` as a second implementation of the `CrossChannelClassifier` interface, alongside the LLM-based classifier renamed to `NewLLMCrossChannelClassifier`.

**Key Design Decisions**:

1. **Interface Reuse**: Reused existing `CandidateStore` and `ChannelAllowQuerier` interfaces from cross_channel.go (no duplication).

2. **Filtering Pipeline** (identical to LLM classifier):
   - Exclude current channel (msg.ChannelID == event.ChannelID)
   - Exclude bot messages (msg.IsBot == true)
   - If allow-list mode enabled (Allowed.HasAny returns true), keep only allowed channels
   - Query limit: 20 candidates from store, filter down to MaxCandidates

3. **Embedding Strategy**:
   - Embed current message once via `cfg.Embedder.Embed(ctx, event.Message.Content)`
   - For each candidate: use `ContentEmbedding` if present (len > 0); else embed via embedder
   - Skip candidates that fail to embed (log at DEBUG level, continue)
   - Minimize embedder calls: reuse stored embeddings when available

4. **Similarity Computation & Ordering**:
   - Compute cosine similarity: `sim := Cosine(currentVec, candVec)` (both L2-normalized)
   - Keep only candidates where `sim >= threshold`
   - Sort descending by similarity (highest first)
   - Truncate to `MaxCandidates`
   - Return ordered slice: `[]*domain.ChannelMessage` with highest similarity first

5. **Defaults**:
   - Threshold: 0.55 (if cfg.Threshold == 0, use default)
   - MaxCandidates: 10 (if cfg.MaxCandidates <= 0, use default)
   - WindowMinutes: 30 (if cfg.WindowMinutes <= 0, use default)

6. **Nil-Safety**:
   - If Store or Embedder is nil, return (nil, nil) — safe no-op
   - If event or event.Message is nil, return (nil, nil)
   - If no candidates after filtering, return (nil, nil)
   - If current message embed fails, return (nil, err) — propagate error

7. **Logging**:
   - DEBUG-level log for each kept candidate: `cross_channel_sim id=%d sim=%.4f channel=%d`
   - WARN-level logs for query failures, allow-list errors, embed errors

**Test Coverage** (8 TDD tests, all PASS):
- `TestSimilarityCrossChannel_EmptyCandidates_ReturnsNil`: empty store → (nil, nil)
- `TestSimilarityCrossChannel_FiltersCurrentChannel`: exclude event.ChannelID
- `TestSimilarityCrossChannel_FiltersBotMessages`: exclude IsBot=true
- `TestSimilarityCrossChannel_FiltersUnallowedChannelsInIncludeListMode`: allow-list filtering
- `TestSimilarityCrossChannel_OrdersByScoreDescending`: highest similarity first
- `TestSimilarityCrossChannel_TruncatesToMaxCandidates`: truncate to MaxCandidates
- `TestSimilarityCrossChannel_NilEmbedderOrStore_NoOp`: nil deps → (nil, nil)
- `TestSimilarityCrossChannel_EmbedErrorReturnsErr`: current message embed error → (nil, err)
- `TestSimilarityCrossChannel_UsesStoredEmbeddings`: embedder called once for current message when all candidates have embeddings

**Files Changed**:
- internal/orchestrator/cross_channel.go: renamed `CrossChannelConfig` → `LLMCrossChannelConfig`, `NewCrossChannelClassifier` → `NewLLMCrossChannelClassifier`, `crossChannelClassifier` → `llmCrossChannelClassifier`
- internal/orchestrator/cross_channel_sim.go (new): `SimilarityCrossChannelConfig`, `NewSimilarityCrossChannelClassifier`, `similarityCrossChannelClassifier`
- internal/orchestrator/cross_channel_sim_test.go (new): 8 TDD tests
- internal/orchestrator/cross_channel_test.go: updated all test calls to use `NewLLMCrossChannelClassifier` and `LLMCrossChannelConfig`
- internal/orchestrator/orchestrator.go: updated type assertion to `*llmCrossChannelClassifier`

**Lessons**:
1. Ordering by similarity (descending) + truncation is simpler than LLM decision logic
2. Reusing existing interfaces (CandidateStore, ChannelAllowQuerier) keeps code DRY
3. Storing embeddings on candidates minimizes embedder calls (critical for performance)
4. Nil-safety at boundaries (Store, Embedder) prevents panics in production
5. DEBUG-level logging for kept candidates aids troubleshooting without noise
6. TDD with pre-populated embeddings in tests ensures deterministic similarity scores to unit norm
- Similarity is computed as dot product (cosine similarity) between normalized centroid and normalized current message embedding
- Decision: `similarity >= threshold` (default 0.55)

**Embedder Call Optimization**:
- Current message: always embedded (1 call)
- Context messages with stored `ContentEmbedding`: reused directly (0 calls)
- Context messages without stored embedding: fallback to embedder (1 call per missing)
- Errors during fallback embedding: message is skipped (logged, not propagated)
- Result: minimal embedder calls when messages have pre-computed embeddings

**Configuration Defaults**:
- `Threshold`: defaults to 0.55 if 0 (tuned for semantic relevance in conversation context)
- `MinContext`: defaults to 1 if 0 (require at least 1 context message)
- `Embedder`: must be non-nil; returns error if nil

**Nil Embedder Handling**:
- If `cfg.Embedder == nil`, `IsRelevant()` returns `(false, errors.New("embedder unavailable"))`
- Allows graceful degradation if embedder is not configured

**LLM Classifier Rename**:
- Old: `NewInWindowRelevance(InWindowRelevanceConfig)` → New: `NewLLMInWindowRelevance(LLMInWindowRelevanceConfig)`
- Old: `InWindowRelevanceConfig` → New: `LLMInWindowRelevanceConfig`
- Backward compatibility aliases provided for T5 wiring: `NewInWindowRelevance()` delegates to `NewLLMInWindowRelevance()`
- Both implementations satisfy the `InWindowRelevance` interface; T5 chooses which to wire

**Test Coverage (TDD)**:
- 10 new tests in `conversation_relevance_sim_test.go`:
  - `TestSimilarityInWindow_AboveThreshold_True`: identical vectors → sim=1.0 → true
  - `TestSimilarityInWindow_BelowThreshold_False`: orthogonal vectors → sim=0.0 → false
  - `TestSimilarityInWindow_EmptyContext_False`: empty context → false, no embedder calls
  - `TestSimilarityInWindow_NilEmbedder_ReturnsError`: nil embedder → error
  - `TestSimilarityInWindow_EmbedError_ReturnsFalseErr`: embedder error on current message → error
  - `TestSimilarityInWindow_UsesStoredEmbeddingsWhenPresent`: stored embeddings reused → embedder called once (current only)
  - `TestSimilarityInWindow_FallsBackWhenEmbeddingMissing`: missing embedding → fallback embed → embedder called twice
  - `TestSimilarityInWindow_CentroidIsL2Normalized`: centroid norm verified ~1.0
  - `TestSimilarityInWindow_DefaultThreshold`: threshold=0 → defaults to 0.55
  - `TestSimilarityInWindow_DefaultMinContext`: MinContext=0 → defaults to 1
- All 10 tests PASS with race detector
- All 6 existing LLM classifier tests still PASS (backward compatible)

**SpyEmbedder Test Helper**:
- Tracks call count, recorded texts, and allows controlled responses/errors
- Enables verification of embedder call optimization
- Supports multiple sequential responses for multi-call scenarios

**Debug Logging**:
- `slog.Debug()` logs: `embedder=similarity sim=%.4f threshold=%.2f decision=%v`
- Helps diagnose relevance decisions in production

**Files Created/Modified**:
- `internal/orchestrator/conversation_relevance_sim.go` (new): similarity implementation
- `internal/orchestrator/conversation_relevance_sim_test.go` (new): 10 unit tests
- `internal/orchestrator/conversation_relevance.go`: renamed LLM constructor/config, added backward compat aliases
- `internal/orchestrator/conversation_relevance_test.go`: updated to use `NewLLMInWindowRelevance`

**Verification**:
- `go build ./...`: SUCCESS
- `go vet ./...`: SUCCESS
- `go test -race -count=1 ./internal/orchestrator -run 'SimilarityInWindow|InWindowRelevance' -v`: 16/16 PASS
- `lsp_diagnostics`: CLEAN (0 errors)

**Lessons**:
1. Centroid pooling is simple and effective for multi-message context representation
2. L2 normalization of centroid ensures unit norm for dot-product similarity
3. Reusing stored embeddings dramatically reduces embedder calls (critical for performance)
4. Fallback embedding with error skipping keeps classifier resilient
5. Default threshold 0.55 balances precision/recall for conversation relevance
6. Backward compatibility aliases enable smooth transition for T5 wiring
7. SpyEmbedder pattern enables precise verification of call optimization


---

## T5 WIRING

### Config Fields Added
- `EmbedModelPath string` (env: `IRIS_EMBED_MODEL_PATH`)
- `EmbedTokenizerPath string` (env: `IRIS_EMBED_TOKENIZER_PATH`)
- `EmbedSimThreshold float64` (env: `IRIS_EMBED_SIM_THRESHOLD`; default: 0.55)

Parsing in `internal/config/config.go` lines 137–155: threshold parse error or empty → 0.55 fallback.

### Config Tests
- `TestEmbedConfig_DefaultsWhenUnset`: paths empty, threshold defaults to 0.55
- `TestEmbedConfig_PathsAndThresholdSet`: paths and threshold set correctly
- `TestEmbedConfig_InvalidThresholdFallsBack`: invalid threshold → 0.55

All pass. See `.sisyphus/evidence/embed-5-config.txt`.

### Main.go Wiring (cmd/iris-bot/main.go)

**Lines 149–196**: Embedder lazy construction + classifier wiring

1. **Embedder lazy init** (lines 149–160):
   - If `cfg.EmbedModelPath != "" && cfg.EmbedTokenizerPath != ""`:
     - Call `embedder.NewONNX(embedder.ONNXConfig{ModelPath: cfg.EmbedModelPath, TokenizerPath: cfg.EmbedTokenizerPath})`
     - On error: log warn, set `emb = nil`, continue with LLM fallback
   - If paths empty: `emb` stays `nil`

2. **Similarity classifiers** (lines 162–196):
   - If `emb != nil`:
     - `inWindowRelevance = orchestrator.NewSimilarityInWindowRelevance(orchestrator.SimilarityInWindowConfig{Embedder: emb, Threshold: cfg.EmbedSimThreshold, MinContext: 1})`
     - `crossChannelClassifier = orchestrator.NewSimilarityCrossChannelClassifier(orchestrator.SimilarityCrossChannelConfig{Store: ..., Allowed: ..., Embedder: emb, Threshold: cfg.EmbedSimThreshold, MaxCandidates: 10, WindowMinutes: 30})`
     - Log: `log.Info("context classifier backend=similarity", "threshold", cfg.EmbedSimThreshold)`
   - Else (fallback):
     - `inWindowRelevance = orchestrator.NewLLMInWindowRelevance(orchestrator.InWindowRelevanceConfig{LLM: ..., Model: cfg.LLMModelRouter})`
     - `crossChannelClassifier = orchestrator.NewLLMCrossChannelClassifier(orchestrator.CrossChannelConfig{Store: ..., Allowed: ..., LLM: ..., Model: cfg.LLMModelRouter})`
     - Log: `log.Info("context classifier backend=llm (embedder disabled)")`

3. **Capture adapter** (line 267):
   - Pass `Embedder: emb` to `ChannelCaptureAdapter` for optional embedding persistence

4. **Orchestrator config** (lines 263–284):
   - Wire chosen `inWindowRelevance` and `crossChannelClassifier` into `orchCfg`

**Startup log line** (one of):
- `context classifier backend=similarity threshold=0.55`
- `context classifier backend=llm (embedder disabled)`

### E2E Test (internal/orchestrator/orchestrator_sim_e2e_test.go)

**Test**: `TestE2ESimilarityInWindowRelevance`

**Setup**:
- `spyEmbedder`: deterministic embeddings for controlled similarity
  - "hello iris" → similarVec (0.1 normalized)
  - "follow up on that" → similarVec (high similarity)
  - "pizza recipe" → dissimilarVec (-0.1 normalized, low similarity)
- `simTestContextStore`: seeded with "hello iris" message
- `SimilarityInWindowRelevance` with threshold 0.55
- Router: `ReasonActiveConversation` (respects relevance decision)

**Steps**:
1. Event "hello iris" (mention): LLM called, reply sent
2. Event "follow up on that" (similar): LLM called, reply sent (high similarity ≥ 0.55)
3. Event "pizza recipe" (dissimilar): NO LLM call, NO reply (low similarity < 0.55)

**Assertions**:
- Event 1: chunks > 0 (LLM called)
- Event 2: chunks increase (LLM called for similar)
- Event 3: chunks do NOT increase further (LLM NOT called for dissimilar)

**Result**: PASS. See `.sisyphus/evidence/embed-5-e2e.txt`.

### Evidence Files

- `.sisyphus/evidence/embed-5-config.txt`: config test output (3 tests PASS)
- `.sisyphus/evidence/embed-5-e2e.txt`: e2e test output (1 test PASS)
- `.sisyphus/evidence/embed-5-fallback.txt`: grep showing both LLM and similarity constructors in main.go (lines 170, 175, 185, 189)

### Verification Summary

- `go build ./...`: ✓
- `go vet ./...`: ✓
- `go test -p 2 -count=1 ./...`: ✓ (all 31 packages pass)
- `go test -count=1 ./internal/orchestrator -run 'E2ESimilarityInWindowRelevance' -v`: ✓
- `CGO_ENABLED=0 go build -o /tmp/iris-bot-test ./cmd/iris-bot`: ✓ (stub paths compile)
- `lsp_diagnostics`: ✓ (clean on all modified files)

### Key Design Decisions

1. **Nil-safe fallback**: If embedder fails to load or paths unset, LLM classifiers activate automatically. No breaking changes.
2. **Lazy construction**: Embedder only loaded if both paths set. Reduces startup overhead when not in use.
3. **Single startup log line**: One info line declares backend for observability. Includes threshold if similarity.
4. **Threshold default**: 0.55 (from T3/T4 defaults). Parse error → fallback to 0.55.
5. **Capture adapter**: Optional embedder passed through for message persistence (T2 integration).
6. **No LLM calls when similarity available**: Orchestrator uses chosen classifier exclusively; no dual-calling.

### Files Modified

- `internal/config/config.go`: +3 fields, +19 lines (parsing)
- `internal/config/config_test.go`: +3 tests (defaults, paths, invalid threshold)
- `cmd/iris-bot/main.go`: +1 import, +47 lines (embedder init + classifier wiring)
- `.env.example`: +5 lines (new env docs)
- `internal/orchestrator/orchestrator_sim_e2e_test.go`: new file (e2e test with spyEmbedder)

### Backward Compatibility

- Existing LLM classifier wiring preserved (renamed to `NewLLMInWindowRelevance`, `NewLLMCrossChannelClassifier`)
- No breaking changes to orchestrator interfaces
- Config fields optional; unset → LLM fallback
- All existing tests pass

## F4 FALSE POSITIVE

**Flagged Issue**: "rate-limit / unrelated refactors" as scope creep

**Root Cause**: The working tree contains pre-existing uncommitted changes from the prior `discord-audit-context` plan session. These changes include deletions of `internal/ratelimit/` package files and modifications to unrelated packages (discord, logger, persona, etc.).

**Verification**:
- `internal/ratelimit/` directory does NOT exist in the current tree (confirmed: `ls internal/ratelimit 2>/dev/null || echo "NO RATELIMIT DIR"` → NO RATELIMIT DIR)
- Git status shows deletions: `D internal/ratelimit/ratelimit.go` and `D internal/ratelimit/ratelimit_test.go` (pre-existing from prior plan)
- No new rate-limit code was introduced by this plan
- All changes in this plan are scoped to: `internal/embedder/embedder_onnx.go` (Close() concurrency fix), `internal/embedder/embedder_onnx_test.go` (new tests), and `.sisyphus/evidence/` files (test output capture)

**Conclusion**: F4's flagged scope creep is a FALSE POSITIVE caused by diff noise from the prior session. The embedding-classifier plan introduces zero rate-limit or unrelated refactors. Pre-existing changes are preserved as-is per instructions.

## T4 PERSONA v1.2.0

**Scope**: Persona rewrite so Iris calls the websearch tool herself instead of telling users "check the wiki", and never leaks URLs or source names in replies.

**Tone changes** (casual Indonesian, aku/kamu register preserved):
- Old `[LORE POLICY]` opened with "Sumber kanon utama Wuthering Waves ada di wiki Fandom: wutheringwaves.fandom.com." and item 1 told Iris to "kasih sitasi berupa judul halaman + URL fandom yang relevan." Both removed.
- New `[LORE POLICY]` item 1: "Kalau lo perlu info yang gak ada di arsip atau memori, panggil tool `websearch` dulu sebelum bilang gak tahu. Pakai query yang fokus, jangan asal lempar nama doang."
- Old item 2 (bilang "belum ada data" dan arahkan ke wiki) replaced with: "Kalau udah cari lewat websearch dan tetep gak nemu, bilang apa adanya... Jangan nyuruh user ke situs lain, jangan sebut-sebut sumber eksternal."
- New URL-leak prohibition as item 2: "Jawaban ke user GAK BOLEH ngeluarin URL, nama situs sumber, atau link mentah, kecuali user minta eksplisit (misalnya 'kasih link' atau 'kasih sumber')."
- Empty-citation fallback changed from "arahkan ke wiki" to "Kalau pertanyaannya soal lore dan lo butuh data baru, panggil tool websearch dulu."
- Rendered citations now emit `- Title: Excerpt` instead of `- Title (URL): Excerpt`. URLs stay purely internal for retrieval bookkeeping.
- `immutablePersona` wording "judul wiki" changed to "judul kanon" so the word "wiki" no longer appears in user-facing output.

**Kept intact**:
- `LoreCitation` validator and its `fandomHost` constant (internal only, never printed).
- `ErrCitationWrongDomain` error message rewritten to "lore citation must link to the canonical lore host" so the error string itself doesn't leak the host name either.
- All 4 identity-lock rules in `[IMMUTABLE PERSONA]`, `[MEMORY CONTEXT]` block, section order (PERSONA < LORE < MEMORY), memory-override guards.
- `Version()` bumped `1.1.0` -> `1.2.0`.

**New test invariants**:
- `TestImmutablePersona_NoURLs`: prompt must contain none of `http://`, `https://`, `fandom`, `wiki`, `kurogames.com`.
- `TestImmutablePersona_MentionsWebsearch`: prompt mentions `websearch` (case-insensitive).
- `TestImmutablePersona_IdentityLockPreserved`: persona markers (I.R.I.S, Intelligent Retrieval & Indexing System, Bahasa Indonesia, Identitas sebagai I.R.I.S, `[IMMUTABLE PERSONA]`) all present.
- `TestPersonaVersion`: exact equality to `"1.2.0"`.
- `TestBuildSystemPrompt_LoreCitationsRendered` updated to assert URL does NOT leak into prompt, only title + excerpt rendered.
- `TestBuildSystemPrompt_RejectsNonFandomCitations` updated to also assert neither `example.com` nor `fandom.com` leaks.

**Verification**: `go test ./internal/persona -count=1 -v` all pass; full `go test -p 2 -count=1 ./...` green; `go build ./...` and `go vet ./...` clean; `lsp_diagnostics` on `internal/persona` reports zero issues; `docker compose build bot` succeeds, `docker compose up -d bot` starts cleanly, `Discord gateway connected` log line present. The "websearch tool not registered" WARN is expected in this environment because `SEARCH_BASE_URL` / `SEARCH_API_KEY` aren't set in compose; that wiring was validated under T3.

## T2 TYPING LIFECYCLE

### Architecture: Typing Spans Full Pipeline

**Decision**: Move typing indicator to the top of the orchestrator pipeline (immediately after router decision) and keep it active through the entire LLM round-trip, including future streaming and tool-call loops.

**Implementation**:

1. **gateway.go SendTyping error logging** (line 147-154):
   - Changed from silently ignoring errors to logging at WARN level
   - Non-fatal: error is returned but caller (orchestrator) decides whether to propagate
   - Format: `ga.logger.Warn("send_typing_failed", "guild", guildID, "channel", channelID, "err", err)`

2. **orchestrator.go handle() lifecycle** (lines 268-307, 405-430, 451-481):
   - **Typing start**: Moved to line 282 (right after router decision, before cross-channel classify)
   - **Lifecycle logging**:
     - `typing_started` at INFO level when `startTyping()` is called (reason: "pipeline_start")
     - `typing_refresh` at DEBUG level every 5s (TypingRepeat) from refresh goroutine
     - `typing_stopped` at INFO level right before `stopTyping()` defer fires (line 430)
   - **Refresh goroutine**: Sends typing immediately, then every TypingRepeat interval
   - **Cleanup**: `stopTyping` defer at end of handle() ensures typing stops after ALL discord sends (including multi-chunk send loop)

3. **Idempotency**: `sync.Once` in `stopTyping` closure ensures calling stop() twice is safe (channel close is guarded)

**Test Coverage**:

- `TestTyping_SpansPipeline`: Fake LLM blocks for 7 seconds; typing recorder counts calls; asserts `calls >= 2` (initial + at least 1 refresh over 7s with 5s repeat)
- `TestTyping_ErrorLogsWarn`: Fake SendTyping returns error; asserts orchestrator still sends message (error is non-fatal) and no panic

**Verification**:
- `go build ./...` ✓
- `go vet ./...` ✓
- `go test -p 2 -count=1 ./internal/orchestrator ./internal/discord` ✓ (all tests pass)
- `go test -p 2 -count=1 ./...` ✓ (full suite passes)
- `lsp_diagnostics` on `internal/orchestrator` and `internal/discord` ✓ (zero errors)
- Docker compose build and start ✓
- Test output shows lifecycle logs in order: `typing_started` → `typing_refresh` (DEBUG, not shown in INFO logs) → `typing_stopped`

**Evidence**:
- `.sisyphus/evidence/stream-2-typing.txt`: Test output showing both new tests passing with 7s latency and error handling
- `.sisyphus/evidence/stream-2-live.txt`: Docker logs showing bot startup (no user messages in test window, but infrastructure ready)

**Downstream Impact**:
- T3 (streaming) can now rely on typing staying visible through the entire response generation
- T4 (tool-call streaming) will inherit the same lifecycle
- T5 (streaming sender) will integrate with this lifecycle without changes to typing logic

## T3 SSE STREAMING

### SSE Parsing & Streaming Implementation

**Public API**:
```go
type StreamCallbacks struct {
    OnDelta func(text string)
    OnDone  func()
    OnError func(err error)
}

func (c *Client) ChatStream(
    ctx context.Context,
    model string,
    guildID int64,
    messages []map[string]string,
    cb StreamCallbacks,
) (finalText string, err error)
```

**Implementation Details**:

**SSE Protocol Handling**:
- POST `/v1/chat/completions` with `stream: true`
- Set `Accept: text/event-stream` header
- Response is line-based: `data: {...json...}\n\n`
- Stream terminates with `data: [DONE]\n\n`
- Use `bufio.Scanner` with 2 MB buffer (default 64k insufficient for large deltas)
- Parse case-sensitively: line must start with `data: ` prefix (SSE spec)
- Handle blank lines between events without panicking

**Delta Extraction**:
- Each SSE chunk shape: `{"choices":[{"delta":{"content":"..."},"finish_reason":null}]}`
- Extract `choices[0].delta.content` when present and non-empty
- Append to accumulator, call `cb.OnDelta(content)`
- Ignore empty deltas (delta object with no content field)

**Error Handling**:
- On non-200 status, network error, or JSON parse error: call `cb.OnError(err)` and return error
- Honor `ctx.Done()` by aborting body reader; return `ctx.Err()`
- On stream end without `[DONE]`: call `cb.OnError()` and return error

**Retry Strategy**:
- Retry only on first-byte failures (before any SSE data emitted)
- Track `emittedFirstDelta` flag: once set, do not retry
- After first delta, network errors are terminal (no retry)
- Respects `MaxRetries` and `RetryDelay` config

**Context Integration**:
- Attach `llm.WithMeta(ctx, &llm.ContextMeta{GuildID: guildID, TriggerReason: "chat_stream"})`
- Audit logging includes final accumulated text and error (if any)

**Test Coverage** (6 cases, all passing):
- `TestChatStream_Emits_DeltasInOrder`: 4 deltas "Ha", "lo", " d", "unia" → "Halo dunia"
- `TestChatStream_HandlesDone`: OnDone called exactly once after [DONE]
- `TestChatStream_NetworkError_CallsOnError`: Connection close mid-stream → OnError called
- `TestChatStream_CtxCancellation_StopsStream`: Cancel ctx after first delta → no further deltas
- `TestChatStream_RetriesOnFirstByteFailure`: 500 on first attempt, success on second → OnDelta fires only on second
- `TestChatStream_IgnoresEmptyDeltas`: Empty delta objects between content chunks → not passed to OnDelta

**Verification**:
- `go test ./internal/llm -count=1 -v -run 'ChatStream'` ✓ (6/6 pass, 0.154s)
- `go test -p 2 -count=1 ./...` ✓ (full suite passes, no regressions)
- `go build ./...` ✓
- `go vet ./...` ✓
- `lsp_diagnostics` ✓ (zero errors, only pre-existing `any` hints)

**Key Design Decisions**:
- Standard library only: `net/http`, `bufio`, `encoding/json`, `strings`, `context`
- No new dependencies introduced
- Callbacks pattern allows flexible downstream integration (T4 tool-call streaming, T5 streaming sender)
- 2 MB buffer limit prevents OOM on pathological single-delta responses
- Retry logic respects streaming semantics: once data flows, connection is terminal

**Downstream Impact**:
- T4 (ChatWithToolsStream) can reuse SSE parsing logic for tool-call deltas
- T5 (StreamingSender) receives accumulated text + callback pattern for per-paragraph flushing
- Existing Chat/ChatWithModel/ChatWithTools paths unchanged (purely additive)

## T4 TOOL STREAMING

### Delta Accumulator Pattern for Fragmented Tool Arguments

**Problem**: OpenAI-compatible Claude proxies emit tool-call arguments as JSON fragments in `delta.tool_calls[].function.arguments`. Must accumulate fragments and parse complete JSON before executing tool.

**Solution**: Index-keyed accumulation with per-call state machine.

**Data Structure**:
```go
type pendingToolCall struct {
    id      string              // Tool call ID (arrives once, early)
    name    string              // Function name (arrives once, early)
    argsBuf strings.Builder     // Accumulates JSON fragments
}
```

**Accumulation Strategy**:
- Maintain `map[int]*pendingToolCall` keyed by `delta.tool_calls[].index`
- For each delta fragment:
  - If `fragment.ID != ""`: set `pending.id = fragment.ID` (idempotent)
  - If `fragment.Function.Name != ""`: set `pending.name = fragment.Function.Name` (idempotent)
  - If `fragment.Function.Arguments != ""`: append to `pending.argsBuf` via `WriteString()`
- When stream ends (`[DONE]`): for each pending call, `json.Unmarshal(argsBuf.String(), &argsMap)`

**Parallel Tool Calls**:
- Multiple indices in single delta: each gets its own entry in map
- All accumulated independently, all executed in order
- Example: `[{index:0,id:"a",function:{name:"t1",arguments:"{}"}}, {index:1,id:"b",function:{name:"t2",arguments:"{}"}}]`

**Error Handling**:
- Invalid JSON in accumulated args: skip Execute, append tool response with `content: "error: invalid JSON arguments"`
- Tool execution error: append tool response with `content: "error: <exec error>"`
- Continue loop (don't abort on single tool failure)

**Multi-Round Loop**:
1. POST `/v1/chat/completions` with `stream: true`, `tools: cfg.Tools`, messages
2. Parse SSE deltas:
   - Text fragments: accumulate + call `cfg.OnDelta(fragment)`
   - Tool-call fragments: accumulate by index
   - Track `finish_reason`
3. When stream ends:
   - If `finish_reason="stop"`: return accumulated text, nil
   - If `finish_reason="tool_calls"`: execute all pending tools, append to message history, loop
4. Max rounds: cap at `cfg.Max` (default 3), return accumulated text on exceed

**Implementation Details**:

**ChatWithToolsStreamConfig**:
```go
type ChatWithToolsStreamConfig struct {
    Model   string                   // LLM model
    GuildID int64                    // Audit logging
    Tools   []map[string]interface{} // OpenAI tool definitions
    Exec    ToolExecutor             // Tool executor
    Max     int                      // Max rounds; default 3 if 0
    OnDelta func(text string)        // Callback on text fragments
}
```

**Helper Functions**:
- `doStreamWithToolsRetryTracking()`: HTTP retry wrapper for streaming tool-call responses
- `parseSSEStreamWithTools()`: SSE parser with tool-call accumulation and text delta callbacks

**Audit Logging**:
- Tool calls logged at DEBUG level with truncated args (max 100 chars)
- Final text and error logged after all rounds complete
- Correlation ID preserved across multi-round loop

**Test Coverage** (6 cases, all passing):
- `TestChatWithToolsStream_NoToolCalls_StreamsText`: Text-only streaming, OnDelta fires per fragment
- `TestChatWithToolsStream_OneToolCall_ThenStreamsText`: Tool call with fragmented args, second round text
- `TestChatWithToolsStream_MaxRounds_StopsGracefully`: Loop terminates at max rounds without panic
- `TestChatWithToolsStream_FragmentedArguments_AssembledBeforeExec`: Args arrive in 4 fragments, assembled before Execute
- `TestChatWithToolsStream_ParallelToolCalls`: Two tool calls in single delta, both executed
- `TestChatWithToolsStream_InvalidArgsJSON_ReportsError`: Malformed JSON args → error response, no Execute

**Verification**:
- `go test ./internal/llm -count=1 -v -run 'ChatWithToolsStream'` ✓ (6/6 pass, 0.008s)
- `go test -p 2 -count=1 ./...` ✓ (full suite passes, 34 packages)
- `go build ./...` ✓
- `go vet ./...` ✓
- `lsp_diagnostics` ✓ (zero errors)

**Key Design Decisions**:
- Index-based keying: handles parallel tool calls naturally
- Idempotent field accumulation: ID and name can arrive in any order across deltas
- strings.Builder for args: efficient concatenation of JSON fragments
- OnDelta callback: allows T5 to stream text in real-time while tool calls accumulate
- Preserve ChatWithTools: non-streaming path unchanged for classifiers

**Downstream Impact**:
- T5 (StreamingSender) receives OnDelta callbacks for per-paragraph flushing
- Classifiers continue using non-streaming ChatWithModel (no change)
- Tool-call loop now supports streaming: faster perceived response time
- Fragmented args pattern reusable for other streaming APIs

## T5 STREAMING SENDER

### Architecture: Per-Channel Rate Limiter + Paragraph-Boundary Flushing

**Decision**: Implement `StreamingSender` with token bucket rate limiter (5 sends/5s per channel) and paragraph-boundary detection (\n\n + 200 bytes).

**Why This Approach**:
- Per-channel isolation: independent rate limits prevent one channel from blocking another
- Paragraph boundaries: natural semantic units, better UX than arbitrary chunk sizes
- Soft threshold (1600 bytes): avoids Discord's 2000-char limit while keeping chunks meaningful
- Token bucket: simple, fair, and handles bursts gracefully with backoff

**Implementation Details**:

**ChannelRateLimiter** (`internal/orchestrator/rate_limiter.go`):
- Maintains per-channel sliding window of send timestamps
- On `Wait(ctx, channelID)`: evicts old timestamps, checks if capacity available
- If at capacity: computes sleep = oldestTimestamp + window - now, waits or returns ctx.Err()
- Max cumulative wait: 10 seconds (configurable default)
- Thread-safe via sync.Mutex

**StreamingSender** (`internal/orchestrator/stream_sender.go`):
- Buffers text fragments in strings.Builder
- Flush criteria:
  1. Paragraph boundary: `\n\n` in buffer AND buffer >= 200 bytes → emit up to last `\n\n`
  2. Soft threshold: buffer >= 1600 bytes → emit full chunks via SplitMessage, keep last partial
  3. Explicit Flush(): emit all remaining via SplitMessage
- Each chunk passes through `limiter.Wait(ctx, channelID)` before SendMessage
- Errors logged as warn, next chunk still tries (resilient)
- Closed after Flush; further Push silently drops
- Per-instance serialization via sync.Mutex

**Integration into Orchestrator**:
- Config fields: `Streaming bool`, `StreamLLM`, `StreamToolsLLM`, `RateLimiter`
- In handle(): if `Streaming && StreamLLM != nil && len(Tools) == 0`:
  - Build StreamingSender, call ChatStream with OnDelta → sender.Push, OnDone → sender.Flush
  - Use returned full text for persistence (existing capture path)
  - Don't also emit chunks (streaming replaces plain Chat)
- Else if `Streaming && StreamToolsLLM != nil && len(Tools) > 0`:
  - Call ChatWithToolsStream with OnDelta callback
  - Flush after return
- Else: fall back to plain Chat or ChatWithTools (non-streaming path)
- Typing goroutine unchanged: already spans full handle() after T2

**Configuration**:
- `IRIS_STREAMING` env var: defaults to true ("false" or "0" disables)
- Rate limiter: 5 sends per 5 seconds per channel (hardcoded, can be made configurable)
- Paragraph boundary: \n\n in buffer AND buffer >= 200 bytes
- Soft threshold: 1600 bytes (uses SplitMessage for chunks <= 2000)

**Test Coverage**:
- `TestRateLimiter_AllowsWithinWindow`: 5 sends succeed immediately
- `TestRateLimiter_BlocksOnBurst`: 7 sends show no more than 5 in any 5s window
- `TestRateLimiter_PerChannelIndependent`: two channels each get 5 sends without blocking
- `TestRateLimiter_ContextCancellation`: respects ctx.Done()
- `TestStreamingSender_EmitsOnParagraphBoundary`: \n\n + 200 bytes triggers flush
- `TestStreamingSender_FlushesRemainder`: Flush() emits all remaining
- `TestStreamingSender_RespectsRateLimit`: many paragraphs respect rate limit
- `TestStreamingSender_HandlesLongParagraphSplits`: >2000 char paragraph splits correctly
- `TestStreamingSender_ClosedDropsSilently`: post-close Push drops silently
- `TestHandle_StreamingPath_SendsPerParagraph`: orchestrator integration sends per paragraph
- `TestHandle_StreamingPath_FallsBackWhenDisabled`: Streaming=false uses plain Chat
- `TestStreamingSender_MultipleChannels`: independent channels don't block each other

**What Works Now**:
- Rate limiter enforces 5 sends/5s per channel with sliding window
- Streaming sender buffers and flushes at paragraph boundaries
- Long messages split correctly (< 2000 chars)
- Per-channel rate limiting is independent
- Streaming can be disabled via IRIS_STREAMING=false
- Non-streaming path preserved for rollback
- All 12 new tests PASS
- Build and vet clean
- Binary builds successfully (17M, statically linked)

**Downstream Impact**:
- Users see Iris's reply stream in real-time, one paragraph at a time
- Discord rate limit respected locally (5 msg/5s per channel)
- Typing indicator stays visible throughout streaming (T2 already handles this)
- Fallback to non-streaming path available if needed
- Tool-calling path can also stream (StreamToolsLLM optional)
- Memory promotion and lock refresh run after Flush (unchanged)

## T6 ESCALATE + PERSONA 1.3.0

### Escalation Architecture: Short-Circuit on Tool Fire

**Design Pattern**: Single-round escalation with tool filtering.

**Flow**:
1. Weak model (haiku) runs with full tool set including `escalate_to_strong_model`.
2. If weak model calls escalate tool, `EscalationAwareExecutor` captures the reason.
3. Orchestrator detects reason != "" after first LLM call.
4. **Short-circuit**: Discard weak model's incomplete response via `StreamingSender.Discard()`.
5. Filter tools: remove `escalate_to_strong_model` from tool list (prevent re-escalation).
6. Re-run with strong model (claude-opus) using filtered tools.
7. Clear executor reason for reusability across requests.
8. **Cap at 1 escalation**: `alreadyEscalated` bool prevents looping.

**Key Implementation Details**:

- **StreamingSender.Discard()**: Resets buffer, marks closed, no send. Idempotent, thread-safe.
- **escalationAware interface** (local to orchestrator): `Reason() string`, `Clear()`. Avoids import cycle.
- **EscalationAwareExecutor.Clear()**: Resets reason after escalation so executor is reusable.
- **Tool filtering**: Iterate cfg.Tools, skip any with function.name == "escalate_to_strong_model".
- **Logging**: `llm_escalated reason=<reason> from=<modelToUse> to=<cfg.StrongModel>`.

**Why This Works**:
- Weak model decides when escalation is needed (no hardcoded heuristics).
- Strong model never sees escalate tool (prevents infinite loops).
- Single sender per round (clean narrative, no mixed responses).
- Reason captured before discard (no data loss).

### Persona v1.3.0: Escalation Rules

**New Rules** (in casual Indonesian):
1. "Kamu default-nya jalan di model haiku yang cepat dan irit."
2. "Kalau pertanyaan butuh analisis mendalam, reasoning multi-langkah, atau referensi lore yang rumit, panggil tool `escalate_to_strong_model` dengan alasan singkat SEBELUM jawab. Contoh: timeline kompleks, teori kanon, analisis patch mendalam, debug build karakter."
3. "Jangan panggil escalate buat greeting, fakta sederhana, atau obrolan ringan."

**Preserved**:
- v1.2.0 rules (no URL leakage, websearch rule, casual tone).
- All existing tests still pass.

**Tests Added**:
- `TestPersonaVersion_1_3_0`: Version == "1.3.0".
- `TestImmutablePersona_MentionsEscalateTool`: Persona contains "escalate_to_strong_model".
- `TestImmutablePersona_KeepsWebSearchRule`: Persona still contains "websearch".
- `TestImmutablePersona_NoURLs_v1_3_0`: No URL leakage regression.

### Wiring in main.go

**Changes**:
1. Import `github.com/eko/iris-bot/internal/tools/escalate`.
2. Register escalate tool: `registry.Register(&tools.ToolDefinition{Tool: escalate.New(), Timeout: 1*time.Second, MaxOutput: 1024})`.
3. Wrap ToolExecutor: `wireadapters.NewEscalationAwareExecutor(&wireadapters.RegistryExecutor{Reg: registry})`.
4. Set `orchCfg.StrongModel = cfg.LLMModelStrong`.
5. Log tools count: `log.Info("tools registered", "count", len(registry.OpenAIFunctions()))`.

**Startup Logs**:
```
escalate tool registered
websearch tool registered
tools registered count=2
```

### Test Coverage

**Orchestrator Tests**:
- `TestHandle_EscalateToolCall_ReRunsWithStrongModel`: Verifies two LLM calls, second uses strong model, escalate tool filtered.
- `TestHandle_EscalateCappedAtOneRound`: Verifies escalation capped at 1 round (no third call).

**Persona Tests**:
- All v1.2.0 tests still pass (no regression).
- v1.3.0 version test passes.
- Escalate tool mention test passes.
- Websearch rule preservation test passes.

### Verification

- `go test ./internal/tools/escalate ./internal/orchestrator ./internal/persona -count=1 -v`: All pass.
- `go test -p 2 -count=1 ./...`: All 38 packages pass.
- `go build ./...`: Clean.
- `go vet ./...`: Clean.
- `CGO_ENABLED=0 go build -o /tmp/iris-bot-test ./cmd/iris-bot`: 17M binary, no CGO.
- `docker compose build bot && docker compose up -d bot`: Startup logs show escalate + websearch registered, tools count=2.


## FINAL WAVE FIXES

### Summary
Fixed four critical blockers in the streaming and retry logic:

1. **Duplicate send after streaming** (orchestrator.go)
   - Added `streamingUsed` flag to prevent re-sending accumulated response after streaming completes
   - Streaming callbacks already emit messages via StreamingSender, so post-response loop must be skipped
   - Test: `TestHandle_StreamingPath_NoDuplicateSend` verifies exactly 3 messages (one per paragraph)

2. **Dead emittedFirstDelta variable** (client.go)
   - Variable was declared but never set to true, making retry logic ineffective
   - Fixed by passing `*bool` to parseSSEStream functions
   - Now correctly prevents retries once first delta is emitted (can't rewind streaming response)
   - Tests: `TestChatStream_NoRetryAfterFirstDelta` (no retry after first delta), existing `TestChatStream_RetriesOnFirstByteFailure` (retries on first-byte failure)

3. **StreamingSender.Flush verification**
   - Confirmed Flush is called exactly once per sender instance
   - Flush is idempotent (sets `closed=true`, returns early on subsequent calls)
   - Escalation path correctly flushes secondSender and skips original sender flush

4. **Docker-compose healthcheck**
   - Removed pgrep-based healthcheck (pgrep not in debian:bookworm-slim)
   - Replaced with `disable: true` (healthcheck not relied upon)

### Changes
- `internal/orchestrator/orchestrator.go`: Lines 390-497 (streamingUsed flag)
- `internal/llm/client.go`: Lines 695-764, 766-833, 904-980, 982-1078 (emittedFirstDelta fixes)
- `internal/orchestrator/orchestrator_streaming_test.go`: New test TestHandle_StreamingPath_NoDuplicateSend
- `internal/llm/client_stream_test.go`: New test TestChatStream_NoRetryAfterFirstDelta
- `docker-compose.yml`: Lines 93-94 (healthcheck disabled)

### Verification
- All tests pass (go test -p 2 -count=1 ./...)
- go vet clean
- CGO_ENABLED=0 build succeeds
- docker compose build bot succeeds
- docker compose up -d bot succeeds, container shows "Up" status
- All expected startup logs present

## RATE LIMITER MAXWAIT FIX

**Defect A — maxWait unused**: The `deadline` computed from `l.maxWait` was never checked in the select loop, allowing the limiter to sleep past its own bound.

**Fix**: Added deadline enforcement in the for loop's select statement:
- Check `remaining := time.Until(deadline)` before each sleep
- Return `ErrRateLimitExceeded` if `remaining <= 0`
- Clamp sleep duration to `min(sleepDuration, remaining)` to respect the deadline
- Lines changed: 43-83 in rate_limiter.go

**Defect B — orphan package-level Wait**: Removed dead code `func Wait(ctx, channelID) error { return nil }` (was lines 33-35).

**Test Added**: `TestRateLimiter_RespectsMaxWait`
- Limiter: maxPerWindow=1, window=30s, maxWait=100ms
- First Wait consumes the slot
- Second Wait returns `ErrRateLimitExceeded` after ~100ms (not blocking until window elapses)
- Test bounded with 500ms context to prevent flake

**Verification**:
- All 5 RateLimiter tests pass
- All 36 project packages pass
- go build ./... clean
- go vet ./... clean (only hint about min modernization)
- lsp_diagnostics clean

## REMOVE STREAM DEADLINE

**Problem**: ChatStream and ChatWithToolsStream had hardcoded 120s timeout wrappers that killed long Opus responses mid-stream.

**Solution**: 
- Removed `context.WithTimeout(ctx, 120*time.Second)` from both streaming functions
- Streaming now governed solely by caller's context + JobTimeout
- Increased JobTimeout default from 45s → 5m (enough for strong model response + tool round-trips)

**Changes**:
1. `internal/llm/client.go` lines 226-287 (ChatStream): removed timeout wrapper, use ctx directly
2. `internal/llm/client.go` lines 289-470 (ChatWithToolsStream): removed timeout wrapper, use ctx directly
3. `internal/orchestrator/orchestrator.go` line 139: JobTimeout 45s → 5m
4. `cmd/iris-bot/main.go` line 362: JobTimeout 45s → 5m

**Verification**:
- ✓ go build, go vet clean
- ✓ All 36 test packages pass
- ✓ TestChatStream_CtxCancelled_AbortsScannerWithinOneSecond still passes (ctx cancellation still works)
- ✓ Docker build + deploy clean, no errors

**Impact**: Long streaming responses (Opus, tool loops) no longer timeout artificially. HTTP client's own Timeout still applies to non-streaming calls. Goroutine+channel cancellation pattern preserved.
