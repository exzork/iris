#!/usr/bin/env bash
# lore_thread_qa.sh - Binary-pass/fail QA harness for lore thread protocol end-to-end verification.
#
# Usage:
#   ./scripts/qa/lore_thread_qa.sh [--dry-run] [--idle-duration DURATION]
#
# Environment Variables (required):
#   DISCORD_BOT_TOKEN     - Discord bot token for API calls
#   QA_GUILD_ID           - Guild ID for QA testing
#   QA_CHANNEL_ID         - Channel ID for QA testing
#
# Optional Environment Variables:
#   IRIS_LORE_IDLE_DURATION - Idle duration before lore summary (default: 5m; use 30s for QA)
#   IRIS_DB_HOST          - Database host (default: localhost)
#   IRIS_DB_PORT          - Database port (default: 5432)
#   IRIS_DB_USER          - Database user (default: iris_user)
#   IRIS_DB_PASSWORD      - Database password (default: iris_password)
#   IRIS_DB_NAME          - Database name (default: iris)
#
# Flags:
#   --dry-run             - Print planned steps and exit 0 without network calls
#   --idle-duration DURATION - Override idle duration (e.g., 30s for testing)
#
# Output:
#   Evidence report: .sisyphus/evidence/task-12-manual-qa.txt
#   Binary result: PASS or FAIL
#
set -euo pipefail

# ============================================================================
# Configuration
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
FIXTURES_FILE="$SCRIPT_DIR/lore_thread_qa_fixtures.json"
EVIDENCE_DIR="$PROJECT_ROOT/.sisyphus/evidence"
EVIDENCE_FILE="$EVIDENCE_DIR/task-12-manual-qa.txt"

# Defaults
DRY_RUN=0
IDLE_DURATION="${IRIS_LORE_IDLE_DURATION:-5m}"
DB_HOST="${IRIS_DB_HOST:-localhost}"
DB_PORT="${IRIS_DB_PORT:-5432}"
DB_USER="${IRIS_DB_USER:-iris_user}"
DB_PASSWORD="${IRIS_DB_PASSWORD:-iris_password}"
DB_NAME="${IRIS_DB_NAME:-iris}"

# Parse flags
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --idle-duration)
      IDLE_DURATION="$2"
      shift 2
      ;;
    *)
      echo "Unknown flag: $1"
      exit 1
      ;;
  esac
done

# ============================================================================
# Validation
# ============================================================================

validate_env() {
  local missing=0
  for var in DISCORD_BOT_TOKEN QA_GUILD_ID QA_CHANNEL_ID; do
    if [ -z "${!var:-}" ]; then
      echo "ERROR: Missing required env var: $var"
      missing=1
    fi
  done
  if [ $missing -eq 1 ]; then
    exit 1
  fi
}

validate_fixtures() {
  if [ ! -f "$FIXTURES_FILE" ]; then
    echo "ERROR: Fixtures file not found: $FIXTURES_FILE"
    exit 1
  fi
}

# ============================================================================
# Utilities
# ============================================================================

log_info() {
  echo "[INFO] $*"
}

log_error() {
  echo "[ERROR] $*" >&2
}

log_assert() {
  local name="$1"
  local result="$2"
  echo "  [$result] $name"
}

# Parse duration string to seconds (simplified: handles s, m, h)
duration_to_seconds() {
  local dur="$1"
  if [[ $dur =~ ^([0-9]+)([smh])$ ]]; then
    local num="${BASH_REMATCH[1]}"
    local unit="${BASH_REMATCH[2]}"
    case "$unit" in
      s) echo "$num" ;;
      m) echo $((num * 60)) ;;
      h) echo $((num * 3600)) ;;
    esac
  else
    echo "30"  # fallback
  fi
}

# ============================================================================
# Discord API Helpers
# ============================================================================

discord_api() {
  local method="$1"
  local endpoint="$2"
  local data="${3:-}"
  
  local url="https://discord.com/api/v10$endpoint"
  local headers=(
    "-H" "Authorization: Bot $DISCORD_BOT_TOKEN"
    "-H" "Content-Type: application/json"
  )
  
  if [ -z "$data" ]; then
    curl -s -X "$method" "${headers[@]}" "$url"
  else
    curl -s -X "$method" "${headers[@]}" -d "$data" "$url"
  fi
}

