---
spec: docs/specs/ui/requirements/quick-terminal.md
created: 2026-08-06
status: draft
---

# Implementation Plan: Quick Terminal Durable Session Lifecycle and Menu Alignment

## Overview

Two user-reported defects remain after the refresh-persistence work shipped:

1. A quick terminal does not survive a page reload in practice. A refreshed tab re-numbers
   ("Terminal 7") and a new shell is created instead of reattaching to the existing one.
2. The plus-button creation menu opens toward the leading (left) edge of the tab strip, overhanging
   the workspace edge, which is awkward.

The persistence infrastructure (durable descriptors, boot payload, resync, reattach by `sessionId`)
already exists and is correct. The reload failure is not a missing-descriptor bug; it is a
**session-lifetime** bug: quick-terminal PTYs share the agent-login `IdleTimeout = 10m` /
`HardTimeout = 30m` in `loginpty`. A quiet or detached terminal is reaped, so on the next boot/resync
`reconcileTab` finds no live session, calls `markUnavailable`, and persists `session_id = NULL` /
`status = exited`. The user then starts a fresh terminal, and `sequence` keeps climbing.

The fix gives quick-terminal host-shell sessions their own lifetime policy: **no idle or hard
timeout**. They end only on explicit tab close (already stops the PTY), natural process exit
(already reported via `HandleSessionExit`), or backend shutdown. To avoid leaking PTYs on shutdown
when the timeout no longer reaps them, the `loginpty.Manager` gains a `StopAll` wired into graceful
shutdown. The menu fix is a one-line alignment change plus coverage.

## Confirmed root cause

- `apps/backend/internal/agent/loginpty/manager.go` `supervise()` applies `IdleTimeout` (10m) and a
  `HardTimeout` (30m) `context.WithTimeout` to **every** session, including host-shell sessions
  started by quick terminals. A quiet or detached quick terminal is terminated by these.
- `apps/backend/internal/quickterminal/service.go` `reconcileTab()` (run by `List`, which boot state
  and GET `/quick-terminal-tabs` both call) sees the reaped session as `!Running` and calls
  `markUnavailable`, persisting `session_id = ""` and `status = exited`. This is why reload shows the
  terminal as gone and a new one is created.
- `apps/web/components/quick-chat/quick-tab-add-menu.tsx` sets `align="end"` on a `w-64`
  `DropdownMenuContent`; the trigger is at the leading edge of the strip, so the menu opens leftward.

## Backend

### Per-session lifetime policy in loginpty

- Add an explicit lifetime policy to `Session`/`StartWithKey` so a session can opt out of the idle
  and hard timeouts. Host-shell sessions (`agentID == HostShellAgentID`) use the no-timeout policy;
  all other agent-login sessions keep the existing 10m/30m behavior. Prefer a small, explicit
  policy value (e.g. a `noTimeout bool` or a `lifetime` struct on the session) set from `agentID`
  inside `StartWithKey`, rather than reading `agentID` string comparisons scattered in `supervise`.
- In `supervise()`, when the policy disables timeouts, do not arm the idle timer and do not wrap the
  context in `WithTimeout`; still handle natural process exit (`exited`) and external `stop()` so
  cleanup, map removal, and `onExit` remain correct. Keep the existing drain/`readDone` behavior.
- Add `Manager.StopAll()` that stops every live session (idempotent), for graceful shutdown.

### Graceful shutdown wiring

- Register a cleanup in `backendapp` that calls `loginMgr.StopAll()` during graceful shutdown, so
  no-timeout quick-terminal PTYs are reaped when the backend stops (they previously relied on the
  30m hard timeout). Follow the existing `addCleanup`/`runCleanups` pattern used for the lifecycle
  manager.

## Frontend

### Menu alignment

- In `quick-tab-add-menu.tsx`, change `align="end"` to `align="start"` on `DropdownMenuContent` so
  the menu opens toward the trailing edge. Preserve the existing mobile bottom-sheet treatment.

### Reload reattachment (verification, likely no behavior change)

