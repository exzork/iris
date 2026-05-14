# LLM Tool Calling + Persona Cleanup

## TL;DR
> **Summary**: Enable OpenAI-compatible function calling so Iris can invoke the existing `websearch` tool when she lacks canonical data, and strip persona rules that tell her to cite URLs or redirect users to external wikis.
> **Deliverables**:
> - `ChatWithTools` code path on `*llm.Client` that sends `tools=[...]`, parses `tool_calls`, executes them via the existing registry, feeds results back, loops until final text.
> - Wire only the orchestrator.handle path to use the tool-calling client. Classifiers (cross-channel, in-window relevance, memory promoter) stay on plain `ChatWithModel`.
> - Register `websearch` tool in `cmd/iris-bot/main.go` as the only enabled tool for now.
> - Persona rewrite: remove "cite URL" and "direct to wiki" instructions. Replace with "when you lack data, call the search tool. Never expose raw URLs or source names in replies unless the user explicitly asked."
> **Effort**: Medium
> **Parallel**: NO — 4 sequential tasks, each short
> **Critical Path**: T1 → T2 → T3 → T4

## Context
### Original Request
- When Iris doesn't have the answer, the LLM should search for it instead of telling the user to check a wiki.
- Never mention wiki URLs or source URLs in replies unless the user explicitly asks.

### Built-on
- `internal/llm/client.go` already has `chatRequest.Tools` and `chatResponse.Choices[].Message.ToolCalls`. Just not populated/surfaced.
- `internal/tools/registry.go` is feature-complete: Register, Execute, per-tool timeout + maxOutput.
- `internal/tools/websearch/tool.go` implements `Tool` and is ready to be registered.
- `internal/tools/schema.go` describes each tool in project terms but does NOT serialize to OpenAI `{type:"function", function:{name,description,parameters}}` format yet.
- `internal/persona/persona.go` hardcodes `fandomHost = "wutheringwaves.fandom.com"` and instructs Iris to "kasih sitasi URL" and "arahkan ke wiki".

### Scope
- Out of scope this plan: exposing admin tools, memesearch, charlookup, itemlookup, patchnotes as function calls. Only `websearch` is turned on so we can validate end-to-end before broadening.
- Out of scope: ranking or summarizing search results beyond what the existing websearch tool already returns. The LLM does the last-mile synthesis.

## Work Objectives
### Core Objective
Iris calls web search herself when needed and never emits URLs or wiki names in user-facing replies.

### Deliverables
- `ChatWithTools(ctx, model, guildID, messages, tools)` method on `*llm.Client` that runs the tool-call loop and returns final assistant text.
- `Schema.ToOpenAIFunction()` serializer on `*tools.Schema` producing the JSON shape OpenAI expects.
- `tools.Registry.OpenAIFunctions()` helper returning `[]map[string]interface{}` for all registered tools.
- Orchestrator handle path switched to `ChatWithTools` with the registry-provided tools and a registry executor.
- Registration of `websearch` in main.go, only when the appropriate provider is configured (HTTP provider needs an env var; if unset, register with a no-op provider that returns "no results" so the LLM gracefully falls back).
- Persona v2: strip fandom URL/citation scaffolding. Add clear rules: "when unsure, call the websearch tool"; "never include URLs or source names unless the user asks".
- Tests covering: ToOpenAIFunction shape, registry serializer, client.ChatWithTools with a fake transport that emits one tool_call then a final text, orchestrator integration with a stub tool-runner.

### Definition of Done
- `go build ./...`, `go vet ./...`, `go test -p 2 -count=1 ./...` pass.
- New unit tests in internal/tools and internal/llm pass.
- Persona tests updated: Iris no longer mentions "fandom", URLs, or "kasih sitasi".
- Redeploy runs clean; live Discord replies for "kapan wuwa rilis?" or "apa yang dilakukan lynae di 3.1?" include actual info from a search rather than "cek wiki Fandom".

### Must Have
- TDD for each task.
- Classifiers (internal/orchestrator/cross_channel*.go, conversation_relevance*.go, memory_promotion.go) MUST continue using plain `ChatWithModel` with no tools — they parse strict JSON and tool_calls would break parsing.
- Tool-call loop has a hard ceiling (max 3 tool calls per user turn) to avoid runaway.
- Tool arguments and result payloads are logged at DEBUG only (never INFO, never raw content in INFO).
- Websearch output MUST NOT be pasted verbatim into the reply; the LLM summarizes. Persona explicitly forbids quoting URLs in replies unless the user asks for them.
- The existing `LoreCitation` / RAG path (persona prompts about lore with citations) still works if lore retrieval returned citations — it just won't force Iris to fabricate wiki URLs when there are none.

