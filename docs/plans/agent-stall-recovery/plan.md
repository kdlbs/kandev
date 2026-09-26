---
spec: docs/specs/agents/requirements/agent-stall-recovery.md
decision: docs/decisions/2026-08-18-never-started-agent-stall-terminal.md
created: 2026-07-29
status: draft
---

# Implementation Plan: Agent Stall Recovery

## Overview

Turn the lifecycle watchdog's existing inactivity observation into one
generation-scoped internal event, persist that event as an actionable notice,
and make the notice visible while the session is still `RUNNING`. The existing
cancel-escalation path remains the recovery mechanism for turns that already
produced activity. A current zero-event snapshot uses the launch failure path
described by ADR-2026-08-18-never-started-agent-stall-terminal.

## Confirmed root cause

The issue's deterministic reproduction leaves a top-level shell tool
`in_progress` after it starts a long-lived process. No further ACP frames reach
Kandev, so `waitForPromptDone` never receives its completion signal. Its
detached context has no deadline, and the five-minute watchdog branch only logs
before looping. The existing `agent.cancel` path already bounds acknowledgement
waits, force-releases a stuck prompt, and reconciles the session to
`WAITING_FOR_INPUT`; detection is not connected to that user-facing recovery.

---

## Backend

### Track and report a stalled prompt

- Extend `AgentExecution` in
  `apps/backend/internal/agent/runtime/lifecycle/types.go` with
  mutex-protected active top-level tool identity.
- Update `handleToolCallEvent` and `handleToolUpdateEvent` in
  `apps/backend/internal/agent/runtime/lifecycle/manager_events.go` to record a
  top-level tool and clear it on terminal updates. Subagent-internal tool calls
  do not replace the top-level identity.
- Update `waitForPromptDone` in
  `apps/backend/internal/agent/runtime/lifecycle/session.go` to check once per
  60 seconds, detect five minutes of inactivity, and publish `agent.stalled`
  once per prompt generation. The payload carries task/session/execution IDs,
  prompt generation, last-activity time, stalled duration, and optional tool
  ID/name/title/status. Include an activity epoch so the orchestrator can
  reject a snapshot that became stale after an agent event arrived.
- Add the payload and publisher support in
  `apps/backend/internal/agent/runtime/lifecycle/event_types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/events.go`, and
  `apps/backend/internal/events/types.go`.

### Persist the actionable notice

- Extend `apps/backend/internal/orchestrator/watcher/watcher.go` with the typed
  `agent.stalled` subscription and wire it in
  `apps/backend/internal/orchestrator/service.go`.
- Add `apps/backend/internal/orchestrator/event_handlers_stall.go` to create a
  neutral status message without changing task/session state.
- Use only the tool display title or name in copy. Metadata carries
  `action_visibility: running` and one `ws_request` action for `agent.cancel`
  with `session_id`, label **Cancel turn**, neutral styling, and stable
  `stall-cancel-turn-button` test ID. The copy says Kandev is still waiting; it
  does not assert that the tool failed.
- Validate that the session is still `RUNNING` and that the event's immutable
  execution/prompt-generation identity is current before persisting. Assign
  the notice the active `turn_id` without lazily creating a successor turn.
- If message persistence fails, log the error and leave the running prompt
  untouched.
- Never-started notices use settled error metadata and do not expose the
  running-only cancel action.

---

## Frontend

### Compact running-session notice

- Update
  `apps/web/components/task/chat/messages/action-message.tsx` so ordinary
  recovery messages keep their current settled-session visibility, while
  `action_visibility: running` messages render only during `RUNNING` when their
  persisted `turn_id` matches the active turn.
- Add the optional visibility metadata contract to
  `apps/web/components/task/chat/types.ts`.
- Render a running-only message as one inline row: muted `text-xs` copy, no
  alert icon, no warning/error color, no tinted background or border, and one
  neutral **Cancel turn** button at the end.
- Extend the action-button presentation with a compact inline mode. The button
  is content-width at every breakpoint, uses the existing small desktop
  height, and retains a minimum 44px height on phones.

### Mobile design contract

- **Desktop outcome:** one low-emphasis inline status row in the chat footer
  says Kandev is still waiting and exposes a neutral **Cancel turn** button.
- **Mobile entry point and outcome:** the same compact row appears in the active
  chat footer; **Cancel turn** remains inline and content-width, has a 44px
  touch height, and returns the session to input-ready state.
