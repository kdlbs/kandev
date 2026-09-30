---
id: coordinator-replay-design
title: Coordinator replay harness design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-REPLAY-001
  - REQ-COORDINATOR-REPLAY-002
  - REQ-COORDINATOR-REPLAY-003
  - REQ-COORDINATOR-REPLAY-004
  - REQ-COORDINATOR-REPLAY-005
---

# Coordinator replay harness System Design

## Purpose and boundaries

The harness re-runs decided past turns against a candidate configuration with
the board frozen and every tool stubbed, and scores what the candidate would
have proposed against what a manager decided. It is a library package,
`internal/coordinator/replay`, with one entry point, `Run(ctx, req)`, called by
the shadow dream and by tests; it has no HTTP route and no tool. Its evaluator,
cases, scoring and planted suite are repository code (`005.3`). It gates
nothing in phase 3.1 (`005.5`).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-REPLAY-001` | [Case selection](#case-selection), [Sandbox](#sandbox) |
| `REQ-COORDINATOR-REPLAY-002` | [Runs and scoring](#runs-and-scoring), [Budget](#budget), [Result row](#result-row) |
| `REQ-COORDINATOR-REPLAY-003` | [Guard](#guard) |
| `REQ-COORDINATOR-REPLAY-004` | [Judge](#judge) |
| `REQ-COORDINATOR-REPLAY-005` | [Planted suite](#planted-suite) |

## Case selection

`cases.Select(coordinatorID)` reads ledger turns with a decided proposal
(`coordinator_outcomes` rows joined on `turn_id`), newest 50 by `(started_at
DESC, id DESC)`, then every turn in the same window with a `coordinator_feedback`
row, at most 50 more, deduplicated by turn id, in the same order (`001.1`). The
window is the span of the first selection.

A case is skipped, with one reason per turn recorded, in this order: `dream_turn`
(trigger `dream`), `no_snapshot` (empty hash, or the snapshot row is missing; turn age plays no part, only whether the row still exists under the [snapshot retention rule](turn-ledger.md#retention)),
`input_gone` (the trigger message or a needed task title cannot be read),
`no_expectation` (no expected reproduced and no expected avoided proposal)
(`001.2`). The first matching reason is stored. Expected reproduced are
proposals of the turn decided `approved` by a manager (`automatic = false`;
an automatic approval is a system decision and is neither expected reproduced nor
expected avoided); expected avoided are `rejected`,
`returned` or `undone`; `edited` is neither (`003.3`). A skipped case counts in
no score.

## Sandbox

A case runs as one sessionless prompt through
`hostutility.Manager.ExecuteProfilePrompt(ctx, profileID, prompt)` (one prompt in,
text and token counts out; no task, no session, no tool round trip). The prompt
is the coordinator's `StandingInstructions` render for the configuration under
test, then the turn's trigger as it was, then the snapshot and stored records
as a data block with task titles read from the original records, then a fixed
answer schema: a JSON list of `{kind, target_task_id, workflow_id, title}`.
`stub.Parse` reads the answer into decision keys and rejects anything else (an
unreadable answer is a failed run). No Kandev tool (coordinator tool profile or MCP server) is offered to the call,
and nothing a model says is executed by Kandev; `stub` holds no store handle
(`001.3`). `ExecuteProfilePrompt` drives the profile's agent CLI, whose own
native tools cannot be removed by the harness, so the harness refuses a profile
whose `AutoApprove` is true (the case is a failed run with reason
`profile_unsafe`), which leaves any native tool that needs a permission denied. The package imports no writer of proposals, activity,
settings or conversation, enforced by an import-boundary test that fails when
`replay` or `stub` imports those packages (`001.4`). The only caller allowed to
start a run is the dream episode's server-side code and tests; the tool
surface has no entry (`001.4`).

## Decision key

`Key(p)`: for `create_task`, `workflow_id + "|" + lower(collapseSpaces(title))`;
for every other kind, `kind + "|" + target_task_id`. Collapse means trimming and
joining whitespace runs with one space; lower-casing uses Unicode simple
folding.

## Runs and scoring

For each case the harness runs the candidate three times and the baseline three
times unless a baseline result exists for the same `(baseline hash, model,
case id, prompt version)`, stored in the result row's per-case data and read by
the next replay (`002.1`). A run yields the list of keys.

Case score = (expected reproduced whose key is in the run + expected avoided
whose key is not) / (expected reproduced + expected avoided). A run's proposals
matching no expected key are not scored; the result counts them per side
(`002.2`). A candidate's score is the median over its three runs of the mean case
score over the cases that ran. Median of three is the middle value after sort,
so it is deterministic; ties are values, not order.

A run that errors, returns unreadable output or exhausts its budget counts as
not run. A case with fewer than two of its three runs is excluded, and the
result reports `cases_ran` (`002.3`). Determinism (`002.5`): runs use a fixed
seed where the runner supports one, cases and keys are ordered, and no map
iteration order reaches a result.

## Budget

`budget.Reserve(bound)` runs before each model call: it reads the coordinator's
spend through the phase 3 spend reader, which already includes this replay's
own `coordinator_replay_results` rows through `ExtraSpend` (below); the harness
adds no separate running total. When the reading is unmeasurable or `spent +
bound >= ceiling`, no run starts and the replay stops `unmeasured`, reason
`budget` (`002.4`). `bound` is the price, computed by
`commoncosts.CalculateCostSubcentsChecked`, of the configured replay maximum
output of 4000 tokens plus the prompt's input tokens estimated as bytes / 3 (a
constant `MaxOutputTokens = 4000` in `replay/constants.go`; the call has no
per-call maximum of its own, so an actual run may exceed `bound` and the next
`Reserve` sees the real figure). A sessionless call writes no usage row, so the
harness prices each run from its reported tokens and model with the same
function (the one the usage recorder uses); a model with no price stops the
replay `unmeasured`, reason `cost_unknown`. Each run's cost is added to the
result row's `cost_subcents`, which starts at 0 when the row is inserted, and the
phase 3 spend reader gains one additive, nil-safe term,
`ExtraSpend(coordinatorID, from, to)`, summing `cost_subcents` of
`coordinator_replay_results` whose `created_at` is in the window, running rows
included (only a NULL cost, which the design never writes, makes the reading
unmeasurable), so replays count toward the 24 hour spend and the ceiling. The
spend design is not edited; this term is listed as a delta in the ADR and has
its own test (a running row is counted once; a zero-cost running row does not
block admission). A replay also has a wall-clock bound of 20 minutes (a constant
`ReplayTimeout`, the dream's episode bound); past it the replay stops
`unmeasured`, reason `budget`, keeping the spend already incurred. Replay cost
counts in "dollars per merged task" because it is part of the coordinator's
spend.

## Guard

The guard evaluates each expected reproduced proposal that the baseline
reproduced in at least two of its three runs; it blocks when the candidate
reproduced it in fewer than two of three (a flip in at least two of three runs)
(`003.1`). Flips carry the turn and proposal ids. A flip blocks whatever the
score or improvement or case count (`003.2`). A replay with no case that ran
returns the guard result `unmeasured`, never `pass`.

## Judge

Constants in `replay/constants.go`: `MinHeldOut = 20`, `MinGain = 0.05`,
`CaseWindow = 50+50`. They have no setter and no config key (`004.4`).

Held-out cases are cases whose turn id is not among the candidate's cited turns
(a candidate with no citations, such as the planted set, holds out every case).
`replay.PromptVersion` is a constant of the harness, bumped on any change to the
prompt frame or the answer schema, and keys baseline reuse.

**Applying a candidate.** `context_diff` replaces the coordinator's context text
with the diff's result; `note_add`, `standing_order_add` and
`standing_order_retire` append or remove one dated section in the instruction
render the way the standing-order sections are rendered; `note_update` and
`note_retire` are never replayed (`no_target`).
Verdict order: the guard `blocked` gives `not_an_improvement`; fewer than
`MinHeldOut` held-out cases that ran gives `unmeasured` with the count (never an
improvement, regression or tie) (`004.2`); score minus baseline on held-out
cases `>= MinGain` gives `improvement` (`004.1`); everything else is
`not_an_improvement` with both scores (`004.3`). The comparison is in
integer thousandths to avoid float boundary flips: 0.05 is 50.

## Result row

`coordinator_replay_results`: `id`, `coordinator_id`, `status` (`running` or
`done`), `dream_id` (nullable),
`item_id` (nullable), `candidate_hash`, `baseline_hash`, `model`, `cases`
(JSON: per case run count, score, skip reason), `candidate_score`,
`baseline_score`, `flips` (JSON), `unmatched_candidate`, `unmatched_baseline`,
`guard` (`pass`, `blocked`, `unmeasured`), `verdict`, `reason` (for
`unmeasured`: `budget`, `cost_unknown`, `too_few`, `no_cases`, `interrupted`),
`prompt_version`, `cost_subcents`, `created_at`.
The row is inserted with `status = 'running'` before the first run and its `cost_subcents` is
updated after every run, so a crash mid-replay keeps the spend already incurred
(the ceiling never undercounts by more than one run); the rest of the row is
written at the end with `status = 'done'`, in one statement conditional on
`status = 'running'` (`002.6`). The settle of a stale row belongs to the dream
scheduler's backstop step (`Scheduler.Tick`, run each backstop pass for every
visited coordinator): `UPDATE ... SET status = 'done', guard = 'unmeasured',
verdict = 'unmeasured', reason = 'interrupted' WHERE status = 'running' AND
created_at < now - 30 minutes`; a late write by a replay that finished after
that matches nothing. `ExtraSpend` counts rows of both statuses.
Retention is 400 days, deleted with the coordinator.

## Planted suite

`replay/testdata/planted/` holds a fixed case set of at least 25 cases (so at
least 20 are held out for every candidate) and candidates as data files: four
known-bad (two flip an approved proposal, one proposes nothing, one repeats a
rejected proposal, which scores as a miss of an expected avoided proposal, not a
flip) and one known-good that reproduces every expected proposal and avoids every
expected avoided one where the baseline does not. The deterministic stub
model reads a marker in the candidate's context and answers from a fixture
table, with no network. `TestPlantedCandidates` runs `Run` over them and fails
unless every bad candidate is `blocked` or `not_an_improvement` and the good
candidate is `improvement`, so both verdict paths are proven (`005.1`). It runs in the ordinary backend test run
in CI; a mutation that makes a bad candidate pass, such as guard disabled by
constant, fails it (`005.2`). A second test loads the constants and asserts
values, so a silent edit is a visible diff. Nothing in the coordinator tool
surface, notes, standing orders or dream items can reference these paths
(`005.3`).

## Dream integration

The dream episode passes each gate-passing item to `Run` as a candidate (see
[shadow dream](shadow-dream.md#gate-and-replay)); the stored result is kept for
the agreement measure (`005.4`).

## Error handling

| Failure | Behaviour |
| --- | --- |
| Case read error | Skip with `input_gone` |
| Run failure | Not run, not a miss |
| Budget | Stop, `unmeasured` (`budget`) |
| Result write fails | Verdict returned to caller, error logged and counted |

## Testing

Selection order and ties, skip-reason precedence, key normalisation, score
arithmetic, median-of-three, guard flips at 1, 2 and 3 of 3, judge boundaries
(0.049, 0.05, 19 and 20 cases), determinism across two runs, the budget stop,
the import-boundary test, and the planted suite.
