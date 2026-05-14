# Issues - persistent-server-memory

Problems, gotchas, and blockers encountered during execution.

## Task 3: Async Embedding Queue Contracts

### No blocking issues encountered
- Queue implementation is straightforward; no concurrency gotchas.
- Tests all pass on first run.
- LSP diagnostics clean after range loop modernization.

### Future considerations (not blockers)
1. **Queue capacity tuning**: Default 32 may need adjustment based on worker throughput in Task 4.
2. **Enqueue timeout tuning**: 10ms is conservative; may be tunable in config in Task 4.
3. **Metrics integration**: Stats are available but not yet wired to observability; Task 4 can add this.

## 2026-05-12 Task 1 verification note
- `lsp_diagnostics` is clean for changed Go files.
- SQL migration has no configured LSP server in this workspace; syntax validated by direct SQL inspection and idempotent DDL shape.
