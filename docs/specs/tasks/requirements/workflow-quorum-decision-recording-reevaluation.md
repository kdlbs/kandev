---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Decision-triggered quorum re-evaluation Requirements



## Overview



Recording a verdict causes the current step's guarded transitions to be evaluated with clear success, skip, and failure outcomes.

## Requirements


### REQ-TASKS-QUORUM-REEVALUATION-001: Decision-triggered quorum re-evaluation



**Intent:** Recording a verdict causes the current step's guarded transitions to be evaluated with clear success, skip, and failure outcomes.

#### Acceptance criteria

#### Re-evaluation on record

- **AC-TASKS-QUORUM-REEVALUATION-001.1:** WHEN a decision is recorded through any surface, THE SYSTEM SHALL
  re-evaluate the current step's `on_turn_complete` guarded transitions before
  returning to the caller, using the task's active session as defined in AC-TASKS-QUORUM-REEVALUATION-001.4
  — including on the human HTTP path, which has no calling session of its own.
- **AC-TASKS-QUORUM-REEVALUATION-001.2:** WHEN that re-evaluation finds a satisfied guard, THE SYSTEM SHALL
  apply the transition, and the recording call SHALL still report success.
- **AC-TASKS-QUORUM-REEVALUATION-001.3:** WHEN that re-evaluation fails or errors, THE SYSTEM SHALL keep the
  recorded decision, report the recording as successful, and surface the
  re-evaluation failure through the observability required by AC-TASKS-QUORUM-DIAGNOSTICS-001.2.
- **AC-TASKS-QUORUM-REEVALUATION-001.4:** THE SYSTEM SHALL resolve the re-evaluation session as
  `GetActiveTaskSessionByTaskID` already defines it
  (`task/repository/sqlite/session.go`): the task's most recently started
  session whose state is one of `CREATED`, `STARTING`, `RUNNING`, or
  `WAITING_FOR_INPUT`, ordered by `started_at` descending, limit one. A task is
  "unresolvable" for this purpose exactly when that query returns no row.
- **AC-TASKS-QUORUM-REEVALUATION-001.5:** WHEN no session is resolvable under AC-TASKS-QUORUM-REEVALUATION-001.4, THE SYSTEM SHALL record
  the decision and skip re-evaluation without erroring, matching the existing
  blank-session behavior in `Engine.RecordParticipantDecision`, and SHALL report
  the skip under AC-TASKS-QUORUM-DIAGNOSTICS-001.1 so a card that recorded a verdict but never re-evaluated
  is distinguishable from one that re-evaluated and found the threshold unmet.
- **AC-TASKS-QUORUM-REEVALUATION-001.6:** WHEN two guarded transitions are both satisfied, THE SYSTEM SHALL
  apply the first in the step's configured `on_turn_complete` action order,
  preserving first-transition-wins (`workflow/engine/engine.go:268-283`).

- **AC-TASKS-QUORUM-REEVALUATION-001.7:** THE SYSTEM SHALL run the AC-TASKS-QUORUM-REEVALUATION-001.1 re-evaluation with the engine in
  **committing mode** (`EvaluateOnly = false`), so the engine applies a satisfied
  transition itself rather than returning a payload for a transport to apply.
  Consequently the AC-TASKS-QUORUM-CONCURRENCY-001.6 operation id SHALL be marked applied only after the
  transition has been applied, or deliberately abandoned under AC-TASKS-QUORUM-CONCURRENCY-001.4 — never
  before. This is stated because the two precedents in tree disagree and only one
  is safe: `Engine.RecordParticipantDecision` passes `EvaluateOnly: true` today
  while `office/engine_dispatcher` does not, and `HandleTrigger` calls
  `markOperationApplied` on its success path only. Under committing mode an apply
  failure returns before the mark, so a retry can still land; under evaluate-only
  the id is marked while no transition has been applied, and every retry then
  returns `Idempotent: true` with the transition lost permanently — the exact
  stuck-card failure this feature exists to eliminate. The existing
  `EvaluateOnly: true` on `RecordParticipantDecision` SHALL therefore change.
  Keeping the apply inside the engine is also what `## What` means by ownership
  following the engine. "Committing mode" here names APPLY-OWNERSHIP — the
  engine performs the apply and marks the operation id after it — and NOT reuse
  of the full trigger path; AC-TASKS-QUORUM-REEVALUATION-001.8 bounds what the re-evaluation is allowed to
  run.

