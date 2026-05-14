# Iris Sliding Conversation Lock

## TL;DR
> **Summary**: Add a 5-minute sliding conversation lock per channel so Iris replies to in-context follow-up messages without a mention or reply.
> **Deliverables**:
> - DB-backed `channel_conversations` table tracking last bot reply per (guild, channel).
> - Reset-on-send: Iris's own reply refreshes the lock's expiry.
> - Router extension: inside an active window, elevate non-trigger messages to a new `ReasonActiveConversation` trigger, pending relevance check.
> - LLM-classified relevance gate scoped to rolling channel context.
> - Nil-safe fallback: missing store or classifier leaves current behavior unchanged.
> **Effort**: Medium
> **Parallel**: YES — 2 waves
> **Critical Path**: T1 → T2 → T3 → T4 → T5

## Context
### Original Request
- Iris must stay in context after she sends a message.
- If a user then mentions or replies to Iris, or the overall text is in context of what Iris was saying, Iris should respond.
- Maintain at least a 5-minute conversation timeout per channel that resets every time Iris sends a message.
- Inside that sliding window, Iris responds even without an @mention or reply, if context still matches.

### Interview Summary
- Relevance check inside the window uses the LLM classifier (fast model).
- Lock is channel-wide: any user in the channel counts while the window is active.
- Classifier failure or timeout means no elevated response (safe fallback).

### Built-on
- Plan `discord-audit-context.md` landed: router include-list with fallback, context builder, cross-channel classifier, memory promoter, LLM audit, immediate typing, rolling `channel_messages` (newest 20).
- Existing router reasons: `ReasonMention`, `ReasonReply`, `ReasonNameMention`, `ReasonBotMessage`, `ReasonExceptionChannel`, `ReasonChannelNotAllowed`.
- Existing orchestrator pipeline runs capture → decide → typing → context build → cross-channel classifier → LLM → send → memory promotion.

## Work Objectives
### Core Objective
Give Iris a conversational presence in channels she has recently spoken in, without creating a broadcast bot.

### Deliverables
- `channel_conversations` table: `(guild_id, channel_id)` unique, `last_bot_reply_at`, `lock_until`, `updated_at`.
- `ChannelConversationRepo` with methods:
  - `Refresh(ctx, guildID, channelID int64, now time.Time, ttl time.Duration) error`
  - `Active(ctx, guildID, channelID int64, now time.Time) (bool, error)`
  - `Clear(ctx, guildID, channelID int64) error`
- Router extension: new `ReasonActiveConversation`. When a non-trigger message arrives in an allowed/fallback channel and the conversation is active, router returns `Respond(ReasonActiveConversation)` gated behind a deferred relevance check signal.
- Relevance classifier call inside orchestrator (reuse `CrossChannelLLM`/fast tier) with strict JSON `{"in_context": bool, "reason": string}`. On failure → no respond.
- Orchestrator: on successful response send, refresh the lock before queue returns.
- Nil-safe behavior: when any new component is nil, revert to pre-lock router semantics.

### Definition of Done (verifiable)
- `go test ./... -count=1` passes.
- `go build ./...` passes.
- `go vet ./...` passes.
- Router test proves `ReasonActiveConversation` appears only inside an active window and only when the new lock store reports `Active=true`.
- Orchestrator test proves the lock refreshes after a fake send completes.
- Relevance gate test proves LLM classifier rejection suppresses the in-window response.
- Evidence files saved per task under `.sisyphus/evidence/conv-lock-*`.

### Must Have
- TDD for all new behavior.
- 5-minute default TTL, configurable via `IRIS_CONV_LOCK_TTL` env in `internal/config/config.go`.
- Classifier uses `llm.WithMeta(ctx, TriggerReason="conversation_lock_relevance")` for Task 3 DEBUG audit coverage.
- Preserve allow-list/exception-list semantics: lock never overrides routing gates.
- Lock is skipped for bot messages and for the bot's own messages.

### Must NOT Have
- No response storm: at most one Iris reply per user message even with the lock.
- No cross-channel expansion via this lock (handled by existing cross-channel classifier).
- No change to existing mention/reply/name triggers.
- No global-state channel lock; must be per `(guild_id, channel_id)`.
- No log spam: DEBUG-gated audit lines.

## Verification Strategy
- TDD: Go unit/integration tests.
- Agent-executed QA scenarios for every task.
- Evidence under `.sisyphus/evidence/conv-lock-{N}-*`.