### Must NOT Have
- No change to `Chat` / `ChatWithModel` signatures (classifiers rely on them).
- No schema migration.
- No new environment variable unless strictly required (a single `IRIS_TOOLS_ENABLED=true` gate is acceptable and defaults to true).
- No deletion of LoreCitation or the existing RAG path.
- No URL in the persona prompt unless it's a tool description (never in user-facing reply instructions).

## Verification Strategy
- TDD via Go unit + integration tests.
- Agent-executed QA scenarios for every task.
- Evidence files under `.sisyphus/evidence/tools-*`.

## Execution Strategy
### Waves
- Wave 1: T1 — schema → OpenAI function serializer.
- Wave 2: T2 — `Client.ChatWithTools` with tool-call loop.
- Wave 3: T3 — orchestrator wiring + registry hook-up + main.go tool registration.
- Wave 4: T4 — persona rewrite + tests.
- Final Wave: oracle F1-F4.

## TODOs

- [x] 1. Add `Schema.ToOpenAIFunction()` and `Registry.OpenAIFunctions()`

  **What to do**:
  - In `internal/tools/schema.go` add method `(s *Schema) ToOpenAIFunction() map[string]interface{}` returning:
    ```
    {
      "type": "function",
      "function": {
        "name": s.Name,
        "description": s.Description,
        "parameters": {
          "type": "object",
          "properties": { <Field.Name>: { "type": <Kind>, "description": <Field.Description> }, ... },
          "required": [<required field names>]
        }
      }
    }
    ```
    Map Kinds: "string"→"string","number"→"number","bool"→"boolean","object"→"object","array"→"array".
  - In `internal/tools/registry.go` add `(r *Registry) OpenAIFunctions() []map[string]interface{}` that iterates all registered tools and returns their serialized schemas (non-admin tools only for now; admin-only tools must NOT appear).
  - Tests in `internal/tools/schema_test.go` and `internal/tools/registry_test.go`:
    - `TestSchema_ToOpenAIFunction_Shape` with a fixture schema with string + number + bool + required/optional fields.
    - `TestRegistry_OpenAIFunctions_SkipsAdminTools`.
    - `TestRegistry_OpenAIFunctions_RoundTrip` (serialize then parse back into structure, assert keys).

  **Must NOT do**: Do not edit the Tool interface. Do not break existing schema tests. Do not change registry admin-only policy.

  **Agent**: `quick`, `[]`.

  **Parallel**: NO | Wave 1 | Blocks: [2,3] | Blocked by: []

  **Acceptance**:
  - [ ] ToOpenAIFunction returns the expected JSON-ready map.
  - [ ] Registry helper returns all non-admin tools and nothing else.
  - [ ] `go test ./internal/tools -count=1 -v` passes.

  **Evidence**: `.sisyphus/evidence/tools-1-schema.txt`, `.sisyphus/evidence/tools-1-registry.txt`.

  **QA Scenarios**:
  ```
  Scenario: Schema serializer shape
    Tool: Bash
    Steps: go test ./internal/tools -run 'Schema_ToOpenAIFunction_Shape' -count=1 -v
    Expected: PASS; output JSON contains keys type=function, function.name, function.parameters.type=object, function.parameters.properties, function.parameters.required.
    Evidence: .sisyphus/evidence/tools-1-schema.txt

  Scenario: Registry skips admin tools
    Tool: Bash
    Steps: go test ./internal/tools -run 'Registry_OpenAIFunctions_SkipsAdminTools' -count=1 -v
    Expected: PASS; returned slice excludes admin-only tools.
    Evidence: .sisyphus/evidence/tools-1-registry.txt
  ```

