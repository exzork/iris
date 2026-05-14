#!/usr/bin/env bash
# live-smoke.sh - optional live smoke tests against real Discord/OpenAI.
# Runs ONLY when IRIS_LIVE_SMOKE=1 is set along with the required credentials.
set -euo pipefail

if [ "${IRIS_LIVE_SMOKE:-0}" != "1" ]; then
  echo "live-smoke: SKIPPED (set IRIS_LIVE_SMOKE=1 to enable)"
  exit 0
fi

for v in DISCORD_TOKEN OPENAI_API_KEY; do
  if [ -z "${!v:-}" ]; then
    echo "live-smoke: SKIPPED (missing env var $v)"
    exit 0
  fi
done

echo "live-smoke: placeholder - add live probes here"
# Intentionally minimal to avoid unintended API usage.
exit 0
