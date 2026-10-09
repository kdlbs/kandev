---
status: draft
system: office
requirements:
  - REQ-OFFICE-GATE-COMMENT-001
  - REQ-OFFICE-GATE-COMMENT-002
  - REQ-OFFICE-GATE-COMMENT-003
  - REQ-OFFICE-GATE-COMMENT-004
  - REQ-OFFICE-GATE-COMMENT-005
---

# Office Gate Comment Wake System Design

## Purpose and boundaries

A comment on a card at `review` or `approval` has to wake the seated agents who
have not decided, and nobody else in that role. Every primitive this needs
already exists: the `on_comment` trigger, the `queue_run_for_each_participant`
action and its workflow-scoped seat population, the decision store the quorum
guard reads, and the stage-aware review and approval prompts. This design adds
three narrow things on top of them and wires them into the shipped template:

- One opt-in configuration key on `queue_run_for_each_participant`,
  `skip_decided`, and one unconditional rule on the same action when it runs
  under `on_comment`: the comment's author is not woken.
- One prompt branch for `task_comment` runs whose resolved stage is `review` or
  `approval`.
- One startup reconciler that appends the template's new `on_comment` fan-out to
  system-owned Office workflows materialized before it existed.

It does not change the quorum guard, seat casting (`review-participant-seats-01.md`),
run causation or the self-trigger allowance, the run reason vocabulary, or any
web UI. It moves no task: autonomy stays with the engine's quorum transitions, as
`taskless-coordinator-authority.md` requires.

## Requirement mapping

| Requirement / criteria | Section |
|---|---|
| AC-OFFICE-GATE-COMMENT-001.1, .2, .3, .4 | Template change; Fan-out selection |
| AC-OFFICE-GATE-COMMENT-001.5, .6 | Idempotency and concurrency |
| AC-OFFICE-GATE-COMMENT-001.7, .8 | Engine-handled comment |
| AC-OFFICE-GATE-COMMENT-001.9, .10, .13, .14, .15, .16 | Failure and recovery |
| AC-OFFICE-GATE-COMMENT-001.11 | Idempotency and concurrency |
| AC-OFFICE-GATE-COMMENT-001.12 | Engine-handled comment |
| AC-OFFICE-GATE-COMMENT-002.1 to .5 | Fan-out selection |
| AC-OFFICE-GATE-COMMENT-002.6, .7 | Engine-handled comment |
| AC-OFFICE-GATE-COMMENT-003.1 to .6 | Prompt |
| AC-OFFICE-GATE-COMMENT-004.1 to .7 | Existing workflows |
| AC-OFFICE-GATE-COMMENT-005.1 to .4 | Fan-out selection; Template change |

## Current behavior (grounding)

- `apps/backend/config/workflows/office-default.yml`: `work` and `done` declare
  `on_comment: queue_run(target: primary, reason: task_comment)`. `review` and
  `approval` declare no `on_comment`.
- Comment producers. `DashboardService.CreateComment`
  (`internal/office/dashboard/service.go`) is the path for a human's comment
  from the web UI (`dashboard/handler_comments.go`) and for an agent's comment
  from its run (`runtime.Actions.PostComment`, whose `Comments` dependency is
  `svcs.Dashboard` in `internal/office/routes.go`). It dispatches
  `engine.TriggerOnComment` synchronously with
  `OnCommentPayload{CommentID, AuthorID}` and operation id
  `commentkeys.TaskComment(commentID)`, then publishes `OfficeCommentCreated`,
  then runs reactivity. The channel-inbound producers,
  `Service.CreateComment` (`internal/office/service/comments.go`, called from
  `service/channels.go`) and `ChannelService.CreateComment`
  (`internal/office/channels/comments.go`), and the session bridge only
  publish `OfficeCommentCreated`: they neither dispatch the engine nor run
  reactivity. The event subscriber `Service.queueCommentRun`
  (`internal/office/service/event_subscribers.go`) dispatches every published
  comment not marked `engine_dispatched`, returns early for
  `Source == "session"`, and its caller `handleCommentCreated` logs a returned
  error and never retries. So a dashboard comment gets at most two
  evaluations, and a channel-inbound comment gets exactly one. Channel tasks
  carry no `workflow_step_id` today.
