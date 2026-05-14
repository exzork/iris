# SearXNG Provider Implementation Summary

## Task Completion

Successfully added a dedicated SearXNG provider for the websearch tool, spun up a SearXNG service in docker-compose on the iris-network, and registered the provider in main.go for end-to-end LLM tool invocation.

## Files Changed

### New Files
- `internal/tools/websearch/searxng_provider.go` — SearXNGProvider implementation
- `internal/tools/websearch/searxng_provider_test.go` — 6 comprehensive unit tests
- `docker/searxng/settings.yml` — SearXNG configuration (limiter disabled, JSON format enabled)

### Modified Files
- `cmd/iris-bot/main.go` — Updated websearch registration to prefer IRIS_SEARXNG_URL with fallback to HTTPProvider
- `docker-compose.yml` — Added searxng service, updated bot environment and depends_on
- `.env.example` — Added IRIS_SEARXNG_URL documentation

## Implementation Details

### SearXNGProvider Contract
- **Endpoint**: `GET <BaseURL>/search?q=<query>&format=json`
- **Response**: `{"results":[{"title","url","content","engine",...}],...}`
- **Field Mapping**: Title, URL, Snippet (from content), Source="searxng", Authoritative flag
- **Limit Enforcement**: Truncated in Go after fetch
- **Error Handling**: ErrEmptyQuery, ErrTimeout, ErrProviderFailure, ErrInvalidResponse
- **HTTP Status**: 4xx/5xx treated as provider failure; context.DeadlineExceeded as ErrTimeout

### Docker Compose Integration
- **Service**: `iris-searxng` (searxng/searxng:latest)
- **Port**: 127.0.0.1:8888:8080 (local-only)
- **Config**: ./docker/searxng/settings.yml mounted at /etc/searxng
- **Network**: iris-network (shared with bot)
- **Key Setting**: `limiter: false` (disables rate limiting for programmatic requests)

### Bot Registration
- **Env Var**: IRIS_SEARXNG_URL (default http://searxng:8080 in compose)
- **Fallback**: SEARCH_BASE_URL + SEARCH_API_KEY (HTTPProvider)
- **Timeout**: 10 seconds per request
- **MaxOutput**: 16 KB
- **Log**: "websearch tool registered provider=searxng base_url=..."

## Test Results

### Unit Tests (6 SearXNG-specific)
✓ TestSearXNG_ReturnsParsedResults — field mapping and Authoritative flag
✓ TestSearXNG_EmptyQuery — returns ErrEmptyQuery without hitting server
✓ TestSearXNG_Timeout — server sleep past timeout → ErrTimeout
✓ TestSearXNG_5xx_ReturnsProviderFailure — HTTP 503 → ErrProviderFailure
✓ TestSearXNG_BadJSON_ReturnsInvalidResponse — malformed JSON → ErrInvalidResponse
✓ TestSearXNG_RespectsLimit — limit=2 against 5-result response yields 2 results

### Full Test Suite
✓ go test ./internal/tools/websearch -count=1 -v — PASS (all 23 tests)
✓ go test -p 2 -count=1 ./... — PASS (all packages)
✓ go build ./... — clean
✓ go vet ./... — clean

### Docker Verification
✓ docker compose up -d searxng — service starts
✓ curl http://127.0.0.1:8888/search?q=hello&format=json — returns valid JSON with results
✓ docker compose up -d bot — bot starts and logs "websearch tool registered provider=searxng base_url=http://searxng:8080"

## Evidence Files
- `.sisyphus/evidence/searxng-provider.txt` — unit test output (all PASS)
- `.sisyphus/evidence/searxng-live.txt` — curl JSON output + bot startup logs showing registration

## Learnings Updated
- `.sisyphus/notepads/embedding-classifier/learnings.md` — added "## SEARXNG PROVIDER" section documenting provider contract, compose integration, settings, and verification

## Verification Checklist
- [x] TDD: tests written first, all pass
- [x] HTTPProvider untouched (fallback preserved)
- [x] docker/searxng/settings.yml has limiter: false
- [x] Bot starts even if searxng unreachable (websearch failures bubble up as tool errors)
- [x] No existing compose services removed
- [x] No secrets baked into image
- [x] .sisyphus/plans/ untouched
- [x] CGO-off build compatible (no CGO dependencies added)

## End-to-End Flow
1. Bot starts with IRIS_SEARXNG_URL=http://searxng:8080
2. Registers SearXNG provider with websearch tool
3. LLM can invoke websearch tool via tool-calling pipeline
4. Tool calls SearXNGProvider.Search(ctx, query, limit)
5. Provider makes GET request to http://searxng:8080/search?q=...&format=json
6. SearXNG returns JSON results
7. Provider maps to SearchResult slice
8. Tool formats results as JSON for LLM
9. LLM receives grounded search results for response composition

All requirements met. Ready for production use.
