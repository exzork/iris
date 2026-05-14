# Tier Default, Typing Fix, Streaming

## TL;DR
> **Summary**: Default LLM tier to `haiku` for casual chat; expose an `escalate_to_strong_model` function tool so the model itself picks a bigger brain when needed; fix typing indicator so it stays visible through the full LLM round-trip (streaming and tool-call loops); enable SSE streaming for the chat call and stream Iris's reply to Discord one paragraph at a time, obeying 5 msg/5 s per channel.
> **Deliverables**:
> - Tier classifier becomes a cheap first-pass gate; actual chat runs with haiku unless the model calls `escalate_to_strong_model`.
> - `SendTyping` error is logged; typing refresh loop covers the whole pipeline including streaming and tool-call rounds.
> - `Client.ChatStream` and `Client.ChatWithToolsStream` emit text deltas through a callback; tool-call deltas are buffered until complete before executing.
> - `StreamingSender` in orchestrator buffers deltas and flushes per paragraph (`\n\n`) or when a soft 1,600-char threshold hits; Discord rate limit respected by a local token bucket (5 sends / 5 s per channel) with backoff on 429.
> **Effort**: Large
> **Parallel**: NO — 6 sequential tasks, each medium.
> **Critical Path**: T1 → T2 → T3 → T4 → T5 → T6

## Context
### Original Request
- Default to haiku for casual conversations.
- Let the model escalate to a stronger model via prompt/tool call when the message or context calls for it.
- Fix Iris's typing indicator (users don't see it).
- Switch LLM to streaming; stream the response to Discord per paragraph; respect Discord send-message rate limit.

### Interview Context (from recon)
- `TierRouter.Classify` runs today but its result is discarded by the non-tool-call path: `o.cfg.LLM.Chat(...)` ignores the chosen tier and uses `c.config.Model` (hardcoded to `LLM_MODEL`, currently `kr/claude-opus-4.7`).
- `ChatWithTools` path already respects the tier-chosen model via `ChatWithToolsConfig.Model`. So every incoming message actually hits the strong model unless classifier promotes haiku/sonnet and the tools path is taken.
- `SendTyping` errors are swallowed; the refresh goroutine stops when `typingStop()` fires at the end of `handle`, meaning typing is cancelled before chunks finish sending. With streaming it would die even sooner.
- Streaming the tool-call loop adds complexity: Claude proxies emit `delta.tool_calls[i].function.arguments` as JSON fragments that must be concatenated before executing the tool.
- discordgo v0.27+ handles 429 internally with exponential backoff, so we primarily need local pacing to avoid hitting that path.

## Work Objectives
### Core Objective
Iris becomes cheaper and faster by default, more observably alive, and visibly "typing" while she actually is — and she streams her reply in real paragraphs.

### Deliverables
- `LLM_MODEL_DEFAULT` behavior change: orchestrator passes the tier-chosen model into `Chat` (new `ChatWithModel`-style entry point) and `ChatStream`.
- `escalate_to_strong_model` function tool registered and handled internally by the orchestrator: when the tool fires, re-run the same messages (minus the tool-call round-trip) against `LLM_MODEL_STRONG` and stream that response instead.
- Persona addendum: "If you need more reasoning horsepower, call `escalate_to_strong_model` with a short reason before answering."
- Typing: `SendTyping` error logged; typing goroutine extended to cover streaming/tool loops; typing ticker runs every 5 s independent of the LLM call lifecycle and stops only after final Discord send (or context cancel).
- New `Client.ChatStream(ctx, cfg, onDelta, onToolCall)` that hits `/v1/chat/completions` with `stream:true`, parses SSE, invokes `onDelta(text)` for content fragments, invokes `onToolCall(callID, name, args)` once a complete tool call is assembled.
- Streaming orchestrator loop that: opens typing → calls ChatStream → buffers deltas → flushes per paragraph via `StreamingSender` → sends final trailing buffer → closes typing → refreshes lock → fires memory promoter.
- Per-channel token bucket (5 sends / 5 s) in the sender adapter with short-jitter backoff on 429.

### Definition of Done (verifiable)
- `go build ./...`, `go vet ./...`, `go test -p 2 -count=1 ./...` pass.
- New unit tests for streaming SSE parser, streaming splitter, per-channel rate limiter, and escalation tool.
- Logs show `model=kr/claude-haiku-4.5` for simple greetings.
- Logs show `escalate_to_strong_model` tool call for complex prompts, then a second `llm_request model=kr/claude-opus-4.7`.
- Discord typing indicator visible during the whole reply, including during streaming.
- Discord reply arrives in multiple messages (one per paragraph) when content is long enough.
- No 429 errors observed during a burst; local token bucket paces sends.