- Both the dashboard and the subscriber skip comments authored by the task's
  assignee before the engine runs: the dashboard via `isSelfComment` (agent
  author equal to `AssigneeAgentProfileID`), the subscriber via the same check.
  Both checks read `GetTaskExecutionFields` and treat a read error as "not the
  assignee". The scheduler's `reactToComment` also skips the legacy assignee
  wake for a self-comment, so a runner-authored comment wakes nobody but
  @-mentions.
- `DashboardService.CreateComment` runs `dispatchCommentEngineTrigger`, then
  `publishCommentCreated` (sets `engine_dispatched` only when handled), then
  `runReactivityForComment` with `SkipAssigneeCommentWake` equal to handled.
  `dispatchCommentEngineTrigger` returns false for a self-comment, for
  `shared.ErrEngineNoSession`, and for any other error (after a warning), so
  today an engine error both wakes the runner and lets the subscriber
  redispatch the same comment once with the same operation id.
- The engine marks an operation applied only when every action succeeded;
  `processActions` returns an empty result on error and `executeCallback`
  returns the callback's error unwrapped. The dispatcher wraps it with `%w`.
  `Engine.handleTrigger` can also fail outside any callback after the session
  is resolved: `isOperationAlreadyApplied` (the `IsOperationApplied` read),
  `loadExecutionContext` (state or step load),
  `validateActionCallbacks` (an action kind with no registered callback,
  `ErrActionNotYetWired`), and `markOperationAppliedForInput` after every
  action succeeded.
- `engine_dispatcher.Dispatcher.HandleTriggerHandled` reports a comment handled
  when the resolved step compiled at least one `on_comment` action
  (`result.ActionCount > 0`). The dashboard passes that as
  `SkipAssigneeCommentWake`, which suppresses the legacy assignee wake in
  `SchedulerService.reactToComment` (`internal/office/scheduler/reactivity.go`)
  while @-mention wakes still run. Today a gated step compiles no `on_comment`
  action, so the comment is unhandled and the **runner** is woken.
- `QueueRunForEachParticipantCallback.Execute`
  (`internal/workflow/engine/phase2_callbacks.go`) builds the population with
  `roleSeatsForFanOut` (sorted by `Position`, then `AgentProfileID`), queues one
  `QueueRunRequest` per seat with `idempotencyKey(in, agentID, taskID)` and
  `queueRunPayload`, which copies `comment_id` and `author_id` from an
  `OnCommentPayload` and then overlays the action's authored `payload`. Errors
  are collected and joined.
- Decisions: `engine.DecisionStore.ListStepDecisions(taskID, stepID)` returns
  every row for the pair, superseded rows included, oldest first.
  `mapDecisionsToSeats` (`internal/workflow/engine/quorum.go`) is the
  approve-style guard's seat mapping.
- Sampled on the live instance: human comments carry `author_id = "user"`;
  agent comments carry the agent profile UUID; agent profile ids are UUIDs. The
  two identifier spaces do not overlap.

## Template change

`office-default.yml` gains, on `review`:

```yaml
      on_comment:
        - type: queue_run_for_each_participant
          config:
            role: reviewer
            reason: task_comment
            skip_decided: true
            payload:
              stage_type: review
```

and the same on `approval` with `role: approver` and `stage_type: approval`.
`work`, `backlog` and `done` are untouched (AC-005.1). The step-entry fan-outs
keep their current config and so keep waking every seat (AC-005.2).

## Fan-out selection

