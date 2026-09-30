# ADR-2026-09-30-coordinator-phase-3-1-record-and-measure: Coordinator phase 3.1, record and measure

**Status:** proposed
**Date:** 2026-09-30
**Area:** backend, frontend, workflow

## Context

[ADR-2026-09-29-coordinator-phase-3-autonomy](2026-09-29-coordinator-phase-3-autonomy.md)
lets a coordinator act unattended within a containment check and a cost
ceiling. It records decisions, not turns: a turn that proposes nothing leaves no
trace, an attended turn has no row of its own, nothing says which model or
prompt produced a decision, and nothing says what came of a decision after it
was made. A later phase (3.5) is meant to let the coordinator propose changes to
its own instructions. That is unsafe to build before there is evidence to judge
it by and a measuring stick shown to work.

Phase 3.1 (Record and measure) builds that base and applies nothing: the
coordinator learns nothing and nothing it would learn is written anywhere a
turn reads. It has six work packages: the turn ledger with a stamp and a query
tool (3.1-1), outcome grading and override capture (3.1-2), the replay harness
(3.1-3), a shadow dream report (3.1-4), Pause (3.1-5) and Watches by project
(3.1-6). The requirements and system designs are the six new pairs under
[`docs/specs/coordinator/`](../specs/coordinator/README.md): turn ledger,
outcomes, replay, shadow dream, pause and, as an addition to permissions, projects
(with the design `watch-projects`). The delivery plan is
[`docs/plans/workspace-coordinator-p3-1/plan.md`](../plans/workspace-coordinator-p3-1/plan.md).

## Decision

### R1: the ledger is a thin table

`coordinator_turns` keyed to `task_session_turns` by `(session_id,
session_turn_id)`, plus a call digest table and a snapshot table. Tokens, cost,
transcripts and step transitions are joined, not copied. The proposal's "task
ids read" column is dropped: the board snapshot and the call digest already hold
every task id a turn saw or targeted, and a third copy would have to be
retained and scoped separately. Confirmed against the code: the phase 3
unattended-turn row, the usage ledger and `task_session_turns` already carry
every joined field.

### R2: replay window and snapshot retention

