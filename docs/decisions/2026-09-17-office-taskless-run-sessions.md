# ADR-2026-09-17-office-taskless-run-sessions: Run-owned Office sessions

**Status:** accepted
**Date:** 2026-09-17
**Area:** backend

## Context

Lightweight routines intentionally enqueue runs without task IDs. The scheduler
specification describes fresh sessions for those runs, but both concrete and
provider-routed launch paths require a task. The September 17 investigation
confirmed repeated coordinator failures before any model invocation. The user
explicitly chose genuine taskless coordinator sessions over creating visible tasks.

Task sessions have task ownership constraints. Runtime admission, event consumers,
resource recovery, and Office pause currently assume task sessions in several
places. Removing the scheduler guard alone cannot deliver a working lifecycle.

## Decision

Office owns durable run sessions, separately from task sessions. A taskless launch
creates neither a visible nor a hidden task. Each execution attempt has a fresh
session identity bound to the exact workspace, Office agent, run and attempt.
The shared agent runtime remains the only process/executor owner.

Extend runtime admission with an explicit, trusted execution-owner discriminator
and an owner-specific admission provider. Existing task owners retain their
current checks. Run-owned admission verifies the durable Office session, claimed
run, workspace pause state and cancellation intent. It must not fall back to task
admission or bypass admission after a failed lookup.

Office lifecycle projection accepts exact run/session/attempt identity; it must
not infer a current taskless run from whichever run an agent happens to hold.
Task-specific consumers do not mutate task state for run-owned events. Runtime
process startup, prompt delivery, skills, permissions, executor handling and
provider classification continue through the existing runtime implementation.

## Consequences

A separate Office session table and lifecycle adapter are required, including
cleanup and usage attribution. Runtime interfaces must express owner identity
without importing Office packages. The Office design owns the vertical
behavior; shared runtime code implements the admission seam. Existing task
tables, task-session foreign keys and task-only APIs stay strict.

**Amended 2026-09-19.** Pause (operator stop controls) and restart
reconciliation, both listed above as required, were cut from
`REQ-OFFICE-TASKLESS-001` on 2026-09-19 after five rounds of spec review and
deferred rather than delivered. See
[Deferred: stop controls and restart recovery](../specs/office/requirements/taskless-run-sessions.md#deferred-stop-controls-and-restart-recovery)
for the accepted gap and the brief a follow-up would start from.

A successful taskless turn ends that session. The next fire or retry starts a new
session and consumes only the existing bounded continuation summary. Persisted
failed runs from the old unsupported-launch path remain history; they are not
silently replayed on upgrade.

## Alternatives Considered

- Visible tasks per fire: rejected by the user and inconsistent with lightweight routines.
- Hidden synthetic tasks: would move the same invented work into storage and retain task lifecycle coupling.
- Nullable task ownership in all task sessions: would broaden task API, cleanup and workflow invariants beyond this capability.
- Host utility inference: intended for sessionless utility completions, not a managed Office tool-using run with its own lifecycle.

See [taskless sessions](../specs/office/system-design/taskless-run-sessions.md).
