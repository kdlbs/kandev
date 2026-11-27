#!/usr/bin/env bash
# Verifies REQ-TASKS-MCP-CREATE-TASK-PROFILE-VALIDATION-001 against a running
# Kandev instance: an unresolvable agent_profile_id is refused and creates
# nothing. Needs no agent and no browser.
set -euo pipefail
BASE=${BASE:?set BASE to the running Kandev instance URL, e.g. http://localhost:28096}
H='Content-Type: application/json'; A='Accept: application/json, text/event-stream'
SID=$(curl -s -X POST "$BASE/mcp" -H "$H" -H "$A" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-03-26","capabilities":{},"clientInfo":{"name":"probe","version":"1"}}}' \
  -D - -o /dev/null | grep -i '^Mcp-Session-Id' | tr -d '\r' | cut -d' ' -f2)
curl -s -X POST "$BASE/mcp" -H "$H" -H "$A" -H "Mcp-Session-Id: $SID" \
  -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' -o /dev/null
call(){ curl -s -X POST "$BASE/mcp" -H "$H" -H "$A" -H "Mcp-Session-Id: $SID" -d "$1"; }
# Prints the refusal and exits nonzero when the call was not refused, so an
# accepted invalid profile fails the probe instead of scrolling past.
refusal(){ python3 -c '
import sys, json
d = json.load(sys.stdin)
r = d.get("result")
if r is None:
    print("no result:", json.dumps(d)[:220]); sys.exit(1)
text = ((r.get("content") or [{}])[0].get("text", "")).strip()
if not r.get("isError"):
    print("accepted, want refused |", text[:220]); sys.exit(1)
print("refused |", text[:220])
'; }

WSID=$(call '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_workspaces_kandev","arguments":{}}}' \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print(json.loads(d["result"]["content"][0]["text"])["workspaces"][0]["id"])')
WF=$(call "{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"list_workflows_kandev\",\"arguments\":{\"workspace_id\":\"$WSID\"}}}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print(json.loads(d["result"]["content"][0]["text"])["workflows"][0]["id"])')
cnt(){ call "{\"jsonrpc\":\"2.0\",\"id\":4,\"method\":\"tools/call\",\"params\":{\"name\":\"list_tasks_kandev\",\"arguments\":{\"workflow_id\":\"$WF\"}}}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print(json.loads(d["result"]["content"][0]["text"]).get("total"))'; }

BEFORE=$(cnt)
echo "tasks before: $BEFORE"
STATUS=0
for V in current_task workspace_default 11111111-2222-3333-4444-555555555555; do
  echo; echo "agent_profile_id=\"$V\""
  if ! call "{\"jsonrpc\":\"2.0\",\"id\":5,\"method\":\"tools/call\",\"params\":{\"name\":\"create_task_kandev\",\"arguments\":{\"title\":\"probe\",\"agent_profile_id\":\"$V\",\"workflow_id\":\"$WF\",\"start_agent\":false}}}" | refusal; then
    STATUS=1
  fi
done

AFTER=$(cnt)
echo; echo "tasks after:  $AFTER"
if [ "$BEFORE" != "$AFTER" ]; then
  echo "FAIL: task count changed from $BEFORE to $AFTER; a refused call created something" >&2
  STATUS=1
fi
if [ "$STATUS" -ne 0 ]; then
  echo "FAIL: create_task_kandev profile validation did not hold" >&2
  exit 1
fi
echo "OK: every invalid agent_profile_id was refused and nothing was created"