- **AC-TASKS-QUORUM-REEVALUATION-001.8:** THE SYSTEM SHALL scope the AC-TASKS-QUORUM-REEVALUATION-001.1 re-evaluation to the step's guarded
  transition actions ONLY. The AC-TASKS-QUORUM-REEVALUATION-001.10 entry point SHALL evaluate those
  `on_turn_complete` actions for which `isTransitionAction` holds and which
  carry a `wait_for_quorum` guard, in configured order, and apply at most one,
  per AC-TASKS-QUORUM-REEVALUATION-001.6 and AC-TASKS-QUORUM-CONCURRENCY-001.4. It SHALL NOT execute non-transition action callbacks, and
  it SHALL NOT be implemented as a plain
  `HandleTrigger(TriggerOnTurnComplete)` call.
  This is stated because the obvious implementation is exactly the wrong one.
  `evaluateActions` guards only actions for which `isTransitionAction` holds and
  routes EVERY other action to `executeCallback` — and that call is not gated on
  `EvaluateOnly`, unlike `applyTransition` and `PersistData`. So a
  HandleTrigger-based re-evaluation re-runs whatever else the step configures on
  `on_turn_complete` — `queue_run`, `clear_decisions`,
  `queue_run_for_each_participant`, `run_code_review`, `create_child_task`,
  `switch_workflow`, `set_workflow_data` are all registered kinds — once per
  RECORDED DECISION rather than once per completed turn. AC-TASKS-QUORUM-CONCURRENCY-001.6's operation id is
  deliberately unique per decision, so the id-dedup does not suppress the repeat
  either. `Office Default` configures only two `move_to_step` actions on
  `on_turn_complete`, so this is invisible on the shipped workflow and on every
  regression in `### Regression`; it would first appear on a workflow that pairs
  a quorum guard with a side-effect action, as `clear_decisions` and
  `queue_run_for_each_participant` already are elsewhere in this same workflow's
  `on_enter`. A reviewer vote is not a turn completion, and this AC is what keeps
  the two from being conflated.
  Boundary: when the current step configures no guarded transition at all, the
  re-evaluation SHALL apply nothing and report `transition_applied = false`
  without erroring. A decision is legitimately recordable at such a step — AC-TASKS-QUORUM-RECORDING-001.3
  gates on participation, not on the presence of a guard — and treating "nothing
  to evaluate" as a failure would reject valid verdicts.

- **AC-TASKS-QUORUM-REEVALUATION-001.9:** THE SYSTEM SHALL implement the AC-TASKS-QUORUM-SLATE-001.9 slate construction, the AC-TASKS-QUORUM-SLATE-001.10
  seat matching, the AC-TASKS-QUORUM-RECORDING-001.4/AC-TASKS-QUORUM-RECORDING-001.5 role resolution and the decision write EXACTLY
  ONCE, in the engine, against the engine's `ParticipantStore` and
  `DecisionStore` ports, and SHALL expose them to both transports through a
  single engine decision entry point. Neither the MCP tool nor
  `office/dashboard` SHALL carry its own copy. This is stated because the
  ownership sentence in `## What` is not satisfiable as written today:
  `DashboardService`'s only engine connection is
  `engineDispatcher shared.WorkflowEngineDispatcher`, whose entire interface
  surface is one `HandleTrigger` method — no slate read, no role resolution, no
  decision write. The office decision path therefore runs through
  `recordTaskDecision` -> `resolveDeciderRole` -> `s.repo.ListAllTaskParticipants`
  in `office/repository/sqlite`, a DIFFERENT Go package from the
  `workflow/repository` one backing the engine's `ParticipantStore`, while both
  read and write the same `workflow_step_participants` table. Left unstated, a
  builder may reasonably implement AC-TASKS-QUORUM-SLATE-001.9's four steps a second time on the
  office side — two canonicalizations over one table, which is precisely the
  read-side/write-side divergence AC-TASKS-QUORUM-SLATE-001.3 exists to prevent.