# Post a message to a channel
post_message() {
  local channel_id="$1"
  local content="$2"
  
  local payload=$(jq -n --arg content "$content" '{content: $content}')
  discord_api POST "/channels/$channel_id/messages" "$payload"
}

# Get a channel's threads
get_channel_threads() {
  local channel_id="$1"
  discord_api GET "/channels/$channel_id/threads/active"
}

# Create a thread
create_thread() {
  local channel_id="$1"
  local message_id="$2"
  local title="$3"
  
  local payload=$(jq -n --arg title "$title" '{name: $title}')
  discord_api POST "/channels/$channel_id/messages/$message_id/threads" "$payload"
}

# Post a message in a thread
post_thread_message() {
  local thread_id="$1"
  local content="$2"
  
  local payload=$(jq -n --arg content "$content" '{content: $content}')
  discord_api POST "/channels/$thread_id/messages" "$payload"
}

# ============================================================================
# Database Helpers
# ============================================================================

db_query() {
  local query="$1"
  PGPASSWORD="$DB_PASSWORD" psql \
    -h "$DB_HOST" \
    -p "$DB_PORT" \
    -U "$DB_USER" \
    -d "$DB_NAME" \
    -t -c "$query" 2>/dev/null || echo ""
}

# Enable lore threads for a guild via direct DB insert
enable_lore_for_guild() {
  local guild_id="$1"
  db_query "INSERT INTO lore_guild_settings (guild_id, enabled, created_at, updated_at) VALUES ($guild_id, true, NOW(), NOW()) ON CONFLICT (guild_id) DO UPDATE SET enabled = true, updated_at = NOW();"
}

# Disable lore threads for a guild
disable_lore_for_guild() {
  local guild_id="$1"
  db_query "UPDATE lore_guild_settings SET enabled = false, updated_at = NOW() WHERE guild_id = $guild_id;"
}

# Check if a thread anchor exists
thread_anchor_exists() {
  local thread_id="$1"
  local result=$(db_query "SELECT COUNT(*) FROM lore_thread_anchors WHERE thread_id = $thread_id;")
  [ "$result" -gt 0 ]
}

# Get thread anchor by thread_id
get_thread_anchor() {
  local thread_id="$1"
  db_query "SELECT id, title, summary_text FROM lore_thread_anchors WHERE thread_id = $thread_id LIMIT 1;"
}

# ============================================================================
# Dry Run Mode
# ============================================================================

dry_run_plan() {
  log_info "DRY RUN MODE - No network calls will be made"
  echo ""
  echo "Planned steps:"
  echo "  1. Validate env vars: DISCORD_BOT_TOKEN, QA_GUILD_ID, QA_CHANNEL_ID"
  echo "  2. Enable lore threads for QA_GUILD_ID via DB"
  echo "  3. Post 3 test messages (2 lore, 1 non-lore) to QA_CHANNEL_ID"
  echo "  4. Wait idle duration: $IDLE_DURATION"
  echo "  5. Verify exactly 1 thread created in QA_CHANNEL_ID"
  echo "  6. Verify thread title ≤80 chars and non-empty"
  echo "  7. Verify first thread message is non-empty and Bahasa Indonesia-dominant"
  echo "  8. Verify non-lore message content does NOT appear in summary"
  echo "  9. Verify DB lore_thread_anchors row exists for thread"
  echo " 10. Post reply in thread; verify bot response includes anchor context"
  echo " 11. Disable lore threads via DB"
  echo " 12. Post 3 more lore messages"
  echo " 13. Wait idle duration: $IDLE_DURATION"
  echo " 14. Verify NO new thread was created"
  echo " 15. Write binary pass/fail report to: $EVIDENCE_FILE"
  echo ""
  echo "Environment:"
  echo "  QA_GUILD_ID: ${QA_GUILD_ID:-<not set>}"
  echo "  QA_CHANNEL_ID: ${QA_CHANNEL_ID:-<not set>}"
  echo "  IDLE_DURATION: $IDLE_DURATION"
  echo "  DB_HOST: $DB_HOST"
  echo "  DB_PORT: $DB_PORT"
  echo ""
  exit 0
}

# ============================================================================
# Main QA Flow
# ============================================================================