## Execution Strategy
### Parallel Execution Waves
- Wave 1: T1 (schema+repo). Solo — blocks everything.
- Wave 2: T2 (router), T3 (relevance classifier) in parallel after T1.
- Wave 3: T4 (orchestrator integration + refresh on send). After T2 and T3.
- Wave 4: T5 (runtime wiring + config + docs). After T4.

### Dependency Matrix
- T1 blocks T2, T3, T4.
- T2 blocks T4.
- T3 blocks T4.
- T4 blocks T5.
- T5 blocks Final Verification.

## TODOs

- [x] 1. Add `channel_conversations` table, domain type, and repository

  **What to do**: Create SQL migration `migrations/003_channel_conversations.sql` for table `channel_conversations` with `guild_id`, `channel_id`, `last_bot_reply_at`, `lock_until`, `updated_at`, and UNIQUE(guild_id, channel_id). Add domain type `ChannelConversation`. Add `ChannelConversationRepo` mirroring the style of `internal/repository/audit_exception.go` and `channel_context.go`. Add `ChannelConversationQuerier` interface in `internal/repository/repository.go`. Register table in `internal/repository/testhelper.go` truncate list. TDD: failing tests first.

  **Must NOT do**: Do not alter existing tables. Do not add vector/embedding columns. Do not cascade-delete `channel_messages` when a conversation row is removed. Do not touch orchestrator/router here.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` — schema + repo + domain.
  - Skills: [] — None match.

  **Parallelization**: Can Parallel: NO | Wave 1 | Blocks: [2,3,4] | Blocked By: []

  **References**:
  - Pattern: `internal/repository/channel_context.go` — upsert style and transaction pattern.
  - Pattern: `internal/repository/audit_exception.go` — simple (guild, channel) row pattern.
  - Pattern: `internal/repository/repository_test.go` — integration test pattern with `setupTestDB`.
  - Pattern: `migrations/002_channel_context.sql` — schema style for new tables.

  **Acceptance Criteria**:
  - [ ] `Refresh` sets `last_bot_reply_at=now`, `lock_until=now+ttl`, upserts the row.
  - [ ] `Active(now)` returns true iff `lock_until > now`.
  - [ ] `Clear` removes the row for the (guild, channel).
  - [ ] `go test ./internal/repository -run 'ChannelConversation' -count=1 -v` passes.

  **QA Scenarios**:
  ```
  Scenario: Refresh creates then extends the lock
    Tool: Bash
    Steps: go test ./internal/repository -run 'ChannelConversation.*Refresh' -count=1 -v
    Expected: Initial Refresh sets lock; second Refresh at now+1s extends lock_until forward.
    Evidence: .sisyphus/evidence/conv-lock-1-refresh.txt

  Scenario: Active flips off after TTL passes
    Tool: Bash
    Steps: go test ./internal/repository -run 'ChannelConversation.*Active' -count=1 -v
    Expected: Active(now) true during TTL; Active(now+ttl+1s) false.
    Evidence: .sisyphus/evidence/conv-lock-1-expiry.txt
  ```

  **Commit**: NO

- [x] 2. Extend router with `ReasonActiveConversation`

  **What to do**: Add `ReasonActiveConversation = "active_conversation"` to `internal/router/types.go`. Extend `TriggerRouter` to accept a `ChannelConversationQuerier` (nil-safe). Add constructor `NewTriggerRouterWithConversation(exceptionRepo, allowedRepo, convRepo, botID)` (or extend existing one with an optional field). When the message has no existing trigger (not mention/reply/name) AND channel is allowed (or fallback does not exclude it) AND `convRepo.Active(now)` returns true → return `Respond(ReasonActiveConversation)`. Keep existing routing priorities. Preserve bot-ignore. TDD: failing tests first.

  **Must NOT do**: Do not bypass exception/allow-list gates. Do not treat bot's own messages as valid conversation continuation. Do not introduce LLM calls here; relevance check is in orchestrator.

  **Recommended Agent Profile**:
  - Category: `quick` — pure router logic change.
  - Skills: [] — None.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: [4] | Blocked By: [1]

  **References**:
  - Pattern: `internal/router/router.go:29-53` — current decision tree.
  - Pattern: `internal/router/router_test.go` — Mock repo style.
  - API/Type: `internal/repository/repository.go` — `ChannelConversationQuerier` (added in T1).

  **Acceptance Criteria**:
  - [ ] Active window + non-trigger message in allowed channel → `Respond(ReasonActiveConversation)`.
  - [ ] Active window but channel not allowed (include-list mode) → `Ignore(ReasonChannelNotAllowed)`.
  - [ ] Active window but bot message → `Ignore(ReasonBotMessage)`.
  - [ ] Active window + mention → stays `ReasonMention` (existing precedence wins).
  - [ ] Inactive window + non-trigger → `Ignore(ReasonNoTrigger)`.
  - [ ] Nil conv repo → identical behavior to current router.

  **QA Scenarios**:
  ```
  Scenario: Active window elevates casual message
    Tool: Bash
    Steps: go test ./internal/router -run 'ActiveConversation' -count=1 -v
    Expected: Mock active=true produces Respond(ReasonActiveConversation); active=false falls through to Ignore(ReasonNoTrigger).
    Evidence: .sisyphus/evidence/conv-lock-2-router-active.txt

  Scenario: Channel guard still wins
    Tool: Bash
    Steps: go test ./internal/router -run 'ActiveConversation.*NotAllowed' -count=1 -v
    Expected: Include-list mode with channel not allowed returns ReasonChannelNotAllowed even when active.
    Evidence: .sisyphus/evidence/conv-lock-2-router-guard.txt
  ```

  **Commit**: NO

- [x] 3. Add in-window relevance classifier

  **What to do**: Add `InWindowRelevance` classifier in `internal/orchestrator/conversation_relevance.go`, reusing `CrossChannelLLM` + `Model` pattern from `cross_channel.go`. Signature: `IsRelevant(ctx, event, context []*domain.ChannelMessage) (bool, error)`. Builds a strict-JSON prompt asking the fast model: `{"in_context": bool, "reason": string}`. Default timeout 4s, overridable via config. Must attach `llm.WithMeta(ctx, TriggerReason="conversation_lock_relevance")`. Fallback rules: classifier error, timeout, parse failure → return `(false, err)`. TDD: failing tests first.

  **Must NOT do**: Do not call memory service. Do not modify context builder output. Do not log raw content at info level.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` — bounded classifier with fallback.
  - Skills: [] — None.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: [4] | Blocked By: [1]

  **References**:
  - Pattern: `internal/orchestrator/cross_channel.go` — classifier prompt + JSON parse + timeout pattern.
  - Pattern: `internal/orchestrator/cross_channel_test.go` — fake LLM test pattern.
  - API/Type: `internal/llm/audit.go` — `llm.ContextMeta` fields.

  **Acceptance Criteria**:
  - [ ] `in_context=true` returns `(true, nil)`.
  - [ ] `in_context=false` returns `(false, nil)`.
  - [ ] Parse/LLM/timeout errors return `(false, err)` and never panic.
  - [ ] Empty context returns `(false, nil)` without calling LLM.
  - [ ] Emits `TriggerReason="conversation_lock_relevance"` via `llm.WithMeta`.

  **QA Scenarios**:
  ```
  Scenario: Relevant follow-up approved
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'InWindowRelevance.*True' -count=1 -v
    Expected: Fake LLM returns {"in_context":true}. Classifier returns (true, nil).
    Evidence: .sisyphus/evidence/conv-lock-3-relevance-true.txt

  Scenario: Irrelevant follow-up rejected safely
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'InWindowRelevance.*(False|Parse|Timeout)' -count=1 -v
    Expected: (false, nil) for explicit false; (false, err) for parse/timeout; no panic.
    Evidence: .sisyphus/evidence/conv-lock-3-relevance-false.txt
  ```

  **Commit**: NO

