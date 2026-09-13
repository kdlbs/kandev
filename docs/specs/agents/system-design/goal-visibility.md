---
status: draft
system: agents
requirements:
  - REQ-AGENTS-GOAL-VISIBILITY-001
created: 2026-09-13
owners:
  - kandev
---

# Agent Goal Visibility System Design

## Purpose and boundaries

Retain the provider's goal snapshot independently from per-turn activity and expose
it in the shared chat composer status row. Existing ACP/session metadata transport
remains authoritative. This adds no scheduler, provider control request, or database schema.

The [Quick Chat selection design](../../ui/system-design/quick-chat-selection.md)
owns which conversation reopens. This design owns the goal attached to that session.
The [combined fix package](../../../plans/quick-chat-selection/plan.md) delivers both.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-GOAL-VISIBILITY-001` | Provider contract; retention and delivery; composer disclosure; verification |

## Provider contract

The inspected `@agentclientprotocol/codex-acp` 1.11.0 implementation emits
`session_info_update` with `_meta.goal`. Its `ThreadGoalSnapshot` maps provider
statuses to `active`, `paused`, `blocked`, `limited`, or `complete`.
`thread/goal/cleared` emits `goal: null`. Goal snapshots also arrive during session loading.
This is an ACP extension, not a guarantee from every ACP provider.

```json
{
  "sessionUpdate": "session_info_update",
  "_meta": {
    "goal": {
      "objective": "Coordinate contributor PR reviews",
      "status": "active",
      "createdAt": 1789079689000,
      "updatedAt": 1789287777000,
      "tokenBudget": null,
      "tokensUsed": 9547009,
      "timeUsedSeconds": 47312,
      "controlMethod": "_session/goal"
    }
  }
}
```

The active example is a fixture shape, not a claim about the investigated session.
That session's fresh raw and normalized captures reported `complete`.
The bridge can suppress updates when only usage changes, so counters are not live progress.
Do not render them as a timer or progress percentage in this package.

Recognize the documented Codex extension through the adapter's provider identity.
Parse goal data defensively into a typed view with objective, status, and provider
timestamps. Optional usage fields remain optional. Ignore `controlMethod` for UI actions.
Do not execute methods named by arbitrary metadata or inspect native transcript files.

## Retention and delivery

Current flow: `adapter_updates.go` converts the frame to `session_info` and carries
`SessionMeta`. Lifecycle forwarding reaches `handleSessionInfoEvent`, which stores
`metadata.acp` and publishes `SessionInfoEventPayload`. The session broadcaster sends
`session.info_updated`; `registerSessionInfoHandlers` updates the frontend session row.

Both `mergedACPSessionInfo` and the frontend currently replace `meta` on unrelated
updates. A frame containing only `codex.threadStatus` can erase the goal.
Extend this path with a narrow, typed goal-retention rule. Keep the public location
`metadata.acp.meta.goal` and the existing `session_meta` payload. Do not deep-merge
all opaque metadata or create a competing task-level goal store.

| Incoming goal field | Required handling |
| --- | --- |
| Absent | Retain the accepted goal; update unrelated metadata by existing semantics |
| Valid snapshot | Replace the retained goal after freshness checks |
| Explicit null | Retain an explicit cleared marker and hide the chip |
| Malformed or unknown status | Do not present it as active; record bounded diagnostic context without objective text |

Preserve property presence through conversion. A null goal is distinct from an
omitted goal. The backend publishes the full retained goal state after persistence,
including explicit null. The frontend uses the same omission/clear semantics for
sparse events and reads the typed view from the selected session row.

Keep handling ordered per Kandev session and current ACP attachment. Do not reuse
cancellation-intent state as a generic lock. Provider timestamps are Unix milliseconds;
compare valid snapshots by goal creation time and update time. Reject older updates
within the same goal. A newer goal can replace a completed goal.
Clear events have no goal timestamp: apply them in accepted stream order and keep
their local revision so an older HTTP hydration response cannot restore the old value.
Reject events from obsolete attachments before updating the session.

Preserve the existing hydration/live-response freshness boundary. Add deferred tests
for clear or completion followed by a stale HTTP response. A session-info event that
arrives before its session row exists must be recovered by authoritative hydration;
do not create an incomplete session row merely to display the chip.
Retained state survives reconnect and ordinary idle states. A provider session change
invalidates the old provider goal until its authoritative load snapshot arrives.
Historical sessions without a retained goal show no chip until a supported snapshot arrives.

## Composer disclosure

Add `AgentGoalChip` and shared detail content under `components/task/chat/`.
Use `ChatStatusBar.sessionId`, not a global active task/session selector, to resolve it.
Place the chip after `RegisteredChangeRequestStatus` and before the queue chip.
Keep the existing Todos, autopilot, dependency, and PR ordering unchanged.
Include goal visibility in `shouldRenderChatStatusBar` so a goal alone renders the row.

Use a small static target icon and the localized label **Goal Active** only for
`status: active`. Do not use a working spinner. The detail content contains a Goal
heading, active status, the objective as plain text, and this localized explanation:
**The agent may continue automatically between replies.**
Preserve whitespace where useful, wrap long tokens, and do not render objective HTML.

Fine-pointer desktop uses a controlled popover above the trigger. Hover, focus,
and click open the same content; pointer movement into the content keeps it open.
Escape dismisses it without closing Quick Chat. Avoid simultaneous tooltip/popover
copies or a focus-dismiss/reopen loop. Match the surrounding compact status-chip density.

Use `useTouchDrawer` from `hooks/use-compact-task-chrome.ts` and the existing Drawer
primitive for coarse pointers. Phone composition uses the same bottom drawer even
with a fine pointer. Its fixed header has a visible close action; the objective
area owns vertical scrolling inside a safe-area-aware dynamic-viewport limit.
The chip remains after PR information and wraps onto another row when needed.
Keep its touch target at least 44 CSS pixels without enlarging fine-pointer desktop chips.

The nearest exemplars are `TodoIndicator` for status-row placement and shared details,
and `RegisteredChangeRequestStatus` for coarse-pointer disclosure selection.
The approved phone design intentionally uses a drawer rather than copying the Todos popover.
Changing sessions or removing the active goal closes the details. Return focus to
the chip when mounted, otherwise use the surrounding composer's stable focus target.
Expose an accessible name, expanded state, and associated details on the trigger.

## Compatibility and verification

No agent control methods are invoked. Goal state does not replace task state, Todo
progress, or autopilot. Disconnect does not silently complete a retained goal;
the existing connection indicator still describes transport availability.
All fixed copy uses i18n, including the five language catalogs and generated Traditional Chinese.

Backend regression tests prove active -> unrelated metadata -> idle retains the goal,
and complete/null remain effective after later updates. Include malformed input,
all supported statuses, attachment changes, duplicates, and older snapshots.
Frontend tests prove sparse merge, stale hydration, session isolation, and disclosure behavior.
Mock ACP scenarios drive active, unrelated, complete, and clear frames through the real
transport. Desktop and phone E2E verify the chip above the composer, disclosure,
long text, idle persistence, reload recovery, and disappearance on completion/clear.
