# Embedding-Based Context Classifier

## TL;DR
> **Summary**: Replace LLM-based `InWindowRelevance` and `CrossChannelClassifier` with cosine-similarity over `sentence-transformers/paraphrase-MiniLM-L3-v2` (384-dim) sentence embeddings, executed in-process via ONNX.
> **Deliverables**:
> - In-process embedder (`internal/embedder`) using ONNX Runtime + HF tokenizer, loading the MiniLM-L3 ONNX export at startup.
> - pgvector `content_embedding` column on `channel_messages`; embedding backfilled by the capture path.
> - New similarity-based `InWindowRelevance` and `CrossChannelClassifier` implementations replacing the existing LLM classifiers.
> - Runtime wiring + config (`IRIS_EMBED_MODEL_PATH`, `IRIS_EMBED_SIM_THRESHOLD`, `IRIS_EMBED_TOKENIZER_PATH`) and nil-safe fallback to the old LLM classifier if the embedder fails to load.
> **Effort**: Medium-Large
> **Parallel**: YES — 3 waves
> **Critical Path**: T1 → T2 → (T3 ‖ T4) → T5 → Final

## Context
### Original Request
- Use `sentence-transformers/paraphrase-MiniLM-L3-v2` as a replacement for the context LLM classifier.

### Interview Summary
- Scope: replace BOTH `InWindowRelevance` and `CrossChannelClassifier` with embedding similarity.
- Backend: ONNX in Go, in-process, no sidecar.
- Threshold: default 0.55, env-overridable.
- Caching: persist per-message embedding as a pgvector column on `channel_messages`.

### Built-on
- `internal/orchestrator/conversation_relevance.go` (LLM classifier, to be replaced).
- `internal/orchestrator/cross_channel.go` (LLM classifier, to be replaced).
- `internal/repository/channel_messages` (rolling store; upserted by capture).
- `migrations/001..003_*.sql`; existing `lore_chunks.embedding vector(1536)` confirms pgvector is already available.

## Work Objectives
### Core Objective
Fast, cheap, deterministic context-relevance decisions without LLM calls or JSON parsing.

### Deliverables
- Embedder interface + ONNX implementation:
  - `type Embedder interface { Embed(ctx, text) ([]float32, error); Dim() int }`
  - Implementation loads ONNX model + tokenizer once at startup; concurrent `Embed` calls must be safe.
  - Unit tests with a lightweight fake and one integration test that actually runs the real model (behind a build tag so CI without the model stays green).
- pgvector `content_embedding vector(384)` column added to `channel_messages` with supporting migration `004_channel_message_embeddings.sql`. Upsert path stores the embedding whenever the capture adapter calls it. Null embedding is allowed (backwards compatible).
- Similarity-based `InWindowRelevance`: returns `(true, nil)` iff cosine similarity between current message embedding and a recent-context centroid ≥ threshold. `(false, nil)` otherwise. `(false, err)` on embed error.
- Similarity-based `CrossChannelClassifier`: for each candidate message, compute cosine similarity against the current message embedding; return the top K candidates above threshold, bounded by existing `MaxCandidates=10` and window rules.
- Runtime wiring + env parsing + nil-safe fallback: if embedder init fails, fall back to the existing LLM classifiers (preserve current behavior as escape hatch).

### Definition of Done
- `go build ./...`, `go vet ./...`, `go test -p 2 -count=1 ./...` pass.
- New embedder tests pass with a deterministic fake. Real-ONNX integration test passes locally when model is present (behind a `//go:build embed_real` tag or similar).
- DB migration applies idempotently; existing row reads without embeddings still work.
- Orchestrator still routes and responds correctly when embedder is nil (legacy LLM path remains active).

### Must Have
- TDD.
- Pooling: **mean pooling over token embeddings masked by attention mask**, followed by **L2 normalization** (so cosine simplifies to dot product).
- Thread-safety: concurrent `Embed` calls must not corrupt the ONNX session. Use a mutex or a session pool.
- Model + tokenizer files configurable via `IRIS_EMBED_MODEL_PATH` (required when embedder enabled) and `IRIS_EMBED_TOKENIZER_PATH`. If either is empty → embedder disabled, log warn, fall back to LLM classifiers.
- Persist the 384-dim embedding via the existing Upsert transaction; never fail the message capture if embedding fails (log and continue).
- All new config env vars documented in `.env.example`.
- Similarity threshold configurable via `IRIS_EMBED_SIM_THRESHOLD` (default "0.55").

