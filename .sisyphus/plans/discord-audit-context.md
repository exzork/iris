# Discord Audit Logging, Channel Context, and Allow-list Routing

## TL;DR
> **Summary**: Add auditable DEBUG-gated LLM telemetry, immediate Discord typing, allow-list channel routing, and database-backed channel context/session memory for richer I.R.I.S replies.
> **Deliverables**:
> - DEBUG-only LLM request/duration audit logs with redacted/truncated content.
> - Immediate typing indicator from accepted trigger through the full LLM pipeline.
> - Include-list channel routing with safe migration fallback from current exclusion behavior.
> - DB-backed rolling per-channel last-20-message map and context builder that injects at least 10 relevant messages.
> - Reply-chain and LLM-classified cross-channel relevance support.
> - Async long-term memory promotion from rolling context.
> **Effort**: Large
> **Parallel**: YES - 4 waves
> **Critical Path**: Task 1 → Task 2 → Tasks 3/4/5 → Task 6 → Task 7 → Task 8

## Context
### Original Request
- Add logs for every LLM request and duration when `DEBUG=true` for audit.
- When Iris recognizes a message for her, immediately set Discord typing until LLM finishes.
- Handle Discord message length limits carefully.
- Change channel routing from exclude-list channel to include-list channel.
- Give LLM context about mentions, replies, and casual Iris chats by including at least 10 recent Discord messages, following relevant messages and replies.
- Each channel has its own session unless topics are relevant across discussions.
- Map each channel to at least the last 20 messages, replacing old with new; Iris may still save important content into memory before it ages out.

### Interview Summary
- Allow-list migration: fallback to current non-excluded behavior until the first allowed-channel row exists, then enforce include-list.
- DEBUG logs: include truncated content, not full raw prompts. Use 512 characters per content field, redacted, head-only with `…` marker.
- Rolling channel map/session storage: database-backed.
- Cross-channel relevance: LLM classifier.
- Test strategy: TDD.

### Metis Review (gaps addressed)
- Added explicit defaults for truncation/redaction, context budgets, reply depth, cross-channel candidate scope, memory promotion timing, allow-list hot-flip behavior, and typing refresh through long LLM calls.
- Guardrail: do not replace existing Discord message splitter; add tests and harden only if tests reveal gaps.
- Guardrail: avoid global cross-channel prompt scans; classify only bounded candidate sessions.

## Work Objectives
### Core Objective
Make I.R.I.S auditable and context-aware while preserving current bot behavior, persona, model routing, and Discord safety guarantees.

### Deliverables
- `DEBUG=true` LLM audit logs with request ID/correlation ID, guild/channel/message IDs, trigger reason, model, tier, duration, status, retry/error class, prompt/message counts, response size, and redacted 512-char content snippets.
- Immediate typing indicator once `TriggerRouter.Decide` returns respond, refreshed until classifier + context build + LLM chat + response send completes or context is canceled.
- Channel allow-list repository, schema, router behavior, admin/settings wiring, and fallback compatibility with `exception_channels` until allow-list rows exist.
- DB-backed rolling channel message repository storing last 20 messages per guild/channel with author, reply reference, timestamps, and content snippets/full content as appropriate.
- Context builder that supplies system prompt + current user + at least 10 relevant recent Discord messages, following reply chain up to depth 3 and classifying bounded cross-channel candidates.
- Async memory promotion classifier/path that can persist important rolling-map content into existing long-term memory before pruning.

### Definition of Done (verifiable conditions with commands)
- `go test ./internal/router ./internal/orchestrator ./internal/llm ./internal/obs ./internal/repository ./internal/app -count=1` passes.
- `go test ./... -count=1` passes.
- `go build ./...` passes.
- With `DEBUG=true`, a test/fake LLM call emits one audit log containing duration and truncated redacted content; with `DEBUG=false`, no content snippets are logged.
- A Discord trigger accepted by router calls `SendTyping` immediately before LLM completion in tests.
- Router tests prove fallback mode before allow-list rows and include-only enforcement after the first row.
- Context-builder tests prove at least 10 relevant messages are included when available and rolling storage prunes to 20 per channel.

