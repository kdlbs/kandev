---
id: "02-outcomes-overrides"
title: "Outcome grading, override capture and measures"
status: draft
wave: 1
depends_on:
  - "01-turn-ledger"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-OUTCOMES-001
  - REQ-COORDINATOR-OUTCOMES-002
  - REQ-COORDINATOR-OUTCOMES-003
acceptance_criteria:
  - AC-COORDINATOR-OUTCOMES-001.1
  - AC-COORDINATOR-OUTCOMES-001.2
  - AC-COORDINATOR-OUTCOMES-001.3
  - AC-COORDINATOR-OUTCOMES-001.4
  - AC-COORDINATOR-OUTCOMES-001.5
  - AC-COORDINATOR-OUTCOMES-001.6
  - AC-COORDINATOR-OUTCOMES-002.1
  - AC-COORDINATOR-OUTCOMES-002.2
  - AC-COORDINATOR-OUTCOMES-002.3
  - AC-COORDINATOR-OUTCOMES-002.4
  - AC-COORDINATOR-OUTCOMES-002.5
  - AC-COORDINATOR-OUTCOMES-002.6
  - AC-COORDINATOR-OUTCOMES-003.1
  - AC-COORDINATOR-OUTCOMES-003.2
  - AC-COORDINATOR-OUTCOMES-003.3
system_design:
  - ../../specs/coordinator/system-design/outcomes.md
---

# Task 02: Outcome grading, override capture and measures (WP 3.1-2)

## Summary

Back-fills what came of each decided proposal, captures manager overrides as
feedback observations, and serves the five measures behind the flag. Grading
and capture record whether or not the flag is on.

## In scope

- `internal/coordinator/outcomes/`: the nil-safe `DecisionObserver` hook
  called after commit by `ApproveProposal`, `RejectProposal` and the undo
  path, `Grade`, `TaskResult` (pure), the grader queue and its three paths
  (decision event, step transition, 24-hour sweep), `overrides.Capture`,
  `reasons.Code`, the measures reader, with tests beside each.
- Store: `coordinator_outcomes` and `coordinator_feedback` with the unique
  index that makes an observation unique per proposal, kind and transition, and `coordinator_moveback_seen` (a history row is judged once); all three are deleted with their coordinator and workspace and pruned after 400 days.
- `GET /coordinators/:id/measures?days=` behind the flag; the agreement
  measure reads an `AgreementSource` interface whose default returns
  `no_data`, which work order 04 implements over ratings and replay verdicts.
- Metrics `coordinator_outcome_grade_failed_total{reason}` and
  `coordinator_override_ignored_total{reason}`.
- The decision observer is also called from the automatic-approval path
  (`approveAutomatically`, `finishClaim`) and `ReturnProposalTx`; the
  `automatic` column, the sweep over `coordinator_proposals` left-joined to
  outcomes (with backfill), the per-task moved-back scan of step-history rows
  with its retries and daily pass, and the counters above, per the
  [outcomes design](../../specs/coordinator/system-design/outcomes.md#override-capture).

## Out of scope

- Any use of an outcome to change configuration (phase 3.5).
- The Learning section that renders the measures (work order 04).
## Acceptance

- Two graders racing produce one row; `merged` and `dropped` never change; a
  reopening is counted once under concurrent graders.
- Cost is NULL, never zero, when usage is missing or unpriced.
- A redelivered override event, a grader run and a second observer store one
  observation; only manager overrides store one, and an authorisation error
  stores none.
- The moved-back matrix (earlier, equal, later step, before the action, task
  the coordinator did not create) stores only the earlier-step case.
- The measures return `null` with a reason, never zero; `days` outside 1 to
  90, empty, repeated or not an integer is 400; other workspace 404;
  coordinator principal 403.

## Validation

- `make -C apps/backend test` for the touched packages, with `synctest` for any timer and no `time.Sleep`; store conformance on SQLite and PostgreSQL for each new table or column.
- `python3 scripts/list-docs.py validate` if a specification changes.
