---
status: draft
system: office
created: 2026-09-26
owners:
  - kandev
---

# Office Gate Comment Wake Requirements

## Overview

A comment is the lever a scheduled coordinator has on a stalled card. The
coordinator may annotate any task in its workspace and may wake agents, but it
never moves a card: `REQ-OFFICE-COORDINATOR-AUTHORITY-004` grants annotation and
`REQ-OFFICE-COORDINATOR-AUTHORITY-005` keeps state mutation denied. Cards move
only when the workflow engine acts on an agent's signal or a quorum. So a
comment on a stalled card has to wake whoever can produce that signal.

The shipped Office workflow does this at `work` and `done`, which wake the
runner on a comment. `review` and `approval` declare no comment handler, so the
comment falls through to the legacy assignee wake of the task's **runner**,
whose turn cannot satisfy either step's quorum. The seated reviewer or
approver, whose verdict alone can move the card, is never woken, so a card
parked at a gate needs a human.

This capability makes a comment on a gated card wake the seated agents in that
gate's role who have not yet decided, tells each of them to decide, and never
wakes the comment's own author. It also brings Office workflows that already
exist up to date without manual database work.

The seats themselves, how they are cast, the order they are woken in and the
quorum that counts them are owned by `review-participant-seats.md`
(`REQ-OFFICE-REVIEW-SEATS-001` to `-005`) and are not restated here.

## Terminology

- **Gated step** - a step whose comment handler declares a participant fan-out
  for a role. In the shipped Office workflow these are `review` (role
  `reviewer`) and `approval` (role `approver`).
- **Seat** and **fan-out population** - a participant seat as defined in
  `review-participant-seats.md`; the fan-out population for a role is the set
  of seats the step's entry fan-out for that role wakes, in the same order
  (`AC-OFFICE-REVIEW-SEATS-003.3`). It is not filtered by the
  `decision_required` flag.
- **Decided seat** - a seat in the fan-out population to which at least one
  non-superseded decision recorded at the gated step for that task maps. A
  decision maps to a seat by the rule the step's approve-style quorum guard
  uses: by the decision's participant identifier when it names the seat,
  otherwise by the decision's role and agent decider identifier equalling the
  seat's role and agent profile identifier. Every other seat is **undecided**.
- **Comment author** - the author identifier carried by the comment's trigger.
  For an agent it is the agent's profile identifier; for a human user it is the
  user identifier.
- **Gate comment wake** - a run queued for a seat because a comment was
  recorded while the task stood at a gated step.

## Requirements

### REQ-OFFICE-GATE-COMMENT-001: A comment on a gated card wakes its undecided seats

**User story:** As a coordinator agent, I want my comment on a card parked in
Review to wake the reviewer who has not decided yet, so that the reviewer's
verdict moves the card without a human.

#### Acceptance criteria

- **AC-OFFICE-GATE-COMMENT-001.1:** When a comment is recorded on an Office task
  whose current step is the shipped `review` step, the system shall queue exactly
  one run with reason `task_comment` for each undecided `reviewer` seat in the
  fan-out population, and shall queue no run for any decided seat.
- **AC-OFFICE-GATE-COMMENT-001.2:** When a comment is recorded on an Office task
  whose current step is the shipped `approval` step, the system shall queue
  exactly one run with reason `task_comment` for each undecided `approver` seat,
  and shall queue no run for any `reviewer` seat that is not also an `approver`
  seat.
- **AC-OFFICE-GATE-COMMENT-001.3:** The system shall queue gate comment wakes in
  the fan-out population's order: ascending seat `position`, then ascending agent
  profile identifier.
- **AC-OFFICE-GATE-COMMENT-001.4:** Each gate comment wake shall carry the
  triggering comment's identifier as `comment_id`, the comment author as
  `author_id`, the gated step's stage type (`review` or `approval`) as
  `stage_type`, and the gated step's identifier as its workflow step.
- **AC-OFFICE-GATE-COMMENT-001.5:** When the same comment's trigger is delivered
  more than once, the system shall not queue a second run for a seat that already
  has a gate comment wake for that comment at that step.
- **AC-OFFICE-GATE-COMMENT-001.6:** When two different comments are recorded on
  the same gated task, the system shall evaluate each independently, reading the
  decided seats afresh for each, and shall queue at most one run per seat per
  comment. A gate comment wake shall not be merged into any other queued run for
  the same seat, including another comment's wake or a still-queued step-entry
  wake, and no other run shall be merged into it.
- **AC-OFFICE-GATE-COMMENT-001.7:** When every seat in the fan-out population is
  decided or excluded under REQ-OFFICE-GATE-COMMENT-002, or the population is
  empty, the system shall queue no gate comment wake and shall report no error.
