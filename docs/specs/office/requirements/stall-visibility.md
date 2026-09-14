---
status: active
system: office
created: 2026-09-01
owners:
  - kandev
---

# Office Stall Visibility Requirements

## Overview

An Office task can stop making progress in two ways that no current backend
detector reports. First, an Office session can accept a step-completion signal
and never reach turn-end; the stuck-signal watchdog
(`internal/orchestrator/stuck_signal_watchdog.go`) excludes every Office task by
construction, so the state is invisible. Second, an Office task can sit at a
review or approval step with a decision-required seat, no decision recorded, and
no run in flight; nothing is stalled in the session sense, because there is no
session, so no session-oriented detector can see it.

The Office system owns this contract because the stuck state is defined by
Office primitives: `tasks.is_from_office`, the participant slate, the decision
ledger, and the Office run queue. The orchestrator's stuck-signal watchdog is an
adjacent contract this capability reads and constrains, but does not own.

This capability makes both states **visible**. It does not recover either one.

## Terminology

- **Stranded signal:** a pending `step_complete_kandev` signal recorded in a
  session's metadata bag that matches the task's current step and has not been
  applied.
- **Reclaim:** the stuck-signal watchdog's recovery action, which cancels the
  agent execution, force-closes the stale turn, moves the session to
  `WAITING_FOR_INPUT`, and applies the requested step transition through
  `reconcileStepCompletionSignalLocked`.
- **Surface:** report a stuck state to an operator or user without changing task,
  session, decision, or run state.
- **Decision-waiting task:** an Office task whose current step carries at least
  one `decision_required` reviewer or approver seat with no active decision row
  for that task and step.
- **In-flight run:** a row in the Office `runs` table for the task with status
  `queued` or `claimed`. The table has no terminal `running` status; `claimed`
  is the executing state.

## Requirements

### REQ-OFFICE-STALL-VISIBILITY-001: Surface an Office task holding a stranded signal

**Intent:** An Office task whose session accepted a completion signal and then
went quiet is currently invisible to every backend detector. Make it visible
without granting the watchdog permission to advance an Office step.

**User story:** As an operator, I want a stranded completion signal on an Office
task to be reported, so that I can intervene before the card sits indefinitely.

The stuck-signal watchdog applies its Office exclusion at two independent sites
(`reconcileWaitingStuckSignalSessionIfDue` and `stuckSignalCandidate`). Both are
**recovery** gates: passing either one leads to a reclaim. Removing the
`IsFromOffice` term from either predicate would make the watchdog cancel an
Office agent and apply the step transition it requested, which is the forged
quorum decision that REQ-OFFICE-STALL-VISIBILITY-003 forbids. The exclusion is
therefore preserved for recovery and lifted only for detection.

#### Acceptance criteria

- **AC-OFFICE-STALL-VISIBILITY-001.1:** When an Office task's session holds a
  stranded signal older than the watchdog threshold, the system shall surface it
  exactly once per distinct signal, and shall not reclaim the session.
- **AC-OFFICE-STALL-VISIBILITY-001.2:** When an Office task reaches the
  `stuckSignalCandidate` evaluation path, the system shall surface the stranded
  signal from that path.
- **AC-OFFICE-STALL-VISIBILITY-001.3:** When an Office task reaches the
  `reconcileWaitingStuckSignalSessionIfDue` evaluation path, the system shall
  surface the stranded signal from that path.
- **AC-OFFICE-STALL-VISIBILITY-001.4:** When a task is not an Office task, the
  system shall preserve the existing reclaim behavior with no change in outcome.
- **AC-OFFICE-STALL-VISIBILITY-001.5:** While a surfaced signal remains pending
  and unchanged, the system shall not re-surface it on subsequent scans.

### REQ-OFFICE-STALL-VISIBILITY-002: Surface a decision-waiting Office task

**Intent:** The most common Office stall is a task parked at a review or approval
step with nobody coming. No session is stuck, so no session-oriented detector
observes it.

**User story:** As an operator, I want an Office task that has been waiting on a
decision with no work in flight to be reported, so that a human can decide it.

#### Acceptance criteria

- **AC-OFFICE-STALL-VISIBILITY-002.1:** When an Office task's current step
  carries at least one `decision_required` reviewer or approver seat, has no
  active decision row for that task and step, has no in-flight run, and has been
  in that state for longer than the configured threshold, the system shall
  surface the task as decision-waiting.
- **AC-OFFICE-STALL-VISIBILITY-002.2:** When the task has an in-flight run, the
  system shall not surface it, regardless of decision state or age.
- **AC-OFFICE-STALL-VISIBILITY-002.3:** When an active decision row exists for
  the task and its current step, the system shall not surface it.
- **AC-OFFICE-STALL-VISIBILITY-002.4:** When the current step carries no
  `decision_required` reviewer or approver seat, the system shall not surface it.
- **AC-OFFICE-STALL-VISIBILITY-002.5:** When the in-flight run state cannot be
  read, the system shall not surface the task and shall record the skip.
- **AC-OFFICE-STALL-VISIBILITY-002.6:** When a task is not an Office task, the
  system shall not evaluate it for this detector.

### REQ-OFFICE-STALL-VISIBILITY-003: Never act on a surfaced Office stall

**Intent:** Office quorum gates exist to stop a step advancing without the
decisions it requires. A detector that repaired what it found would forge those
decisions.

**User story:** As a workspace owner, I want stall detection to report and never
remediate, so that an automated sweep cannot manufacture an approval.

#### Acceptance criteria

- **AC-OFFICE-STALL-VISIBILITY-003.1:** When either detector surfaces an Office
  task, the system shall not change the task's workflow step.
- **AC-OFFICE-STALL-VISIBILITY-003.2:** When either detector surfaces an Office
  task, the system shall not write a decision row.
- **AC-OFFICE-STALL-VISIBILITY-003.3:** When either detector surfaces an Office
  task, the system shall not queue a run, cancel an agent, or close a turn.
- **AC-OFFICE-STALL-VISIBILITY-003.4:** When either detector surfaces an Office
  task, the system shall not change any session state.

## Out of scope

- Automatic recovery, step advancement, decision synthesis, or run queueing for
  any Office stall. This is the explicit non-goal above.
- Changing reclaim behavior for non-Office tasks. The existing stuck-signal
  watchdog contract is unchanged for them.
- Making `lastActivityAt` meaningful for passthrough sessions. Passthrough
  remains excluded from both detection and recovery.
- Detecting a long-running Office turn that never signalled at all. That is the
  in-process stall ticker's domain.
- Replacing the workspace-scoped `Stall Session Watchdog` automation. Retiring it
  is a follow-up once backend coverage is confirmed in the field.