### Must Have
- TDD for every behavior change.
- No raw secrets/tokens/API keys in audit logs; pass all existing redaction tests or add equivalent assertions.
- Discord messages split below 2000 characters; preserve markdown/code-block safety if current splitter supports it.
- Existing `eno/*` model exclusion and tier routing remain intact.
- Existing memory/persona/lore behavior remains intact unless explicitly connected through memory promotion.

### Must NOT Have
- No full prompt dumps in normal logs.
- No unbounded global cross-channel LLM classification.
- No behavior where adding one allow-list channel silently responds in all channels.
- No human-only verification steps.
- No source edits outside the task scope.

## Verification Strategy
> ZERO HUMAN INTERVENTION - all verification is agent-executed.
- Test decision: TDD with Go unit/integration tests.
- QA policy: Every task has agent-executed scenarios.
- Evidence: `.sisyphus/evidence/task-{N}-{slug}.{ext}`

## Execution Strategy
### Parallel Execution Waves
Wave 1: Task 1 foundation schema/types/contracts.
Wave 2: Tasks 2, 3, 4 can proceed in parallel after contracts exist.
Wave 3: Tasks 5, 6 integrate context assembly, LLM classification, logging, and memory promotion.
Wave 4: Tasks 7, 8 harden Discord behavior and end-to-end wiring.

### Dependency Matrix (full, all tasks)
- Task 1 blocks Tasks 2, 4, 5, 6.
- Task 2 blocks Task 8.
- Task 3 blocks Task 8.
- Task 4 blocks Tasks 5, 6, 8.
- Task 5 blocks Tasks 6, 8.
- Task 6 blocks Task 8.
- Task 7 blocks Task 8.
- Task 8 blocks final verification.

### Agent Dispatch Summary (wave → task count → categories)
- Wave 1 → 1 task → `unspecified-high`
- Wave 2 → 3 tasks → `quick`, `unspecified-high`, `unspecified-high`
- Wave 3 → 2 tasks → `unspecified-high`, `deep`
- Wave 4 → 2 tasks → `quick`, `unspecified-high`

## TODOs
> Implementation + Test = ONE task. Never separate.
> EVERY task MUST have: Agent Profile + Parallelization + QA Scenarios.

- [x] 1. Add channel context and allow-list data contracts

  **What to do**: First write failing repository/domain/config tests. Add domain types for Discord message metadata required by context: message ID, guild ID, channel ID, user ID, author display/name if available, content, attachments flag/count, reply-to message ID/channel ID, created/edited/deleted timestamps, and trigger/source labels. Add DB-backed repositories/tables for `allowed_channels` and rolling `channel_messages`/sessions. Repository behavior: upsert messages, keep only newest 20 per guild/channel, query recent channel messages, query bounded candidate sessions for a guild/user/channel, check whether any allow-list rows exist, and check whether channel is allowed. Keep existing `exception_channels` repo for fallback compatibility. Add migrations using the repository's existing schema/test pattern.

  **Must NOT do**: Do not delete `exception_channels` or break existing bootstrap/admin tests in this task. Do not add LLM calls here.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: schema, repositories, and domain contract changes touch multiple packages.
  - Skills: [] - No available specialized skill required.
  - Omitted: [`git-master`] - No commit requested.

  **Parallelization**: Can Parallel: NO | Wave 1 | Blocks: [2,4,5,6] | Blocked By: []

  **References**:
  - Pattern: `internal/repository/audit_exception.go` - repository style for channel-based rows.
  - Pattern: `internal/repository/repository_test.go` - DB integration test pattern.
  - Pattern: `migrations/001_init.sql` - current SQL migration/schema location.
  - API/Type: `internal/domain/types.go:89-108` - extend or complement current Discord message/event types.
  - Pattern: `internal/settings/keys_test.go` - settings/key validation style if new config keys are added.

  **Acceptance Criteria**:
  - [ ] Failing tests are written before implementation and pass after implementation.
  - [ ] Repository test proves newest 20 messages per guild/channel are retained and older rows are pruned/replaced.
  - [ ] Repository test proves allow-list empty state is distinguishable from channel-not-allowed state.
  - [ ] Existing `exception_channels` tests still pass unchanged.
  - [ ] `go test ./internal/repository ./internal/domain -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: Rolling channel map prunes to 20
    Tool: Bash
    Steps: Run `go test ./internal/repository -run 'Test.*Channel.*Message|Test.*Allowed.*Channel' -count=1 -v`.
    Expected: Test inserts 25 messages for one guild/channel and query returns exactly 20 newest in chronological/context order.
    Evidence: .sisyphus/evidence/task-1-channel-contracts.txt

  Scenario: Allow-list empty fallback state is explicit
    Tool: Bash
    Steps: Run repository allow-list test that checks `HasAllowedChannels=false`, then insert one row and check `HasAllowedChannels=true` plus per-channel allow result.
    Expected: Empty allow-list does not equal denied; first row flips state for router consumers.
    Evidence: .sisyphus/evidence/task-1-allowlist-state.txt
  ```

  **Commit**: NO | Message: `feat(discord): add channel context data contracts` | Files: [internal/domain, internal/repository]

