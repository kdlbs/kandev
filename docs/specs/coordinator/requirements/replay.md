---
id: coordinator-replay
title: Coordinator replay harness
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-10-02
---

# Coordinator replay harness Requirements

## Overview

Nothing can yet measure whether a change to the coordinator (a new context
text, a model, a note) helps. The replay harness re-runs past decided turns
against a candidate configuration, with the board frozen and every tool stubbed,
and compares what the candidate would have done with what the manager
actually decided. Phase 3.1 builds the harness and proves it can tell a bad
candidate from a good one before anything relies on it; it gates nothing in
phase 3.1. The evaluator, its cases and its scoring are outside anything a
coordinator or a dream can change.

## Terminology

- **Case:** one past turn that has at least one decided proposal, replayable
  because its snapshot and inputs still exist.
- **Candidate:** one change to the coordinator's instructions to test (a
  context text, a note, a standing order added or retired), applied to the same
  prompt frame, model and agent profile as the current configuration. A
  candidate never carries a model or profile of its own.
- **Baseline:** the coordinator's current configuration, replayed the same
  way.
- **Decision key:** what identifies a proposal for comparison: for
  `create_task` the workflow id and the title lower-cased with whitespace
  collapsed, for every other kind the kind and the target task id.
- **Expected reproduced:** a case's proposal that a manager approved without
  edits, which a candidate should produce again.
- **Expected avoided:** a case's proposal that a manager rejected or undid,
  which a candidate should not produce again. A proposal that was returned,
  approved with edits or approved automatically (and not undone) is neither
  expected reproduced nor expected avoided.
- **Case score:** expected reproduced plus expected avoided that a run met,
  over all expected reproduced and avoided of the case, in integer thousandths
  rounded half up.
- **Flip:** an expected reproduced proposal that a candidate misses.
- **Held-out case:** a case whose turn is not among the turns the candidate
  cites as its evidence.
- **Unmeasured:** the result of a candidate the harness cannot judge, shown
  as such and never as an improvement.
- **Run:** one model call over one case; a replay makes three runs per case
  and side, and the three attempts are numbered 1 to 3.
- **Ran:** a case ran on a side when at least two of its three attempts on
  that side succeeded.
- **Compared cases:** the cases that ran on both the candidate side and the
  baseline side. Every score the judge uses is over compared cases.

## Requirements

### REQ-COORDINATOR-REPLAY-001: Cases

**Intent:** The harness replays a bounded, fair set of past decisions.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-001.1:** For a coordinator, the system shall select
  as cases the newest 50 turns with a decided proposal, then up to 50 more
  turns that hold an override and a decided proposal and started within the 90
  days before the replay, not already selected; each group is ordered by the
  ledger turn's `started_at` descending with ties by the ledger turn `id`
  descending, and the cases run in that order, the first group before the
  second. With no such turn the replay has no cases (`no_cases`).
- **AC-COORDINATOR-REPLAY-001.2:** A turn shall be skipped, with the reason
  recorded per turn, when it was a dream (`dream_turn`), when its snapshot
  hash is empty or no snapshot row exists for it (`no_snapshot`; a snapshot
  survives while any turn of the last 90 days references it, so a turn's age
  alone never skips it), when its trigger message or a task title the replay
  needs is confirmed absent (`input_gone`), or when it has no expected
  reproduced and no expected avoided proposal (`no_expectation`), the first
  match in that order. A read that fails shall not skip a turn: the replay
  stops `unmeasured` with the reason `read_failed`. A skipped case shall not
  count in any score.
- **AC-COORDINATOR-REPLAY-001.3:** A case shall run as one prompt that holds
  only the instruction render for the configuration under test, the turn's
  trigger as stored, the turn's frozen snapshot and the titles of the tasks the
  replay needs (the turn's trigger task and the targets of its expected
  proposals; a title is a label read when the replay runs, never board state),
  and never the turn's own proposals, outcomes or decisions. No Kandev tool or
  MCP server shall be offered to the model, and the replay shall refuse, as
  `unmeasured` with the reason `profile_unsafe`, a profile that is auto-approve,
  has a command prefix or has any CLI flag. The candidate's proposals shall be
  read from a structured answer and recorded as data, and nothing the model
  says shall be executed by Kandev.
- **AC-COORDINATOR-REPLAY-001.4:** A replay shall never write to a task,
  proposal, activity row, setting or conversation of the coordinator, never
  start a task session of the coordinator's workspace, and never be
  triggered by a coordinator principal; its only writes are its own result row.
  The replay package's dependency closure shall contain no package that writes
  those records, and a test shall fail when it does.

### REQ-COORDINATOR-REPLAY-002: Runs and scores

**Intent:** A replay produces a score that repeats.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-002.1:** A replay shall run each case three times
  for the candidate and three times for the baseline, reusing the baseline's
  stored attempts of a case when the baseline's instruction hash, the model, the
  case and the prompt version are unchanged, and shall score a side as the
  median of its attempt means: attempt k's mean is the mean case score of attempt
  k over the compared cases whose attempt k succeeded, and the median is taken
  over the attempts that have at least one such case (the lower middle value
  when two remain, the value itself when one remains).
- **AC-COORDINATOR-REPLAY-002.2:** A proposal produced by a run that matches
  no expected proposal by decision key shall not be scored; the result shall
  report, for each side, the total of such proposals over all its successful
  attempts of the compared cases, counting each decision key once per attempt.
- **AC-COORDINATOR-REPLAY-002.3:** When a run fails (the model call errors,
  exceeds its per-call time, or its output cannot be read), the attempt shall
  count as failed, never as a miss; a case that did not run on a side shall be
  excluded from every score and from the guard, and the result shall list how
  many cases ran on each side and how many were compared.
