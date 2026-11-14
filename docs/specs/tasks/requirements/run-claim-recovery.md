---
status: draft
system: tasks
created: 2026-11-13
updated: 2026-11-14
owners:
  - kandev
---

# Run claim recovery requirements

## Overview

An old claim does not prove that its agent stopped. Recovery must preserve a
run while its associated session still performs the work.
The task system owns this contract because it owns the shared runs queue.
Office supplies the session and execution information.

## Requirements

### REQ-TASKS-RUN-CLAIM-RECOVERY-001: Safe recovery of aged claims

**Intent:** Recover abandoned claims without relaunching active work.

#### Acceptance criteria

- **AC-TASKS-RUN-CLAIM-RECOVERY-001.1:** When an associated execution remains
  active, the stale-claim sweep shall leave the run claimed, including after 30 minutes.
- **AC-TASKS-RUN-CLAIM-RECOVERY-001.2:** When the execution check fails or its
  result is uncertain, the sweep shall leave that claim unchanged and log the reason.
- **AC-TASKS-RUN-CLAIM-RECOVERY-001.3:** When an aged claim is abandoned before
  launch, the sweep shall return it to normal queue admission.
- **AC-TASKS-RUN-CLAIM-RECOVERY-001.4:** A preserved run shall retain its
  original claim instant, session, checkout, and launch accounting.
  Its original completion shall finish through the existing lifecycle path.
- **AC-TASKS-RUN-CLAIM-RECOVERY-001.6:** When cancellation, completion, or a new
  claim changes the observed row, recovery shall not overwrite that change.
- **AC-TASKS-RUN-CLAIM-RECOVERY-001.7:** The guard shall protect task-bound and
  taskless runs. Recovery shall retain SQLite and PostgreSQL compatibility.

Criterion numbers retain their earlier meanings. Criterion 001.5 and requirement
002 from the broader draft remain reserved and outside this repair.
They covered separate launch-conflict handling and automatic historical reconciliation.

## Out of scope

- A new recovery framework, execution registry, heartbeat, schema, or run status.
- Automatic scanning or repair of historical parked runs.
- Provider-classification changes for unrelated launch conflicts.
- Changes to concurrency ceilings, retry policy, or maximum session duration.
- A general repair for missed lifecycle events or crashed-session recovery.
- Changes to the reporter's live database.

Existing stranded rows need separate, targeted cleanup after their executions
stop. That cleanup must not report work as completed without completion evidence.

## Related documents

- [Design](../system-design/run-claim-recovery.md).
- [Fix package](../../../plans/office-stale-claim-recovery/plan.md).
- [Shared scheduling](run-scheduling.md).