- [x] 2. Replace exclusion routing with include-list fallback semantics

  **What to do**: Write failing router tests first. Update router contracts to accept an allow-list querier plus existing exception querier fallback. Behavior: if no `allowed_channels` rows exist for a guild, preserve current behavior by continuing to honor `exception_channels` exclusions; if at least one allowed-channel row exists, respond only in allowed channels and ignore all other channels, with `exception_channels` no longer consulted in include-list mode. Preserve bot-message ignore and current trigger reasons for `message_mention`, `message_reply`, and `message_content`. Rename/add reasons such as `ReasonChannelNotAllowed` while keeping old exception reason only for fallback path.

  **Must NOT do**: Do not require all guilds to configure channels before the first deploy. Do not remove existing exception commands unless admin replacements are also in scope for Task 8.

  **Recommended Agent Profile**:
  - Category: `quick` - Reason: focused router logic once contracts exist.
  - Skills: [] - No special skill available.
  - Omitted: [`playwright`] - No browser/UI involved.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: [8] | Blocked By: [1]

  **References**:
  - Pattern: `internal/router/router.go:29-53` - current routing decisions.
  - Test: `internal/router/*_test.go` if present, otherwise mirror `internal/app/app_test.go` fake dependencies.
  - API/Type: `internal/repository/repository.go` - repository interface convention.

  **Acceptance Criteria**:
  - [ ] Router tests prove empty allow-list uses exception-channel fallback.
  - [ ] Router tests prove first allowed-channel row flips to include-list enforcement.
  - [ ] Router tests prove non-trigger content remains ignored even in allowed channels.
  - [ ] `go test ./internal/router -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: Fallback before allow-list exists
    Tool: Bash
    Steps: Run router test with `HasAllowedChannels=false`, `IsException=false`, mention event.
    Expected: Decision responds with mention reason.
    Evidence: .sisyphus/evidence/task-2-router-fallback.txt

  Scenario: Include-list enforced after first row
    Tool: Bash
    Steps: Run router test with `HasAllowedChannels=true`, current channel not allowed, mention event.
    Expected: Decision ignores with channel-not-allowed reason.
    Evidence: .sisyphus/evidence/task-2-router-allowlist.txt
  ```

  **Commit**: NO | Message: `feat(router): enforce allowed channel routing` | Files: [internal/router, internal/repository]

