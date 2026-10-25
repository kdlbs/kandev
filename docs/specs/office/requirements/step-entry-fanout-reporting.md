---
status: draft
system: office
created: 2026-09-02
owners:
  - kandev
---

# Step Entry Fan-Out Reporting Requirements

## Overview

`queue_run_for_each_participant` joins every participant failure into one error
and logs it as a single interpolated string. A reader cannot tell which
participant failed, how many matched, or whether it matched nobody. The same
callback also has no stated behaviour for a faulty declaration - an empty role,
a participant with no agent profile, a role matching nobody - and no stated
marker outcome for any of them, so a configuration typo can leave a position
permanently unrunnable.

This requirement is separated from
[`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md),
which discovered it, for the reason
[`step-entry-recovery-scan.md`](step-entry-recovery-scan.md) is separate: the
contract is one callback's, not convergence's. It keeps the
`AC-OFFICE-STEP-ENTRY-DISPATCH-007.*` identifiers it was authored under, so
existing citations continue to resolve.

## Terminology

Terms defined in
[`step-entry-sequence-execution.md`](step-entry-sequence-execution.md) -
**arrival**, **step entry**, **entry identity** - carry the same meaning here.
**Marker** and **marker-bearing kind** are defined in
[`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md).

- **Declaration fault:** a fan-out that cannot enqueue for a participant
  because the declaration or the participant slate is wrong, rather than
  because an enqueue was attempted and failed. The three are fixed by
  AC-OFFICE-STEP-ENTRY-DISPATCH-007.3, 007.4 and 007.5.
- **Enqueue failure:** an enqueue that was attempted for a participant and did
  not succeed. This is the only class that makes a marker `failed`.

## Requirements

### REQ-OFFICE-STEP-ENTRY-DISPATCH-007: The fan-out is correct about what it did

**Intent:** A reader, and the recovery scan, can both tell exactly what the
fan-out matched, enqueued, skipped and failed, and a faulty declaration is
correctable rather than terminal.