- **AC-OFFICE-GATE-COMMENT-001.8:** When a comment is recorded on a task at a
  gated step and the workflow engine resolves a session for the task, the system
  shall not queue the legacy assignee `task_comment` wake for the task's runner,
  whether or not any gate comment wake was queued, except in the engine failure
  cases AC-OFFICE-GATE-COMMENT-001.15 names. An agent @-mentioned in the
  comment shall still be woken by the existing mention wake.
- **AC-OFFICE-GATE-COMMENT-001.9:** When the recorded decisions for the gated
  step cannot be read, the system shall queue a gate comment wake for every seat
  in the fan-out population except those excluded under
  REQ-OFFICE-GATE-COMMENT-002, and shall log a warning naming the task, the step,
  the role and the read error.
- **AC-OFFICE-GATE-COMMENT-001.10:** When queueing a wake for one seat fails, the
  system shall still attempt every remaining seat and shall report every failure
  together.
- **AC-OFFICE-GATE-COMMENT-001.11:** When a seat's decision is recorded
  concurrently with a comment's evaluation, the system can queue one gate comment
  wake for that seat from that comment; it shall not queue more than one.
- **AC-OFFICE-GATE-COMMENT-001.12:** When a woken reviewer records `approved`
  and it holds the only decision-required `reviewer` seat, the task shall move
  from `review` to `approval` by the step's existing `all_approve` guard, and
  when it records `rejected` the task shall move to `work` by the existing
  `any_reject` guard. The gate comment wake itself shall move no task, write no
  seat and record no decision.
- **AC-OFFICE-GATE-COMMENT-001.13:** When a gate comment wake fan-out fails
  for a comment recorded through the Office dashboard comment service (a human's
  comment from the web UI or an agent's comment from its run), for any reason
  the fan-out action itself reports, including an unreadable fan-out
  population, a failed enqueue for at least one seat, a fan-out action that
  declares no role, or a fan-out action whose own run-queueing or participant
  dependency is absent, the system shall still not queue the legacy
  assignee `task_comment` wake for the task's runner, shall log a warning whose
  fields or error text name the task, the step, the role, the comment and the
  error, and shall leave the comment unmarked as engine-dispatched so that the
  comment event subscriber evaluates it exactly once more. That second
  evaluation shall wake only seats that do not already hold a gate comment wake
  for that comment (AC-OFFICE-GATE-COMMENT-001.5), and when it fails too, the
  system shall log the failure and make no further attempt for that comment. An
  agent @-mentioned in the comment shall still be woken by the existing mention
  wake. When the fan-out action declares no role, the warning shall name the
  step and state that the role is missing in place of naming a role. A fan-out
  action kind for which no handler is registered at all is not a failure the
  fan-out action reports; it is governed by AC-OFFICE-GATE-COMMENT-001.15.
- **AC-OFFICE-GATE-COMMENT-001.14:** When the comment handler of a step that
  declares no comment-handler participant fan-out fails, including at the
  shipped `work` and `done` steps, the system shall behave exactly as before this
  change, including queueing the legacy assignee wake.
- **AC-OFFICE-GATE-COMMENT-001.15:** When a comment is recorded through the
  Office dashboard comment service on a task at a gated step, the workflow
  engine resolves a session for the task, and the comment handler fails with
  any failure that the fan-out action does not report under
  AC-OFFICE-GATE-COMMENT-001.13, the system shall behave as before this change:
  it shall log a warning, queue the legacy assignee `task_comment` wake for the
  runner, and leave the comment unmarked as engine-dispatched so the subscriber
  evaluates it once more. Such failures include reading whether the comment's
  operation was already applied, loading the task's workflow state or step,
  finding no registered handler for one of the step's comment actions before
  any action runs, another comment action declared on the step before the
  fan-out failing so that the fan-out does not run, and recording the
  comment's operation as applied after every action succeeded. Seats already
  woken for that comment shall not be woken again on that second evaluation
  (AC-OFFICE-GATE-COMMENT-001.5). When a comment action declared before the
  fan-out fails on the second evaluation too, the fan-out does not run for that
  comment and no seat is woken by it.
- **AC-OFFICE-GATE-COMMENT-001.16:** When a comment reaches the workflow engine
  only through the comment event subscriber, because its producer publishes the
  comment without dispatching it (the channel-inbound comment paths), the
  subscriber's evaluation shall be the comment's only evaluation: a fan-out
  failure shall be logged, shall not be retried, and shall queue no legacy
  assignee wake, as for any failure on that path today.

### REQ-OFFICE-GATE-COMMENT-002: A comment never wakes its own author

**User story:** As an operator, I want a reviewer's own comment not to wake that
reviewer, so that a seat does not spend a run answering itself.

#### Acceptance criteria

- **AC-OFFICE-GATE-COMMENT-002.1:** When the comment author equals a seat's agent
  profile identifier, the system shall queue no run for that seat from that
  comment, whether the seat is decided or undecided.
- **AC-OFFICE-GATE-COMMENT-002.2:** When the comment author holds one seat in the
  role and is not the task's assignee (AC-OFFICE-GATE-COMMENT-002.7), every other
  undecided seat in that role shall still be woken under
  REQ-OFFICE-GATE-COMMENT-001.
- **AC-OFFICE-GATE-COMMENT-002.3:** When the comment author is a human user, the
  system shall exclude no seat on account of authorship.
- **AC-OFFICE-GATE-COMMENT-002.4:** When the comment's trigger carries no author
  identifier, the system shall exclude no seat on account of authorship.
- **AC-OFFICE-GATE-COMMENT-002.5:** Author exclusion shall apply to every
  participant fan-out declared under a step's comment handler, in any workflow,
  whether or not that fan-out skips decided seats. It shall not apply to a
  participant fan-out under any other trigger.
- **AC-OFFICE-GATE-COMMENT-002.6:** A comment that mirrors an agent's own session
  output shall continue to queue no run through the step's comment handler.
- **AC-OFFICE-GATE-COMMENT-002.7:** When a comment on a task at a gated step is
  authored by an agent that is the task's assignee (its runner), including when
  that agent also holds a seat on the task under self-review, the system shall
  queue no gate comment wake for any seat from that comment and no legacy
  assignee wake, and shall not read the step's decisions for it, so
  AC-OFFICE-GATE-COMMENT-001.9 does not apply. An agent @-mentioned in the
  comment shall still be woken by the existing mention wake. This holds when the
  task's assignee is read successfully. When that read fails, the comment shall
  be treated as not authored by the runner, as today: it reaches the comment
  handler and REQ-OFFICE-GATE-COMMENT-001 and -002 apply to it, so its author is
  still excluded from any seat it holds.

### REQ-OFFICE-GATE-COMMENT-003: The woken seat is told to decide

**User story:** As a reviewer agent woken by a comment, I want to see the comment
and be told to record a verdict, so that I do not treat the wake as a work
request.

#### Acceptance criteria

- **AC-OFFICE-GATE-COMMENT-003.1:** When a `task_comment` run's resolved stage is
  `review`, its prompt shall identify the task, state that the agent is reviewing
  it, name the comment author using the existing comment-author label, quote the
  comment body, and instruct the agent to re-examine the work in light of the
  comment and record a verdict.
- **AC-OFFICE-GATE-COMMENT-003.2:** When a `task_comment` run's resolved stage is
  `approval`, its prompt shall satisfy AC-OFFICE-GATE-COMMENT-003.1 with
  "approving" in place of "reviewing".
- **AC-OFFICE-GATE-COMMENT-003.3:** When the woken agent holds a decision seat on
  the task at prompt assembly, the reason-specific prompt body shall end with the
  same decision command contract the review and approval stage prompt bodies end
  with. When it does not, the prompt shall omit that contract. The reason-specific
  body is the text before the sections appended to every run's prompt (one-time
  workflow move instructions, missed ticks, handoff context and runtime context),
  which follow it unchanged.