### Must NOT Have
- No network calls at inference time.
- No Python, sidecar service, or Docker compose change in this plan (model loads from local path).
- No pgvector-index on the new column yet (premature; rolling table is only newest 20 per channel — linear scan is fine).
- No deletion of `conversation_relevance.go` / `cross_channel.go` LLM types until new implementations are wired (keep as `LLMClassifier` escape-hatch).
- No regression on existing tests.

## Verification Strategy
- TDD via Go unit tests for embedder, classifier, repo, and orchestrator wiring.
- Integration test gated on `//go:build embed_real` that loads the model from `IRIS_EMBED_MODEL_PATH`.
- Evidence files: `.sisyphus/evidence/embed-*.txt`.
- Agent-executed manual QA via oracle (Final Wave).

## Execution Strategy
### Waves
- Wave 1: T1 (embedder package). Solo; blocks everything.
- Wave 2: T2 (migration + repo embedding persistence). After T1.
- Wave 3: T3 (InWindowRelevance similarity) and T4 (CrossChannelClassifier similarity) in parallel. After T2.
- Wave 4: T5 (config + wiring + e2e). After T3 and T4.
- Final Wave: oracle F1-F4.

## TODOs

- [x] 1. Add in-process embedder package using ONNX + HF tokenizer

  **What to do**: Create `internal/embedder` with:
  - `type Embedder interface { Embed(ctx context.Context, text string) ([]float32, error); Dim() int; Close() error }`.
  - `type ONNXConfig struct { ModelPath, TokenizerPath string; MaxSeqLen int; BatchSize int }`.
  - `func NewONNX(cfg ONNXConfig) (Embedder, error)` that loads the ONNX model and HuggingFace tokenizer, validates the output tensor shape is `[1, seq, 384]` (or `[1, 384]` for pooled), and returns an Embedder.
  - Implementation responsibilities:
    - Tokenize text with the HF tokenizer (uses `knights-analytics/hugot` or `daulet/tokenizers` + `yalue/onnxruntime_go` — pick one; see Research Note below).
    - Run model → mean-pool token outputs with attention mask → L2 normalize → return `[]float32` of length 384.
    - Guard concurrency with a `sync.Mutex` around the ONNX session (simplest correct option).
    - Expose `Dim()` reporting 384.
  - Tests in `internal/embedder/embedder_test.go`:
    - `TestFakeEmbedderDeterministic` via an injected fake (hash → pseudo-vector) to keep CI hermetic.
    - `TestMeanPoolingWithMask` covering the pooling math.
    - `TestONNXIntegration` behind `//go:build embed_real`: loads the real model from env path, asserts dim=384 and that two paraphrases are more similar than two unrelated strings (e.g. "I'm tired" / "I need sleep" vs "I'm tired" / "pizza recipe"). Skip when env is unset.

  **Research Note**: `github.com/knights-analytics/hugot` bundles onnxruntime bindings + HF tokenizer and has prebuilt support for sentence-transformers pipelines. If it loads our target model cleanly, prefer it. Otherwise fall back to `yalue/onnxruntime_go` + `daulet/tokenizers`. Document the final choice in the notepad.

  **Must NOT do**: Do NOT make HTTP calls. Do NOT depend on the memory embedding client (`internal/llm/embedding.go`) — that client speaks to a remote endpoint and is 1536-dim. Do NOT introduce cgo if a pure-Go path exists; if cgo is required, gate with build tags and document the runtime dep.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` — third-party native deps + careful concurrency.
  - Skills: [] — none match.

  **Parallelization**: Can Parallel: NO | Wave 1 | Blocks: [2,3,4,5] | Blocked By: []

  **References**:
  - HF model card: https://huggingface.co/sentence-transformers/paraphrase-MiniLM-L3-v2 (384-dim, mean pooling, L2-normalize).
  - Pattern: `internal/llm/embedding.go` (network-based embedder; same `Embed(ctx, text) ([]float32, error)` signature style).
  - Pattern: `internal/lore/rag/retriever.go` (uses `Embed` port).

  **Acceptance Criteria**:
  - [ ] Fake embedder tests pass without the real model.
  - [ ] With `IRIS_EMBED_MODEL_PATH` + `IRIS_EMBED_TOKENIZER_PATH` set and the ONNX model present, the real-model integration test passes (`go test -tags embed_real ./internal/embedder -v`).
  - [ ] `Embed` is safe under parallel calls (use `-race` in the test run).
  - [ ] `Close()` releases resources without panic.

  **QA Scenarios**:
  ```
  Scenario: Concurrent Embed is race-free
    Tool: Bash
    Steps: go test -race -count=1 ./internal/embedder -v
    Expected: PASS with no DATA RACE detected.
    Evidence: .sisyphus/evidence/embed-1-race.txt

  Scenario: Paraphrases score higher than unrelated pairs
    Tool: Bash
    Steps: IRIS_EMBED_MODEL_PATH=... IRIS_EMBED_TOKENIZER_PATH=... go test -tags embed_real ./internal/embedder -run Paraphrase -v
    Expected: sim("I'm tired","I need sleep") > sim("I'm tired","pizza recipe").
    Evidence: .sisyphus/evidence/embed-1-paraphrase.txt
  ```

  **Commit**: NO

- [x] 2. Store `content_embedding` on `channel_messages` and backfill via capture

  **What to do**:
  - Migration `migrations/004_channel_message_embeddings.sql`:
    ```sql
    ALTER TABLE channel_messages ADD COLUMN IF NOT EXISTS content_embedding vector(384);
    ```
    No index yet (rolling window is at most 20 rows per channel).
  - Extend `domain.ChannelMessage` with `ContentEmbedding []float32` (nullable by convention: `len(...) == 0` means absent).
  - Update `ChannelMessageRepo.Upsert` to persist the embedding when non-empty. Keep the existing prune-to-20 transaction. Add/extend `ListRecent`, `GetByID`, `ListByUserAcrossChannels` to scan the new column.
  - Extend the capture flow so it calls `Embedder.Embed(ctx, normalizedContent)` before upsert. If `Embedder` is nil or returns an error, capture proceeds without the embedding (log debug). Wire via a new optional `CaptureEmbedder` field on whatever struct currently performs capture (inspect the current orchestrator capture hook — if none, pass through `ChannelCapture` adapter in `internal/app/wire/adapters.go`).
  - Tests:
    - Repo test `TestChannelMessage_UpsertWithEmbedding_PersistsAndReads`.
    - Repo test `TestChannelMessage_UpsertWithoutEmbedding_StillWorks` (nil embedding).
    - Capture test: `TestCaptureEmbedsThenUpserts` with a fake embedder.
    - Capture test: `TestCaptureFailsSoftOnEmbedderError`.

  **Must NOT do**: Do NOT break older rows created before this migration. Do NOT add pgvector ANN index in this task. Do NOT block capture on embed errors.

  **Recommended Agent Profile**:
  - Category: `unspecified-high` — schema + repo + integration.

  **Parallelization**: Can Parallel: NO | Wave 2 | Blocks: [3,4,5] | Blocked By: [1]

  **References**:
  - Pattern: `migrations/002_channel_context.sql` — schema style.
  - Pattern: `internal/repository/channel_context.go` — Upsert/Prune transaction.
  - Pattern: `internal/repository/testhelper.go` — truncate ordering (already lists channel_messages).
  - API/Type: `internal/domain/types.go` — `ChannelMessage`.

  **Acceptance Criteria**:
  - [ ] Migration applies idempotently.
  - [ ] `go test ./internal/repository -run 'ChannelMessage.*Embedding' -count=1 -v` passes.
  - [ ] `go test ./internal/orchestrator -run 'Capture.*Embed' -count=1 -v` passes.
  - [ ] Existing repo tests still pass.

  **QA Scenarios**:
  ```
  Scenario: Embedding round-trip
    Tool: Bash
    Steps: Upsert a message with a 384-dim fixture embedding; ListRecent returns it with identical slice.
    Expected: PASS with exact vector bytes preserved.
    Evidence: .sisyphus/evidence/embed-2-repo.txt

  Scenario: Capture tolerates embed failure
    Tool: Bash
    Steps: Capture with fake embedder returning error → repo receives Upsert with empty embedding; no error propagated up.
    Expected: PASS.
    Evidence: .sisyphus/evidence/embed-2-capture-softfail.txt
  ```

  **Commit**: NO

- [x] 3. Similarity-based `InWindowRelevance`

  **What to do**:
  - Add `SimilarityInWindowRelevance` (or rename file `conversation_relevance_sim.go` alongside the existing LLM version) implementing the same `InWindowRelevance` interface.
  - Config: `SimilarityInWindowConfig{ Embedder embedder.Embedder; Threshold float64; MinContext int }`. Defaults: Threshold=0.55, MinContext=1.
  - Behavior:
    1. If `Embedder` nil → return `(false, errors.New("embedder unavailable"))` (upstream keeps legacy LLM path if wired).
    2. If `len(context) < MinContext` → `(false, nil)` (no false positives in empty rooms).
    3. Embed current event content via `Embedder`.
    4. For each context message, use its `ContentEmbedding` if present; else fall back to on-the-fly embed. All vectors are already L2-normalized (T1 invariant).
    5. Compute cosine similarity as dot product against a context centroid (mean of normalized context vectors, renormalized). Return `(sim >= threshold, nil)`.
  - Keep the old LLM-based `InWindowRelevance` as `LLMInWindowRelevance` (rename) so `cmd/iris-bot/main.go` can choose either at wire time.
  - Attach `llm.ContextMeta{TriggerReason: "conversation_lock_relevance"}` is no longer meaningful for the embedding path; instead add a DEBUG log line `sim=X threshold=Y decision=Z` gated on `cfg.Debug`.
  - Tests:
    - `TestSimilarityInWindow_AboveThreshold_True` (deterministic vectors).
    - `TestSimilarityInWindow_BelowThreshold_False`.
    - `TestSimilarityInWindow_EmptyContext_False`.
    - `TestSimilarityInWindow_EmbedError_ReturnsFalseErr`.
    - `TestSimilarityInWindow_UsesStoredEmbeddingsWhenPresent` (fake embedder counts calls; should be exactly 1 call for current message if all context has stored embeddings).

  **Must NOT do**: Do NOT re-embed messages that already have `ContentEmbedding`. Do NOT call the LLM. Do NOT return panics on empty vectors.

  **Recommended Agent Profile**:
  - Category: `unspecified-high`.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: [5] | Blocked By: [1,2]

  **References**:
  - Pattern: `internal/orchestrator/conversation_relevance.go` (LLM version — keep as `LLMInWindowRelevance`).
  - Pattern: `internal/orchestrator/cross_channel.go` for error fallback shape.

  **Acceptance Criteria**:
  - [ ] All similarity-based tests pass deterministically with a fake embedder.
  - [ ] No LLM port reference in the new file.
  - [ ] Uses stored embeddings when available, minimizing embedder calls.

  **QA Scenarios**:
  ```
  Scenario: Relevant above threshold
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'SimilarityInWindow_AboveThreshold' -count=1 -v
    Expected: PASS; returns (true, nil) at sim=0.7 threshold=0.55.
    Evidence: .sisyphus/evidence/embed-3-above.txt

  Scenario: Irrelevant below threshold
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'SimilarityInWindow_BelowThreshold' -count=1 -v
    Expected: PASS; returns (false, nil) at sim=0.3 threshold=0.55.
    Evidence: .sisyphus/evidence/embed-3-below.txt
  ```

  **Commit**: NO

- [x] 4. Similarity-based `CrossChannelClassifier`

  **What to do**:
  - Add `SimilarityCrossChannelClassifier` alongside the existing LLM version. Same `CrossChannelClassifier` interface.
  - Config: `SimilarityCrossChannelConfig{ Store, Allowed, Embedder, Threshold, MaxCandidates, WindowMinutes }`. Defaults: Threshold=0.55, MaxCandidates=10, WindowMinutes=30.
  - Behavior:
    1. Query candidates via `Store.ListByUserAcrossChannels(guildID, userID, WindowMinutes, 20)` (unchanged rule).
    2. Apply existing filters: exclude current channel, bot messages, and channels failing `Allowed.IsAllowed` in include-list mode.
    3. Embed the current event content once.
    4. For each candidate, use `ContentEmbedding` if present; else embed on-the-fly.
    5. Compute cosine similarity; keep candidates with sim ≥ threshold, sorted descending by similarity, truncated to `MaxCandidates`.
    6. Return the kept subset. Empty → nil, nil.
  - Rename the old LLM class to `LLMCrossChannelClassifier` without changing its behavior so wire-time can select either.
  - Tests mirror the LLM classifier tests + one new one: `TestSimilarityCrossChannel_OrdersByScore` asserts the top-K are the highest-similarity ones.

  **Must NOT do**: Do NOT relax allow-list guards. Do NOT scan all channels globally.

  **Recommended Agent Profile**:
  - Category: `unspecified-high`.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: [5] | Blocked By: [1,2]

  **References**:
  - Pattern: `internal/orchestrator/cross_channel.go` (keep logic for filtering; swap classification).
  - Pattern: `internal/orchestrator/cross_channel_test.go` for fake-LLM pattern — reuse fake-embedder style.

  **Acceptance Criteria**:
  - [ ] All existing cross-channel behavioral tests pass against the new similarity implementation (allow-list guard, bot filter, current-channel exclusion, empty → nil).
  - [ ] Top-K ordering asserted.
  - [ ] No LLM port reference.

  **QA Scenarios**:
  ```
  Scenario: Relevant candidate merged
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'SimilarityCrossChannel.*Merge' -count=1 -v
    Expected: PASS.
    Evidence: .sisyphus/evidence/embed-4-merge.txt

  Scenario: Allow-list guard still excludes unallowed channels
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'SimilarityCrossChannel.*Allowed' -count=1 -v
    Expected: PASS.
    Evidence: .sisyphus/evidence/embed-4-guard.txt
  ```

  **Commit**: NO

- [x] 5. Runtime wiring, config, and e2e

  **What to do**:
  - `internal/config/config.go`: add fields `EmbedModelPath string`, `EmbedTokenizerPath string`, `EmbedSimThreshold float64`. Parse env `IRIS_EMBED_MODEL_PATH`, `IRIS_EMBED_TOKENIZER_PATH`, `IRIS_EMBED_SIM_THRESHOLD` (default 0.55; malformed → default).
  - `cmd/iris-bot/main.go`:
    - If both embed paths are set, construct `embedder.NewONNX(...)`. On error, log warn and fall back to LLM classifiers (preserve current behavior).
    - Wire capture adapter with the embedder so messages get stored with embeddings.
    - When embedder is available: construct `SimilarityInWindowRelevance` and `SimilarityCrossChannelClassifier` and pass them to orchestrator.
    - When embedder is unavailable: construct legacy `LLMInWindowRelevance` and `LLMCrossChannelClassifier` (renamed).
  - `.env.example`: document all three new envs plus the fallback behavior.
  - `scripts/regression.sh`: no new network step; extend only to run `go vet` and `go test -p 2 -count=1 ./...` (if not already).
  - Add e2e test `internal/orchestrator/orchestrator_sim_e2e_test.go`:
    - Seed fake rolling context, fake embedder with hand-crafted vectors.
    - Send mention → reply + refresh (existing).
    - Send non-mention in-context → reply (similarity above threshold), refresh.
    - Send non-mention off-topic → no reply (similarity below threshold).
    - Assert embedder call counts (reuse cached context embeddings).

  **Must NOT do**: Do NOT change existing LLM_MODEL_* envs. Do NOT break the admin/exception/allow-list flows. Do NOT delete the LLM classifier types.

  **Recommended Agent Profile**:
  - Category: `unspecified-high`.

  **Parallelization**: Can Parallel: NO | Wave 4 | Blocks: [Final] | Blocked By: [3,4]

  **References**:
  - Pattern: `cmd/iris-bot/main.go` existing embedder/router/orchestrator wiring block.
  - Pattern: `.env.example` existing tiered LLM envs.

  **Acceptance Criteria**:
  - [ ] `go test -p 2 -count=1 ./...`, `go build ./...`, `go vet ./...` pass.
  - [ ] `IRIS_EMBED_SIM_THRESHOLD=0.8` overrides default and is observable via `Config`.
  - [ ] With embedder disabled, legacy LLM path still works end-to-end (regression).
  - [ ] New e2e test demonstrates similarity-based conversation lock path.

  **QA Scenarios**:
  ```
  Scenario: E2E similarity-based sliding window
    Tool: Bash
    Steps: go test ./internal/orchestrator -run 'E2E.*Sim' -count=1 -v
    Expected: Mention replies + refreshes; on-topic follow-up replies; off-topic follow-up ignored.
    Evidence: .sisyphus/evidence/embed-5-e2e.txt

  Scenario: Embedder disabled falls back to LLM
    Tool: Bash
    Steps: go test ./internal/config -run 'EmbedPath.*Missing' -count=1 -v && grep -n "LLMInWindowRelevance" cmd/iris-bot/main.go
    Expected: Config defaults OK; main.go constructs the legacy classifier when paths are empty.
    Evidence: .sisyphus/evidence/embed-5-fallback.txt
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
- No LLM calls are made for in-window relevance or cross-channel classification when the embedder is available.
- Iris still responds to mentions/replies/name triggers; sliding window behavior still works.
- Fallback to LLM classifiers is automatic when the embedder isn't configured.
- Embeddings persist and are reused across the rolling window to minimize inference cost.
- Full Go test/build/vet suite passes.
