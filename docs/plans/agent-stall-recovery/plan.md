---
spec: docs/specs/agent-stall-recovery/spec.md
decision: docs/decisions/2026-07-29-agent-stall-user-controlled-recovery.md
created: 2026-07-29
status: draft
---

# Implementation Plan: Agent Stall Recovery

## Overview

Turn the lifecycle watchdog's existing inactivity observation into one
generation-scoped internal event, persist that event as an actionable warning,
and make the warning visible while the session is still `RUNNING`. The existing
cancel-escalation path remains the sole recovery mechanism; no timeout changes
session or process state automatically.

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
  ID/name/title/status.
- Add the payload and publisher support in
  `apps/backend/internal/agent/runtime/lifecycle/event_types.go`,
  `apps/backend/internal/agent/runtime/lifecycle/events.go`, and
  `apps/backend/internal/events/types.go`.

### Persist the actionable warning

- Extend `apps/backend/internal/orchestrator/watcher/watcher.go` with the typed
  `agent.stalled` subscription and wire it in
  `apps/backend/internal/orchestrator/service.go`.
- Add `apps/backend/internal/orchestrator/event_handlers_stall.go` to create a
  warning status message without changing task/session state.
- Use only the tool display title or name in copy. Metadata carries
  `action_visibility: running` and one `ws_request` action for `agent.cancel`
  with `session_id`, label **Cancel turn**, destructive styling, and stable
  `stall-cancel-turn-button` test ID.
- If message persistence fails, log the error and leave the running prompt
  untouched.

---

## Frontend

### Running-session action warning

- Update
  `apps/web/components/task/chat/messages/action-message.tsx` so ordinary
  recovery messages keep their current settled-session visibility, while
  `action_visibility: running` messages render only during `RUNNING`.
- Add the optional visibility metadata contract to
  `apps/web/components/task/chat/types.ts`.
- Reuse the existing `ActionButtons` responsive presentation and add a
  pause/cancel icon mapping if needed. The action remains compact on desktop
  and full-width with a minimum 44px height on phones.

### Mobile design contract

- **Desktop outcome:** an amber warning in the chat footer explains the stall
  and exposes a compact **Cancel turn** button.
- **Mobile entry point and outcome:** the same warning appears in the active
  chat footer; **Cancel turn** is a visible full-width touch action and returns
  the session to input-ready state.
- **Nearest shipped exemplar:** `ActionMessage` supplies the responsive action
  card and `mobile-pause-resume-recovery.spec.ts` supplies the touch cancellation
  flow.
- **Presentation:** inline in the existing chat footer; no drawer or new
  navigation layer is needed for a frequent, single-step recovery action.
- **Hierarchy and geometry:** warning copy precedes the primary recovery action;
  chat remains the sole scroll owner, existing safe-area behavior is retained,
  and the action uses the shared 44px mobile button treatment.
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
- **What:** the orchestrator persists one warning with sanitized tool copy and
  the exact `agent.cancel` action without transitioning session state.
  **File:**
  `apps/backend/internal/orchestrator/event_handlers_stall_test.go` and
  `apps/backend/internal/orchestrator/watcher/watcher_test.go`.
  **How:** direct handler test with the existing message-creator fake plus a
  memory-bus watcher dispatch test.
- **What:** running-only action messages render during `RUNNING`, hide after
  settlement, and do not change ordinary recovery-message visibility.
  **File:**
  `apps/web/components/task/chat/messages/action-message.test.tsx`.
  **How:** rendered store-backed component tests.

## E2E Tests

- **Scenario:** a running session with a persisted stall warning exposes
  **Cancel turn**, and activating it makes the session input-ready without a
  backend restart.
  **File:**
  `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`.
  **What to verify:** warning copy, action request, warning/status disappearance,
  idle composer, and unchanged task URL.
- **Scenario:** the same recovery is reachable by touch on a phone viewport.
  **File:**
  `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts`.
  **What to verify:** full-width action is at least 44px high, `.tap()` settles
  the session, and the document has no horizontal overflow.
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

- [ ] [Task 01: Detect and publish stalled prompts](task-01-detect-stalled-prompt.md)

Wave 2:

- [ ] [Task 02: Persist an actionable stall warning](task-02-persist-stall-warning.md)

Wave 3:

- [ ] [Task 03: Render the warning during running sessions](task-03-render-running-warning.md)

Wave 4:

- [ ] [Task 04: Prove desktop and mobile recovery](task-04-e2e-stall-recovery.md)

## Required workflow verification

After all targeted task checks pass:

1. Run `make fmt`.
2. Run `make typecheck test lint`.
3. Commit the explicit changed paths with a Conventional Commit message.

These broad commands are included because the Improve workflow explicitly
requires them; the task-level commands remain the TDD evidence for each change.

## Risks and non-goals

- Event silence remains ambiguous, so the warning is advisory and cannot alter
  lifecycle state by itself.
- Tool titles are already user-visible display data; raw command arguments must
  not be copied into the warning.
- The warning is persisted once per prompt generation so reloads retain it. It
  remains historical/actionable while that generation is still `RUNNING` and
  disappears when the session settles.
- No schema migration, public HTTP endpoint, configurable timeout, automatic
  process termination, or relaxed `RUNNING` prompt-admission rule is planned.