- **AC-COORDINATOR-REPLAY-002.4:** A replay shall be charged to the
  coordinator's spend, priced with the price table the usage recorder uses from
  the tokens a run reports, or from an estimate when the run reports no tokens.
  It shall not start a run when the coordinator's 24 hour spend cannot be
  measured, or when that spend plus the bounds of the runs in flight plus this
  run's bound reaches the coordinator's cost ceiling (no ceiling set admits
  every run), and a replay stopped for that reason, or by its time bound, shall
  be `unmeasured` with the reason `budget`; a run whose model has no price
  shall make the replay `unmeasured` with the reason `cost_unknown`. A replay's
  cost shall count in that 24 hour spend while the replay is running and after
  it.
- **AC-COORDINATOR-REPLAY-002.5:** Two replays of the same candidate and the
  same cases with a model that answers identically shall return identical
  scores, flips and verdicts.
- **AC-COORDINATOR-REPLAY-002.6:** Each replay shall store one result row
  holding the candidate's hash, the baseline's, the model, the cases run and
  skipped with reasons and each attempt's decision keys, both scores over the
  compared cases and both over the held-out compared cases, the unmatched
  counts, the flips (the turn and proposal of each), the guard result and the
  verdict of `003` and `004`, the reason when `unmeasured`, its cost and its
  time. A replay started again for the same dream item shall make no model call
  and return the stored result of a finished row, or report that the replay is
  still running. A replay whose row cannot be written
  shall report that to its caller, which shall not store the verdict.

### REQ-COORDINATOR-REPLAY-003: Regression guard

**Intent:** A candidate that would have changed a decision the manager was
happy with is blocked, whatever else it improves.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-003.1:** The guard shall always run, over the
  compared cases. A side reproduces an expected reproduced proposal in a case
  when its decision key is in more than half of that side's successful attempts
  of the case. The guard shall block a candidate that does not reproduce an
  expected reproduced proposal that the baseline reproduces (a flip: with three
  successful attempts, a miss in at least two), and shall name the turns and
  proposals it flipped.
- **AC-COORDINATOR-REPLAY-003.2:** A flip shall block whatever the candidate's
  score, its improvement or the number of cases. A replay with no compared
  case shall not pass the guard: its guard result shall be `unmeasured`.
- **AC-COORDINATOR-REPLAY-003.3:** A proposal the manager approved with edits
  shall not be an expected reproduced proposal and shall not block.

### REQ-COORDINATOR-REPLAY-004: Improvement judge

**Intent:** An improvement is claimed only when there is enough data to claim
it.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-004.1:** The judge shall report a candidate as an
  improvement only when the guard passed, at least 20 held-out compared cases
  exist, and its score exceeds the baseline's on those cases by at least 0.05
  (50 thousandths).
- **AC-COORDINATOR-REPLAY-004.2:** With the guard not blocked and fewer than 20
  held-out compared cases, the judge shall report the candidate `unmeasured`
  (reason `too_few`) with the count and shall never report it as an
  improvement, a regression or a tie.
- **AC-COORDINATOR-REPLAY-004.3:** A blocked guard whatever the case count, or
  a guard that passed with a gain under 0.05 or a negative one, shall be reported
  `not_an_improvement`, with the two scores.
- **AC-COORDINATOR-REPLAY-004.4:** The judge's threshold, the minimum case
  count and the case window shall be constants of the harness, changeable
  only by a code change, and shall not be settable by a coordinator, a
  manager's request or a candidate.

### REQ-COORDINATOR-REPLAY-005: Proof of the harness

**Intent:** The measuring stick is shown to work before it decides anything.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-005.1:** The repository shall hold a fixed case set with at least 20 held-out cases and a fixed set of planted candidates with known-bad effects (two flip at least one expected reproduced proposal, one drops every proposal, one repeats a rejected proposal, which the harness scores as a miss of an expected avoided proposal) and one known-good candidate, and a test that runs the harness over them with a deterministic stub model and no network, and fails unless each planted candidate gets its recorded guard result and verdict (the two that flip and the one that drops every proposal are `blocked` and `not_an_improvement`; the one that repeats a rejected proposal passes the guard and is `not_an_improvement`) and the known-good candidate is reported `improvement`. One bad candidate shall be built so that its score would exceed the baseline's by the gain threshold were the guard off.
- **AC-COORDINATOR-REPLAY-005.2:** That test shall run in the backend's
  ordinary test run in continuous integration, and a change that makes any
  planted candidate pass shall fail it.
- **AC-COORDINATOR-REPLAY-005.3:** The evaluator, the cases, the scoring and
  the planted set shall be code and fixtures of the repository. The scorer
  shall read nothing at run time except the selected cases and the runs'
  answers, the thresholds shall be constants with no setter or key, and no
  coordinator tool shall reach the replay package or its fixtures; tests shall
  assert the constants' values and that dependency boundary.
- **AC-COORDINATOR-REPLAY-005.4:** Every shadow dream item that passes the
  gate shall be replayed (`AC-COORDINATOR-SHADOW-DREAM-004.1`), and its stored
  scores shall stay available for comparison with the owner's ratings of the
  same items.
- **AC-COORDINATOR-REPLAY-005.5:** In phase 3.1 the harness shall gate no
  approval, model change, setting or standing order; its verdict shall be
  shown only on the shadow report.

## Out of scope

- Gating a dream item going Active, a model or profile change, a standing-order
  promotion or an automatic raise (phase 3.5).
- A candidate that changes the model or agent profile (phase 3.5 model-change protocol).
- Replaying a turn of a coordinator on another coordinator's board.
- Replaying with live tools, a real board, or write access.
- A picker for candidates by a manager, or a replay of a manager-authored
  change (phase 3.5 model-change protocol).
- Scoring a turn that made no proposal.