K is the newest 50 decided turns plus every turn holding an override in that
window, at most 50 more. Snapshots are kept 90 days, ledger rows 400 days (the
activity log's retention). Snapshots hold ids, states and times only, never text,
so a replay reads titles from the original records and skips a case
(`input_gone`) when one is gone.

### R3: held-out minimum

The improvement judge reports an improvement only with at least 20 held-out
cases that ran and a score at least 0.05 above the baseline; below 20 the item is
`unmeasured`, never an improvement, a regression or a tie. The regression guard
always runs and a single flip of an approved-without-edit decision blocks. The
constants are code, not configuration.

### R4: metrics

Approval-without-edit rate, override recurrence, dollars per merged task, the
time a proposal waits for a decision, and the agreement between owner ratings
and replay verdicts on the same shadow items. Each is `null` with a reason, never
zero, when it cannot be computed.

### R5: project selection ships with 3.1

Phase 2 (Watches by board) is built and in the integration line, so the card is
part of 3.1, as the proposal's default says.

### R6: a multi-repository task

A task is in scope when any of its repositories is watched. A project is a
`RepositorySet` (resolved live) or a single repository, never an Office project:
giving a task an Office `project_id` would make it an Office task. The filter is
stored as a list of entries with a kind, so another kind can be added without a
redesign. Every place that applies the workflow watch applies the projects
filter through one function.

The Lean of the proposal was right in all six cases. The only change is the
dropped "task ids read" column of R1 and the entry list of R6 holding both sets
and loose repositories.

## The wake-source amendment

Phase 3 states that a coordinator is woken only by episodes on its own tasks.
This ADR amends it: the **shadow dream is a system-defined, bounded episode and
the only added wake source**. It starts only from a server-side trigger evaluated
on the wake backstop pass (autonomy on, Shadow enabled by a manager, not paused,
at least 5 completed non-dream turns and a decided proposal or override in its
window, 24 hours since the last dream), never from a tool, a message, a wake or a
delivery. It runs under phase 3's containment check and spend ceiling: it is an
ephemeral coordinator-origin session, so its usage counts toward the 24 hour
spend and the ceiling stops it as it stops an unattended turn. In 3.1 it cannot
write: its tool profile is the single read tool `list_coordinator_turns_kandev`, a
constant not derived from the May do settings, and every other action from that
session is refused as unknown. Its output is a stored report. User-defined wake
sources remain phase 4.

## Flag boundary

`features.coordinatorPhase31` (`KANDEV_FEATURES_COORDINATOR_PHASE31`) is
registered with restart required, `prod`, `dev` and `e2e` profiles off, and is
effective only with `features.coordinator`, `coordinatorPhase2` and
`coordinatorPhase3`.

**Ship ahead (3.1-A and 3.1-B recording), independent of the flag**, under
`features.coordinator` and `coordinatorPhase2` alone: the turn ledger rows, call
digests and board snapshots, the stamp, the `turn_id` links, outcome grading,
override capture and the daily retention job, with their tables and columns.

**Behind the flag:** the query tool and every tool profile change, every read
route (ledger, measures, dreams, ratings, learning), the Learning section and
its screens, the shadow dream (scheduler, episode, gate, replay), the replay
harness's entry point, Pause (state, route, precondition, controls), and Projects
(fields, filter, enforcement, settings and setup UI, copilot hint).

**Why flag-off is safe.** Recording only observes events already published and
writes its own tables; it adds no tool, prompt text, route, admission check or
decision input, and a recording failure is logged and swallowed, so it can never
fail or delay a turn, a proposal or a decision. Migrations are additive and
replayable on SQLite and PostgreSQL, so a phase 3 database upgrades in place.
Read routes answer 404, and the flag-off boot differs from phase 3 only by extra
write-side tables, columns and observers. The recorded data is dense in what a
later phase needs and small in what it holds: no text, no argument, no result.

## Deltas to earlier designs

No full-size design is edited. Pause runs before `Admit` and does not renumber
the eight checks; the dream's spend counting relies on the existing
coordinator-origin spend read; the projects filter is a new design that adds a
second predicate beside the permissions design's workflow watch. The
permissions requirements gain REQ-COORDINATOR-PERMISSIONS-005 and a definition of
a watched task.

## G3.1 conditions

1. Phase 3 is merged with its exit met.
2. The phase 3.1 requirements, designs and plan are reviewed READY by a
   subagent and by the owner.
3. This ADR, including the wake-source amendment, is posted on
   [#3752](https://github.com/kdlbs/kandev/issues/3752) and
   [#4044](https://github.com/kdlbs/kandev/issues/4044) with screenshots of the
   Learning section and Pause, and a kdlbs maintainer replies without objecting
   or 10 working days pass (recorded here with the date).
4. R1 to R6 are recorded (above).

**Status of G3.1:** pending. Work orders 01 and 02 (recording) and 06 (projects)
need condition 1 only and may start when phase 3 merges; work orders 03, 04 and
05 need all four.

## Residual risk

- The shadow report can propose text the owner finds plausible; because nothing
  applies it and every item is replayed, the report is evidence, not authority.
- A replay is only as fair as its cases; small workspaces give `unmeasured`, by
  design.
- The ledger retains a digest of tool calls, which names task ids. It is limited
  to the coordinator's watch set on read and holds no text.
- Pause stops turns at the next ceiling-stop path; a failed stop leaves a turn
  running while the state says paused, and the state refuses every new start.

## Prior art

- **Wiki leg.** The vault resolved to
  `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki` with QMD collection
  `wiki`; reading it failed with "Operation not permitted", and
  `obsidian-wiki` and `qmd` are not installed. No claim here rests on the wiki.
- **saas-kb leg.** The saas-kb tool is not available in this environment, so
  nothing rests on it.
- **Source analysis.** Part 1 of the post-phase-3 proposal (revision 2), the
  learning-loop proposal and the task-grouping study; the regression guard plus
  a held-out improvement judge follows the proposal's gbrain rule.
- **Phase 3 designs.** Reused: admission, containment, spend and the usage
  observer, the automatic class and the activity log's retention.

## Consequences

- Ten new tables and columns on `coordinators`, `coordinator_proposals`,
  `coordinator_activity` and `coordinator_unattended_turns`, plus observers,
  a retention job and, behind the flag, a scheduler tick, routes and the
  Learning section.
- The wake backstop pass gains one call (the dream tick); admission is unchanged.
- A planted-regression suite runs in CI on every backend run.
- Screens: Learning section and report detail, the Pause control on the
  autonomy strip and in settings, the Projects part of Watches and setup, and the
  copilot's scope hint.
- Copy in six locales, no em dash.

## Alternatives considered

- **A view over existing tables instead of a ledger.** Rejected: a turn that
  proposes nothing has no row to view.
- **Storing tokens and cost on the row.** Rejected: they change after the turn as
  usage is priced; joining keeps one source of truth.
- **Reading the model from the agent profile.** Rejected: the profile states what
  was configured, not what answered.
- **Gating recording behind the flag.** Rejected: evidence accrues only once
  recording exists, and recording carries no risk.
- **Using Office projects.** Rejected: it would make each such task an Office
  task.
- **Letting the dream call Kandev write tools with a "shadow" instruction.**
  Rejected: safety by instruction is not safety; the profile is a constant.
- **A manual "Dream now".** Deferred; the trigger is the only start.
