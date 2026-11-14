---
status: current
system: platform
requirements:
  - REQ-PLATFORM-SILENT-RESTORE-QUEUE-RESUME-001
created: 2026-10-20
owners:
  - kandev
---

# Silent Restore Queue Resume Design

## Boundary and mapping

| Criteria | Implementation boundary |
| -------- | ----------------------- |
| .1       | `features.resumeQueueAfterSilentRestore` read into `orchestrator.ServiceConfig` at startup |
| .2       | `CommitSilentRestore` success, committed already-resumed reconciliation, and legacy recovery success authorize resumption; allocation or a pending readiness callback does not |
| .2       | `restoreSilentRestartCandidate` consumes an existing queued-dispatch drain request after route/lifecycle locks release |
| .3       | `messagequeue.PauseAutoRunForRecovery` and `ResumeAutoRunAfterRecovery` share session admission with explicit policy writes; recovery block ID and exact queue identity retain pause ownership |

## Flow

`pauseSilentRestoreQueue` delegates an identity-bound policy snapshot and
conditional pause to one session admission. A successful pause records its
existing recovery block ID and exact queue identity in a process-local owner
slot. Repeated pauses for that same recovery retain the original ownership;
an already-paused user queue grants none. A policy read failure returns an
error without using the display status API's compatibility fallback.

Successful explicit Auto-run writes and ordinary conditional pauses revoke recovery
ownership under the same admission gate, including an explicit OFF write while
already paused. Resumption checks both owner and identity, reads policy without
a fallback, and enables it within that gate. Rejected stale-identity writes do
not revoke the current owner; a failed resume write retains its owner for an
existing caller's subsequent attempt. A failed or pending restore keeps the
pause. Ownership is not persisted or inferred after a backend restart.

Only the durable commit path resumes a launched candidate. The launch return
value cannot prove native readiness or a committed checkpoint. Already-resumed
and legacy paths resume only after their existing recovery transaction succeeds.
With the flag off, these paths retain the pause.

Successful resumption publishes status and marks the existing queued-dispatch
drain pending. The outer restore entrypoint requests that drain through the
existing owned worker after nested route and lifecycle locks release. Ordinary
queue admission, Auto-run, recovery, promptability and cancellation guards still
decide dispatch. The ceiling sweeper is not an ordinary-message scheduler.

This extends the durable reattachment restore flow in
[durable agent reattachment](durable-agent-reattachment.md).

## Validation

On 2026-10-23, explicit temporary overlays against the published implementation
reproduced newer user pauses being overwritten through both policy APIs,
same-recovery fallback losing the original enabled state, an unreadable saved
OFF policy becoming ON, and an allocated but uncommitted candidate resuming.
The public port also reproduced a committed queue waiting indefinitely without
another event because its ceiling sweep did not include an idle queue drain.

Nine focused probe functions passed with recovery ownership and commit-only
resumption, covering those cases, a different recovery owner, default OFF,
preexisting pauses, and failed commit followed by retry. The real outer restore
entrypoint reached ordinary provider dispatch without another boot event. That
probe was also checked through dispatch completion before fixture cleanup.
Existing selected silent recovery, ordinary dispatch recovery and Auto-run tests
passed in the orchestrator and messagequeue packages. The probes are temporary;
this follow-up adds no standing test or CI job.

Final review also reproduced ownership being revoked by a rejected stale
incarnation write and being lost on a failed resume write. Both failed cases
passed after ownership updates moved behind successful repository writes in the
same admission. A successful same-value OFF write still revoked ownership.