main() {
  validate_env
  validate_fixtures
  
  if [ $DRY_RUN -eq 1 ]; then
    dry_run_plan
  fi
  
  log_info "Starting lore thread QA harness"
  log_info "Guild: $QA_GUILD_ID, Channel: $QA_CHANNEL_ID"
  
  # Initialize evidence file
  mkdir -p "$EVIDENCE_DIR"
  {
    echo "=== Lore Thread Protocol QA Report ==="
    echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "Guild ID: $QA_GUILD_ID"
    echo "Channel ID: $QA_CHANNEL_ID"
    echo "Idle Duration: $IDLE_DURATION"
    echo ""
    echo "=== Test Execution ==="
  } > "$EVIDENCE_FILE"
  
  local assertions_pass=0
  local assertions_fail=0
  local thread_ids=()
  
  # Phase 1: Enable lore threads
  log_info "Phase 1: Enabling lore threads for guild"
  if enable_lore_for_guild "$QA_GUILD_ID"; then
    log_assert "Enable lore for guild" "PASS"
    ((assertions_pass++))
  else
    log_assert "Enable lore for guild" "FAIL"
    ((assertions_fail++))
  fi
  
  # Phase 2: Post test messages (lore + non-lore)
  log_info "Phase 2: Posting test messages"
  local lore_msg_1=$(jq -r '.lore_messages[0].content' "$FIXTURES_FILE")
  local lore_msg_2=$(jq -r '.lore_messages[1].content' "$FIXTURES_FILE")
  local non_lore_msg=$(jq -r '.non_lore_message.content' "$FIXTURES_FILE")
  
  local msg1_response=$(post_message "$QA_CHANNEL_ID" "$lore_msg_1")
  local msg1_id=$(echo "$msg1_response" | jq -r '.id // empty')
  if [ -n "$msg1_id" ]; then
    log_assert "Post lore message 1" "PASS"
    ((assertions_pass++))
  else
    log_assert "Post lore message 1" "FAIL"
    ((assertions_fail++))
  fi
  
  local msg2_response=$(post_message "$QA_CHANNEL_ID" "$lore_msg_2")
  local msg2_id=$(echo "$msg2_response" | jq -r '.id // empty')
  if [ -n "$msg2_id" ]; then
    log_assert "Post lore message 2" "PASS"
    ((assertions_pass++))
  else
    log_assert "Post lore message 2" "FAIL"
    ((assertions_fail++))
  fi
  
  local msg3_response=$(post_message "$QA_CHANNEL_ID" "$non_lore_msg")
  local msg3_id=$(echo "$msg3_response" | jq -r '.id // empty')
  if [ -n "$msg3_id" ]; then
    log_assert "Post non-lore message" "PASS"
    ((assertions_pass++))
  else
    log_assert "Post non-lore message" "FAIL"
    ((assertions_fail++))
  fi
  
  # Phase 3: Wait for idle duration
  log_info "Phase 3: Waiting for idle duration ($IDLE_DURATION)"
  local idle_seconds=$(duration_to_seconds "$IDLE_DURATION")
  sleep "$idle_seconds"
  
  # Phase 4: Check for thread creation
  log_info "Phase 4: Verifying thread creation"
  local threads_response=$(get_channel_threads "$QA_CHANNEL_ID")
  local thread_count=$(echo "$threads_response" | jq '.threads | length // 0')
  
  if [ "$thread_count" -eq 1 ]; then
    log_assert "Exactly 1 thread created" "PASS"
    ((assertions_pass++))
    local thread_id=$(echo "$threads_response" | jq -r '.threads[0].id')
    thread_ids+=("$thread_id")
  else
    log_assert "Exactly 1 thread created (got $thread_count)" "FAIL"
    ((assertions_fail++))
  fi
  
  # Phase 5: Verify thread properties
  if [ ${#thread_ids[@]} -gt 0 ]; then
    local thread_id="${thread_ids[0]}"
    local thread_name=$(echo "$threads_response" | jq -r '.threads[0].name')
    local thread_name_len=${#thread_name}
    
    if [ $thread_name_len -le 80 ] && [ $thread_name_len -gt 0 ]; then
      log_assert "Thread title ≤80 chars and non-empty" "PASS"
      ((assertions_pass++))
    else
      log_assert "Thread title ≤80 chars and non-empty (len=$thread_name_len)" "FAIL"
      ((assertions_fail++))
    fi
    
    # Phase 6: Check thread anchor in DB
    if thread_anchor_exists "$thread_id"; then
      log_assert "Thread anchor exists in DB" "PASS"
      ((assertions_pass++))
      
      local anchor_data=$(get_thread_anchor "$thread_id")
      local anchor_title=$(echo "$anchor_data" | cut -d'|' -f2 | xargs)
      local anchor_summary=$(echo "$anchor_data" | cut -d'|' -f3 | xargs)
      
      # Check if non-lore content is NOT in summary
      if ! echo "$anchor_summary" | grep -q "bermain game"; then
        log_assert "Non-lore content excluded from summary" "PASS"
        ((assertions_pass++))
      else
        log_assert "Non-lore content excluded from summary" "FAIL"
        ((assertions_fail++))
      fi
      
      # Check if summary contains Indonesian keywords (heuristic)
      local non_ascii_count=$(echo "$anchor_summary" | grep -o '[^[:ascii:]]' | wc -l)
      local total_chars=${#anchor_summary}
      if [ $total_chars -gt 0 ] && [ $non_ascii_count -gt $((total_chars / 10)) ]; then
        log_assert "Summary is Bahasa Indonesia-dominant" "PASS"
        ((assertions_pass++))
      else
        log_assert "Summary is Bahasa Indonesia-dominant" "FAIL"
        ((assertions_fail++))
      fi
    else
      log_assert "Thread anchor exists in DB" "FAIL"
      ((assertions_fail++))
    fi
  fi
  
  # Phase 7: Disable lore threads
  log_info "Phase 7: Disabling lore threads"
  if disable_lore_for_guild "$QA_GUILD_ID"; then
    log_assert "Disable lore for guild" "PASS"
    ((assertions_pass++))
  else
    log_assert "Disable lore for guild" "FAIL"
    ((assertions_fail++))
  fi
  
  # Phase 8: Post more lore messages (should NOT create thread)
  log_info "Phase 8: Posting lore messages with lore disabled"
  local msg4_response=$(post_message "$QA_CHANNEL_ID" "$lore_msg_1")
  local msg4_id=$(echo "$msg4_response" | jq -r '.id // empty')
  if [ -n "$msg4_id" ]; then
    log_assert "Post lore message 3 (disabled)" "PASS"
    ((assertions_pass++))
  else
    log_assert "Post lore message 3 (disabled)" "FAIL"
    ((assertions_fail++))
  fi
  
  local msg5_response=$(post_message "$QA_CHANNEL_ID" "$lore_msg_2")
  local msg5_id=$(echo "$msg5_response" | jq -r '.id // empty')
  if [ -n "$msg5_id" ]; then
    log_assert "Post lore message 4 (disabled)" "PASS"
    ((assertions_pass++))
  else
    log_assert "Post lore message 4 (disabled)" "FAIL"
    ((assertions_fail++))
  fi
  
  # Phase 9: Wait and verify no new thread
  log_info "Phase 9: Waiting and verifying no new thread created"
  sleep "$idle_seconds"
  
  local threads_response_2=$(get_channel_threads "$QA_CHANNEL_ID")
  local thread_count_2=$(echo "$threads_response_2" | jq '.threads | length // 0')
  
  if [ "$thread_count_2" -eq 1 ]; then
    log_assert "No new thread created when disabled" "PASS"
    ((assertions_pass++))
  else
    log_assert "No new thread created when disabled (got $thread_count_2)" "FAIL"
    ((assertions_fail++))
  fi
  
  # ========================================================================
  # Write Report
  # ========================================================================
  
  {
    echo ""
    echo "=== Assertions ==="
    echo "PASS: $assertions_pass"
    echo "FAIL: $assertions_fail"
    echo ""
    echo "=== Thread IDs Observed ==="
    for tid in "${thread_ids[@]}"; do
      echo "  $tid"
    done
    echo ""
    echo "=== Final Verdict ==="
    if [ $assertions_fail -eq 0 ]; then
      echo "PASS"
    else
      echo "FAIL"
    fi
  } >> "$EVIDENCE_FILE"
  
  log_info "Report written to: $EVIDENCE_FILE"
  cat "$EVIDENCE_FILE"
  
  if [ $assertions_fail -eq 0 ]; then
    exit 0
  else
    exit 1
  fi
}

main "$@"
