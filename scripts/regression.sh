#!/usr/bin/env bash
# regression.sh - run the full offline regression suite for iris-bot.
#
# Verifies build, vet, and tests. Does NOT require Discord or OpenAI credentials.
# Live smoke tests live in scripts/live-smoke.sh and are gated by env vars.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "=== go mod tidy check ==="
go mod download

echo "=== go vet ==="
go vet ./...

echo "=== go build ==="
go build ./...

echo "=== go test ./... ==="
go test ./... -count=1

echo "=== migration files smoke check ==="
if [ -f migrations/001_init.sql ] && [ -f migrations/002_channel_context.sql ]; then
  echo "✓ Both migration files present (001_init.sql, 002_channel_context.sql)"
else
  echo "✗ Missing migration files"
  exit 1
fi

echo "=== doc checks ==="
if [ -x docs/scripts/check-docs.sh ]; then
  bash docs/scripts/check-docs.sh
fi
if [ -x docs/scripts/check-persona-claims.sh ]; then
  bash docs/scripts/check-persona-claims.sh
fi

echo "regression: OK"
