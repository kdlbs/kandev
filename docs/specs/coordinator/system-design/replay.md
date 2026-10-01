---
id: coordinator-replay-design
title: Coordinator replay harness design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-10-02
requirements:
  - REQ-COORDINATOR-REPLAY-001
  - REQ-COORDINATOR-REPLAY-002
  - REQ-COORDINATOR-REPLAY-003
  - REQ-COORDINATOR-REPLAY-004
  - REQ-COORDINATOR-REPLAY-005
---

# Coordinator replay harness System Design

## Purpose and boundaries

The harness re-runs decided past turns against a candidate instruction change
with the board frozen and every tool stubbed, and scores what the candidate would
have proposed against what a manager decided. It is a library package,
`internal/coordinator/replay`, with one entry point, `Harness.Run(ctx, req)`,
called by the shadow dream and by tests; it has no HTTP route and no tool. Its
evaluator, cases, scoring and planted suite are repository code (`005.3`). It
gates nothing in phase 3.1 (`005.5`). The `ExtraSpend` term of the spend reader
(below) is additive and nil-safe and is deliberately not gated by
`features.coordinatorPhase31`; the replay itself is only reachable from the dream,
which is.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-REPLAY-001` | [Case selection](#case-selection), [Sandbox](#sandbox) |
| `REQ-COORDINATOR-REPLAY-002` | [Run contract](#run-contract), [Runs and scoring](#runs-and-scoring), [Budget](#budget), [Result row](#result-row) |
| `REQ-COORDINATOR-REPLAY-003` | [Guard](#guard) |
| `REQ-COORDINATOR-REPLAY-004` | [Judge](#judge) |
| `REQ-COORDINATOR-REPLAY-005` | [Planted suite](#planted-suite) |

## Run contract

`replay.New(deps Deps) *Harness`. `Deps` holds only interfaces defined in
`replay`, so the package imports no coordinator, office or task package:
`Cases` (reads turns, outcomes, snapshots, trigger text and titles), `Prompts`
(`Run(ctx, profileID, prompt) (text, model, promptTokens, responseTokens,
error)`), `Prices` (`Lookup(model) (pricing, ok)`), `Spend` (`Reading(ctx,
coordinatorID, now) (windowSubcents, measurable, ceilingSubcents *int64)`),
`Instructions` (`Render(ctx, coordinatorID, Override) (text, hash, error)`),
`Results` (the result-row store) and `Clock`. The adapters over the real
packages live in `replay/wire`, which the dream's composition root builds; the
package `replay` and `replay/stub` do not import it.

`Request`: `CoordinatorID`; `Candidate` (`Kind`, `Text`, `TargetID` for
`standing_order_retire`, `CitedTurnIDs`); `DreamID` and `ItemID` (both empty in
tests, both set from the dream). `Result`: `RowID`, `Guard`, `Verdict`,
`Reason`, `CandidateScore`, `BaselineScore`, `HeldOutCandidateScore`,
`HeldOutBaselineScore`, `Flips`, `CasesRan`, `CasesCompared`, `Skipped`,
`CostSubcents`. `Run` returns `(Result, error)`: an error only when the row
could not be inserted (nothing ran, no cost) or the final write failed
(`ErrResultNotStored`, the Result still valid; the caller must not store the
verdict, and the row is later settled `interrupted`). The profile run is the
coordinator's own agent profile, resolved by `Prompts`; the model priced is the
model id that profile resolves to. A candidate `Kind` outside the six dream
item kinds is a caller bug and returns an error.

Idempotency: the row has a unique key on `(dream_id, item_id)` when both are
set. A second `Run` for the same pair finds the row; a `done` row returns its
stored result and makes no model call, a `running` row returns the in-progress
error `ErrReplayRunning` without starting a run (the dream lease already
serialises production callers). Context cancellation stops the replay at once,
keeps the spend, and stores `unmeasured` with the reason `cancelled`.

## Case selection

`cases.Select(coordinatorID)` reads ledger turns (`coordinator_turns`) that have
at least one `coordinator_outcomes` row with `turn_id` equal to the turn's `id`
and a decision in (`approved`, `edited`, `rejected`, `returned`, `undone`),
ordered `coordinator_turns.started_at DESC, coordinator_turns.id DESC`, the
first 50 (group one). Group two is turns of the same coordinator with a
`coordinator_feedback` row and a decided proposal, `started_at` within the 90
days before now, not in group one, ordered the same way, the first 50 (`001.1`).
`distinct` applies to the turn before the limit. An outcome row not yet graded
still counts as decided (grading only fills the final value). Cases run in
group one then group two order. Zero turns gives no cases: guard `unmeasured`,
verdict `unmeasured`, reason `no_cases`.

A case is skipped, with one reason per turn recorded, in this order: `dream_turn`
(trigger `dream`), `no_snapshot` (the snapshot hash is empty or no
`coordinator_turn_snapshots` row exists; the snapshot is kept while any turn of
the last 90 days references it, so turn age plays no part), `input_gone` (the
trigger message or a needed task title is confirmed absent: a read that returns
not-found), `no_expectation` (no expected reproduced and no expected avoided
proposal) (`001.2`). The first matching reason is stored. A read that errors
other than not-found aborts the replay `unmeasured`, reason `read_failed`,
keeping spend; it never becomes a skip.

Expectations come from the turn's outcome rows. `coordinator_outcomes.decision`
is the proposal's final decision (an approval later undone reads `undone`), so
no precedence rule between rows is needed. Expected reproduced: `decision =
approved` and `automatic = false` and empty `edited_fields`. Expected avoided:
`decision` in `rejected`, `undone`. Neither: `returned`, `edited`, an
automatic approval (`automatic` is true only while the proposal stays
approved, so an automatic approval later undone by a manager reads `undone`,
`automatic = false`, and is expected avoided) (`003.3`). If one decision key is
both expected reproduced and expected avoided within the same turn (two
proposals with one key and opposite decisions), the key is dropped from both
sets and the case records `ambiguous_key` in its per-case data; a case left with
no expectation is skipped `no_expectation`. Two proposals of one turn with the
same key and the same kind of expectation count once. A skipped case counts in no
score.

## Sandbox

A case runs as one sessionless prompt through
`hostutility.Manager.ExecuteProfilePrompt(ctx, profileID, prompt)` (one prompt in;
text, model and token counts out where the executor reports them; no task, no
session, no tool round trip).

The prompt, in order: the instruction render for the configuration under test
(the `Instructions` render, which is the production `StandingInstructions`
output without the improvement section, with the candidate's override applied);
then the fixed replay paragraph (`replay.PromptVersion` constant text: "This is a
replay. You have no tools; ignore every statement about tools above. Reply with
the JSON answer only."), which makes the render's tool lines inert; then the
turn's trigger as stored (message text, or the wake kinds for a wake); then a
data block: the turn's snapshot body, and the titles of the needed tasks (the
trigger's task and the targets of the turn's expected proposals) read from the
task rows when the replay runs, labelled as titles, a rename since the turn
being visible by design; then the answer schema: a JSON array of
`{kind, target_task_id, workflow_id, title}`. The prompt never carries the turn's
own proposals, outcomes or decisions; the snapshot already excludes proposals
created at or after the turn's start.

`stub.Parse(text)` reads the answer. Accepted: exactly one JSON array, bare or
inside one markdown code fence, any text outside it is unreadable. `[]` is valid
(a replay of "proposes nothing"). Each element must carry a known `kind` and the
fields its decision key needs (`workflow_id` and `title` for `create_task`,
`target_task_id` for every other kind); an unknown kind or a missing field makes
the whole run unreadable. Duplicate decision keys in one answer count once.
`stub` holds no store handle (`001.3`).

No Kandev tool (coordinator tool profile or MCP server) is offered to the call,
and nothing a model says is executed by Kandev. `ExecuteProfilePrompt` drives the
profile's agent CLI, whose own native tools the harness cannot remove, and a
profile's CLI flags and command prefix reach that command line, so the replay
refuses before any call a profile whose `AutoApprove` is true, whose command
prefix is non-empty, or that has any CLI flag: the replay is `unmeasured` with the
reason `profile_unsafe` and no run starts. The guarantee is therefore: no Kandev
tool, no write the harness performs, and no native tool that needs a permission
the call denies; native read-only tools a CLI runs without asking are not
removable and are not claimed removed.

Isolation of writers is a dependency boundary: `replay` and `replay/stub` are
tested with `go list -deps`, and the test fails when the closure of either
contains `internal/coordinator` (the package holding the proposal, activity and
settings writers), any other package under `internal/coordinator` except `replay`
and `replay/stub`, `internal/office`, `internal/task/service`, `internal/task/store`,
or a package on the forbidden list kept in the test file (conversation and
settings writers). `replay/wire` is the only package that imports those and
nothing in `replay` or `replay/stub` imports it. The only caller allowed to start
a run is the dream episode's server-side code and tests; the tool surface has no
entry (`001.4`).

## Decision key

`Key(p)`: for `create_task`, `workflow_id + "|" + normalise(title)`; for every
other kind, `kind + "|" + target_task_id`. `normalise(t)` is
`strings.ToLower(strings.Join(strings.Fields(t), " "))`: whitespace runs collapse
to one space, ends trimmed, then Go's Unicode lower-casing.

## Runs and scoring

For each case the harness makes three attempts for the candidate and three for
the baseline unless a baseline attempt set exists for the same `(baseline hash,
model, case id, prompt version)`. The baseline hash is the instruction render's
SHA-256 (the stamp's `prompt_hash` function, lower-case hex, with no override);
the model is the profile's resolved model id, and an empty model never reuses.
Reuse reads the newest `done` result row of the coordinator by `created_at DESC,
id DESC` that holds that case's attempts under that key, ignoring rows with
reason `interrupted` or `cancelled`, and takes only attempts that succeeded; a
case whose reusable attempts number fewer than three runs the missing attempts
fresh. Each attempt stores `ok` or `failed` and its sorted decision keys in the
row's per-case data (`002.1`).

Case score for an attempt = (expected reproduced whose key is in the attempt +
expected avoided whose key is not) / (expected reproduced + expected avoided),
as integer thousandths: `(2000*n + d) / (2*d)` in integer division, d > 0. Mean
of integer thousandths is the same half-up rounding of `sum/count`. A side's score:
for attempt index k in 1..3, the mean over compared cases whose attempt k
succeeded; the score is the median of the means of the indices that have at least
one such case (the middle value of three, the lower of two, the only one of one;
no index gives no score, the replay then has no compared case). Held-out scores
use only held-out compared cases, the same way. An attempt's proposals matching
no expected key are not scored; the result stores for each side the sum over all
its successful attempts of the compared cases of such distinct keys per attempt
(`002.2`).

An attempt that errors, exceeds `RunTimeout` or returns unreadable output is
failed. A case ran on a side with at least two successful attempts; the result
reports `cases_ran` per side and `cases_compared` (`002.3`). Determinism
(`002.5`): runs use a fixed seed where the runner supports one, cases and keys
are ordered, scheduling concurrency never reaches a score (results are collected
by case and attempt index), and no map iteration order reaches a result.

## Budget

`Concurrency = 4` calls run at once; each call is bounded by `RunTimeout = 90s`.
Before each call `budget.Reserve(bound)` runs under one mutex: it reads the
coordinator's spend through the phase 3 spend reader (which already includes this
replay's own `coordinator_replay_results` rows through `ExtraSpend`, below) and
refuses when the reading is unmeasurable, or when a ceiling is set and `window
spend + the bounds of calls in flight + bound >= ceiling` (no ceiling admits);
on admit it adds `bound` to the in-flight sum, and it removes it when the call
ends and its real cost has been written to the row. A refusal stops the replay
`unmeasured`, reason `budget`, after the calls in flight finish (`002.4`).
`bound` is the price of `MaxOutputTokens = 4000` output tokens plus the prompt's
input tokens estimated as bytes / 3 (constant in `replay/constants.go`; the call
has no per-call maximum, so an actual run may exceed `bound` and the next
`Reserve` sees the real figure).

Pricing a run: the executors behind `ExecuteProfilePrompt` do not report token
counts today (they return zero), so a run that reports zero prompt and zero
response tokens is priced from estimates, prompt bytes / 3 and response bytes / 3;
a run that reports tokens is priced from them. The model priced is the profile's
resolved model id, never a provider-reported one. The price is
`commoncosts.CalculateCostSubcentsChecked` over the pricing `Prices.Lookup`
returns (the same lookup the usage recorder uses, behind an interface so tests
stub it); a lookup that finds no price, errors or times out makes the replay
`unmeasured`, reason `cost_unknown`, keeping the spend already incurred. A
sessionless call writes no usage row, so each run's cost is added to the result
row's `cost_subcents` (starting at 0 when the row is inserted) in a single
`UPDATE ... SET cost_subcents = cost_subcents + ?`, which is race-free across
concurrent runs. A failed cost update is logged and counted and the in-memory
total is carried to the final write, which writes the larger of the two.

The phase 3 spend reader (`Service.Spend`) gains one additive, nil-safe term,
`ExtraSpend(coordinatorID, from, to)`, summing `cost_subcents` of
`coordinator_replay_results` whose `created_at` is in the window, running rows
included (only a NULL cost, which the design never writes, makes the reading
unmeasurable), so replays count toward the 24 hour spend and the ceiling,
including the `stopped_at_ceiling` check of a real unattended turn, which the
manager sees as the same spend figure with no separate attribution. The spend
design is not edited; this term is listed as a delta in the ADR and has its own
test (a running row is counted once; a zero-cost running row does not block
admission). A replay also has a wall-clock bound of 20 minutes (a constant
`ReplayTimeout`, the dream's episode bound; a full 100-case replay is 600
calls, which at four at once and the per-call bound may not finish, and
baseline reuse is what makes repeat replays fit); past it the replay stops
`unmeasured`, reason `budget`, keeping the spend already incurred. Replay cost
counts in "dollars per merged task" because it is part of the coordinator's
spend.

## Guard

Over compared cases only. For each expected reproduced proposal of a case, a side
reproduces it when its key is in more than half of that side's successful attempts
of the case (three attempts: two; two attempts: both). The guard evaluates each
proposal the baseline reproduces and blocks when the candidate does not
reproduce it (`003.1`); a case that did not run on either side is not evaluated
(a baseline-ran, candidate-excluded case simply leaves the compared set). Flips
carry the turn id and the proposal id. A flip blocks whatever the score or
improvement or case count (`003.2`). A replay with no compared case returns the
guard result `unmeasured`, never `pass`.

## Judge

Constants in `replay/constants.go`: `MinHeldOut = 20`, `MinGainThousandths = 50`,
`CaseWindow = 50+50`, `MaxOutputTokens = 4000`, `ReplayTimeout = 20m`,
`RunTimeout = 90s`, `Concurrency = 4`. They have no setter and no config key
(`004.4`).

Held-out cases are compared cases whose turn id is not among the candidate's cited
turns (a candidate with no citations, such as the planted set, holds out every
case). `replay.PromptVersion` is a constant of the harness, bumped on any change
to the prompt frame, the replay paragraph or the answer schema, and keys baseline
reuse.

**Applying a candidate.** The `Instructions` renderer receives an `Override` and
returns the production render (without the improvement section) with it
applied:
- `context_diff`: the item's text is the full replacement context text and
  replaces the coordinator's context in the render.
- `standing_order_add`: the render's standing-orders section gains one last
  numbered line `N+1. (added candidate, id candidate) <text>` (the same
  sanitiser and renderer as `StandingOrdersSection`, so a section appears even
  when no order exists).
- `standing_order_retire`: the order whose id equals `TargetID` is removed from
  the rendered list (the remaining orders renumbered); no such active order is
  `no_target`.
- `note_add`: one fixed section `Candidate note: <text>` is appended at the end
  of the render (the phase 3.1 instruction has no note section, so the template
  is part of `PromptVersion`).
- `note_update`, `note_retire`: never replayed, `no_target`.
`no_target` is a replay-level `unmeasured` with the reason `no_target`, stored,
with no model call and no cost. An empty resulting context text is applied as
empty. The baseline render uses the orders and context current when the replay
starts.

Verdict order: no compared case: guard `unmeasured`, verdict `unmeasured`
(`no_cases`); guard `blocked` gives `not_an_improvement`; fewer than `MinHeldOut`
held-out compared cases gives `unmeasured` (`too_few`) with the count (never an
improvement, regression or tie) (`004.2`); held-out candidate score minus held-out
baseline score in thousandths `>= MinGainThousandths` gives `improvement`
(`004.1`); everything else is `not_an_improvement` with both scores (`004.3`). The
shadow dream shows a guard `blocked` as `blocked` on its report; the row's
`verdict` stays `not_an_improvement`.

## Result row

`coordinator_replay_results`: `id`, `coordinator_id`, `status` (`running` or
`done`), `dream_id` (nullable), `item_id` (nullable), `candidate_hash` (SHA-256 of
the canonical JSON of the candidate's kind, text and target id), `baseline_hash`,
`model`, `cases` (JSON: per case id and turn id, skip reason, per attempt for each
side `ok`/`failed` and sorted keys, case scores, `ambiguous_key`), `candidate_score`,
`baseline_score` (both over compared cases), `heldout_candidate_score`,
`heldout_baseline_score`, `cases_ran_candidate`, `cases_ran_baseline`,
`cases_compared`, `flips` (JSON), `unmatched_candidate`, `unmatched_baseline`,
`guard` (`pass`, `blocked`, `unmeasured`), `verdict`, `reason` (for `unmeasured`:
`budget`, `cost_unknown`, `too_few`, `no_cases`, `no_target`, `profile_unsafe`,
`read_failed`, `cancelled`, `interrupted`), `prompt_version`, `cost_subcents`,
`created_at`, `finished_at`. A unique index on `(dream_id, item_id)` where both
are non-null.
The row is inserted with `status = 'running'` before the first run (an insert
failure means the replay does not start and `Run` returns the error) and its
`cost_subcents` is updated after every run, so a crash mid-replay keeps the spend
already incurred; the rest of the row is written at the end with `status =
'done'`, in one statement conditional on `status = 'running'` (`002.6`); when it
matches no row (the row was settled or deleted) `Run` returns `ErrResultNotStored`.
The settle of a stale row is a store method owned by this work order,
`SettleStale(now)`: `UPDATE ... SET status = 'done', guard = 'unmeasured',
verdict = 'unmeasured', reason = 'interrupted' WHERE status = 'running' AND
created_at < now - 30 minutes`; work order 04's scheduler calls it in its backstop
step for every visited coordinator (a coordinator holding a `running` result row is
always visited); a late write by a replay that finished after the settle matches
nothing. `ExtraSpend` counts rows of both statuses. Retention is 400 days (by
`created_at`), deleted with the coordinator in the same transaction as its other
rows; an update or `ExtraSpend` read after that finds nothing.

## Planted suite

`replay/testdata/planted/` holds a fixed case set of at least 25 cases (so at
least 20 are held out for every candidate) and candidates as data files: four
known-bad (two flip an approved proposal, one proposes nothing, one repeats a
rejected proposal, which scores as a miss of an expected avoided proposal, not a
flip) and one known-good that reproduces every expected proposal and avoids every
expected avoided one where the baseline does not. At least one bad candidate is
built to reproduce every approved proposal except one flipped, and to avoid every
rejected one the baseline repeats, so that its held-out score exceeds the
baseline's by at least the gain threshold and only the guard stops it. The
deterministic stub model (a `Prompts` and `Prices` and `Spend` stub, no network)
reads a marker in the candidate's context and answers from a fixture table.
`TestPlantedCandidates` runs `Run` over them and asserts each candidate's exact
guard result and verdict: the two flippers `blocked` and `not_an_improvement`, the
drop-all candidate `blocked` (it flips every baseline-reproduced approval) and
`not_an_improvement`, the repeat-rejected candidate `pass` and
`not_an_improvement`, and the good candidate `pass` and `improvement`, so both
verdict paths are proven (`005.1`). It runs in the ordinary backend test run in
CI; a mutation that makes a bad candidate pass, such as the guard disabled by
constant, fails it because the guard-only-stopped candidate would then be
`improvement` (`005.2`). A second test loads the constants and asserts values, so
a silent edit is a visible diff, and the import-boundary test above asserts the
dependency closure (`005.3`).

## Dream integration

The dream episode passes each gate-passing item to `Run` as a candidate (see
[shadow dream](shadow-dream.md#gate-and-replay)) with its dream and item ids; the
stored result is kept for the agreement measure (`005.4`).

## Error handling

| Failure | Behaviour |
| --- | --- |
| Not-found on trigger message or needed title | Skip with `input_gone` |
| Any other case read error | Stop `unmeasured`, `read_failed` |
| Run failure | Failed attempt, not a miss |
| Budget or time bound | Stop, `unmeasured` (`budget`) |
| No price, lookup error | Stop, `unmeasured` (`cost_unknown`) |
| Refused profile | `unmeasured` (`profile_unsafe`), no run |
| Context cancelled | `unmeasured` (`cancelled`), spend kept |
| Row insert fails | Replay does not start, error returned |
| Final write matches nothing or fails | `Result` returned with `ErrResultNotStored`, logged and counted; caller does not store the verdict |

## Testing

Selection order and ties, skip-reason precedence, key normalisation, score
arithmetic with rounding, median of three and of two, guard flips at 1, 2 and 3
of 3 and with two-attempt cases, judge boundaries (0.049, 0.05, 19 and 20
held-out compared cases, and a blocked guard with 19), determinism across two
runs, the budget stop with calls in flight, the zero-token estimate pricing, the
no-price stop, the idempotent second `Run`, the stale settle, the
import-boundary test, store conformance on SQLite and PostgreSQL, and the planted
suite.