`QueueRunForEachParticipantAction` gains `SkipDecided bool`, read by
`readQueueRunForEachParticipantConfig` from `config["skip_decided"]`. Only a
boolean `true` enables it; absent, `false`, or any non-boolean value leaves it
off (AC-005.3). The key joins `queueActionDigest` so an action with and without
it keep distinct idempotency salts.

`QueueRunForEachParticipantCallback` gains `Decisions DecisionStore` and
`Logger`. Both construction sites set them: `buildWorkflowCallbacks`
(`internal/orchestrator/workflow_callbacks.go`) and `engineOnEnterCallback`
(`internal/orchestrator/event_handlers_workflow.go`).

`Execute` computes the population exactly as today, then filters it in this
order, preserving the population's order:

1. **Author exclusion** - only when the trigger payload is an
   `OnCommentPayload` (value or pointer, via `commentPayload`) with a non-empty
   `AuthorID`: drop every seat whose `AgentProfileID` equals `AuthorID`.
   Unconditional on `SkipDecided` (AC-002.1, .2, .4, .5). A human author's id
   never equals a profile id, so it excludes nobody (AC-002.3). No other trigger
   excludes anyone.
2. **Decided filter** - only when `SkipDecided` is true: read
   `ListStepDecisions(taskID, in.Step.ID)` once, drop rows whose
   `SupersededAt` is non-nil, map the remainder with `mapDecisionsToSeats` over
   the population, and drop every seat that received a mapping (AC-001.1, .2).

The step is `in.Step.ID`, the step the engine evaluated the trigger at, which is
the task's current step. Roles are per action, so an `approval` comment reads
only `approver` seats; an agent holding both a reviewer and an approver seat is
one `approver` seat there (AC-001.2).

Ignoring superseded rows is at least as permissive as the guard: a row is
superseded only when the same decider records again at the step, which leaves a
newer active row, or on rework, after which re-entry's `clear_decisions` deletes
the step's rows. Where the two readings could differ, this one wakes a seat the
guard would count, never skips a seat the guard waits on.

Each surviving seat gets one `QueueRunRequest` exactly as today: reason
`task_comment`, `WorkflowStepID = in.Step.ID`, payload `comment_id`,
`author_id` and `stage_type` (AC-001.4; the run service adds `task_id` and
`workflow_step_id` to the persisted payload). Order is the population order
(AC-001.3).

## Engine-handled comment

With the template change a gated step compiles one `on_comment` action, so
`HandleTriggerHandled` returns handled and the legacy assignee wake is
suppressed, whether or not any seat survived filtering (AC-001.7, .8). An empty
survivor set returns `ActionResult{}` with no error. @-mention wakes are
unaffected. When the dispatcher resolves no session the comment stays unhandled
and today's legacy path runs; that case is out of scope in the requirements.

Session-bridged comments keep returning early in `queueCommentRun`, so a seat's
own turn output never reaches this action (AC-002.6).

The assignee short-circuit in both producers is kept, not narrowed, including
its fail-open on an assignee read error: when `GetTaskExecutionFields` fails the
comment is treated as not the runner's and reaches the engine, where author
exclusion still drops any seat the runner holds (AC-002.7). A comment by
the task's runner at a gated step never reaches the engine, so it wakes no seat,
reads no decision (AC-001.9 cannot fire) and, through `reactToComment`'s
self-comment check, queues no legacy wake; @-mentions still run (AC-002.7).
Under self-review the runner is also a seat, and its own comment waking the
other seats is recorded as out of scope rather than carved into the
short-circuit, because narrowing it would route every runner reply at `work`
and `done` through the engine as well.

The action queues runs only. A woken reviewer's verdict goes through the
existing `RecordParticipantDecision` path and the step's `all_approve` /
`any_reject` guards move the task (AC-001.12).

## Idempotency and concurrency

