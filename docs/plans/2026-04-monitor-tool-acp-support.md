# Monitor Tool Support in the Claude ACP Adapter

**Date:** 2026-04-25
**Status:** proposed
**PR:** TBD
**Decision:** —

## Problem

Newer versions of the Claude Code agent SDK (used via `@agentclientprotocol/claude-agent-acp` ≥ 0.31.0) ship a `Monitor` tool that runs a long-lived background script and streams each stdout line back to the model as a synthetic user turn. Production users running long polling skills (`pr-fixup`, `loop`) report that the kandev chat UI:

1. Shows the `Monitor` tool card as "completed" within 1 second of starting, even though the watch lasts 30+ minutes.
2. Shows nothing at all during the watch period — when the agent stays quiet between events, no chat output reaches the UI.
3. "Avalanches" output when the user types a follow-up message: kandev's `messageQueue` holds the new prompt until `agent.ready` fires, which only happens when the wrapper's `prompt()` call returns at the end of the watch.

Investigation (captured in `acp-debug` JSONL traces, see `acp-debug/claude-acp-prompt-*.jsonl`) showed:

- `claude-agent-acp`'s wrapper keeps `prompt()` alive for the full Monitor lifetime — `session_state_changed=idle` is only emitted once Monitor exits. So `EventTypeComplete` is delayed for the entire watch.
- The wrapper emits the `tool_call_update(Monitor)` with `status=completed` *immediately* after Monitor registers — its `rawOutput` is the literal banner `Monitor started (task X, timeout Yms)…`, **not** a real completion.
- Each Monitor event is injected into the SDK as a "user" turn carrying a `<task-notification><task-id>X</task-id>...<event>Y</event></task-notification>` envelope. The wrapper *suppresses* `user_message_chunk` for plain-text user turns (`acp-agent.js:712-719`), so kandev never sees the events directly.
- The events do leak through indirectly when the model produces an `agent_message_chunk` whose text starts with `Human: <task-notification>...` — the model auto-completing the suppressed user turn.

We want to fix this without modifying `claude-agent-acp`.

## Design

All work lands in **`apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go`** (the ACP transport adapter inside agentctl) plus minor frontend additions. No new database tables, no upstream patches.

### Wire-format quick reference

```
tool_call (Monitor)            _meta.claudeCode.toolName=Monitor, rawInput.command=…
tool_call_update (Monitor)     _meta.claudeCode.toolResponse.taskId=X
                               status=completed
                               rawOutput="Monitor started (task X, timeout Yms)…"   ← REGISTRATION, not completion
agent_message_chunk            text="Human: <task-notification><task-id>X</task-id>
                                       …<event>Y</event></task-notification>"      ← per event, parsed out
… [model output, possibly empty for long stretches] …
[prompt() returns, EventTypeComplete fires]                                          ← real Monitor end
```

### Adapter changes

1. **In-memory tracking** — add `activeMonitors map[string]map[string]string` to `Adapter` (sessionID → taskID → toolCallID), guarded by the existing `Adapter.mu`.

2. **Recognize Monitor registration** in `convertToolCallResultUpdate` (around line 1363):
   - When `_meta.claudeCode.toolName == "Monitor"` AND `rawOutput` matches `^Monitor started \(task (?P<task>[a-z0-9]+),`:
     - Record `activeMonitors[sessionID][taskID] = toolCallID`.
     - Override outgoing `ToolStatus` from `completed` to `in_progress`.
     - Pass through the rawOutput text unchanged so the card still shows the banner.

3. **Parse Monitor events** in `convertMessageChunk` (around line 976):
   - Compile once: `taskNotifRE = regexp.MustCompile((?s)Human:\s*<task-notification>\s*<task-id>([^<]+)</task-id>.*?<event>([^<]*)</event>\s*</task-notification>)`.
   - For each match:
     - Look up `activeMonitors[sessionID][taskID]` → toolCallID. If not found, leave the chunk alone (defensive — preserves text in case the parser ever misses).
     - Emit a synthetic `EventTypeToolUpdate` with the matched toolCallID, status `in_progress`, and the event body appended via the normalized payload's `content` field. Bump an event counter on the Monitor's normalized payload so the title can read e.g. "Monitor (3 events)".
   - Replace the matched block with empty string. If the remaining text is whitespace-only, return `nil` (drop the chunk). Otherwise emit a normal message chunk with the cleaned text.

