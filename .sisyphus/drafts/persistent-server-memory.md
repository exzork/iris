# Draft: Persistent Server Memory

## Requirements (confirmed)
- User changed `LLM_MODEL` to `claude-haiku-4.5`.
- User wants the bot to always remember context in a server.
- User states the stack has PostgreSQL vector database and ONNX runtime.
- User wants an implementation inspired by `https://github.com/alash3al/stash`.
- User wants adaptation because `stash` supports one LLM provider only.
- User wants this bot to use the LLM provider from `.env` and embedding models through ONNX runtime.
- User wants the plan updated so the bot recognizes each user's behavior within a server.
- User clarified this means Iris should know each user's personality and adjust how she interacts with that user.

## Technical Decisions
- Memory scope: server/guild shared memory plus per-user behavior profiles scoped inside each guild.
- Behavior profile purpose: personalize Iris' tone, phrasing, response length, humor/formality, and interaction style for that user in that server only.
- Behavior profile guardrail: do not infer sensitive traits, protected classes, diagnoses, secrets, moderation labels, or global cross-server personality.
- Capture policy: save all server messages into long-term memory.
- Test strategy: TDD using existing test infrastructure.
- Pending exploration: exact hook points for Discord context/memory retrieval and persistence.
- Pending exploration: existing LLM provider abstraction and env variable names.
- Pending exploration: existing pgvector schema/migrations.
- Pending exploration: existing ONNX embedding implementation.

## Research Findings
- Background explore launched for bot architecture and integration points: `bg_7b3f3107`.
- Background explore launched for test infrastructure: `bg_5854bd7c`.
- Background librarian launched for `stash` repo architecture/adaptation: `bg_407b2e2d`.
- README confirms Go Discord bot, PostgreSQL + pgvector, OpenAI-compatible LLM provider, Docker Compose, wiki-grounded Indonesian responses, and lightweight per-user memory.
- Bot architecture findings: `cmd/iris-bot/main.go` wires LLM clients, memory service, embedder, orchestrator, and adapters.
- Config findings: `internal/config/config.go` owns env loading for LLM provider/model settings and embedding paths.
- LLM findings: `internal/llm/client.go`, `internal/llm/adapter.go`, and `internal/llm/router.go` provide OpenAI-compatible chat and model-tier routing.
- Existing memory findings: repo has memory service/repository/orchestrator/context-builder components; plan should extend these rather than introduce an unrelated service.
- Test findings: CI, Makefile targets, regression script, fake LLM/Discord/embedding clients, repository integration tests, Docker pgvector stack, and live-smoke scripts exist.
- `stash` findings: architecture centers on Brain + Embedder + Reasoner + PostgreSQL/pgvector; adaptation should reuse ideas while replacing OpenAI embedding calls with local ONNX embeddings and using this repo's `.env` LLM abstraction.

## Open Questions
- Whether recall should be injected into all responses or only when similarity/confidence threshold passes.
- Whether all-message capture needs retention/privacy controls, exclusion channels, or admin opt-out.
- Exact behavior dimensions should default to communication style, recurring preferences/interests, topic affinity, helpful response format, and interaction cadence; avoid sensitive trait inference.
- Plan updated with dedicated Task 6 for guild-scoped user behavior/personality profiles, Task 7 for safe prompt injection of behavior hints, and Task 11 for E2E personality isolation tests.

## Scope Boundaries
- INCLUDE: planning server-side persistent contextual memory, per-user behavior profiles scoped by guild, vector storage, ONNX embeddings, `.env`-selected LLM provider compatibility, and QA strategy.
- EXCLUDE: source-code implementation by Prometheus; execution belongs to `/start-work` after plan generation.
