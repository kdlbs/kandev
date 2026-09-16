---
status: current
system: agents
requirements:
  - REQ-AGENTS-SESSION-CEILING-001
---

# Session Concurrency Ceiling System Design

## Purpose and boundaries

The orchestrator owns one admission controller shared by manual starts,
resumes, workflow starts, queue drains, and dynamic relaunches. The controller
does not own task metadata or provider execution. It returns a reservation or a
typed refusal. The task repository owns the durable deferred-launch record.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SESSION-CEILING-001` | [Admission and replay](#admission-and-replay), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `sessionCeilingController` | Counts persisted `STARTING`/`RUNNING` sessions and process-local reservations. It admits, refuses, confirms, and releases launches. |
| Orchestrator launch seams | Pass the launch origin and complete replay payload to the controller. They consume a reservation only after launch success. |
| `deferCeilingRefusal` | Merges the ceiling-owned keys into task `deferred_launch` with compare-and-set semantics. |
| Ceiling sweep | Lists tasks with ceiling records, validates eligibility, and dispatches the stored launch kind. |
| Task repository and service | Store and update the shared deferred record. Prompt edits update both legacy top-level data and the nested ceiling payload. |
| Executor callbacks | Confirm or release only when the callback execution still owns the session row. |

## Data and contracts

The shared record uses `ceiling_deferred`, `ceiling_launch_kind`,
`ceiling_launch_payload`, `ceiling_launch_origin`, `ceiling_reason_code`, and
`ceiling_queued_at`. The payload is nested so replay does not confuse a resume,
prompt ensure, or dynamic relaunch with a task start. A different pending
payload returns `ErrCeilingLaunchConflict`; the existing record stays unchanged.

The controller resolves `KANDEV_MAX_CONCURRENT_SESSIONS` once at startup. An
unset or invalid value uses half the host CPU count, with a minimum of two. Zero
means unlimited. The value is an environment-only startup setting.

## Admission and replay

The flow is:

```text
launch request
  -> orchestrator seam
  -> sessionCeilingController.admit
  -> reservation, or typed refusal
  -> deferCeilingRefusal(task CAS) when automatic refusal
  -> executor launch and callback
  -> confirm/release reservation
  -> ceiling sweep replays the stored kind when capacity is available
```

Manual origins bypass refusal and write a manual-override audit entry. Automatic
origins never become manual during replay. The sweep clears a record only after
successful dispatch. A repeated refusal leaves the original timestamp and
payload in place.

## Failure and recovery

Reservation ownership is session-keyed. A stale process-start callback first
checks the persisted session execution identity; it cannot release a successor's
reservation. A dynamic relaunch reports three outcomes: succeeded, deferred, or
failed. A deferred detached relaunch leaves the automation run and its durable
record for the sweep. Queue dispatch treats a seam-3 refusal as retryable so the
original message remains in the queue.

Office automatic starts do not use workflow-step auto-start eligibility when the
sweep evaluates a `start` record. This keeps Office scheduling ownership in the
Office path.

## Persistence

`deferred_launch` is updated with a read-compare-write retry loop. The ceiling
keys share the task record with other launch intents and are removed as a group
after replay or a terminal drop. A prompt edit preserves unrelated keys and
updates the nested replay payload.

## Security

The controller receives launch data from trusted backend paths. The nested
payload is excluded from plugin host data. No prompt or environment value is
executed during replay; it is passed to the same existing launch seam after
task and session eligibility checks.

## Observability

Admission decisions, refusal reasons, reservation expiry, replay drops, and
manual overrides use the existing orchestrator logs and task status messages.
The stored reason and population snapshot make a refusal diagnosable after a
restart.