- [x] 3. Add DEBUG-gated LLM audit logging with duration

  **What to do**: Write failing LLM/obs tests first. Add audit timing around every LLM chat/classifier request, including `Chat` and `ChatWithModel`. Logs must appear only when `DEBUG=true` or equivalent config debug flag is enabled. Include request/correlation ID, guild ID, channel/message IDs when present in context, model, model tier if known, message count, truncated/redacted content snippets, start/end/duration_ms, status, retry count/error class, and response length. Implement content truncation as 512 Unicode-safe characters head-only plus `…`; apply existing secret redactor before truncation or ensure secrets are removed in output. Propagate context metadata from orchestrator/context builder to LLM client without changing public behavior for existing callers.

  **Must NOT do**: Do not log full API keys, full prompts, full responses, or raw attachments. Do not emit content snippets when debug is false.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: crosses config, obs, LLM client, and app context propagation.
  - Skills: [] - No special skill available.
  - Omitted: [] - None.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: [8] | Blocked By: []

  **References**:
  - Pattern: `internal/llm/client.go:93-168` - LLM chat entry points.
  - Pattern: `internal/llm/router.go` - classifier uses `ChatWithModel` and must be audited.
  - Test: `internal/obs/logger_test.go`, `internal/obs/middleware_test.go` - JSON log and duration verification patterns.
  - Test: `internal/llm/client_test.go` - fake HTTP tests for LLM calls.

  **Acceptance Criteria**:
  - [ ] TDD tests fail before implementation and pass after.
  - [ ] `DEBUG=true` emits exactly one audit record per LLM HTTP request/classifier request.
  - [ ] `DEBUG=false` does not emit content snippets or duration audit records beyond existing normal logs.
  - [ ] Test proves secret-like text is redacted and content is capped at 512 characters.
  - [ ] `go test ./internal/llm ./internal/obs -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: Debug audit log includes duration
    Tool: Bash
    Steps: Run targeted LLM audit test with fake server delaying 25ms and debug enabled.
    Expected: Parsed JSON log has `duration_ms >= 25`, model, status=success, message_count, and truncated content.
    Evidence: .sisyphus/evidence/task-3-llm-audit-debug.txt

  Scenario: Debug disabled suppresses content snippets
    Tool: Bash
    Steps: Run same test with debug disabled and message containing `sk-test-secret`.
    Expected: No raw secret and no content snippet fields appear in logs.
    Evidence: .sisyphus/evidence/task-3-llm-audit-disabled.txt
  ```

  **Commit**: NO | Message: `feat(llm): add debug audit timing logs` | Files: [internal/llm, internal/obs, internal/config]

- [x] 4. Capture Discord messages into rolling channel sessions

  **What to do**: Write failing gateway/orchestrator/repository tests first. Ensure every relevant Discord message event observed by the bot is persisted/upserted into the rolling DB channel map before routing drops or responds, subject to bot-message filtering. Capture reply metadata from Discord payloads if available; if current gateway type lacks it, extend event mapping. Persist enough metadata for context: IDs, channel/guild/user, timestamp, content, reply reference, trigger classification, and deleted/edited markers if events exist. Prune to 20 newest per channel after upsert. Ensure this map is independent from long-term memory.

  **Must NOT do**: Do not store messages from bots unless needed for Iris's own reply linkage and explicitly marked. Do not block Discord gateway event handling on slow DB calls without existing queue/backpressure protections.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: gateway/event/domain/repository integration.
  - Skills: [] - No special skill available.
  - Omitted: [] - None.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: [5,6,8] | Blocked By: [1]

  **References**:
  - Pattern: `internal/discord/gateway.go` - Discord event conversion and bot filtering.
  - Pattern: `internal/orchestrator/orchestrator.go:207-248` - accepted event handling.
  - Test: `internal/discord/gateway_test.go` - event callback verification.
  - Test: `internal/orchestrator/orchestrator_test.go` - async queue/typing tests.

  **Acceptance Criteria**:
  - [ ] Tests prove incoming user messages are saved to rolling map even if router later ignores them due to no trigger or channel not allowed.
  - [ ] Tests prove bot messages are not saved unless explicitly marked as Iris response metadata.
  - [ ] Tests prove reply reference fields are populated from Discord payloads.
  - [ ] `go test ./internal/discord ./internal/orchestrator ./internal/repository -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: Ignored casual message still updates channel map
    Tool: Bash
    Steps: Run orchestrator/gateway test with non-trigger user message in observed channel.
    Expected: Router ignores response, repository fake records one rolling message upsert.
    Evidence: .sisyphus/evidence/task-4-capture-ignored.txt

  Scenario: Reply metadata captured
    Tool: Bash
    Steps: Run gateway mapping test with Discord reply/reference payload.
    Expected: Domain event contains reply message/channel IDs and repository receives them.
    Evidence: .sisyphus/evidence/task-4-reply-metadata.txt
  ```

  **Commit**: NO | Message: `feat(discord): capture rolling channel context` | Files: [internal/discord, internal/orchestrator, internal/domain, internal/repository]