### Must Have
- TDD for all new code.
- Streaming is opt-in via config flag `IRIS_STREAMING=true` default true, fallback to current non-streaming path on env `=false` to allow rollback.
- Tool-call path still supports streaming. Delta tool arguments must be accumulated keyed by call index before Execute is called.
- Classifiers (cross-channel, in-window relevance, memory promoter) stay on plain non-streaming `ChatWithModel` with no tools. They parse strict JSON.
- Typing sender + Discord sender are independent adapters: typing doesn't contend with rate limit.
- Persona v1.3.0.

### Must NOT Have
- No dependency on a third-party SSE library; parse text/event-stream manually (Go stdlib is enough).
- No change to the existing `Chat` / `ChatWithModel` / `ChatWithTools` signatures (classifiers depend on them).
- No hardcoded per-tier costs or tokens.
- No auto-escalation based on message length or classifier — escalation must be LLM-initiated via the tool.
- No URL leakage regression in Iris replies (persona contract from plan 4 stays).

## Verification Strategy
- TDD via Go unit + integration tests.
- Agent-executed QA scenarios on each task.
- Evidence under `.sisyphus/evidence/stream-*`.

## Execution Strategy
### Sequential Waves
- T1 — Fix non-tool-call path to honor tier choice (quickest win, unblocks streaming).
- T2 — Typing fix + error logging + lifecycle spanning full pipeline.
- T3 — SSE streaming in `Client.ChatStream` (no tools).
- T4 — Tool-calling streaming in `Client.ChatWithToolsStream` with delta buffering.
- T5 — Orchestrator integration: streaming send + rate limiter + paragraph flush.
- T6 — `escalate_to_strong_model` tool + persona v1.3.0.
- Final Wave — four oracle reviewers.

## TODOs

- [x] 1. Route tier-chosen model through every chat path

  **What to do**:
  - Add `ChatWithModelStream` method on `*llm.Client` later in T3; for now add `Chat` alias that accepts an explicit model and have a new unexported helper share the body between `Chat`, `ChatWithModel`, and `ChatWithTools`.
  - In orchestrator `handle()` where it currently does `cfg.LLM.Chat(ctx, event.GuildID, messages)` in the non-tools path, switch to `cfg.LLM.ChatWithModel(ctx, modelToUse, event.GuildID, messages)` so the tier decision is honored regardless of whether tools fired.
  - Config wiring: ensure `LLMModelDefault=kr/claude-haiku-4.5`. Read current default in `.env` (likely still sonnet/opus) and change production `.env` accordingly.
  - Default `LLM_MODEL` (used by `chatCfg.Model` fallback) stays as is; it's only used if tier routing fails.
  - Tests:
    - Orchestrator: `TestHandle_NonToolsPath_UsesTierModel` verifies the fake `LLMCaller` receives `ChatWithModel` with the tier's model for a casual query. (Test uses a fake TierRouter returning haiku.)
    - No regression on existing `TestHandle_*` tests.

  **Must NOT do**: Do NOT alter `Chat` / `ChatWithModel` signatures. Do NOT change classifier call sites.

  **Agent**: `quick`, `[]`.

  **Parallel**: NO | Blocks: [2-6] | Blocked By: []

  **Acceptance**:
  - [ ] Casual greeting log shows `model=kr/claude-haiku-4.5` in `llm_request`.
  - [ ] `go test ./internal/orchestrator -run 'TierModel' -count=1 -v` passes.

  **QA Scenarios**:
  ```
  Scenario: Tier model reaches Chat call
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'TestHandle_NonToolsPath_UsesTierModel' -count=1 -v
    Expected: PASS; fake LLM records model=haiku.
    Evidence: .sisyphus/evidence/stream-1-tier-honored.txt
  ```

