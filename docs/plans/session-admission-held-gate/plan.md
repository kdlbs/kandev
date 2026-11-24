---
status: done
requirements:
  - REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001
system_design:
  - ../../specs/tasks/system-design/bounded-session-queue-admission.md
---

# Session Admission Held-Gate Fix

## Outcome and authority

Fix for [kdlbs/kandev#5469](https://github.com/kdlbs/kandev/issues/5469): a
session's queue admission gate stayed held for over an hour. Every later
resume launch and deferred `move_task` failed with
`queue session admission timed out`.

Implements criteria .5-.7 of `REQ-TASKS-BOUNDED-QUEUE-ADMISSION-001`.

- [Requirements](../../specs/tasks/requirements/bounded-session-queue-admission.md)
- [System design](../../specs/tasks/system-design/bounded-session-queue-admission.md)
- [Work order 01](task-01-bound-lifecycle-waits-under-admission.md)

## Root cause

Confirmed mechanism: the gate releases only when its callback returns. Two
admitted callbacks wait on the per-session lifecycle lock through
`acquireSessionLifecycleLock`, which has no timeout and no context:

- `refreshFocusedLiveSession`
  (`apps/backend/internal/orchestrator/idle_session_focus.go:104`). It runs
  under `focusTaskSession`'s prompt admission whenever the task page is opened
  or focused (`launchResume` with the `session_focus` activation source).
- The workspace idle-suspension revalidation
  (`apps/backend/internal/orchestrator/idle_session_reaper.go:387`). Its 5s
  revalidation context does not bound the lifecycle-lock wait.

While anything holds the lifecycle lock, whichever of these is waiting keeps
admission. Every other admission caller then exhausts its 30s budget. The
blocked callback never logs, which matches the silent hold in the report.

Unconfirmed: what held the lifecycle lock for over an hour. A trace of the
reported first-launch failure and the clarification watchdog found no
long holder: the synchronous relaunch returns once the agent is ready, the
initial prompt is sent in a goroutine, and `sendPrompt` releases its client
lease immediately. The timeout log (.6) shows how long a recurrence has held
the gate.

## Reproduction and regression

Unit level, no runtime: hold the session lifecycle lock, start an admitted
focus refresh or idle revalidation, then call `WithSessionAdmission` for the
same session. Before the fix the second call returns
`ErrSessionAdmissionTimeout` (shortened budget in test). After the fix it is
admitted. Exact tests are named in the work order.

## Dependency order

One sequential work order. No waves, no delegation.

## Risks and exclusions

- Skipping a focus refresh only omits the idle-parking focus marker for that
  open. The next focus or scan retries. Skipping a revalidation leaves the
  candidate for the next reaper tick, which matches the existing
  "candidate will be retried" behavior.
- Excluded: the idle-suspended branch of `focusTaskSession`, which calls
  `ResumeTaskSessionWithOptions` under admission. Bounding the resume path's
  lifecycle lock changes resume ownership and needs its own design. Also
  excluded: `IsAgentReadyForPrompt` blocking on the agentctl client RWMutex,
  and finding the long lifecycle-lock holder. A recurrence needs a goroutine
  dump to name the holder.
- No schema, settings, UI, or public-docs changes. The MCP error code changes
  from `INTERNAL_ERROR` to `UNAVAILABLE` only for the admission-timeout case.