- **Redelivery** of one comment reuses operation id
  `commentkeys.TaskComment(commentID)`, so `idempotencyKey` yields the same key
  per (step, task, agent, action digest) and the run queue refuses the duplicate
  (AC-001.5). Both producers may dispatch the same comment; that is a
  redelivery.
- **Distinct comments** carry distinct operation ids; each evaluation reads
  decisions afresh (AC-001.6). The idempotency key starts with the
  `commentkeys.TaskComment` prefix, which `shouldCoalesceRun`
  (`internal/runs/service/service.go`) already excludes from coalescing, so a
  gate comment wake is never merged with another run in either direction. No
  change to the run service is needed; a test pins it.
- **Decision racing a comment:** the decision read and the enqueue are not in
  one transaction and take no lock. A decision committing after the read leaves
  that seat woken once for that comment (AC-001.11); its later verdict records
  as a new row under the existing supersede rule.
- **Partial failure then redelivery (dashboard comments):** already-queued
  seats are refused by key; failed seats are retried. The operation is not
  marked applied on error, so the subscriber's second dispatch re-runs the
  fan-out (AC-001.13). There is at most one such redelivery per comment,
  because the subscriber handles each `OfficeCommentCreated` event once. A
  channel-inbound comment has no second dispatch (AC-001.16).

## Prompt

`SchedulerIntegration.buildPromptContext`
(`internal/office/service/scheduler_integration.go`) additionally, for reason
`task_comment` when the parsed payload has a non-empty `stage_type`, sets
`pc.StageType` from a new `resolveGateCommentStage(ctx, parsed)` in
`internal/office/service/review_stage.go`:

1. `stepID := parsed["workflow_step_id"]`.
2. When `stepID` is non-empty, read `GetWorkflowStepStageType(ctx, stepID)`.
   When that returns no error and a non-empty value, return it.
3. Otherwise return `parsed["stage_type"]`.

The task's current step is never read, so a card that moved after the wake
was queued still gets the stage of the step it was queued at (AC-003.4). The
shared `resolveReviewStage`, `shouldResolveAuthoritativeStage` and
`resolveMissingReviewStepID` are not changed: their current-step fallback and
their reason gate keep serving `task_assigned` and `task_review_requested`
exactly as today. A `task_comment` without `stage_type` skips this entirely and
its `PromptContext` is unchanged (AC-003.5).

`BuildPrompt`'s `RunReasonTaskComment` branch calls a new
`buildGateCommentPrompt` when `pc.StageType` is `review` or `approval`, and
`buildTaskCommentPrompt` otherwise. `buildGateCommentPrompt` writes, in order:
the task reference and title with "You are reviewing" / "You are approving";
the author line in the existing `From:` form; the comment body under a
`Comment:` heading, or the sentence that the triggering comment is no longer
available when `pc.CommentBody` is empty because the comment failed to load
(AC-003.6); an instruction to re-examine the work in light of the comment and
record a verdict; and, last, `writeDecisionContract` when
`hasDecisionAction(pc.AllowedActions)` (AC-003.1 to .3). That is the end of the
reason-specific body. `BuildPrompt` then appends one-time instructions, missed
ticks, handoff and runtime context unchanged, as for every reason, so AC-003.3's
"ends with" is asserted on the output of `buildGateCommentPrompt`, not on the
full prompt. `AllowedActions` is seat-derived at run-context build
(`ContextBuilder.resolveAvailableActions`), independent of reason. Prompt text
is agent-facing, not web UI copy, and follows the existing prompt builders'
literal strings.

## Existing workflows

A new startup reconciler, `healBuiltinWorkflowStepOnCommentFanOut`, in
`internal/task/repository/sqlite/`, registered in `base_schema.go` directly after
`healBuiltinWorkflowStepOnAgentError`. It follows that reconciler:

- For each embedded template (`workflowcfg.LoadTemplates`), each step, each
  `on_comment` action of type `queue_run_for_each_participant` with a non-empty
  `role`, find rows via `findSystemOwnedWorkflowSteps(templateID, stepName)`.
  Only `is_system = 1` rows are returned (AC-004.4).