- **AC-TASKS-QUORUM-REEVALUATION-001.10:** THE SYSTEM SHALL extend `shared.WorkflowEngineDispatcher` with an
  additive **write-side** method that records a decision and performs the AC-TASKS-QUORUM-REEVALUATION-001.1
  re-evaluation, and SHALL wire it at the existing dispatcher construction site
  (`backendapp/main.go`, where `officeenginedispatcher.New` is already built
  from the shared engine instance and the task repo). `HandleTrigger` and every
  existing caller of it SHALL be unchanged. This is the write half of the
  minimum port change that makes AC-TASKS-QUORUM-REEVALUATION-001.9 buildable; without naming it, "call into
  the engine's decision API" has no referent. It is the FIRST of exactly two
  additive methods this feature adds to that interface — AC-TASKS-QUORUM-REEVALUATION-001.14 names the
  second, read-side one. Earlier wording said "one additive method", which read
  as forbidding the read-side method that AC-TASKS-QUORUM-DIAGNOSTICS-001.4 cannot be built without.
- **AC-TASKS-QUORUM-REEVALUATION-001.11:** THE AC-TASKS-QUORUM-REEVALUATION-001.10 entry point's return SHALL carry the decision row id
  and the persisted `decided_at`, because AC-TASKS-QUORUM-REEVALUATION-001.12 promises
  `publishDecisionRecorded` is preserved unchanged and that function publishes
  `"decision_id": d.ID` and `"created_at": d.CreatedAt` off the
  `DecisionRecord`. Once the persist moves behind the entry point, office can no
  longer source those two values itself. Without this the builder must either
  widen the entry point's return undocumented or locally synthesise an id and a
  timestamp, and a locally taken timestamp need not equal the row's stored
  `decided_at` — publishing an event whose `created_at` disagrees with the row
  it describes. AC-TASKS-QUORUM-RECORDING-001.11 observes both fields on the tool surface and they are the
  same two values.
  Concretely: the entry point SHALL stamp `decided_at` once, persist that value,
  and return THAT value — never re-read the clock to build the response. Two
  clock reads around one write are the defect this clause exists to forbid; they
  would put `created_at` on the published event ahead of the row every time,
  which is invisible in review and wrong in every timeline that joins the two.
- **AC-TASKS-QUORUM-REEVALUATION-001.12:** THE SYSTEM SHALL confine the office-side change to the resolve,
  persist and re-evaluate core of `recordTaskDecision`. Its surrounding
  behavior — `publishDecisionRecorded`, `logDecisionActivity`,
  `runReactivityForDecision`, and the `DecisionRecord` shape returned to the
  handler — SHALL be preserved, and every other reader of
  `ListAllTaskParticipants` (task inbox, scheduler reactivity, the task-detail
  handler) SHALL keep today's step-scoped behavior, as `## Out of scope`
  already states. This bounds the refactor: the card repoints one function's
  core, it does not migrate `office/dashboard` onto the engine wholesale.
- **AC-TASKS-QUORUM-REEVALUATION-001.13:** WHEN the engine decision entry point is not wired — `main.go`
  already returns early and logs "workflow engine not initialised; office engine
  dispatcher disabled" when `orchestratorSvc.WorkflowEngine()` is nil, leaving
  `DashboardService.engineDispatcher` nil — THE SYSTEM SHALL reject the decision
  call with an error and write no row, on BOTH transports. It SHALL NOT fall back
  to a second office-side implementation of AC-TASKS-QUORUM-SLATE-001.9, which would resurrect exactly
  the divergence AC-TASKS-QUORUM-REEVALUATION-001.9 forbids, and it SHALL NOT silently persist a decision that
  can never be counted. This mirrors the existing `decisionStoreNotWiredErr`
  guard in `recordTaskDecision` rather than inventing a new failure mode.
