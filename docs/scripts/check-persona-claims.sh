#!/usr/bin/env bash
set -eu
# Fail if docs contain unsupported personality claims (e.g., invented backstory).
# Heuristic: flag lines claiming I.R.I.S is a character with specific traits not backed by wiki.
banned=(
  "I.R.I.S is a human"
  "I.R.I.S loves"
  "I.R.I.S hates"
  "I.R.I.S was born"
  "secretly"
  "canon confirms"
)
flagged=0
for term in "${banned[@]}"; do
  if grep -rInF --include='*.md' "$term" README.md docs/ 2>/dev/null; then
    echo "FLAG: $term"
    flagged=$((flagged + 1))
  fi
done
if [ "$flagged" -gt 0 ]; then
  echo "Persona claim check FAILED: $flagged unsupported claims"
  exit 1
fi
echo "Persona claim check PASSED"
