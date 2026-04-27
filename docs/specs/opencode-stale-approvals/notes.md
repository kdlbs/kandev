# Stale OpenCode permission approvals — investigation notes

GH issue: https://github.com/kdlbs/kandev/issues/717

## ACP frame capture

Captured raw ACP wire frames between `acpdbg` and `opencode acp` (v1.14.28) on
2026-04-27. With OpenCode's default permission policy, no
`session/request_permission` is emitted — the agent auto-approves bash/edits
locally. After forcing `permission: { bash: "ask" }` via a workspace
`opencode.json`, OpenCode emits the permission flow as expected.

JSONL: `/tmp/acpdbg-opencode/opencode-acp-prompt-20260427-214819.jsonl`

Observed nominal frame order (per turn, single bash tool call):

1. `session/update` `tool_call` — status `pending`
2. `session/update` `tool_call_update` — status `in_progress`
3. `session/request_permission` — JSON-RPC request awaiting reply
4. (acpdbg auto-replies `-32601 method not found`)
5. `session/update` `tool_call_update` — status `failed` (treated as denial)
6. `session/update` `usage_update`, then turn ends

## Finding

Under happy path, OpenCode posts `session/request_permission` only **between**
the in-progress notification and the terminal status. So the kandev adapter's
existing dedup logic (`activeToolCalls[req.ToolCallID]` lookup) sees the entry
present and skips emitting a synthetic `tool_call`.

The reported symptom — permission cards appearing **after** the user already
approved and the agent finished the turn — is not reproducible with `acpdbg`'s
canned replies, but the gap is real in our flow:

- `convertToolCallResultUpdate` at adapter.go:1701 deletes
  `activeToolCalls[id]` as soon as a terminal `tool_call_update` arrives.
- A subsequent (out-of-order or duplicate) `session/request_permission` for
  that same tool call ID then misses the dedup branch (`alreadyTracked=false`),
  emits a synthetic `tool_call` event with status `pending_permission`, and
  registers a pending message that nothing later resolves.
- `process/manager.handlePermissionRequest` blocks indefinitely on
  `ResponseCh`. There is no turn-complete sweep for permissions analogous to
  `clarification.Canceller`. Even if the user closes the page, the pending
  message persists across reloads.
- `UpdatePermissionMessage` failures from `RespondToPermission` and the cancel
  handler are only `Warn`-logged, so silent drops never surface.

## Fix scope

1. Adapter guard in `acp/adapter.go:handlePermissionRequest`: track recently
   terminal tool-call IDs; on permission request for one, auto-cancel upstream
   and skip emitting any kandev event.
2. Turn-complete sweep: on `agent_complete`, mark any pending
   `permission_request` messages for the session as `expired` (mirroring
   `ClarificationCanceller`).
3. Bump `UpdatePermissionMessage` failure logs from `Warn` to `Error`.

Both defensive and orthogonal — fix #1 prevents new ghost messages, fix #2
recovers from any that slipped through (other agents, prior bugs, restarts).
