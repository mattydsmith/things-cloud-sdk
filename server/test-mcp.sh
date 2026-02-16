#!/usr/bin/env bash
set -euo pipefail

# MCP integration test — exercises all write tools with a named test cycle.
# Usage: ./test-mcp.sh [cycle_name] [base_url]
# Example: ./test-mcp.sh 001 https://things-cloud-mttsmth.fly.dev

CYCLE="${1:-001}"
BASE="${2:-https://things-cloud-mttsmth.fly.dev}"
PREFIX="[test-${CYCLE}]"
ENDPOINT="${BASE}/mcp"

PASS=0
FAIL=0
CREATED_TASK=""
CREATED_PROJECT=""
CREATED_AREA=""
CREATED_TAG=""

# --- helpers ---

mcp_call() {
  local tool="$1" args="$2"
  sleep 1  # avoid Things Cloud 429 rate limiting
  curl -s --max-time 60 "${ENDPOINT}" \
    -X POST \
    -H "Content-Type: application/json" \
    --data-raw "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"${tool}\",\"arguments\":${args}}}"
}

extract_text() {
  python3 -c "import sys,json; r=json.loads(sys.stdin.read()); print(r['result']['content'][0]['text'])" 2>/dev/null || echo ""
}

field() {
  local json="$1" key="$2"
  echo "$json" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('$key',''))" 2>/dev/null || echo ""
}

has_uuid() {
  local json="$1" uuid="$2"
  echo "$json" | python3 -c "
import sys,json
items=json.loads(sys.stdin.read())
print('true' if any(i.get('uuid')=='$uuid' for i in items) else 'false')
"
}

check() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    printf "  \033[32mPASS\033[0m  %s\n" "$label"
    PASS=$((PASS + 1))
  else
    printf "  \033[31mFAIL\033[0m  %s (got: %s, want: %s)\n" "$label" "$got" "$want"
    FAIL=$((FAIL + 1))
  fi
}

# --- test cycle ---

echo ""
echo "=== MCP Write Tool Test Cycle: ${CYCLE} ==="
echo "    Endpoint: ${ENDPOINT}"
echo "    Prefix:   ${PREFIX}"
echo ""

# 1. Create tag
echo "--- Create Tag ---"
RESP=$(mcp_call "things_create_tag" "{\"title\":\"${PREFIX} Tag\",\"shorthand\":\"t${CYCLE}\"}" | extract_text)
CREATED_TAG=$(field "$RESP" uuid)
check "tag created" "$([ -n "$CREATED_TAG" ] && echo ok || echo '')" "ok"
echo "    uuid: ${CREATED_TAG}"

# 2. Create area
echo "--- Create Area ---"
RESP=$(mcp_call "things_create_area" "{\"title\":\"${PREFIX} Area\"}" | extract_text)
CREATED_AREA=$(field "$RESP" uuid)
check "area created" "$([ -n "$CREATED_AREA" ] && echo ok || echo '')" "ok"
echo "    uuid: ${CREATED_AREA}"

# 3. Create project (in area, with deadline)
echo "--- Create Project ---"
RESP=$(mcp_call "things_create_project" "{\"title\":\"${PREFIX} Project\",\"note\":\"Test project notes\",\"when\":\"anytime\",\"deadline\":\"2099-12-31\",\"area\":\"${CREATED_AREA}\"}" | extract_text)
CREATED_PROJECT=$(field "$RESP" uuid)
check "project created" "$([ -n "$CREATED_PROJECT" ] && echo ok || echo '')" "ok"
echo "    uuid: ${CREATED_PROJECT}"

# 4. Create task (in project, with tag, note, deadline, today)
echo "--- Create Task ---"
RESP=$(mcp_call "things_create_task" "{\"title\":\"${PREFIX} Task\",\"note\":\"Test task notes\",\"when\":\"today\",\"deadline\":\"2099-12-31\",\"project\":\"${CREATED_PROJECT}\",\"tags\":\"${CREATED_TAG}\"}" | extract_text)
CREATED_TASK=$(field "$RESP" uuid)
check "task created" "$([ -n "$CREATED_TASK" ] && echo ok || echo '')" "ok"
echo "    uuid: ${CREATED_TASK}"