- [x] 5. Build relevant LLM context from channel sessions and replies

  **What to do**: Write failing context-builder tests first. Create a context/session builder that receives the current event and returns LLM messages: system prompt, compact context transcript, and current user message. It must include at least 10 relevant recent messages when available, selected from the current channel's rolling 20 and reply chain. Follow explicit reply chain up to depth 3; if referenced message is outside rolling DB but Discord fetch support exists, fetch it; otherwise include a marker that reply ancestor is unavailable. Preserve chronological order and labels like channel ID/name if available, author/user ID, timestamp, and whether a line is current/reply/context. Enforce a context budget default of 10 minimum, 20 current-channel max, plus at most 10 cross-channel candidates before token/character trim. Use safe truncation for each message to avoid model overflow.

  **Must NOT do**: Do not send raw unbounded channel history. Do not let context messages override system/persona instructions. Do not invent missing reply content.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: core prompt construction with tests and budget rules.
  - Skills: [] - No special skill available.
  - Omitted: [] - None.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: [6,8] | Blocked By: [1,4]

  **References**:
  - Pattern: `internal/orchestrator/orchestrator.go:224-234` - current LLM message construction to replace.
  - Pattern: `internal/app/app.go` - existing app-level LLM prompt construction if this path is active in runtime.
  - API/Type: `internal/persona/persona.go` - system prompt/persona must remain authoritative.
  - Test: `internal/app/app_test.go` and `internal/orchestrator/orchestrator_test.go` - fake LLM captures prompt messages.

  **Acceptance Criteria**:
  - [ ] Builder test with 15 channel messages includes at least 10 relevant prior messages plus current message.
  - [ ] Builder test with reply chain depth 4 includes only 3 ancestors and marks deeper ancestor omitted.
  - [ ] Builder test preserves system prompt as first message and current user message as final user message.
  - [ ] Existing persona/memory tests still pass.
  - [ ] `go test ./internal/app ./internal/orchestrator ./internal/persona -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: Mention receives recent context
    Tool: Bash
    Steps: Run context-builder test with 15 prior messages and current mention.
    Expected: Captured LLM request contains system prompt, a context block with >=10 prior messages, and final current user message.
    Evidence: .sisyphus/evidence/task-5-context-mention.txt

  Scenario: Reply chain followed with depth limit
    Tool: Bash
    Steps: Run context-builder test with nested replies A<-B<-C<-D<-current.
    Expected: Context contains D,C,B only and deterministic omission note for A.
    Evidence: .sisyphus/evidence/task-5-context-reply-depth.txt
  ```

  **Commit**: NO | Message: `feat(app): build discord session context for llm` | Files: [internal/app, internal/orchestrator, internal/domain]

- [x] 6. Add LLM-classified cross-channel relevance and memory promotion

  **What to do**: Write failing tests first. Add a bounded classifier that evaluates cross-channel candidate sessions only from: same guild, recent activity window, same user participation or recent Iris participation, and max 10 candidate messages/summaries. Use router/fast model if tier router exists. Classifier output must be strict JSON or a tiny enum, with fallback to no cross-channel merge on parse/error/timeout. Add async post-response memory-promotion classifier that reviews current message + selected context and decides whether to store durable memory via existing memory service. Scope memory per guild unless existing memory API requires another scope. Promotion must not block sending the Discord response; log failures in debug/audit style without user-facing error.

  **Must NOT do**: Do not scan every guild/channel. Do not merge private/unallowed channels into context. Do not store obvious persona override/injection attempts as memory.

  **Recommended Agent Profile**:
  - Category: `deep` - Reason: LLM classification, safety, fallback behavior, and memory integration need careful reasoning.
  - Skills: [] - No special skill available.
  - Omitted: [] - None.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: [8] | Blocked By: [1,4,5]

  **References**:
  - Pattern: `internal/llm/router.go` - fast model classifier style and fallback.
  - Pattern: `internal/memory/service.go` and `internal/memory/service_test.go` - memory gate and persistence behavior.
  - Pattern: `internal/safety/injection_test.go` - persona override/injection guardrails.
  - API/Type: `internal/app/app.go` - current memory/RAG integration path.

  **Acceptance Criteria**:
  - [ ] Tests prove classifier merges relevant cross-channel candidate and excludes irrelevant candidate.
  - [ ] Tests prove classifier parse/error/timeout falls back to current-channel-only context.
  - [ ] Tests prove unallowed channels are never included as cross-channel context after allow-list enforcement.
  - [ ] Tests prove memory promotion runs async and does not delay `SendMessage` completion.
  - [ ] `go test ./internal/app ./internal/memory ./internal/llm ./internal/safety -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: Relevant cross-channel topic merged
    Tool: Bash
    Steps: Run classifier test with two channels discussing same lore topic and fake classifier returning merge=true.
    Expected: Context builder includes bounded messages from both channels with labels.
    Evidence: .sisyphus/evidence/task-6-cross-channel-merge.txt

  Scenario: Classifier failure isolates session
    Tool: Bash
    Steps: Run classifier test with fake LLM timeout/invalid JSON.
    Expected: No cross-channel messages included; response pipeline continues.
    Evidence: .sisyphus/evidence/task-6-cross-channel-fallback.txt
  ```

  **Commit**: NO | Message: `feat(memory): classify session relevance and promotion` | Files: [internal/app, internal/memory, internal/llm]