- **AC-OFFICE-GATE-COMMENT-003.4:** For a `task_comment` run that carries a
  non-empty `stage_type` value, the resolved stage shall be the stage type of the step named by the run's `workflow_step_id` value when
  that value is present and that step's stage type reads without error and is
  non-empty, and otherwise the run's `stage_type` value. The task's current step
  shall not be consulted. When the resolved stage is neither `review` nor
  `approval`, the prompt shall be the existing comment prompt. A `task_comment`
  run without a `stage_type` value resolves no stage and is governed by
  AC-OFFICE-GATE-COMMENT-003.5, whatever step its `workflow_step_id` names.
- **AC-OFFICE-GATE-COMMENT-003.5:** When a `task_comment` run carries no
  `stage_type` value, its prompt shall be identical to the prompt the same run
  receives today.
- **AC-OFFICE-GATE-COMMENT-003.6:** When the triggering comment cannot be loaded
  at prompt assembly, a review or approval `task_comment` prompt shall still
  frame the review or approval and the verdict instruction, shall state that the
  triggering comment is no longer available, and shall quote nothing.

### REQ-OFFICE-GATE-COMMENT-004: Existing Office workflows gain the behavior

**User story:** As an operator of a workspace onboarded before this change, I
want my Office board to wake reviewers on a comment without re-onboarding or
editing the database.

#### Acceptance criteria

- **AC-OFFICE-GATE-COMMENT-004.1:** When the backend starts, every system-owned
  workflow materialized from a shipped template shall have, on each step whose
  same-named template step declares a comment-handler participant fan-out for a
  role, that fan-out action, equal in content to the template's, unless the step
  already declares a comment-handler participant fan-out for that role.