- Per row, `tryHealWorkflowStepEvents` with a mutator: if the stored
  `OnComment` already holds a `queue_run_for_each_participant` whose trimmed
  `role` equals the template role, change nothing (AC-004.3, .5); otherwise
  append a copy of the template action to the end of `OnComment` (AC-004.1,
  .2). Every other trigger's list is round-tripped unchanged.
- The write is conditional on the events column still equalling what was read.
  A lost race re-reads, up to 5 attempts, then logs a warning naming template,
  step name, workflow and step and returns nil (AC-004.6).

The live test board `7561d51f` (workspace `95542bf3`) is a system-owned
`office-default` row (`is_system = 1`, `workflow_template_id = office-default`)
whose `Review` and `Approval` steps have no `on_comment`, so one restart gives it
the fan-out (AC-004.7). Its non-standard legacy on_enter reasons are untouched.

## Failure and recovery

- Decision read fails, or `Decisions` is nil while `SkipDecided` is true: log a
  warning with task id, step id, role and error, and wake every seat left after
  author exclusion (AC-001.9). A wasted run is cheaper than a gate left parked,
  which is the defect this capability exists to remove.
- One seat's enqueue fails: unchanged collect-and-continue, errors joined
  (AC-001.10).
- Missing `role`, nil `Adapter` or nil `Participants` (the action's own
  run-queueing and participant dependencies): under `on_comment` these are
  fan-out failures and wrap the sentinel below; under any other trigger they
  are unchanged. They are distinct from an action kind with no registered
  callback at all, which `validateActionCallbacks` rejects before any action
  runs and which is an AC-001.15 failure.
- **Comment fan-out incomplete (AC-001.13).** The engine package gains an
  exported sentinel `ErrCommentFanOutIncomplete`. When `in.Trigger` is
  `TriggerOnComment`, every error `Execute` returns wraps it with `%w`: the
  missing-dependency and missing-role errors, the population-read error and
  the joined per-seat errors. The wrap uses Go's multi-`%w` form so an error
  that already wraps `ErrActionNotYetWired` keeps it, and its text names the
  task id, `in.Step.ID` and the role, for example
  `fmt.Errorf("%w: task %s step %s role %q: %w", ErrCommentFanOutIncomplete,
  taskID, in.Step.ID, role, err)`, where `role` is the action's configured role
  and is empty when the action config is nil. The role is quoted so
  that a missing role renders as `role ""`, and the wrapped inner error for
  that case keeps today's text, `queue_run_for_each_participant missing role`,
  so the warning names the step and states that the role is missing
  (AC-001.13). That text is how the dashboard's
  warning names the step and role: the dashboard itself has only the task and
  comment ids. The step-entry fan-out returns its errors as today. The sentinel survives `executeCallback` and the dispatcher's
  `%w`. `dispatchCommentEngineTrigger` returns a small result carrying
  `handled` and `suppressAssigneeWake` instead of one bool. On
  `errors.Is(err, engine.ErrCommentFanOutIncomplete)` it logs a warning naming
  task, step, comment and error and returns `handled = false`,
  `suppressAssigneeWake = true`. `CreateComment` publishes the event without
  `engine_dispatched` and passes `suppressAssigneeWake` as
  `SkipAssigneeCommentWake`, so the runner is not woken, @-mentions run, and the
  subscriber's `queueCommentRun` redispatches the comment once with the same
  operation id. Seats queued by the first attempt are refused by idempotency
  key; the rest are retried. A second failure is returned by the subscriber as
  today, which logs it, and nothing retries again.
