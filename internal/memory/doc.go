// Package memory implements per-user selective memory and per-guild server
// memory (recall, behavior hints, async capture).
//
// # Architecture
//
// The server-memory side borrows the four-role shape from stash
// (https://github.com/alash3al/stash) and maps it onto existing iris
// components:
//
//   - Store:    Postgres channel_messages with content_embedding vector(384),
//               IVFFLAT cosine index, all rows scoped by guild_id.
//   - Embedder: in-process ONNX runtime in internal/embedder (dim 384).
//               Async workers in queue.go and embedding_worker.go fill pending
//               rows.
//   - Brain:    GuildRecallService and BehaviorProfileService retrieve
//               guild-scoped context and per-(guild,user) behavior hints.
//   - Reasoner: ContextBuilder composes prompt context handed to internal/llm.
//
// # Provider boundary contract
//
//   - The chat LLM provider is reused through internal/llm and the standard
//     .env configuration. This package must not introduce a second provider
//     abstraction or carry its own provider keys.
//   - Embeddings are produced locally by internal/embedder (ONNX runtime).
//     This package must not import or call provider embedding SDKs
//     (github.com/sashabaranov/go-openai, github.com/openai/openai-go, etc.).
//     TestMemoryPackage_NoDirectProviderSDKImport in provider_boundary_test.go
//     enforces this at build time by scanning imports with go/parser.
//   - Guild isolation is enforced at the service layer. GuildRecallService
//     refuses GuildID=0 and all queries scope on guild_id. User behavior is
//     scoped by (guild_id, user_id) and filtered through the sensitive-content
//     redactor before persistence.
//
// # Relationship to stash
//
// stash is inspiration only. It is not vendored, not a dependency, and there
// is no API, schema, wire-format, or protocol parity with it. Do not add
// compatibility shims or mirror its HTTP surface. If a stash concept is
// useful here, reimplement it in terms of iris primitives (pgx, the existing
// embedder, the existing llm client).
package memory
