---
status: draft
system: office
created: 2026-09-06
owners:
  - kandev
---

# Office: Run Provenance Classification Requirements

## Overview

[Office: Pre-Launch Budget Enforcement](budget-enforcement.md) decides whether a
run launches. Four of its criteria, and the whole of [Built-In Default Spend
Ceiling](budget-default-ceiling.md), turn on whether the run is *attended* or
*unattended*. This document owns that classification: what it reads, what it
returns for every possible input, and the build-time test that stops a new run
reason from silently changing a run's provenance.

It is separated from the admission decision because it is the decision's
**input**, not one of its gates: classification happens once, before any policy
is read, and no gate may revise it. Keeping it here also keeps the admission
document inside its size limit, which three consecutive rounds of edits have
pressed against.

## Terminology

This capability's shared vocabulary, including *run provenance*, *attended run*,
*unattended run* and the polarity warning about `IsPeriodicTasklessWake`, is
defined in the [Office glossary](../glossary.md) under **Budget enforcement**.

## Requirements

### REQ-OFFICE-BUDGET-007: A run's provenance is a total function of its reason

**Intent:** Every criterion that treats attended and unattended runs differently
needs an answer for every run, including a run carrying a reason the build has
never seen. A classifier with a gap would hand those runs to whichever branch the
implementation happened to fall through to, which is how a safety gate acquires
an unexamined default.

The safe default here is the opposite of the one the idle-skip gate uses on the
same column, and that inversion is the single most likely implementation
mistake; it is recorded as a warning in the glossary rather than left to be
rediscovered.

**User story:** As an operator running Office unattended, I want a run's
provenance decided by an explicit list rather than by a fallthrough, so that
adding a wake reason cannot quietly exempt it from the ceiling.

#### Acceptance criteria

- **AC-OFFICE-BUDGET-007.1:** When a run is dequeued for processing, the system
  shall classify its provenance as attended or unattended from `runs.reason`,
  before evaluating any policy. Classification is a total function of the
  reason string: it shall succeed for every value, and no later gate shall
  change the provenance it produced.
- **AC-OFFICE-BUDGET-007.2:** The attended allowlist shall be exactly:
  `task_assigned`, `task_comment`, `task_review_requested`,
  `task_changes_requested`, `task_blockers_resolved`,
  `task_children_completed`, `approval_resolved`, `routine_dispatch_event`,
  `manual_resume_after_failure`, `task_mentioned`, `task_reopened`,
  `task_reopened_via_comment`, `task_unblocked`, `task_ready_to_close`,
  `stage_pending`, `stage_changes_requested`, and the legacy literals
  `review_started`, `approval_started`, `blockers_resolved`,
  `children_completed`. Every other value, **including a value the build does
  not recognize**, shall classify as unattended.