- [x] 7. Make typing immediate and verify Discord message limit handling

  **What to do**: Write failing orchestrator tests first. Change typing behavior so `SendTyping` is called immediately after router decision accepts the message, not after 500ms. Continue refreshing typing at a safe interval (keep existing 5s unless tests/config show another value) until context build + classifier + LLM + response send completes or context cancels. Preserve `TypingAfter` only if needed as an optional delayed mode, but default requested behavior is immediate. Add/strengthen tests for `SplitMessage` around 2000-character boundaries, newline preference, long words, markdown/code fences if currently supported, and multi-chunk send order. Ensure send errors on one chunk are logged/handled according to existing pattern without panics.

  **Must NOT do**: Do not keep the first typing event delayed by default. Do not send chunks above `DiscordMessageLimit`.

  **Recommended Agent Profile**:
  - Category: `quick` - Reason: focused orchestrator behavior already exists.
  - Skills: [] - No special skill available.
  - Omitted: [] - None.

  **Parallelization**: Can Parallel: YES | Wave 4 | Blocks: [8] | Blocked By: []

  **References**:
  - Pattern: `internal/orchestrator/orchestrator.go:221-289` - typing lifecycle and send splitting.
  - Test: `internal/orchestrator/orchestrator_test.go` - `typingRecorder`, async wait helpers, splitter tests.
  - API/Type: `internal/orchestrator` `DiscordMessageLimit` and `SplitMessage` definitions.

  **Acceptance Criteria**:
  - [ ] Test proves first `SendTyping` occurs before fake LLM unblocks and without waiting for `TypingAfter`.
  - [ ] Test proves typing repeats during a long fake LLM call and stops after response send/cancel.
  - [ ] Splitter tests prove every chunk is `<= DiscordMessageLimit`.
  - [ ] `go test ./internal/orchestrator -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: Immediate typing while LLM is pending
    Tool: Bash
    Steps: Run orchestrator test with fake LLM blocked on channel and typing recorder.
    Expected: Typing recorder has >=1 call before LLM is released.
    Evidence: .sisyphus/evidence/task-7-immediate-typing.txt

  Scenario: Long Discord response split safely
    Tool: Bash
    Steps: Run splitter test with 4500-character response including newlines and a long unbroken segment.
    Expected: Sender receives ordered chunks, each <=2000 characters, content recombines to original or documented normalized equivalent.
    Evidence: .sisyphus/evidence/task-7-message-limit.txt
  ```

  **Commit**: NO | Message: `fix(discord): type immediately and harden message splitting` | Files: [internal/orchestrator]

