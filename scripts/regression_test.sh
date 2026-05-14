#!/usr/bin/env bash
set -euo pipefail
for s in scripts/regression.sh scripts/live-smoke.sh; do
  if [ ! -x "$s" ]; then
    echo "FAIL: $s is not executable or missing"
    exit 1
  fi
done
echo "regression scripts present and executable"