- **AC-OFFICE-BUDGET-007.3:** A test shall fail the build when a run-reason
  string literal exists in the Office source that the provenance classifier
  places in neither the attended allowlist of `AC-OFFICE-BUDGET-007.2` nor the
  explicit unattended list of `AC-OFFICE-BUDGET-007.4`. The test shall derive
  its inventory by reading the source, not from a list of literals maintained
  alongside it, and shall fail on an unclassified literal at build time rather
  than on an unrecognized value at run time.
  **The inventory shall be derived from an enumerated set of declaration
  blocks, and never by matching constant identifiers.** Identifier matching
  cannot work here and shall not be reintroduced: a live run reason is declared
  today under three unrelated naming conventions — `RunReason*`, `WakeReason*`
  and `TaskCommentReason` — so no identifier pattern narrow enough to exclude
  non-run-reason constants also covers all three. The enumerated blocks shall
  be exactly: (a) the run-reason constant block in
  `internal/office/scheduler/run.go`; (b) both constant blocks in
  `internal/office/service/run.go`, the `RunReason*` block and the separate
  `legacyRunReason*` block; (c) the run-reason constant block in
  `internal/office/shared/runreasons.go`; (d) the `WakeReason*` constant block
  in `internal/office/routing/types.go`, which declares three of
  `AC-OFFICE-BUDGET-007.4`'s six unattended literals under a doc comment
  stating that its values must stay in sync with the scheduler constants;
  (e) the constant block in `internal/runs/commentkeys` declaring
  `TaskCommentReason`, a live run reason declared outside `internal/office`;
  (f) the `runReasonTaskAssigned` constant in
  `internal/office/onboarding`; and (g) the constant block in
  `internal/office/service/failure.go` declaring
  `RunReasonManualResumeAfterFailure`. Block (g) is in the same package as
  block (b) but a different file, so enumerating a package's run reasons by one
  file does not reach it. Each block shall be identified by package and
  declaration and never by line number. When an enumerated block cannot be
  located, because it was renamed, moved or deleted, the test shall fail rather
  than pass on the smaller inventory, so that a refactor cannot silently empty
  the net.
  **Within an enumerated block every constant is inventoried by default.** A
  constant in one of those blocks that is not a run reason shall be excluded
  only by being named in an exclusion list carried with the test, which today
  shall be exactly `shared.RoutineSourceCron`, `commentkeys.TaskCommentPrefix`,
  `commentkeys.EngineDispatchedValue`, `InboxKindAgentRunFailed`,
  `InboxKindAgentPausedAfterFails` and `autoPauseReasonPrefix`. The last three
  share block (g) with its one run reason; `InboxKindAgentRunFailed` is an inbox
  kind whose value reads exactly like a run reason, which is why exclusion is by
  name rather than by inferring intent from the value. Inclusion is the default
  so that a reason added to an existing block fails the build until somebody
  classifies it, which is the purpose of this criterion; exclusion is by name so
  that removing a literal from the inventory is a deliberate and reviewable act
  rather than a consequence of how it was spelled. A constant declared as an
  alias of another enumerated constant contributes no new literal.
  **A declaration block outside the enumeration is not covered**, which is the
  cost of enumerating rather than matching, and the test shall carry a comment
  saying so: a run reason declared in an unlisted block will not be caught, and
  extending the enumeration is part of adding such a block to the tree.
  `AC-OFFICE-BUDGET-007.2`'s unattended catch-all remains the runtime safe
  default for exactly that case.
  Membership shall be decided by exact string equality and never by substring,
  so that `task_reopened` is not conflated with `task_reopened_via_comment`,
  and a literal declared in more than one block shall count once. A literal
  appearing in both `AC-OFFICE-BUDGET-007.2`'s allowlist and
  `AC-OFFICE-BUDGET-007.4`'s list shall also fail the test, the two being
  disjoint by construction. This test exists so that adding a reason cannot
  silently change a run's provenance.
- **AC-OFFICE-BUDGET-007.4:** The explicit unattended list
  `AC-OFFICE-BUDGET-007.3` tests against shall be exactly: `agent_error`,
  `budget_alert`, `heartbeat`, `routine_dispatch`, `routine_dispatch_cron`,
  `routine_trigger`. It shall be maintained as an enumeration in its own right
  and shall never be computed as the complement of `AC-OFFICE-BUDGET-007.2`'s
  allowlist. A derived list would make `AC-OFFICE-BUDGET-007.3`'s test a
  tautology that no newly added reason could ever fail, since every literal
  would be a member of one list or the other by construction — which would leave
  the build-time net this requirement exists to provide with nothing to catch.
  This list is a build-time artifact only. It does not narrow
  `AC-OFFICE-BUDGET-007.2`'s runtime rule, under which a value in neither list,
  including one the build has never seen, still classifies as unattended.

## Out of scope

- **Distinguishing an agent-caused event wake from a human-caused one.** Named
  and reasoned about in [budget-enforcement.md](budget-enforcement.md)'s Out of
  scope; `AC-OFFICE-BUDGET-007.2`'s allowlist is what that exclusion constrains.
- **Changing `IsPeriodicTasklessWake` or the idle-skip gate.** That classifier
  reads the same column with the opposite safe default and is left untouched.
- **A causation identifier on the run row.** Provenance here is derived from
  `runs.reason` alone.