- **AC-OFFICE-GATE-COMMENT-004.2:** A fan-out added under
  AC-OFFICE-GATE-COMMENT-004.1 shall be appended after every comment-handler
  action the step already declares, and every other action declared on the step,
  under any trigger, shall be preserved in content and order.
- **AC-OFFICE-GATE-COMMENT-004.3:** When a step already declares a comment-handler
  participant fan-out for the role, in any configuration, the system shall leave
  that step's comment handler unchanged.
- **AC-OFFICE-GATE-COMMENT-004.4:** The system shall not modify any workflow that
  is not system-owned.
- **AC-OFFICE-GATE-COMMENT-004.5:** Running the startup reconciliation again shall
  change nothing.
- **AC-OFFICE-GATE-COMMENT-004.6:** When a step is changed by another writer
  between the moment it is read and the moment it is written back, the system
  shall not overwrite that change: it shall re-read and apply the addition on top
  of the observed state, up to a bounded number of attempts, and after the last
  attempt shall leave the step unchanged and log a warning naming the workflow and
  step. Neither outcome shall prevent the backend from starting.
- **AC-OFFICE-GATE-COMMENT-004.7:** For a workspace whose Office workflow was
  materialized before this change and whose gated steps were otherwise
  unmodified, AC-OFFICE-GATE-COMMENT-001.1 and -001.2 shall hold after one
  backend restart.

### REQ-OFFICE-GATE-COMMENT-005: Behavior outside gated steps is unchanged

#### Acceptance criteria

- **AC-OFFICE-GATE-COMMENT-005.1:** A comment on a task at the shipped `work`,
  `backlog` or `done` step shall queue the same runs, with the same reasons and
  payloads, as before this change.
- **AC-OFFICE-GATE-COMMENT-005.2:** Entering a gated step shall wake every seat in
  the role's fan-out population, as before this change; a step-entry fan-out
  shall neither skip decided seats nor exclude any author.
- **AC-OFFICE-GATE-COMMENT-005.3:** A participant fan-out that does not opt in to
  skipping decided seats shall wake every seat in its population, less any
  author excluded under AC-OFFICE-GATE-COMMENT-002.5.
- **AC-OFFICE-GATE-COMMENT-005.4:** Seat casting, quorum thresholds, the quorum
  guard's slate, run causation attribution and the self-trigger allowance shall be
  unchanged.

## Out of scope

Each exclusion below is a contract, not an omission.

- **Waking the runner on a gated-step comment.** At `review` and `approval` a
  comment no longer reaches the runner through the legacy assignee wake
  (AC-OFFICE-GATE-COMMENT-001.8). A commenter who needs the runner @-mentions it.
  A follow-up that wanted both would add a primary `queue_run` beside the fan-out
  and decide how a runner turn at a gated step interacts with its quorum guard.
- **Merging runs for one seat.** Three comments queue three runs per undecided
  seat, and one arriving while its step-entry run is queued adds a second run:
  comment-keyed runs are never coalesced. A follow-up would need a (task, seat,
  step) coalescing key and a rule for which comment's payload the merged run
  delivers.
- **A gate comment wake whose card leaves the gate before the run starts.** The
  run keeps the step it was queued at, as a step-entry wake does; reconciling
  it with the task's current step belongs to the run-claim path.
- **Waking other seats on the runner's own gate comment.** Both producers drop
  an assignee-authored comment before the engine sees it (AC-002.7), so under
  self-review the runner's gate comment wakes no other seat. A follow-up would
  narrow that short-circuit at gated steps and show `work` and `done` still
  skip the runner's own comment.
- **A gated task with no resolvable agent session.** A task that never had a
  session (for example placed at `review` by hand) keeps today's legacy assignee
  wake and wakes no seat; recovery is re-entering the step, as in
  `review-participant-seats.md`.
- **A failing comment action ahead of the fan-out.** A system-owned step keeps
  its own comment actions ahead of the reconciled fan-out (AC-004.2); the
  shipped template has none at the gates. If one fails on both evaluations
  (AC-001.15), no seat is woken. A follow-up would run the fan-out despite
  earlier failures, keeping abort-on-first-failure for other triggers.
- **Operator-owned workflows.** Never rewritten; the operator copies the
  template's comment-handler fan-out in through the workflow editor or import.
- **Human seats and human deciders.** Seats reference agent profiles only; a
  human decision that names no seat does not make any seat decided.
- **Showing seats on the task page.** Tracked separately.
- **The `backlog` step's missing handlers.** Unchanged, as in
  `review-participant-seats.md`.
- **New run reasons, metrics or UI copy.** The wake reuses `task_comment`, emits
  no new counter, and changes no rendered web UI.