- [x] 4. Integrate lock + relevance gate into orchestrator

  **What to do**: Extend `orchestrator.Config` with optional `ConversationRefresher`, `ConversationActive`, and `InWindowRelevance` fields (or a single `Conversation` port wrapping all three) plus `ConversationLockTTL time.Duration` with default 5m. In `handle(j)`:
    1. If router decision is `ReasonActiveConversation`, run relevance gate against current rolling context. If `false`, log debug and return without responding.
    2. After successful `SendMessage` of the final chunk, call `ConversationRefresher.Refresh(ctx, guildID, channelID, now, ttl)` before returning. Log non-fatal errors at warn.
  Update existing tests. Add a new test covering: active window → relevance true → LLM called → refresh called. Another test: active window → relevance false → no LLM, no send, no refresh. TDD: failing tests first.

  **Must NOT do**: Do not refresh the lock on failed sends. Do not reintroduce mention/reply triggers here. Do not block response pipeline on refresh failure.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` — multi-file orchestrator integration.
  - Skills: [] — None.

  **Parallelization**: Can Parallel: NO | Wave 3 | Blocks: [5] | Blocked By: [2,3]

  **References**:
  - Pattern: `internal/orchestrator/orchestrator.go:207-290` — handle worker body.
  - Pattern: `internal/orchestrator/cross_channel.go` integration hook.
  - Pattern: `internal/orchestrator/memory_promotion.go` detached async pattern (for refresh error logging).

  **Acceptance Criteria**:
  - [ ] In-window relevance=true → LLM called, response sent, lock refreshed once.
  - [ ] In-window relevance=false → no LLM call, no send, no refresh.
  - [ ] Nil refresher/gate fields → behave like previous Task 1-8 orchestrator.
  - [ ] Mention still wins: ReasonMention responses still refresh the lock.
  - [ ] Send error does NOT refresh lock.

  **QA Scenarios**:
  ```
  Scenario: Active in-window message answered and lock refreshed
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'ActiveConversationHappyPath' -count=1 -v
    Expected: Fake sender observes one response; fake refresher recorded one call.
    Evidence: .sisyphus/evidence/conv-lock-4-happy.txt

  Scenario: Irrelevant in-window message ignored
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'ActiveConversationRelevanceFalse' -count=1 -v
    Expected: Sender received nothing; refresher not called; LLM chat not invoked.
    Evidence: .sisyphus/evidence/conv-lock-4-suppressed.txt
  ```

  **Commit**: NO

- [x] 5. Runtime wiring, config, and docs

  **What to do**: In `cmd/iris-bot/main.go` construct `ChannelConversationRepo`, the relevance classifier (using the existing `chatClient` + router-tier model), and inject them into `orchCfg`. Add `IRIS_CONV_LOCK_TTL` config parsing (default 5 minutes) in `internal/config/config.go` + `config_test.go`. Update `.env.example`. Extend `scripts/regression.sh` if needed (no new network). Append `## CONVERSATION LOCK` learnings to notepad. TDD: failing tests first for config parse + wire adapter construction.

  **Must NOT do**: Do not touch prior plan files. Do not remove existing audit/Task 6 wiring. Do not add a new top-level package.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` — wiring + config + docs.
  - Skills: [] — None.

  **Parallelization**: Can Parallel: NO | Wave 4 | Blocks: [Final] | Blocked By: [4]

  **References**:
  - Pattern: `cmd/iris-bot/main.go:150-269` — existing orch wiring block.
  - Pattern: `internal/app/wire/adapters.go` — adapter style.
  - Pattern: `internal/config/config.go` — env parsing; mirror `LLMModelRouter` block.

  **Acceptance Criteria**:
  - [ ] `IRIS_CONV_LOCK_TTL` parses `Xm`/`Xs` strings; invalid value → 5m default; empty → 5m default.
  - [ ] `orch.Start()` receives non-nil conv components when DB is available; nil otherwise.
  - [ ] End-to-end fake test showing Iris replies without mention during active window, and stops after TTL.
  - [ ] `go test ./... -count=1`, `go build ./...`, `go vet ./...` pass.

  **QA Scenarios**:
  ```
  Scenario: End-to-end sliding window
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'E2E.*Conversation' -count=1 -v
    Expected: First message mentions Iris → reply + lock. Second message (no mention, relevant) → reply + lock refresh. Third message after TTL → ignored.
    Evidence: .sisyphus/evidence/conv-lock-5-e2e.txt

  Scenario: Config default + override
    Tool: Bash
    Steps: go test ./internal/config -run 'ConvLockTTL' -count=1 -v
    Expected: Default 5m; env `2m` parsed; env `garbage` falls back to default.
    Evidence: .sisyphus/evidence/conv-lock-5-config.txt
  ```

  **Commit**: NO

## Final Verification Wave
- [x] F1. Plan Compliance Audit — oracle
- [x] F2. Code Quality Review — oracle
- [x] F3. Real Manual QA — oracle
- [x] F4. Scope Fidelity Check — oracle

## Commit Strategy
- Do not commit unless the user explicitly requests it.

## Success Criteria
- After Iris answers, any in-channel message within 5 minutes that the LLM classifier marks relevant triggers a reply.
- Send refreshes the window; irrelevant or post-timeout messages are ignored.
- Existing mention/reply/name triggers still work unchanged.
- Allow-list, exception list, bot filter, and safety rules still gate responses.
- Full Go test/build/vet suite passes.
