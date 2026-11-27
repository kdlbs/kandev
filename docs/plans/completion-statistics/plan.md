---
created: 2026-10-18
status: implemented
requirements:
  - REQ-TASKS-COMPLETION-STATS-001
system_design:
  - ../../specs/tasks/system-design/completion-statistics.md
legacy_specs: []
---

# Implementation Plan: Completion Statistics

## Overview

Align the three completion statistics queries with committed task state in one
sequential work order. ROOT reviewed the design and separately released
implementation to the same primary session. Task 01 is implemented and locally
verified; ordinary PR delivery is authorized, with merge reserved for ROOT.

Tasks owns completion state and its projections. The new vertical pair specifies
Statistics consumption while referencing existing completion-entry criteria.
No separate UI/analytics or Platform semantic specification is needed.

## Scope

Correct global totals, repository totals, and completed activity for unarchived
tasks. Preserve the archived metric, filters, attribution, clocks, timestamp
fallback, other aggregates, DTO/error contracts and native Statistics surfaces.
Do not change lifecycle, metadata, schemas, APIs, metrics, caches, frontend copy
or layout, all-time horizons, or add performance/freshness work.

## Evidence and technical approach

Admission on 2026-10-18: managed worktree
`/home/jcfs/.kandev/tasks/count-task-completio_626xqamu/kandev`, branch
`feature/count-task-completio-efi`, clean HEAD and actual remote main
`97b8e0d302e2eddb3939bd977659fce777bc82a4`. All ten qualified dependency blobs
match ROOT's receipt and source base `d0e53c9a6ff813ba1e7f70e84a5b21eaf85260b3`.

Read-only receipt:
`/tmp/kandev-root-stats-completion-discovery-t772wtgw/final-step-qualified-probe-6v1cb9pc/qualified-proof.json`.
ROOT's real workflow repository/service -> `TaskService.MoveTask` -> persisted
task -> analytics trace proves unchecked actual final entry remains TODO while
global/repository completed counts incorrectly return 1. Checked-final and
ordinary-middle controls passed. Actual Go exit 1, package 0.734 s; wrapper exit
0 is distinct. No replay or private probe source access is part of this plan.
Completed activity is independently confirmed by source inspection only until
permanent regression coverage executes its actual query.

Replace position inference in `stats.go` with state comparison, retain archive
compatibility, and remove unused step joins/projection. Add real-schema owning
analytics tests; correct legacy fixtures that accidentally equated final position
with completion. No source or permanent tests change during this design phase.

## Tests

New `completion_statistics_test.go` tests (split helpers into a neighboring test
file if needed for limits) provide the following acceptance mapping:

| Planned test | Acceptance criteria | Evidence |
| --- | --- | --- |
| `TestCompletionStatisticsCommittedState` | .1, .3, .8 | Actual three queries; state matrix, checked/unchecked final and non-final controls; readback unchanged |
| `TestCompletionStatisticsWorkflowEdits` | .2, .8 | Rename/reorder/setting writes, actual final eligibility; completed elsewhere/missing step stays counted |
| `TestCompletionStatisticsCompatibility` | .4, .5, .6, .7 | Mixed archived/states, ephemeral/automation/other workspace, multi-repo/deleted/empty repo, creation versus session/commit ranges; other aggregates |
| `TestCompletionStatisticsTimestampBuckets` | .3, .6, .7 | Multiple completed sessions, latest wins over archive, fallback/no timestamp/outside horizon, exact dates/zero-filled spine |
| `TestCompletionStatisticsQueryDialects` | .1, .4 | Existing repository query builder and rebinding through SQLite/pgx driver modes; supplement all-three-query executable SQLite coverage |

All IDs above have prefix `AC-TASKS-COMPLETION-STATS-001`. Generated SQL checks
are supplementary and cannot claim live PostgreSQL behavior. Existing affected
aggregate/exclusion/range tests retain numeric expectations with corrected task
state fixtures. Run all focused additions, controls, dialect fixtures and only
actually changed legacy tests before one complete owning analytics repository
race package. Use `GOTOOLCHAIN=local`, `GOMAXPROCS=2`, process-local test
`GOMEMLIMIT=512MiB`, `-trimpath -tags=fts5 -p=1 -count=1`. Focused RED/GREEN uses
Go five minutes inside GNU timeout six minutes; the race package uses Go ten
minutes inside GNU timeout eleven minutes, each with `--kill-after=10s`.
Scoped changed-Go lint has a separate process-local `GOMEMLIMIT=3GiB` cap.
Unexpected failures/timeouts require actual join/current-owned absence and
bounded ROOT evidence, without unconditional replay or resource increases.