- [x] 2. Add `Client.ChatWithTools` with tool-call loop

  **What to do**:
  - New public method on `*llm.Client` in internal/llm/client.go:
    ```go
    type ToolExecutor interface {
        Execute(ctx context.Context, name string, args map[string]interface{}) (string, error)
    }

    type ChatWithToolsConfig struct {
        Model   string
        GuildID int64
        Tools   []map[string]interface{} // OpenAI schema (from Registry.OpenAIFunctions)
        Exec    ToolExecutor
        Max     int // max tool-call rounds; default 3
    }

    func (c *Client) ChatWithTools(ctx context.Context, messages []map[string]string, cfg ChatWithToolsConfig) (string, error)
    ```
  - Loop:
    1. POST /v1/chat/completions with messages + tools. `tool_choice: "auto"` if tools non-empty.
    2. If response has no tool_calls, return `Choices[0].Message.Content`.
    3. Else, for each tool_call: marshal to the assistant message, call `cfg.Exec.Execute(ctx, name, args)`, append a `{"role":"tool","tool_call_id":..., "content": <result>}` message.
    4. Increment round counter; if > cfg.Max, return whatever text the LLM produced last (or error if still no content).
    5. Loop.
  - Keep the existing stream=false + retry behavior. Reuse `doRequest` if practical; otherwise a dedicated POST is fine since the request body shape is similar.
  - Attach `llm.WithMeta(..., TriggerReason="chat_with_tools")` for audit continuity.
  - Tests in `internal/llm/client_tools_test.go` using an httptest.Server:
    - `TestChatWithTools_NoTools_FallsThroughToText` — tools slice empty, LLM returns plain text.
    - `TestChatWithTools_OneToolCall_ThenText` — first response contains tool_calls with one call, second response returns text.
    - `TestChatWithTools_MaxRounds_Exceeded_ReturnsLastText`.
    - `TestChatWithTools_ToolExecuteError_PropagatedAsToolMessage` — tool returns error; the loop sends `content: "error: ..."` to the model and continues.

  **Must NOT do**: Do NOT modify `Chat` or `ChatWithModel`. Do NOT add streaming. Do NOT log raw tool arguments or tool results at INFO — use DEBUG.

  **Agent**: `unspecified-high`, `[]`.

  **Parallel**: NO | Wave 2 | Blocks: [3] | Blocked by: [1]

  **Acceptance**:
  - [ ] Client posts JSON with a populated `tools` array when the registry has tools.
  - [ ] Tool calls are executed and responses round-tripped.
  - [ ] Loop terminates gracefully at max rounds.
  - [ ] `go test ./internal/llm -count=1 -v` passes.

  **Evidence**: `.sisyphus/evidence/tools-2-tool-loop.txt`.

  **QA Scenarios**:
  ```
  Scenario: Tool-call loop executes one call then returns text
    Tool: Bash
    Steps: go test ./internal/llm -run 'ChatWithTools_OneToolCall_ThenText' -count=1 -v
    Expected: PASS; fake transport receives two POSTs, second request includes role=tool with tool_call_id, final return equals the second response's assistant content.
    Evidence: .sisyphus/evidence/tools-2-tool-loop.txt

  Scenario: Max rounds cap
    Tool: Bash
    Steps: go test ./internal/llm -run 'ChatWithTools_MaxRounds_Exceeded' -count=1 -v
    Expected: PASS; loop exits after Max rounds without infinite recursion.
    Evidence: .sisyphus/evidence/tools-2-tool-loop.txt
  ```

- [x] 3. Wire tool calling into orchestrator and register websearch

  **What to do**:
  - Add a `ToolCallingLLM` port to orchestrator.Config:
    ```go
    type ToolCallingLLM interface {
        ChatWithTools(ctx context.Context, messages []map[string]string, cfg llm.ChatWithToolsConfig) (string, error)
    }
    ```
    Keep the existing `LLM LLMCaller` field unchanged (classifiers still use it). Add a NEW optional field `ToolsLLM ToolCallingLLM` and an optional `ToolExecutor llm.ToolExecutor` and an optional `Tools []map[string]interface{}` slice for pre-serialized tool schemas.
  - In orchestrator.handle: if `cfg.ToolsLLM != nil && len(cfg.Tools) > 0`, call `cfg.ToolsLLM.ChatWithTools(ctx, messages, llm.ChatWithToolsConfig{Model: model, GuildID: guild, Tools: cfg.Tools, Exec: cfg.ToolExecutor, Max: 3})` INSTEAD of `cfg.LLM.Chat(...)`. Fall back to `cfg.LLM.Chat(...)` when any of those are nil.
  - In `internal/app/wire/adapters.go` add an adapter for `ToolCallingLLM` wrapping `*llm.Client.ChatWithTools` and a `RegistryExecutor` adapter satisfying `llm.ToolExecutor` by calling `registry.Execute(ctx, tools.ExecuteRequest{Tool: name, Args: args, GuildID: 0, UserID: 0})` (guild/user irrelevant to websearch; future tools may need actual scoping — accept that limitation for now).
  - In `cmd/iris-bot/main.go`:
    - Build a `*tools.Registry`, register `websearch` via `registry.Register(&tools.ToolDefinition{Tool: websearch.New(provider)})` where `provider` comes from the existing HTTP provider constructor (inspect `internal/tools/websearch/http_provider.go` for the exact constructor signature and required envs). If the required env is unset, skip registration and log one warn line.
    - Set `orchCfg.Tools = registry.OpenAIFunctions()`, `orchCfg.ToolsLLM = &wireadapters.ToolCallingLLMAdapter{Client: chatClient}`, `orchCfg.ToolExecutor = &wireadapters.RegistryExecutor{Reg: registry}`.
  - Orchestrator tests: add one e2e-ish test `TestHandle_UsesToolsLLMWhenConfigured` with a fake ToolsLLM recording the call. Another `TestHandle_FallsBackToPlainLLMWhenNoTools`.

  **Must NOT do**: Do NOT pipe tools into classifiers. Do NOT add a new env for tool allow-list (future work).

  **Agent**: `unspecified-high`, `[]`.

  **Parallel**: NO | Wave 3 | Blocks: [4] | Blocked by: [2]

  **Acceptance**:
  - [ ] Orchestrator e2e test with fake ToolsLLM shows it is called and plain LLM is not.
  - [ ] `go build ./...` and full tests pass.
  - [ ] main.go registers websearch when provider envs are set; logs warn and continues when they aren't.

  **Evidence**: `.sisyphus/evidence/tools-3-orchestrator.txt`, `.sisyphus/evidence/tools-3-main.txt`.

  **QA Scenarios**:
  ```
  Scenario: Orchestrator prefers ToolsLLM when configured
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'Handle_UsesToolsLLMWhenConfigured' -count=1 -v
    Expected: PASS; fake ToolsLLM recorded one call, fake LLM recorded zero calls.
    Evidence: .sisyphus/evidence/tools-3-orchestrator.txt

  Scenario: Graceful fallback when no tools
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'Handle_FallsBackToPlainLLMWhenNoTools' -count=1 -v
    Expected: PASS; fake LLM recorded one call.
    Evidence: .sisyphus/evidence/tools-3-orchestrator.txt

  Scenario: Main wiring grep
    Tool: Bash
    Steps: grep -n 'registry.Register\|ChatWithTools\|ToolExecutor\|OpenAIFunctions' cmd/iris-bot/main.go
    Expected: Non-empty match showing websearch registration and ToolsLLM/ToolExecutor wiring into orchCfg.
    Evidence: .sisyphus/evidence/tools-3-main.txt
  ```

