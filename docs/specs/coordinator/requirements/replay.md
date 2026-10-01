---
id: coordinator-replay
title: Coordinator replay harness
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
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
- **Candidate:** a configuration to test: a context text, and optionally a
  model or agent profile, applied to the same tools and prompt frame as the
  current configuration.
- **Baseline:** the coordinator's current configuration, replayed the same
  way.
- **Decision key:** what identifies a proposal for comparison: for
  `create_task` the workflow id and the title lower-cased with whitespace
  collapsed, for every other kind the kind and the target task id.
- **Expected reproduced:** a case's proposal that a manager approved without
  edits, which a candidate should produce again.
- **Expected avoided:** a case's proposal that a manager rejected or undid,
  which a candidate should not produce again.
- **Case score:** expected reproduced plus expected avoided that the
  candidate met, over all expected reproduced and avoided of the case.
- **Flip:** an expected reproduced proposal that a candidate misses.
- **Held-out case:** a case whose turn is not among the turns the candidate
  cites as its evidence.
- **Unmeasured:** the result of a candidate the harness cannot judge, shown
  as such and never as an improvement.
- **Run:** one execution of a candidate over the cases; a replay is three runs.

## Requirements

### REQ-COORDINATOR-REPLAY-001: Cases

**Intent:** The harness replays a bounded, fair set of past decisions.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-001.1:** For a coordinator, the system shall select
  as cases the newest 50 turns with a decided proposal, and every turn in the
  same window that holds an override, at most 50 more, in the order newest
  first with ties by ledger row id descending, without repeating a turn.
- **AC-COORDINATOR-REPLAY-001.2:** A turn shall be skipped, with the reason
  recorded per turn, when its board snapshot is empty or older than 90 days
  (`no_snapshot`), when its trigger message or a task title it needs no
  longer exists (`input_gone`), when it has no expected reproduced and no
  expected avoided proposal (`no_expectation`), or when it was a dream
  (`dream_turn`). A skipped case shall not count in any score.
- **AC-COORDINATOR-REPLAY-001.3:** A case shall run as one prompt that holds the case's frozen snapshot and stored records, never live board state, with no tool available to the model: the candidate's proposals shall be read from a structured answer and recorded as data, and nothing shall be executed.
- **AC-COORDINATOR-REPLAY-001.4:** A replay shall never write to a task,
  proposal, activity row, setting or conversation of the coordinator, never
  start a task session of the coordinator's workspace, and never be
  triggered by a coordinator principal.

### REQ-COORDINATOR-REPLAY-002: Runs and scores

**Intent:** A replay produces a score that repeats.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-002.1:** A replay shall run each case three times
  for the candidate and reuse the baseline's runs of the same cases when the
  baseline configuration, model and case are unchanged, and shall score a
  candidate as the median over its three runs of the mean case score over the
  cases that ran.
- **AC-COORDINATOR-REPLAY-002.2:** A proposal produced by a run that matches
  no expected proposal by decision key shall not be scored; the result shall
  report how many such proposals each side produced.
- **AC-COORDINATOR-REPLAY-002.3:** When a run fails (the model call errors,
  its output cannot be read, or its budget is exhausted), the case's run shall
  count as not run, never as a miss; a case with fewer than two of its three
  runs shall be excluded, and the result shall list how many cases ran.
- **AC-COORDINATOR-REPLAY-002.4:** A replay shall be charged to the coordinator's spend, priced from the tokens each run reports with the price table the usage recorder uses: it shall not start a run when the coordinator's spend cannot be measured or the cost of the runs so far plus a run's bound reaches the ceiling, and a replay stopped for that reason shall be `unmeasured` with the reason `budget`; a run whose model has no price shall make the replay `unmeasured` with the reason `cost_unknown`.
- **AC-COORDINATOR-REPLAY-002.5:** Two replays of the same candidate and the
  same cases with a model that answers identically shall return identical
  scores, flips and verdicts.