- **Any other comment-handler error (AC-001.14, .15).** Anything not wrapping
  the sentinel keeps today's path: `handled = false`,
  `suppressAssigneeWake = false`, a warning, the legacy assignee wake and one
  subscriber redispatch. At `work` and `done` that is a `queue_run` failure
  (AC-001.14). At a gated step it is every failure the fan-out callback did not
  report (AC-001.15). In `Engine.handleTrigger` order after the session is
  resolved these are: the `IsOperationApplied` read error from
  `isOperationAlreadyApplied`; a state or step load error from
  `loadExecutionContext`; `ErrActionNotYetWired` from
  `validateActionCallbacks`; an error from an `on_comment` action declared
  before the fan-out, after which `evaluateActions` stops and the fan-out's
  `Execute` never runs, so no sentinel exists; and a `MarkOperationApplied`
  error after every action succeeded. These are storage, wiring or
  operator-configuration failures, not a stalled gate, and the dispatcher
  returns no step or action information with them, so the dashboard cannot
  classify them without a new dispatcher contract. Keeping the legacy wake
  leaves the comment visible to the runner rather than silently dropped. In
  the marker-write case the redispatch finds every seat's key already queued
  and refuses each duplicate (AC-001.5), then marks the operation applied. In
  the preceding-action case the redispatch runs that action first again; if it
  fails again the fan-out never runs for that comment and no seat is woken,
  which the requirements record as out of scope. The shipped template declares
  no other `on_comment` action at `review` or `approval`, so this arises only on
  a system-owned row whose step already carried its own comment actions
  (AC-004.2).
- **Channel-inbound comments (AC-001.16).** The subscriber's `queueCommentRun`
  is their first and only evaluation. Its `dispatchEngineTrigger` returns any
  error, sentinel or not, to `handleCommentCreated`, which logs it. Nothing
  retries, and no legacy wake runs because these producers never call
  reactivity. No code change on this path.

## Permissions and security boundaries

No new authority. Who may comment on which task stays owned by
`taskless-coordinator-authority.md`; a comment still only queues runs for agents
already seated on the task by the workflow. The self-trigger allowance and
causation depth continue to bound runaway wakes (AC-005.4).

## Persistence and migration

No schema change. The reconciler rewrites `workflow_steps.events` JSON on
system-owned rows only, once per missing role, idempotently.

## Observability

No new counter. Queued runs carry reason `task_comment` and `comment_id`, so the
existing comment run-status lookup and run history show them. The decision-read
fallback and the reconciler's exhausted retries log warnings; the reconciler
logs nothing when it changes nothing.

## Testing

- Engine unit tests next to `queue_run_test.go`: skip-decided with one decided
  and one undecided seat; superseded-only decision counts as undecided; author
  exclusion with and without `skip_decided`; human author excludes nobody;
  empty `AuthorID`; non-comment trigger excludes nobody; decision read error
  and nil store fail open; `skip_decided` non-boolean off; digest differs with
  the key.
- Orchestrator-level test through the real engine and the shipped template: a
  comment on a Review task with one undecided reviewer queues exactly one run;
  the reviewer's own comment queues none; an Approval comment wakes only
  undecided approvers; the reviewer's `approved` decision moves the task to
  Approval; Work and Done comments unchanged.
- Engine unit tests: under `on_comment`, a population-read failure and a
  one-seat enqueue failure both return an error for which
  `errors.Is(err, ErrCommentFanOutIncomplete)` holds, and the same failures
  under step entry do not.
- Dashboard test: a gated-step comment with a resolved session suppresses the
  legacy assignee wake while an @-mention still wakes; a dispatcher returning
  the sentinel suppresses the legacy wake, publishes without
  `engine_dispatched` and logs; a dispatcher returning any other error keeps
  the legacy wake (AC-001.13, .14).
- Orchestrator-level test for AC-001.13: a first dispatch with one seat's
  enqueue failing, then the subscriber redispatch, ends with exactly one run per
  undecided seat and no runner `task_comment` run.
- Runner-authored gate comment (AC-002.7): a comment by the assignee agent at
  Review queues no seat run and no legacy run, with the decision store never
  read.