# 5. Get task — verify fields
echo "--- Get Task ---"
TASK=$(mcp_call "things_get_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "title matches" "$(field "$TASK" title)" "${PREFIX} Task"
check "status is open" "$(field "$TASK" status)" "open"
check "project matches" "$(field "$TASK" project_id)" "${CREATED_PROJECT}"

# 6. Get area
echo "--- Get Area ---"
AREA=$(mcp_call "things_get_area" "{\"uuid\":\"${CREATED_AREA}\"}" | extract_text)
check "area title" "$(field "$AREA" title)" "${PREFIX} Area"

# 7. Get tag
echo "--- Get Tag ---"
TAG=$(mcp_call "things_get_tag" "{\"uuid\":\"${CREATED_TAG}\"}" | extract_text)
check "tag title" "$(field "$TAG" title)" "${PREFIX} Tag"

# 8. Edit task
echo "--- Edit Task ---"
RESP=$(mcp_call "things_edit_task" "{\"uuid\":\"${CREATED_TASK}\",\"title\":\"${PREFIX} Task (edited)\",\"note\":\"Updated notes\"}" | extract_text)
check "edit ok" "$(field "$RESP" status)" "updated"

TASK=$(mcp_call "things_get_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "title updated" "$(field "$TASK" title)" "${PREFIX} Task (edited)"

# 9. Move to someday
echo "--- Move to Someday ---"
RESP=$(mcp_call "things_move_to_someday" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "move to someday" "$(field "$RESP" status)" "moved_to_someday"

# 10. Move to anytime
echo "--- Move to Anytime ---"
RESP=$(mcp_call "things_move_to_anytime" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "move to anytime" "$(field "$RESP" status)" "moved_to_anytime"

# 11. Move to inbox
echo "--- Move to Inbox ---"
RESP=$(mcp_call "things_move_to_inbox" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "move to inbox" "$(field "$RESP" status)" "moved_to_inbox"

# 12. Move to today
echo "--- Move to Today ---"
RESP=$(mcp_call "things_move_to_today" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "move to today" "$(field "$RESP" status)" "moved_to_today"

# 13. Complete task
echo "--- Complete Task ---"
RESP=$(mcp_call "things_complete_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "complete ok" "$(field "$RESP" status)" "completed"

TASK=$(mcp_call "things_get_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "status is completed" "$(field "$TASK" status)" "completed"

# 14. Uncomplete task
echo "--- Uncomplete Task ---"
RESP=$(mcp_call "things_uncomplete_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "uncomplete ok" "$(field "$RESP" status)" "uncompleted"

TASK=$(mcp_call "things_get_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "status is open again" "$(field "$TASK" status)" "open"

# 15. Trash task
echo "--- Trash Task ---"
RESP=$(mcp_call "things_trash_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "trash ok" "$(field "$RESP" status)" "trashed"

# 16. Untrash task
echo "--- Untrash Task ---"
RESP=$(mcp_call "things_untrash_task" "{\"uuid\":\"${CREATED_TASK}\"}" | extract_text)
check "untrash ok" "$(field "$RESP" status)" "restored"

# 17. List today — verify task appears
echo "--- List Today ---"
TODAY=$(mcp_call "things_list_today" "{}" | extract_text)
check "task in today" "$(has_uuid "$TODAY" "$CREATED_TASK")" "true"

# 18. List project tasks
echo "--- List Project Tasks ---"
PROJ=$(mcp_call "things_list_project_tasks" "{\"project_uuid\":\"${CREATED_PROJECT}\"}" | extract_text)
check "task in project" "$(has_uuid "$PROJ" "$CREATED_TASK")" "true"

# --- cleanup ---

echo ""
echo "--- Cleanup ---"
mcp_call "things_trash_task" "{\"uuid\":\"${CREATED_TASK}\"}" > /dev/null 2>&1 && echo "    Trashed task"
mcp_call "things_trash_task" "{\"uuid\":\"${CREATED_PROJECT}\"}" > /dev/null 2>&1 && echo "    Trashed project"
echo "    Note: area '${CREATED_AREA}' and tag '${CREATED_TAG}' cannot be trashed via API"

# --- summary ---

echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed (cycle ${CYCLE}) ==="
echo ""

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