- **Nearest shipped exemplar:** `ActionMessage` supplies the message/action
  behavior and `mobile-pause-resume-recovery.spec.ts` supplies the touch
  cancellation flow. The existing stacked full-width mobile `ActionButtons`
  geometry is intentionally not reused for this low-emphasis notice.
- **Presentation:** inline in the existing chat footer; no drawer or new
  navigation layer is needed for a frequent, single-step recovery action.
- **Hierarchy and geometry:** notice copy truncates before the trailing action;
  chat remains the sole scroll owner, existing safe-area behavior is retained,
  the row does not introduce a card or stacked action area, and the action uses
  the shared 44px mobile touch height without expanding to full width.
- **Shared logic:** message metadata, session-state visibility, and the
  `agent.cancel` handler are identical across viewports.

---

## Tests

- **What:** a top-level `in_progress` tool is retained as stall context while
  subagent tools do not replace it and terminal updates clear it.
  **File:**
  `apps/backend/internal/agent/runtime/lifecycle/manager_events_test.go`.
  **How:** table-driven lifecycle event tests.
- **What:** five minutes without events publishes one generation-scoped
  `agent.stalled` event, does not complete the prompt, and does not duplicate
  on later checks.
  **File:** `apps/backend/internal/agent/runtime/lifecycle/session_test.go`.
  **How:** `testing/synctest` with the real ticker and event bus.
- **What:** the orchestrator persists one neutral notice with sanitized tool
  copy and the exact `agent.cancel` action without transitioning session state.
  **File:**
  `apps/backend/internal/orchestrator/event_handlers_stall_test.go` and
  `apps/backend/internal/orchestrator/watcher/watcher_test.go`.
  **How:** direct handler test with the existing message-creator fake plus a
  memory-bus watcher dispatch test.
- **What:** running-only action messages render as a compact neutral row during
  `RUNNING`, hide after settlement, and do not change ordinary recovery-message
  visibility.
  **File:**
  `apps/web/components/task/chat/messages/action-message.test.tsx`.
  **How:** rendered store-backed component tests.

## E2E Tests

- **Scenario:** a running session with a persisted stall notice exposes
  **Cancel turn**, and activating it makes the session input-ready without a
  backend restart.
  **File:**
  `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`.
  **What to verify:** neutral compact copy, action request, notice
  disappearance, idle composer, and unchanged task URL.
- **Scenario:** the same recovery is reachable by touch on a phone viewport.
  **File:**
  `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts`.
  **What to verify:** the content-width action is at least 44px high, the seeded
  notice remains one compact row, `.tap()` settles the session, and the document
  has no horizontal overflow.
- Seed the `RUNNING` session and actionable status message through the existing
  E2E-only `seedTaskSession` and `seedSessionMessage` helpers. The cancel path's
  missing-live-execution reconciliation provides a deterministic end-to-end
  recovery without waiting five wall-clock minutes; lifecycle timing remains
  covered by the `synctest` regression.

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation. No task is marked
parallel-safe because each task consumes the event or metadata contract created
by the previous one.

Wave 1:

- [x] [Task 01: Detect and publish stalled prompts](task-01-detect-stalled-prompt.md)

Wave 2:

- [x] [Task 02: Persist an actionable stall notice](task-02-persist-stall-warning.md)

Wave 3:

- [x] [Task 03: Render the notice during running sessions](task-03-render-running-warning.md)

Wave 4:

- [x] [Task 04: Prove desktop and mobile recovery](task-04-e2e-stall-recovery.md)

## Required workflow verification

After all targeted task checks pass:

1. Run `make fmt`.
2. Run `make typecheck test lint`.
3. Commit the explicit changed paths with a Conventional Commit message.

These broad commands are included because the Improve workflow explicitly
requires them; the task-level commands remain the TDD evidence for each change.

## Risks and non-goals

- Event silence remains advisory after genuine activity. A current zero-event
  snapshot is the explicit launch-failure exception recorded in the ADR.
- Tool titles are already user-visible display data; raw command arguments must
  not be copied into the notice.
- The notice is persisted once per prompt generation and assigned the active
  turn ID so reloads retain it only for the affected turn. A delayed event is
  rejected when the session has settled or execution/generation ownership has
  moved on; a later `RUNNING` turn cannot resurrect the old notice.
- No schema migration, public HTTP endpoint, configurable timeout, automatic
  process termination, or relaxed `RUNNING` prompt-admission rule is planned.