4. **Defensive `Human:` echo filter** — same function, after parsing: if the cleaned text is `Human:` followed by a partial/unmatched `<` block (i.e., the regex didn't match but the prefix is suspicious), drop. Conservative pattern: `^Human:\s*(<[^>]*)?$`.

5. **Prompt-end sweep** — at the end of `Prompt()` (around line 614, alongside `cancelActiveToolCalls`):
   - For each remaining entry in `activeMonitors[sessionID]`, emit a synthetic terminal `tool_call_update(Monitor)` with `status=completed` and a final `content` line "Monitor exited" (so the card's terminal state has a clear marker).
   - Clear `activeMonitors[sessionID]`.

6. **Replay-time rebuild** for session restart resilience — in `LoadSession`, while `isLoadingSession` is true (line 817 onward), the existing logic *suppresses* replay notifications from going to clients. Add an inverse hook here: parse Monitor `tool_call`s and `tool_call_update`s out of the replay stream and reconstruct `activeMonitors[sessionID]`. Then, immediately after `isLoadingSession` is cleared (line 475), emit a synthetic `tool_call_update` with `status=cancelled` and content "Monitor ended (session restart)" for each entry, and clear the map. This handles the case where the agent process restarted and the actual scripts are dead.

### Backend orchestration changes

In **`apps/backend/internal/orchestrator/event_handlers_agent.go`** (or a sibling, depending on where session-recovery hooks live):

- After `RecoverInstances()` reconnects to a live agentctl, if any Monitor `tool_call`s in the DB are still in `in_progress`, re-broadcast their latest `tool_call_update` so frontends that connected during the gap window converge to the right state. Single SQL scan + WS publish.

### Frontend changes

In **`apps/web/components/task/chat/messages/`**:

- New `monitor-card.tsx` component, rendered when a tool_call has `toolName == "Monitor"`. Renders:
  - Title: `Monitor: <command-summary>` with a count badge (`N events`).
  - Status pill: `watching` | `ended` | `ended (session restart)` driven by latest `status`.
  - Inline event list, scrollable, newest-last.
  - Spinner while `status == "in_progress"`.
- `use-processed-messages.ts` — Monitor `tool_call_update`s with `status=in_progress` are valid activity messages; ensure grouping logic at line 212 doesn't drop them.
- `apps/web/lib/state/slices/session/` — accumulate Monitor events into the tool's normalized payload as they arrive so the card can read them straight from the store.

### Restart matrix

| Scenario | What restarts | How state recovers |
|---|---|---|
| Browser refresh | SPA only | SSR fetches messages from DB, `tool_call.status=in_progress` in DB drives the watching card. New events arrive over the WS once it reconnects. |
| Backend restart | Go backend only | agentctl + adapter in-memory map survive (they live alongside the agent process). WS reconnect flushes the small buffered window. Orchestrator re-broadcasts in-progress Monitors on startup as a belt-and-braces. |
| Agent process restart | agentctl + agent (e.g., container kill) | LoadSession replay rebuilds `activeMonitors`. Stale-sweep at end of replay marks them `cancelled (session restart)`. New prompts start fresh Monitor cards. |

## Test Strategy

### Mock agent extension

`apps/backend/cmd/mock-agent/handler.go` + `script.go` already supports `e2e:tool_use(...)`, `e2e:message(...)`, etc. Add:

- `e2e:monitor_start(taskId, command)` — emits `tool_call(Monitor)` then `tool_call_update(Monitor)` with the registration-banner rawOutput and `_meta.claudeCode.toolResponse.taskId`.
- `e2e:monitor_event(taskId, body)` — emits an `agent_message_chunk` with the `Human: <task-notification>…</task-notification>` envelope.
- `e2e:monitor_end(taskId)` — emits the final terminal update.

These three primitives let tests script the full wire contract without depending on the real Claude Code SDK.

### Go unit tests

Land in **`apps/backend/internal/agentctl/server/adapter/transport/acp/conversion_test.go`** following the existing table-driven `newTestAdapter()` + `drainEvents()` pattern. Cases:

1. Monitor registration update → status rewritten to `in_progress`; `activeMonitors` populated.
2. Plain `agent_message_chunk` text passes through unchanged (regression guard).
3. Single `<task-notification>` block in chunk → synthetic `tool_call_update` with event body; remaining text emitted only if non-empty.
4. Multiple blocks in one chunk → one synthetic update each, in order.
5. `<task-notification>` with unknown taskID → fall through, leave chunk text intact.
6. Two concurrent Monitors → events route to the right toolCallID.
7. `Human:` echo without closing tag → dropped.
8. End of `Prompt()` with active Monitors → terminal updates emitted, map cleared.

In **`load_suppression_test.go`**: replay containing Monitor `tool_call`s rebuilds `activeMonitors`; post-replay sweep emits `cancelled (session restart)` updates.

In **`ordering_race_test.go`**: race between concurrent monitor_event and monitor_end (run with `-race`).

### Playwright E2E

New file **`apps/web/e2e/tests/chat/monitor.spec.ts`** using the `backend` + `apiClient` fixtures from `e2e/fixtures/`. Each test scripts the mock-agent via `monitor_start` / `monitor_event` / `monitor_end` directives.

1. **Watching state visible**: assert Monitor card with `data-testid="monitor-card"`, status text "watching", event count badge updates to 3 over time, raw `Human: <task-notification>` text never appears, terminal `monitor_end` flips status to "ended".
2. **Page reload mid-monitor**: emit start + 2 events, `await page.reload()`, assert "watching" + 2 events still rendered, then emit 3rd event + end and assert they land. (Case A)
3. **Backend restart mid-monitor**: same as above but `backend.restart()`. (Case B)
4. **Agent process restart**: emit start + events, force a fresh `session/new` (mock-agent's `--resume false` path), assert original card flips to "ended (session restart)" and a new card appears for the next start. (Case C)
5. **User message during Monitor**: type a message; assert queue indicator visible; let Monitor end; assert message is delivered as a new prompt and lands in chat history.

### Frontend component tests

Vitest test next to `monitor-card.tsx`: render with `(status, events[])` permutations, snapshot transitions. One case in `use-processed-messages.test.ts`: Monitor `in_progress` tool_call_update doesn't break activity grouping.

### Out of scope

- No integration test against the real `@agentclientprotocol/claude-agent-acp` (too slow, model-flaky). Mock-agent fully covers the wire contract.
- No fuzz tests on the parser; grammar is small and wire format is not user-controlled.

## Implementation Notes

_To be filled after the PR lands. Capture any deviations from this plan, surprises in the wire format from real claude-agent-acp versions, and migration considerations if the Monitor envelope format changes upstream._