**Scope note.** This requirement governs four things a reader might expect to be
separate: what the fan-out reports (007.1, 007.2, 007.6), how it treats a faulty
declaration (007.3, 007.4, 007.5), what order it enumerates participants in
(007.7, 007.8, 007.9), and what marker state it leaves behind (007.10). They are
one requirement because they are one callback's contract for one arrival, and
because each is only meaningful given the others: a count is not observable
without the enumeration order that produces it, and a declaration fault is not
actionable unless the marker state lets the author correct it and re-enter.
Splitting them would let the diagnostics ship without the marker policy and
still satisfy a requirement, while leaving a position permanently unrunnable -
the failure mode this initiative exists to remove.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.1:** When enqueuing a run for one
  participant fails, the system shall attempt every remaining matching
  participant and shall emit one ERROR per failure carrying task id, step id,
  participant id, agent profile id, and cause as discrete fields.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.2:** When the fan-out finishes, whether
  or not participants failed, the system shall emit one INFO carrying task id,
  step id, role, matching-participant count, enqueued count, and
  idempotency-suppressed count as discrete fields.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.3:** When the fan-out matches zero
  participants, the system shall enqueue no run, shall not fail the entry, and
  shall emit a WARNING carrying step id, step name, and role.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.4:** When a matching participant has an
  empty agent profile id, the system shall skip that participant, emit the
  AC-OFFICE-STEP-ENTRY-DISPATCH-007.1 ERROR, and continue. The participant is
  counted in the matching total and not in the enqueued total. It is a
  declaration fault, not an enqueue failure: it shall not make the marker
  `failed` and shall not be recorded as the marker cause, per 007.8 and 007.10.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.5:** When the fan-out is declared with an
  empty or absent role, the system shall enqueue no run and shall emit an ERROR
  carrying workflow id and step id as discrete fields. An empty role shall not
  mean all roles.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.6:** Every record required by this
  requirement and by REQ-OFFICE-STEP-ENTRY-DISPATCH-006 in
  [`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md)
  shall carry its named fields as discrete structured fields, not interpolated
  into a message string.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.7:** The participant enumeration order
  shall be an ordered list of named columns ending in a unique tiebreak column,
  and runs shall be enqueued in that order. That order shall remain
  `Position ASC, AgentProfileID ASC`, which the fan-out implements today.
  `AgentProfileID` is unique across the returned set once role-and-agent
  de-duplication has run, so it is already a valid final tiebreak and no change
  is needed to satisfy the unique-last-column rule. It shall **not** be replaced
  by the participant row id:
  [`review-participant-seats.md`](review-participant-seats.md)
  AC-OFFICE-REVIEW-SEATS-003.3 binds this same fan-out to ascending `position`
  order and then ascending **agent profile identifier** order; that criterion is
  in force, is not amended here, and a row-id tiebreak would silently violate it
  wherever row-creation order and agent-profile-id order diverge. The legacy
  `role ASC` leading key is not reintroduced either: the fan-out filters to one
  role before sorting, so a leading role key is a no-op.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.8:** When a fan-out ends with its marker
  `failed`, the cause recorded on that marker shall be the first **enqueue
  failure** in the AC-OFFICE-STEP-ENTRY-DISPATCH-007.7 order. The marker's own
  cause field is the destination; a cause carried only in a log record shall not
  satisfy this criterion, because the recovery scan reads the marker and not the
  log. The ordering ranges over enqueue failures only: a declaration fault shall
  never be recorded as the marker cause even when it is first in order, because
  by 007.10 it does not make the marker `failed`. The de-duplicated matching-set
  size is reported once, as 007.2's matching-participant count, and shall not be
  restated as a second count under another name.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.9:** A test shall assert the order
  against two participants sharing a position, so changing the tiebreak column
  fails it. The fixture shall create those two participants so that
  agent-profile-id order and row-creation order **disagree**. A fixture whose
  rows are already in agent-profile-id order passes under either tiebreak and
  shall not satisfy this.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.10:** The fan-out's marker outcome shall
  be decided by cause, not by how many runs were enqueued. A zero-participant
  match (007.3), a participant skipped for an empty agent profile id (007.4),
  and an empty or absent role (007.5) are declaration faults: they shall leave
  the marker `done`, with their ERROR and WARNING records as the outcome, and
  this shall hold whether or not other participants in the same fan-out
  enqueued successfully. The marker shall end `failed` only when an enqueue was
  attempted and did not succeed. Correcting the declaration and re-entering the
  step shall therefore be sufficient to recover, and a configuration typo shall
  not leave a position permanently unrunnable.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-007.11:** A declaration fault shall also be
  reported to the calling dispatcher as a successful action. The callback shall
  return no error for a zero-participant match (007.3), a participant skipped for
  an empty agent profile id (007.4), or an empty or absent role (007.5), so the
  `StepEntryActionResult` the dispatcher records agrees with the `done` marker
  007.10 requires for the same action. Returning an error while the marker ends
  `done` shall not satisfy this: the action's two observable outcomes would
  disagree, and nothing would say which the recovery scan or a reader should
  believe. An **enqueue failure** keeps today's behaviour and shall be returned
  as an error, agreeing in turn with the `failed` marker 007.10 requires for it.
  This criterion governs the return value only; the ERROR and WARNING records
  those faults emit are unchanged and remain required.

## Out of scope

- **Quorum's participant set.** De-duplication and enumeration order apply to
  the fan-out only; quorum evaluation (`computeGuardOutcome`,
  `EvaluateStepQuorum`) reads its own set and is unchanged.
- **A per-participant retry path.** A participant whose enqueue failed is
  retried only by the whole-action retry of
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.7 in
  [`step-entry-recovery-scan.md`](step-entry-recovery-scan.md).

## Prior art

Carried from
[`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md),
whose Prior art section records the wiki and saas-kb legs in full. The
load-bearing position is `concepts/agent-replay-non-idempotence.md` (0.91,
`lifecycle: draft`): re-running an *author* is not idempotent. That is why a
declaration fault leaves the marker `done` rather than `failed` - a `failed`
marker invites a retry, and retrying a fan-out that never had a valid
declaration would re-run authors once the declaration is corrected.