- **AC-TASKS-QUORUM-REEVALUATION-001.14:** THE SYSTEM SHALL extend `shared.WorkflowEngineDispatcher` with a
  second additive method, **read-only**, that evaluates every guarded
  `on_turn_complete` transition configured at a task's current step and returns
  one entry per guard together with the AC-TASKS-QUORUM-DIAGNOSTICS-001.12 `reevaluation_blocked` value:

  `EvaluateStepQuorum(ctx, taskID) (QuorumSnapshot, error)`, where
  `QuorumSnapshot` carries `StepID`, `ReevaluationBlocked bool`, and
  `Guards []QuorumGuardState`, and each `QuorumGuardState` carries
  `TargetStepID`, `Role`, `Threshold`, `RequiredCount`, `ReceivedCount`,
  `Satisfied`, `Reason` and `Error`.

  This method SHALL apply no transition, write no row, execute no action
  callback, emit no AC-TASKS-QUORUM-DIAGNOSTICS-001.2 log record and increment no AC-TASKS-QUORUM-DIAGNOSTICS-001.3 counter. It SHALL
  reuse the AC-TASKS-QUORUM-SLATE-001.9 slate construction, the AC-TASKS-QUORUM-SLATE-001.10 seat matching and the AC-TASKS-QUORUM-DIAGNOSTICS-001.7
  reason precedence — it is a second CALLER of that logic, never a second
  implementation of it, per AC-TASKS-QUORUM-REEVALUATION-001.9.

  **Ordering.** `Guards` SHALL be returned in the step's configured
  `on_turn_complete` action order, so that AC-TASKS-QUORUM-DIAGNOSTICS-001.11's selection rule and AC-TASKS-QUORUM-REEVALUATION-001.6's
  first-transition-wins read off one and the same order. This is the named
  ordering AC-TASKS-QUORUM-DIAGNOSTICS-001.11 depends on; without it AC-TASKS-QUORUM-DIAGNOSTICS-001.11's "first such entry" has no
  referent.

  **Nil / empty / error.** A task with no bound `workflow_step_id` SHALL return
  an empty `Guards`, `ReevaluationBlocked = false` and no error, which is what
  AC-TASKS-QUORUM-DIAGNOSTICS-001.5 requires. A step with a bound id but no guarded transition SHALL
  likewise return an empty `Guards` and no error; AC-TASKS-QUORUM-DIAGNOSTICS-001.11 aggregates both to
  `clear`. A store error SHALL surface as a per-guard `evaluation_error` entry
  per AC-TASKS-QUORUM-DIAGNOSTICS-001.1, not as a method-level error, so one failing guard does not blank
  the others. A method-level error is reserved for the not-wired case.

  **Not wired.** When the dispatcher is nil, per the AC-TASKS-QUORUM-REEVALUATION-001.13 condition, the
  AC-TASKS-QUORUM-DIAGNOSTICS-001.4 handler SHALL return an error rather than falling back to an
  office-side evaluation — the read side carries the same no-fallback rule as
  the write side, and for the same reason.

  **Unknown task.** An unresolvable `taskID` SHALL surface as a method-level
  error, distinct from the unbound-step case of AC-TASKS-QUORUM-DIAGNOSTICS-001.5 which is a successful
  empty result; the AC-TASKS-QUORUM-DIAGNOSTICS-001.4 handler SHALL map it exactly as the sibling
  `GET /tasks/:id/decisions` route already maps an unknown task. Stated because
  AC-TASKS-QUORUM-DIAGNOSTICS-001.5's "return empty rather than error" is easy to over-apply to a task that
  does not exist at all, which would report a missing task as a healthy one.

  **Idempotency and concurrency.** The method is a pure read: two concurrent
  calls compute independently from committed state and neither observes nor
  mutates the other. It needs no operation id.

  AC-TASKS-QUORUM-DIAGNOSTICS-001.4's HTTP payload SHALL be a direct projection of this snapshot, so the
  endpoint and the tool's `guards` field cannot drift apart.

  Without this AC there is no legal way to build AC-TASKS-QUORUM-DIAGNOSTICS-001.4 at all: AC-TASKS-QUORUM-REEVALUATION-001.9 forbids
  `office/dashboard` from carrying its own copy of the slate machinery, and
  `DashboardService`'s only engine connection is
  `engineDispatcher shared.WorkflowEngineDispatcher`, whose declared surface is
  `HandleTrigger` alone. AC-TASKS-QUORUM-DIAGNOSTICS-001.4, AC-TASKS-QUORUM-DIAGNOSTICS-001.5, AC-TASKS-QUORUM-DIAGNOSTICS-001.6, AC-TASKS-QUORUM-DIAGNOSTICS-001.10, AC-TASKS-QUORUM-DIAGNOSTICS-001.11 and AC-TASKS-QUORUM-DIAGNOSTICS-001.12 all
  depend on this method existing.

  **Implementation note, not a requirement:** the in-tree precedent for adding a
  dispatcher capability is a separate optional interface plus a type assertion
  (`handledWorkflowEngineDispatcher` in `office/dashboard/service_tasks.go`
  reaching `HandleTriggerHandled`), rather than widening the shared interface.
  Either shape satisfies this AC; the requirement is that the method exist and
  be reachable from the AC-TASKS-QUORUM-DIAGNOSTICS-001.4 handler, not which of the two idioms carries it.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.