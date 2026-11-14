---
status: draft
system: office
created: 2026-11-14
owners:
  - kandev
---

# Office Stale-Claim Recovery for Live Executions Requirements

## Overview

The runs processor returns a run to `queued` when it has stayed `claimed` for
longer than the stale-claim age (30 minutes). For a task-bound run, the claim
normally ends when the agent session's turn completes. A session that works for
longer than the stale-claim age is still alive when the sweep runs, so the run
is claimed and launched a second time. The second launch fails because the
first execution still owns the task session, and the routed launch path parks
the run under `blocked_provider_action_required`. Nothing lifts that park, and
the parked row keeps counting against the agent's concurrent-session limit
([#5299](https://github.com/kdlbs/kandev/issues/5299)).

Office owns the stale-claim sweep. The orchestrator owns task sessions and
their agent runtime, and answers whether a session is still live.

## Terminology

- **Stale claim** - a run in `claimed` whose `claimed_at` is older than the
  stale-claim cutoff.
- **Live execution** - the run's task session is `STARTING`, or it is `RUNNING`
  and its agent runtime answers a liveness probe.

## Requirements

### REQ-OFFICE-STALE-CLAIM-001: Stale-claim recovery keeps live executions

**Intent:** A long agent turn is not an abandoned claim. Recovery must not
start a second launch for a task session that is still running.

#### Acceptance criteria

- **AC-OFFICE-STALE-CLAIM-001.1:** When a task-bound run has a stale claim and
  its task session has a live execution, the sweep shall keep the run `claimed`
  and move its `claimed_at` to the sweep time instead of returning it to
  `queued`.
- **AC-OFFICE-STALE-CLAIM-001.2:** The session checked shall be the session
  recorded on the run, or the run's (task, agent) session when none is
  recorded. A session of another task or agent shall never keep the claim.
- **AC-OFFICE-STALE-CLAIM-001.3:** When the execution is no longer live, the
  probe fails, or no probe is configured, the sweep shall return the run to
  `queued` exactly as before. Taskless runs keep the existing sweep.
- **AC-OFFICE-STALE-CLAIM-001.4:** Renewing a claim shall be conditional on
  the run still being `claimed` with a claim older than the cutoff, so a
  concurrent completion, cancellation, or requeue is never overwritten.