- [x] 8. Wire runtime configuration, admin commands, and end-to-end pipeline

  **What to do**: Write failing app/wire/bootstrap/config tests first. Wire new repositories and services through `cmd/iris-bot/main.go`, `internal/app/wire`, orchestrator/app constructors, and config. Add or update admin commands/settings for allowed channels, preserving old exception list as fallback/migration support. Ensure `DEBUG=true` from `.env` reaches audit logging. Ensure context capture, context builder, cross-channel classifier, memory promotion, LLM audit logging, typing, and splitting all operate in the active runtime path. Update `.env.example` and docs only if they are generated as planning-approved implementation artifacts during execution. Add regression script coverage if appropriate.

  **Must NOT do**: Do not remove backwards-compatible `LLM_MODEL*` config. Do not require manual DB changes outside app migrations/bootstrap. Do not log content when `DEBUG=false`.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` - Reason: full wiring and compatibility checks across runtime.
  - Skills: [] - No special skill available.
  - Omitted: [`git-master`] - No commit requested.

  **Parallelization**: Can Parallel: NO | Wave 4 | Blocks: [Final Verification] | Blocked By: [2,3,4,5,6,7]

  **References**:
  - Pattern: `cmd/iris-bot/main.go` - runtime wiring and tier router construction.
  - Pattern: `internal/app/wire/adapters.go` - adapter style.
  - Pattern: `internal/bootstrap/bootstrap_test.go` - bootstrap/idempotence tests.
  - Pattern: `.env.example` - existing env documentation.
  - Command: `bash scripts/regression.sh` - existing regression suite.

  **Acceptance Criteria**:
  - [ ] Config tests prove `DEBUG=true` enables audit logging and default false disables it.
  - [ ] Wire tests prove all new repos/services are non-nil and active in runtime constructors.
  - [ ] Admin/settings tests prove allowed-channel add/remove/list works and fallback semantics remain documented/testable.
  - [ ] End-to-end fake app/orchestrator test proves an accepted message is captured, context-built, audited, typed, answered, split, and optional memory promotion is scheduled.
  - [ ] `go test ./... -count=1`, `go build ./...`, and `bash scripts/regression.sh` pass.

  **QA Scenarios**:
  ```
  Scenario: End-to-end debug audit and context reply
    Tool: Bash
    Steps: Run targeted app/orchestrator integration test with DEBUG=true, 12 prior channel messages, fake LLM, and fake sender.
    Expected: LLM receives >=10 context messages; audit log has duration and truncated content; typing occurs before LLM completion; sender receives safe chunk(s).
    Evidence: .sisyphus/evidence/task-8-e2e-debug-context.txt

  Scenario: Runtime regression suite
    Tool: Bash
    Steps: Run `go test ./... -count=1 && go build ./... && bash scripts/regression.sh`.
    Expected: All commands exit 0.
    Evidence: .sisyphus/evidence/task-8-regression.txt
  ```

  **Commit**: NO | Message: `feat(discord): wire audit context runtime` | Files: [cmd/iris-bot, internal/app, internal/config, internal/bootstrap, .env.example]

## Final Verification Wave (MANDATORY — after ALL implementation tasks)
> 4 review agents run in PARALLEL. ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.
> **Do NOT auto-proceed after verification. Wait for user's explicit approval before marking work complete.**
> **Never mark F1-F4 as checked before getting user's okay.** Rejection or user feedback -> fix -> re-run -> present again -> wait for okay.
- [x] F1. Plan Compliance Audit — oracle
- [x] F2. Code Quality Review — unspecified-high
- [x] F3. Real Manual QA — unspecified-high
- [x] F4. Scope Fidelity Check — deep

## Commit Strategy
- Do not commit unless the user explicitly requests a commit.
- If commits are requested later, prefer one atomic commit after all tests pass: `feat(discord): add audit context sessions`.

## Success Criteria
- I.R.I.S responds only in allowed channels after allow-list rows exist, while fallback preserves current behavior before configuration.
- Every LLM request is auditable under `DEBUG=true` with duration and bounded redacted content snippets.
- Accepted Discord messages show typing immediately and keep it alive while LLM/context work is pending.
- LLM prompts include relevant recent Discord context, including at least 10 messages when available, reply-chain context, and bounded LLM-classified cross-channel context.
- Rolling channel context remains capped at 20 messages per channel and important content can still be promoted to durable memory.
- Full Go test/build/regression suite passes.