## End-to-end evidence

ROOT's qualified first-party move-to-persisted-task-to-analytics trace supplies
lifecycle integration evidence; permanent real-writer analytics consumers prove
the projection, including the previously unexecuted activity query. Native
Statistics uses unchanged DTO fields on both viewports. No frontend code or
viewport interaction changes, so no artificial Playwright or web typecheck gate
is added. Public documentation assessment: internal design only at this phase;
existing README/feature-status Statistics descriptions remain accurate.

## Work orders

- [x] [Task 01: Count committed task completions](task-01-count-committed-completions.md)

## Verification results

Design validation on 2026-10-18:

- Catalog validation: exit 0, 386 decisions and 1638 specifications.
- Initial historical spec-linter tests: exit 0, 36 tests, not repeated and not a
  future implementation gate because no linter code changes. All-spec lint: exit 0.
- Catalog discovery includes both new Tasks documents. Local Markdown links,
  requirement/acceptance references, owner/design/plan coverage and all four
  files' whitespace: exit 0. Exactly four untracked artifacts, nothing staged.
- Initial wrong PR-docs wrapper: actual docs-only records returned `exempt`
  with no errors, but the wrapper required `covered` and exited 1.
- Separate historical prospective `stats.go` trigger with actual four-document
  contents: exit 0, `covered`, zero errors. No product file changed for that
  check; it does not replace actual docs-only coverage validation.
- Corrected actual four-document change-set wrapper: native join `524583`,
  exit 0, `exempt`, zero errors, exactly four actual changed document paths and
  no prospective production record. Future actual code changes require `covered`.

The design-only checkpoint above is historical. Implementation results:

- Frozen dependency install: pnpm 9.15.9, 937 cached packages, exit 0,
  native `97869` joined `7b0423`.
- Causal RED: actual three analytics consumers fail false-positive/negative
  numeric assertions, with valid checked-final and non-final controls. Exit 1,
  package 2.845 s, native `62320` joined `0d792c`.
- First GREEN attempt: exit 1, package 2.979 s, native `63455` joined `9fd8a7`.
  Numeric behavior passed; workflow-edit metadata compared an in-memory
  pre-session baseline against a real writer's later initial-session metadata.
  Corrected the oracle to compare full persisted snapshots after session setup.
  This fixture error is distinct from the product defect.
- Corrected focused GREEN: all five new top-level tests, 13 subtests and the
  changed legacy fixture passed, exit 0, package 2.902 s,
  native `23484` joined `277255`. No production changes followed this run.
- One complete owning repository race package: exit 0, 7.143 s,
  native `13850` joined `80cf4e`. Live PostgreSQL not run: DSN unset;
  generated SQLite/pgx repository SQL checks passed in focused GREEN.
- Scoped changed-Go lint: four actual Go paths, exit 0, zero issues,
  native `17481` joined `9bf4e1`, under process-local 3 GiB and GNU six-minute
  timeout with ten-second kill grace.
- Actual changed-code documentation coverage, catalog/all-spec/reference and
  whitespace checks pass. Normal hooks and remote delivery are recorded with
  the commit/PR evidence rather than asserted in advance.

All original local command handles above joined; no local background work is
left running. No resource increase or blind replay occurred. The sole heavy
lease remains held through normal hooks; serial merge authority is not granted.

## Risks

- Real constructor bootstrap can invalidate an assumed final step. Verify
  persisted order and positive controls before interpreting failures.
- Existing fixtures may encode the bug; repair intended task state rather than
  lowering assertions. Historical totals intentionally change for false counts.
- Totals and activity legitimately differ when no timestamp or an out-of-window
  timestamp exists. Preserve session-time precedence over archive time.
- Live PostgreSQL is unverified unless separately authorized; keep its DSN unset.