- [x] 2. Typing indicator: log errors + span full pipeline

  **What to do**:
  - `internal/discord/gateway.go`: the `SendTyping` call currently ignores `session.ChannelTyping` errors. Log `warn` on error with guild/channel IDs. No behavior change beyond logging.
  - `internal/orchestrator/orchestrator.go`: the `typingStop` defer triggers before streaming chunks finish because `handle()` returns while goroutines run. Refactor so the typing goroutine is kept alive until the LAST Discord send completes (including rate-limit waits). Use a shared `sync.WaitGroup` on the send path OR have the streaming sender invoke `typingStop` itself when it's truly done.
  - Ensure typing also fires BEFORE the tier classifier runs (it's the first visible feedback to the user). Move immediate `SendTyping` to the very top of `handle` after `Decide` returns Respond — before cross-channel classify, before relevance gate, before context fetch.
  - Test that fake typing recorder sees at least N calls spaced ~5 s apart for a long-running fake LLM.

  **Must NOT do**: No new dependencies. No visible behavior when `Decide=Ignore`.

  **Agent**: `quick`, `[]`.

  **Parallel**: NO | Blocks: [3-6] | Blocked By: [1]

  **Acceptance**:
  - [ ] Typing log line `typing_started` appears once per accepted event AND at least one `typing_refresh` for responses lasting > 5 s.
  - [ ] `SendTyping` error surfaces as a warn log instead of silent drop.
  - [ ] Existing orchestrator tests still pass.

  **QA Scenarios**:
  ```
  Scenario: Typing refreshes across long LLM call
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'TestTyping.*SpansPipeline' -count=1 -v
    Expected: typingRecorder observes >= 2 calls when fake LLM takes 7 s.
    Evidence: .sisyphus/evidence/stream-2-typing.txt

  Scenario: Live typing visible
    Tool: Bash
    Steps: docker compose build bot && docker compose up -d bot && operator sends a mention that triggers a long reply. Check Discord for the typing dots through the entire wait.
    Expected: Typing dots visible until Iris finishes.
    Evidence: .sisyphus/evidence/stream-2-live.txt
  ```

- [x] 3. SSE streaming for `Client.ChatStream` (no tools)

  **What to do**:
  - New public method on `*llm.Client`:
    ```go
    type StreamCallbacks struct {
        OnDelta    func(text string)
        OnDone     func()
        OnError    func(err error)
    }

    func (c *Client) ChatStream(ctx context.Context, model string, guildID int64, messages []map[string]string, cb StreamCallbacks) (finalText string, err error)
    ```
  - Hits `/v1/chat/completions` with `stream: true`. Parse SSE: lines are `data: {json}` terminated by `\n\n`; the stream ends with `data: [DONE]`.
  - For each line:
    - Unmarshal into `{"choices":[{"delta":{"content":"..."}}]}`.
    - Append delta to accumulated text and call `cb.OnDelta(text)` with the incremental text fragment.
  - On stream end (`[DONE]`) call `cb.OnDone()` and return the accumulated text.
  - On network error / non-200 / malformed data call `cb.OnError(err)` and return that error.
  - Preserve retries: wrap the HTTP call so a stream that fails before first byte can retry (up to `MaxRetries`).
  - Unit tests with httptest.Server emitting SSE:
    - `TestChatStream_Emits_DeltasInOrder`.
    - `TestChatStream_HandlesDone`.
    - `TestChatStream_NetworkError_CallsOnError`.
    - `TestChatStream_CtxCancellation_StopsStream`.
    - `TestChatStream_RetriesOnFirstByteFailure`.

  **Must NOT do**: Do not touch `Chat` / `ChatWithModel`. Streaming is additive.

  **Agent**: `unspecified-high`, `[]`.

  **Parallel**: NO | Blocks: [4-6] | Blocked By: [2]

  **Acceptance**:
  - [ ] Streaming client passes unit tests.
  - [ ] No regressions on existing LLM tests.

  **QA Scenarios**:
  ```
  Scenario: SSE parse roundtrip
    Tool: Bash
    Steps: go test ./internal/llm -run 'ChatStream' -count=1 -v
    Expected: all 5 new tests PASS.
    Evidence: .sisyphus/evidence/stream-3-sse.txt
  ```

- [x] 4. Tool-calling streaming `Client.ChatWithToolsStream`

  **What to do**:
  - New method on `*llm.Client`:
    ```go
    type ChatWithToolsStreamConfig struct {
        Model         string
        GuildID       int64
        Tools         []map[string]interface{}
        Exec          ToolExecutor
        Max           int
        OnDelta       func(text string)
    }
    func (c *Client) ChatWithToolsStream(ctx context.Context, messages []map[string]string, cfg ChatWithToolsStreamConfig) (finalText string, err error)
    ```
  - Behavior per round:
    - POST with `stream:true`, `tools`, `tool_choice:"auto"`.
    - Parse SSE. Delta fragments may include `delta.content` (call OnDelta) OR `delta.tool_calls[i].function.arguments` (accumulate in map keyed by `i`).
    - When a delta includes `finish_reason:"tool_calls"` or stream ends while tool calls are present, execute each complete tool call via `cfg.Exec`, append the usual assistant+tool turns, loop.
    - When the stream ends with `finish_reason:"stop"` return accumulated text.
    - Max rounds cap 3; on exceed, return whatever text produced so far.
  - Tests with httptest.Server:
    - `TestChatWithToolsStream_NoToolCalls_StreamsText`.
    - `TestChatWithToolsStream_OneToolCall_ThenStreamsText` — first response streams `delta.tool_calls` with fragmented JSON args, then returns `finish_reason:"tool_calls"`; ToolExecutor fake records invocation; second response streams final text.
    - `TestChatWithToolsStream_MaxRounds_StopsGracefully`.
    - `TestChatWithToolsStream_FragmentedArguments_AssembledBeforeExec` — args arrive over 3 fragments; ToolExecutor receives unified JSON.

  **Must NOT do**: Do not call from classifier paths.

  **Agent**: `unspecified-high`, `[]`.

  **Parallel**: NO | Blocks: [5,6] | Blocked By: [3]

  **Acceptance**:
  - [ ] New tests pass.
  - [ ] No regressions.

  **QA Scenarios**:
  ```
  Scenario: Fragmented tool args reassembled
    Tool: Bash
    Steps: go test ./internal/llm -run 'ChatWithToolsStream_FragmentedArguments' -count=1 -v
    Expected: PASS.
    Evidence: .sisyphus/evidence/stream-4-tools-stream.txt
  ```

- [x] 5. Orchestrator integrates streaming + rate-limited paragraph sender

  **What to do**:
  - New orchestrator component `StreamingSender`:
    - Buffers incoming text deltas.
    - Emits a Discord message when buffer contains `\n\n` AND current buffer length >= 200 chars, OR buffer length >= 1600 chars (soft paragraph threshold), OR stream ends.
    - Always keeps chunks under 2000 chars; if a single paragraph exceeds 2000, fall through to existing splitter.
    - Per-channel token bucket: 5 sends / 5 s, simple in-memory map `guildchannel → bucket`. Sends wait for a free token (max wait 10 s; if exceeded, drop with warn).
    - Sends are serialized per channel via a lock so ordering is preserved.
  - Integrate into orchestrator `handle()`:
    - When `IRIS_STREAMING=true`, call `ChatStream` (or `ChatWithToolsStream` when tools are wired) with an `OnDelta` that routes into `StreamingSender.Push(text)`.
    - After stream finishes, call `StreamingSender.Flush()` to emit remaining buffer.
    - After `Flush`, stop typing and proceed to lock refresh + memory promoter.
    - When `IRIS_STREAMING=false`, keep the existing non-streaming path.
  - Config: `Config.Streaming bool` + env `IRIS_STREAMING` default true.
  - Persist Iris's final reply exactly once (current behavior); capture uses the full text from ChatStream's return value.
  - Tests:
    - `TestStreamingSender_EmitsOnParagraphBoundary`.
    - `TestStreamingSender_FlushesRemainder`.
    - `TestStreamingSender_RespectsRateLimit` — push 10 paragraphs quickly; record timestamps; verify no burst > 5 within any 5 s window.
    - `TestStreamingSender_HandlesLongParagraphSplits`.
    - Orchestrator integration: `TestHandle_StreamingPath_SendsPerParagraph`.

  **Must NOT do**: Don't break non-streaming fallback. Don't double-persist bot replies. Don't block main goroutine on rate limiter waits > 10 s.

  **Agent**: `unspecified-high`, `[]`.

  **Parallel**: NO | Blocks: [6] | Blocked By: [4]

  **Acceptance**:
  - [ ] Long response arrives as multiple Discord messages at paragraph boundaries.
  - [ ] No more than 5 sends / 5 s per channel (measured by test).
  - [ ] `go test ./internal/orchestrator -count=1 -v` passes.

  **QA Scenarios**:
  ```
  Scenario: Rate limiter enforces 5/5s per channel
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'StreamingSender_RespectsRateLimit' -count=1 -v
    Expected: PASS with measured timing proving no burst exceeds limit.
    Evidence: .sisyphus/evidence/stream-5-rate-limit.txt

  Scenario: Paragraph flush
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'StreamingSender_EmitsOnParagraphBoundary' -count=1 -v
    Expected: PASS; sender fires exactly one Send per paragraph boundary.
    Evidence: .sisyphus/evidence/stream-5-paragraph.txt
  ```

- [x] 6. `escalate_to_strong_model` tool + persona v1.3.0

  **What to do**:
  - New internal-only tool in `internal/tools/escalate/escalate.go`:
    - Schema: `{"type":"function","function":{"name":"escalate_to_strong_model","description":"Call when the current model cannot answer reliably. Triggers a re-run with a stronger model.","parameters":{"type":"object","properties":{"reason":{"type":"string","description":"Short reason why a stronger model is needed."}},"required":["reason"]}}}`.
    - `Run(ctx, args)` returns a marker string `"ESCALATED:"+reason+""` and sets a context-propagated sentinel so the orchestrator can detect it.
  - Register the tool in `cmd/iris-bot/main.go` alongside websearch.
  - In orchestrator's streaming tool-call loop, when `ToolExecutor` reports the tool name was `escalate_to_strong_model`:
    - Short-circuit the current round.
    - Start a NEW streaming call with `cfg.LLM_MODEL_STRONG`, the same `messages` (minus the failed tool-call round-trip artifacts; keep system + user + cross-channel/prior context), fresh tool set (include escalate again so it doesn't loop infinitely — cap max escalations per request at 1 via a context-scoped flag).
    - Stream that to the user as usual.
    - Log `llm_escalated reason=<reason> model=<strong>`.
  - Persona `internal/persona/persona.go`:
    - Add rule near top of lorePolicy/immutablePersona:
      - "Kamu default-nya pakai model haiku. Kalau pertanyaan user butuh analisis mendalam, reasoning multi-langkah, atau referensi lore yang rumit, panggil tool `escalate_to_strong_model` dengan alasan singkat SEBELUM jawab."
    - Bump `version` to `"1.3.0"`.
    - Keep no-URL-leakage rule and websearch rule.
  - Tests:
    - `internal/tools/escalate/escalate_test.go`: schema shape, Run returns marker, validation errors on missing reason.
    - Orchestrator: `TestHandle_EscalateToolCall_ReRunsWithStrongModel` with fake ToolsLLM that first returns a tool_call for escalate, then a plain text on the second call; assert second ChatStream invocation received `Model=LLM_MODEL_STRONG`.
    - Persona: `TestImmutablePersona_MentionsEscalateTool`, `TestPersonaVersion_1_3_0`.

  **Must NOT do**: Don't allow escalation to loop (strong→strong). Don't escalate inside classifiers. Don't expose escalate reasons in user-facing reply.

  **Agent**: `unspecified-high`, `[]`.

  **Parallel**: NO | Blocks: [Final] | Blocked By: [5]

  **Acceptance**:
  - [ ] Persona tests v1.3.0 pass.
  - [ ] Live reply to "jelasin teori waktu kanon di WuWa 3.1 secara detail" triggers `llm_escalated` log + second `llm_request model=kr/claude-opus-4.7`.
  - [ ] Simple greeting does NOT trigger escalate.

  **QA Scenarios**:
  ```
  Scenario: Escalate tool triggers re-run with strong model
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'EscalateToolCall_ReRunsWithStrongModel' -count=1 -v
    Expected: PASS; second ChatStream received Model=opus.
    Evidence: .sisyphus/evidence/stream-6-escalate.txt

  Scenario: Persona v1.3.0
    Tool: Bash
    Steps: go test ./internal/persona -count=1 -v
    Expected: PASS; assertions for escalate tool and version=1.3.0 match.
    Evidence: .sisyphus/evidence/stream-6-persona.txt
  ```

## Final Verification Wave
- [x] F1. Plan Compliance Audit — oracle
- [x] F2. Code Quality Review — oracle
- [x] F3. Real Manual QA — oracle
- [x] F4. Scope Fidelity Check — oracle

## Commit Strategy
- Do not commit unless explicitly asked.

## Success Criteria
- Simple messages route to haiku (cheap + fast).
- Complex messages escalate via tool call to opus.
- Typing indicator visible through the whole reply, including streaming and tool-call rounds.
- Iris streams her reply in paragraphs, no more than 5 messages / 5 s per channel.
- No regressions in classifiers (they stay on plain `ChatWithModel`).
