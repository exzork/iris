#!/usr/bin/env bash
set -eu
required=(
  "README.md:## Overview"
  "README.md:## Quickstart"
  "README.md:## Discord Message Content Intent"
  "README.md:## Environment Variables"
  "README.md:## Docker Compose"
  "README.md:## Running Migrations"
  "README.md:## Admin Commands"
  "README.md:## Development"
  "docs/runbook.md:## Prerequisites"
  "docs/runbook.md:## Initial Deployment"
  "docs/runbook.md:## Troubleshooting"
  "docs/runbook.md:## Rotating Secrets"
  "docs/admin-commands.md:!iris help"
  "docs/admin-commands.md:!iris exception add"
  "docs/architecture.md:## Event Flow"
  "docs/architecture.md:## Data Layer"
  "docs/architecture.md:## Lore Retrieval"
  "docs/architecture.md:## Safety"
)
missing=0
for item in "${required[@]}"; do
  file="${item%%:*}"
  marker="${item#*:}"
  if ! grep -qF "$marker" "$file" 2>/dev/null; then
    echo "MISSING: $file :: $marker"
    missing=$((missing + 1))
  else
    echo "OK: $file :: $marker"
  fi
done
if [ "$missing" -gt 0 ]; then
  echo "Doc check FAILED: $missing sections missing"
  exit 1
fi
echo "Doc check PASSED"