- [x] 4. Persona rewrite (remove wiki URL directives)

  **What to do**:
  - Edit `internal/persona/persona.go`:
    - Remove `fandomHost` constant from `immutablePersona`/`lorePolicy` strings.
    - Replace the "Kalau jawab soal lore, kasih sitasi" / "arahkan ke wiki" rules with:
      - "Kalau kamu butuh info yang gak ada di arsip, panggil tool `websearch` dulu."
      - "Jawaban ke user gak boleh ngeluarin URL, nama wiki, atau link sumber kecuali user minta secara eksplisit."
      - "Kalau udah nyari dan tetep gak ada, bilang terus terang: 'belum ada data yang pasti' — gak usah nyebut wiki."
    - Keep the rest of the persona (casual Indonesian, archivist tone, no persona override, etc.) intact.
    - Bump persona `version` to `"1.2.0"`.
  - Update `internal/persona/persona_test.go`:
    - Assert `immutablePersona` does NOT contain "fandom", "wiki", or any URL substring.
    - Assert it DOES contain `websearch`.
    - Keep existing identity-lock and lock-tone assertions.
  - The `LoreCitation` struct and its URL validation stay as-is — still used by the RAG path to validate retrieved citations internally; it's just not surfaced to the user unless asked.

  **Must NOT do**: Do NOT delete `LoreCitation` or its tests. Do NOT delete the RAG memory path.

  **Agent**: `writing`, `[]`.

  **Parallel**: NO | Wave 4 | Blocks: [Final Verification] | Blocked by: [3]

  **Acceptance**:
  - [ ] Persona tests pass with the new tone and no wiki URLs.
  - [ ] `go test -p 2 ./... -count=1` passes.
  - [ ] Live reply to an unknown-lore question no longer contains `fandom` or URLs.

  **Evidence**: `.sisyphus/evidence/tools-4-persona.txt`, `.sisyphus/evidence/tools-4-deploy.txt`.

  **QA Scenarios**:
  ```
  Scenario: Persona strips URLs and references websearch
    Tool: Bash
    Steps: go test ./internal/persona -count=1 -v
    Expected: PASS; assertions confirm no "fandom", no "wiki", no "http" in immutablePersona and "websearch" present.
    Evidence: .sisyphus/evidence/tools-4-persona.txt

  Scenario: Deploy and smoke startup
    Tool: Bash
    Steps: docker compose build bot && docker compose up -d bot && sleep 8 && docker compose logs --tail=20 bot
    Expected: Four startup INFO lines, no error, backend=similarity, gateway connected.
    Evidence: .sisyphus/evidence/tools-4-deploy.txt
  ```

## Final Verification Wave
- [ ] F1. Plan Compliance Audit — oracle
- [ ] F2. Code Quality Review — oracle
- [ ] F3. Real Manual QA — oracle
- [ ] F4. Scope Fidelity Check — oracle

## Success Criteria
- On an unknown-lore question, Iris calls `websearch`, summarizes, replies without URLs/wiki names.
- Classifier paths (similarity gate, cross-channel, memory promoter) remain LLM-text-only and continue to work.
- Persona explicitly prohibits URL/source leakage in replies and instructs Iris to invoke `websearch` when stumped.
- Full Go test/build/vet suite passes.