- **AC-COORDINATOR-REPLAY-002.6:** Each replay shall store one result row
  holding the candidate's hash, the baseline's, the model, the cases run and
  skipped with reasons, both scores, the flips (the turn and proposal of
  each), the mode verdict of `003` and `004`, its cost and its time.

### REQ-COORDINATOR-REPLAY-003: Regression guard

**Intent:** A candidate that would have changed a decision the manager was
happy with is blocked, whatever else it improves.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-003.1:** The guard shall always run. It shall block
  a candidate that flips any expected reproduced proposal in at least two of
  three runs where the baseline reproduced it in at least two of its three
  runs, and shall name the turns and proposals it flipped.
- **AC-COORDINATOR-REPLAY-003.2:** A flip shall block whatever the candidate's
  score, its improvement or the number of cases. A replay with no case that
  ran shall not pass the guard: its guard result shall be `unmeasured`.
- **AC-COORDINATOR-REPLAY-003.3:** A proposal the manager approved with edits
  shall not be an expected reproduced proposal and shall not block.

### REQ-COORDINATOR-REPLAY-004: Improvement judge

**Intent:** An improvement is claimed only when there is enough data to claim
it.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-004.1:** The judge shall report a candidate as an
  improvement only when the guard passed, at least 20 held-out cases ran, and
  its score exceeds the baseline's on those cases by at least 0.05.
- **AC-COORDINATOR-REPLAY-004.2:** With fewer than 20 held-out cases that ran,
  the judge shall report the candidate `unmeasured` with the count and shall
  never report it as an improvement, a regression or a tie.
- **AC-COORDINATOR-REPLAY-004.3:** A score within 0.05 of the baseline's,
  below it, or a blocked guard shall be reported `not_an_improvement`, with
  the two scores.
- **AC-COORDINATOR-REPLAY-004.4:** The judge's threshold, the minimum case
  count and the case window shall be constants of the harness, changeable
  only by a code change, and shall not be settable by a coordinator, a
  manager's request or a candidate.

### REQ-COORDINATOR-REPLAY-005: Proof of the harness

**Intent:** The measuring stick is shown to work before it decides anything.

#### Acceptance criteria

- **AC-COORDINATOR-REPLAY-005.1:** The repository shall hold a fixed case set with at least 20 held-out cases and a fixed set of planted candidates with known-bad effects (two flip at least one expected reproduced proposal, one drops every proposal, one repeats a rejected proposal, which the harness scores as a miss of an expected avoided proposal) and one known-good candidate, and a test that runs the harness over them with a deterministic stub model and no network, and fails unless every planted candidate is blocked or reported `not_an_improvement` and the known-good candidate is reported `improvement`.
- **AC-COORDINATOR-REPLAY-005.2:** That test shall run in the backend's
  ordinary test run in continuous integration, and a change that makes any
  planted candidate pass shall fail it.
- **AC-COORDINATOR-REPLAY-005.3:** The evaluator, the cases, the scoring and
  the planted set shall be code and fixtures of the repository; no
  coordinator tool, standing order, note or dream item shall be able to
  name, edit or disable them.
- **AC-COORDINATOR-REPLAY-005.4:** Every shadow dream shall be replayed
  (`AC-COORDINATOR-SHADOW-DREAM-004.1`), and its stored scores shall stay
  available for comparison with the owner's ratings of the same items.
- **AC-COORDINATOR-REPLAY-005.5:** In phase 3.1 the harness shall gate no
  approval, model change, setting or standing order; its verdict shall be
  shown only on the shadow report.

## Out of scope

- Gating a dream item going Active, a model or profile change, a standing-order
  promotion or an automatic raise (phase 3.5).
- Replaying a turn of a coordinator on another coordinator's board.
- Replaying with live tools, a real board, or write access.
- A picker for candidates by a manager, or a replay of a manager-authored
  change (phase 3.5 model-change protocol).
- Scoring a turn that made no proposal.