- The reattach path already exists (stable `tabId` → `managerKey`, reconcile trusts backend
  `running`). With sessions no longer reaped, confirm no additional frontend change is needed for
  reload reattachment; only add regression coverage. If a residual client-side path still starts a
  fresh shell for a `running` restored descriptor, fix it in the terminal tab view / reconcile so a
  restored live descriptor only attaches by `sessionId`.

## Tests

- **What:** host-shell sessions are not reaped by idle or hard timeout, while agent-login sessions
  still are; `StopAll` stops all live sessions and is idempotent; natural exit and external stop
  still clean up manager maps and fire `onExit` under the no-timeout policy.
  **Files:** `apps/backend/internal/agent/loginpty/manager_test.go`.
  **How:** `testing/synctest` to advance fake time past 10m/30m and assert a host-shell session is
  still running while an agent-login session has exited; direct `StopAll` assertions.
- **What:** `reconcileTab` keeps a still-running host-shell session as `running` with its
  `sessionId` after simulated long idle (no reaping), and only `markUnavailable` when the manager
  truly has no entry.
  **Files:** `apps/backend/internal/quickterminal/service_test.go`.
  **How:** table-driven service tests against a manager holding a live long-idle session and one
  with no entry.
- **What:** graceful-shutdown cleanup invokes `loginMgr.StopAll()`.
  **Files:** the relevant `apps/backend/internal/backendapp/*_test.go` for cleanup wiring.
  **How:** assert the registered cleanup stops a seeded live session.
- **What:** the add menu renders with start alignment and existing items.
  **Files:** an existing `apps/web/components/quick-chat/*` test (extend, do not add a new file
  unless none covers this component).
  **How:** assert the `DropdownMenuContent` alignment prop / rendered side and the two menu items.

## E2E Tests

- **Scenario:** GIVEN a running quick terminal left idle past the old 10m idle window (fast-forward
  or a shortened test policy hook), WHEN the user reloads and reopens Quick Chat, THEN the same tab
  and sequence reattach to the same PTY without a new numbered terminal.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts` (extend the existing reattach spec).
  **What to verify:** single tab, unchanged sequence/label, same-session marker output, and no
  second shell. Close every surviving terminal in `finally`.
- **Scenario:** GIVEN the plus menu on a desktop pointer viewport, WHEN it opens, THEN it opens
  toward the trailing edge and stays within the viewport.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts` or the existing quick-chat menu
  spec.
  **What to verify:** menu bounding box is to the right of the trigger and within the viewport.

## Verification Results

_To be filled in during implementation._

## Implementation Waves And Parallel Candidates

Wave 1 (parallel-safe, independent surfaces):

- [ ] [Task 01: Per-session lifetime policy and StopAll in loginpty](task-01-loginpty-lifetime-policy.md)
- [ ] [Task 02: Fix add-menu alignment](task-02-add-menu-alignment.md)

Wave 2 (depends on Task 01):

- [ ] [Task 03: Shutdown wiring, reconcile coverage, and reload E2E](task-03-shutdown-and-reattach-verification.md)

Task 02 is fully independent of the backend tasks and can land in parallel. Task 03 depends on the
Task 01 policy/`StopAll` surface.

## Risks

- Removing timeouts means a runaway or forgotten shell can live for the entire backend uptime.
  Mitigation: explicit tab close already stops the PTY, natural exit is reported, and `StopAll`
  reaps everything on shutdown. This matches the user's explicit request and stays bounded by
  process lifetime.
- The lifetime policy must not weaken agent-login OAuth reaping. Keep the policy keyed strictly to
  `HostShellAgentID` and assert the agent-login path still times out.
- `supervise` cleanup correctness (map removal, `onExit`) must hold when the idle timer and timeout
  context are absent. Cover natural exit and external stop under the no-timeout policy.
- E2E time-advancement for a 10m idle window is expensive; prefer a test-only shortened policy hook
  or fake-clock injection rather than a real 10-minute wait.
