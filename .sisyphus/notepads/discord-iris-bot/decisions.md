
## Task 16: Incremental MediaWiki Ingestion
- Kept ingestion execution model as explicit `RunOnce` batch orchestration to enforce non-blocking, resumable behavior.
- Cursor advancement occurs only after a page completes chunk processing successfully; cursor is not advanced past pages with chunk/embed/store failures.
- Added store-level `ChunkExists` check plus hash dedupe (`Deduper`) to avoid re-indexing existing chunks and minimize repeated embedding work.
