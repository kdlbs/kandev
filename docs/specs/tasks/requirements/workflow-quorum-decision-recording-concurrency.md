---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Quorum ordering and concurrency Requirements



## Overview



Concurrent verdicts and transition races preserve decision history and apply a guarded transition at most once.

## Requirements


### REQ-TASKS-QUORUM-CONCURRENCY-001: Quorum ordering and concurrency



**Intent:** Concurrent verdicts and transition races preserve decision history and apply a guarded transition at most once.

#### Acceptance criteria

#### Ordering, idempotency, concurrency

- **AC-TASKS-QUORUM-CONCURRENCY-001.1:** THE SYSTEM SHALL order decisions for threshold evaluation by
  `decided_at` ascending, tie-broken by `id` ascending — the existing
  `ListStepDecisions` contract (`phase2_sqlite.go:594`) — and SHALL count only
  the last row per **AC-TASKS-QUORUM-SLATE-001.9 seat** under that ordering, decisions being matched to
  seats by AC-TASKS-QUORUM-SLATE-001.10. This clause governs the approve-style thresholds
  (`all_approve`, `all_decide`, `majority_approve`, `n_approve:<N>`) ONLY;
  `any_reject` is keyed on decider identity and needs no seat, per AC-TASKS-QUORUM-VERDICT-001.3 and
  AC-TASKS-QUORUM-VERDICT-001.4. The scoping is explicit because an unqualified "last row per
  `participant_id`" is precisely the reading that reintroduces the
  discarded-human-rejection defect AC-TASKS-QUORUM-VERDICT-001.3 exists to fix — production funnels every
  threshold through one `latestDecisionsPerParticipant` call today, so reusing
  this rule for `any_reject` is the path of least resistance.
- **AC-TASKS-QUORUM-CONCURRENCY-001.2:** WHEN the same participant records a second decision for the same
  `(task_id, step_id)`, THE SYSTEM SHALL mark the prior row `superseded_at` and
  insert the new one in a single transaction, and the new verdict SHALL replace
  the old one for threshold purposes.
- **AC-TASKS-QUORUM-CONCURRENCY-001.3:** WHEN two decisions are recorded concurrently for the same
  `(task_id, step_id)` by different participants, THE SYSTEM SHALL persist both,
  and at most one guarded transition SHALL be applied for the resulting step
  change. The engine's operation-id mechanism CANNOT provide this:
  `HandleTrigger` dedupes by exact `OperationID` match only
  (`isOperationAlreadyApplied`) and offers no mutual exclusion across distinct
  ids, and two concurrent decisions necessarily observe different decision sets
  and so compute different ids. Mutual exclusion SHALL therefore be enforced at
  apply time per AC-TASKS-QUORUM-CONCURRENCY-001.4, not by the operation id.
- **AC-TASKS-QUORUM-CONCURRENCY-001.4:** WHEN a transition is applied after a quorum re-evaluation, THE
  SYSTEM SHALL apply it only if the task's `workflow_step_id` still equals the
  step the guard was evaluated against, and SHALL abandon the apply otherwise.
  AC-TASKS-QUORUM-CONCURRENCY-001.10 defines the port this needs and why the existing one cannot express it.
  The loser of a concurrent race SHALL report `transition_applied = false` with
  its decision still persisted, and SHALL NOT be reported as an error: losing
  the race is a normal outcome, not a failure.
- **AC-TASKS-QUORUM-CONCURRENCY-001.5:** WHEN two decisions are recorded concurrently by the *same*
  participant for the same `(task_id, step_id)`, THE SYSTEM SHALL leave exactly
  one non-superseded row for that participant.
- **AC-TASKS-QUORUM-CONCURRENCY-001.6:** WHEN a re-evaluation is triggered twice for the same recorded
  decision, THE SYSTEM SHALL apply the transition at most once, keyed on a
  **deterministic** operation id of the form
  `decision:<task_id>:<step_id>:<decision_row_id>`. Determinism is the whole
  point: `Engine.RecordParticipantDecision` currently builds its operation id
  with a `time.Now().UnixNano()` suffix, which yields a fresh id on every call
  and therefore deduplicates nothing. That existing construction SHALL NOT be
  reused as-is. The repo's convention for this primitive is a deterministic
  content-derived key — see `childCompletionOperationID`
  (`orchestrator/event_handlers_children_completed.go`).