- Engine unit test: under `on_comment`, a missing-role action returns an error
  for which `errors.Is(err, ErrCommentFanOutIncomplete)` holds and whose text
  contains the step id, `role ""` and `missing role`; with a nil `Adapter` both the sentinel and
  `ErrActionNotYetWired` match.
- Dashboard test for AC-001.15: a dispatcher returning a non-sentinel error for
  a gated-step comment keeps the legacy wake and publishes without
  `engine_dispatched`.
- Engine unit tests for AC-001.15 under `on_comment` at a step whose
  `on_comment` list is a failing `queue_run` followed by the fan-out, and for
  an `IsOperationApplied` store error: the returned error does not satisfy
  `errors.Is(err, ErrCommentFanOutIncomplete)` and the fan-out queues no run.
- Subscriber test for AC-001.16: a comment published without
  `engine_dispatched` whose dispatch returns the sentinel is logged by
  `handleCommentCreated`, and no second dispatch happens for that event.
- Assignee-read failure (AC-002.7): with `GetTaskExecutionFields` failing, a
  runner-authored gate comment reaches the fan-out and the runner's own seat is
  excluded as author.
- Stage resolution tests for `resolveGateCommentStage`: step stage wins over the
  payload; step read error, empty step stage and missing `workflow_step_id`
  each fall back to the payload; a task whose current step differs from the
  run's step still resolves the run's step; a `task_comment` run without
  `stage_type` whose `workflow_step_id` names a review step never calls the
  resolver and gets today's prompt (AC-003.4, .5).
- Prompt builder tests for AC-003.1 to .6, including byte-identical output for a
  `task_comment` run without `stage_type`, and a check that
  `buildGateCommentPrompt`'s output ends with the `writeDecisionContract` text
  when the seat has a decision action.
- Reconciler tests modelled on the `on_agent_error` reconciler's: append when
  absent, preserve other actions and order, leave a role already present,
  leave `is_system = 0`, idempotent second run, CAS retry exhaustion warns and
  leaves the row unchanged.

## Prior art

- **Our wiki.** Searched vault `/Users/henry/Documents/henry/wiki` (resolved from
  `~/.obsidian-wiki/config.henry`) through QMD collection `wiki` with lex
  `on_comment reviewer approval seat wake kandev`, lex `coordinator annotate wake
  never move card`, and a vector query on comment-wakes-reviewer. Hits were the
  `kandev` entity page (Office component inventory), `oversight-queue` and
  unrelated review pages; none addresses gate comment wakes or seat fan-out. A
  direct grep of the vault was refused by the sandbox, so the check is QMD-only.
  Nothing useful to cite or depart from.
- **Other products.** saas-kb `search_saas_docs`, category `ai_sdlc`: "comment on
  task in review wakes reviewer agent" (Multica, Paperclip, OpenHands, Warp hits)
  and, filtered to Paperclip, "issue comment triggers assigned agent heartbeat
  wake mention". Paperclip (vendor claim) wakes the issue **owner** on any
  comment (`issue_commented`), wakes a non-owner only by @-mention
  (`issue_comment_mentioned`), and passes the triggering comment id to the run.
  Multica and Warp describe reviewer agents but no comment-driven wake.
- **What we do differently.** Paperclip's owner-only rule is exactly today's
  kandev behaviour at gates (the runner is the owner) and is the defect: at a
  gate the actor who can move the card is the seat holder, not the owner. We keep
  Paperclip's two sound parts, @-mention as the additive route to anyone and the
  comment id carried into the run, and add what an owner model has no notion
  of: waking by seat role, skipping seats that already decided, and never waking
  the author.

## Related ADRs

- [ADR-0004](../../../decisions/0004-task-model-unification.md), cited by
  `models.GenericActionType` as the origin of the `on_comment` trigger and the
  generic action set this design configures. No new ADR: the change configures
  existing primitives and adds no boundary.