- **AC-TASKS-QUORUM-CONCURRENCY-001.7:** THE SYSTEM SHALL NOT accept a client-supplied idempotency key on the
  decision tool. A repeated call is a new verdict and supersedes the previous one
  per AC-TASKS-QUORUM-CONCURRENCY-001.2. Consequence, stated so a builder does not quietly invent a key: an
  agent that retries after a timeout on a call that actually succeeded records a
  second verdict rather than a no-op. This is acceptable because the verdict is
  the same value and AC-TASKS-QUORUM-CONCURRENCY-001.2 leaves exactly one non-superseded row; it is NOT
  acceptable to paper over it with a synthesized key derived from the arguments,
  which would make a deliberate change-of-mind silently fail.
- **AC-TASKS-QUORUM-CONCURRENCY-001.8:** THE SYSTEM SHALL generate the decision row id BEFORE the write and
  pass it in `DecisionInfo.ID`, following the existing office precedent
  (`recordTaskDecision` sets `ID: uuid.New().String()` on the row it hands to
  `RecordStepDecision`). It SHALL NOT follow `Engine.RecordParticipantDecision`,
  which leaves `ID` blank and lets the repository generate one. The
  `DecisionStore.RecordStepDecision(ctx, d DecisionInfo) error` signature is
  UNCHANGED by this feature.
  The two in-tree precedents disagree and only one is usable: the port returns
  no id, so a blank-`ID` write leaves AC-TASKS-QUORUM-CONCURRENCY-001.6's operation id
  `decision:<task_id>:<step_id>:<decision_row_id>` unconstructible, and AC-TASKS-QUORUM-REEVALUATION-001.11
  unsatisfiable since office would have no id to publish. Recovering the id by
  reading the row back after the write is NOT permitted — under the AC-TASKS-QUORUM-CONCURRENCY-001.3
  concurrency this must coexist with, a read-back can select a different
  decider's row for the same `(task_id, step_id)`. Pre-generation also makes the
  operation id available before the write, which is what lets AC-TASKS-QUORUM-REEVALUATION-001.7 mark it
  applied strictly after the apply.
- **AC-TASKS-QUORUM-CONCURRENCY-001.9:** WHEN a task re-enters a guarded step, THE SYSTEM SHALL clear that
  step's decisions before the round's participants are queued, so a new round
  starts with zero decisions — the existing `clear_decisions` ordering on
  `on_enter`. AC-TASKS-QUORUM-REGRESSION-001.6 makes this observable rather than assumed, and names what it
  means when it does not hold.

- **AC-TASKS-QUORUM-CONCURRENCY-001.10:** THE SYSTEM SHALL add exactly one method to the engine's transition
  port — `ApplyTransitionIfAtStep(ctx, taskID, sessionID, expectedStepID,
  toStepID, trigger) (applied bool, err error)` — used ONLY by the AC-TASKS-QUORUM-CONCURRENCY-001.4 quorum
  apply, and SHALL leave the existing `ApplyTransition(...) error` and every
  transition that uses it semantically unchanged. `HandleResult` SHALL gain an
  additive `TransitionAbandoned` field so an abandoned apply is distinguishable
  from both a transition and an error. This is required because AC-TASKS-QUORUM-CONCURRENCY-001.4's outcome
  is not expressible today: `ApplyTransition` receives `fromStepID` and ignores
  it, its orchestrator implementation unconditionally sets
  `task.WorkflowStepID = toStepID`, and a single `error` return cannot separate
  "lost the race" from "the write failed". The compare-and-swap SHALL read the
  task's `workflow_step_id` inside the same transaction that performs the update
  — `updateTaskWithWorkflowStepAdmission` already opens one and already takes the
  workspace and workflow-step locks — so no new locking primitive is introduced
  and no non-quorum transition changes behavior. `applied = true` means the task
  left `expectedStepID`; it does NOT mean the task was WIP-admitted at the
  target, because a step at capacity queues the task there and that is still a
  completed transition.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.