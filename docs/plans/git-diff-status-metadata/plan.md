---
created: 2026-10-02
status: completed
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
legacy_specs: []
---

# Implementation plan: Git diff status metadata

## Overview

Repair shared comparison-file classification in one sequential work order.
The confirmed whole-section substring search labels ordinary modifications
added/deleted/renamed when path or content contains metadata-looking phrases.
The parent reproduced six real `ShowCommit` failures (three paths, three added
content cases); the ordinary control passed. Archive:
`/tmp/kandev-commit-status-metadata-repro.go`. Parent owned handle 66585 joined
with expected exit 1, package time 0.453s; temporary test was removed.
This package records supplied evidence, not a new permanent test run.

## Scope

- Small raw extended-header status helper and its use in `git_log.go`.
- Focused parser, real `ShowCommit`/cumulative, and real-router repository regressions.
- Owning Platform requirement/design, boundary note, and one public reference clarification.
- Preserve paths/bytes, counts, comparison semantics, budgets, schemas, and routing.
- Approved test-only CI repair: inject the external-ID race fixture dependency
  during shared service construction, before its active cleanup worker starts.

Git commands/environments, workspace mutation, live porcelain/NUL numstat,
history providers, frontend layout, and new status enums are excluded.

## Technical approach

Follow the [metadata design](../../specs/platform/system-design/git-diff-file-metadata.md).
Remove whole-section `strings.Contains` status checks only. Classify full raw
extended metadata before applying existing budgets. Do not change the adjacent
path/count helpers or build a general parser abstraction.

| Caller / transport | Identity and intended behavior | Evidence |
| --- | --- | --- |
| Local `ShowCommit` | SHA and selected repository; full section status and uncapped patch | Real Git process and registered commit HTTP route |
| Local `GetCumulativeDiff` | Existing base-to-worktree comparison; same classifier with current budgets | Real Git process and selected/aggregate HTTP route |
| Aggregate cumulative HTTP | Repository-qualified keys, distinct same-name files, stored base and repository metadata | Two real task-root repositories with distinct sentinels |
| Existing WebSocket projection | Existing status fields forwarded through current result shapes | No transport changes; real producer/HTTP serialization evidence |
| Provider-only history | Existing provider contract | Unchanged; not routed through this parser |

## Tests

| Criteria | Permanent regression and location |
| --- | --- |
| .1 | `TestParseCommitDiffWithOptions_StatusMetadata` in new process `git_log_status_metadata_test.go`: all three phrases across paths, added/removed/context lines and hunk text, raw indentation, binary boundaries |
| .2 | Same parser table plus `TestGitDiffStatusMetadataCallers` in new process `git_log_status_metadata_integration_test.go`: real added/deleted/renamed controls, ordinary/mode-only/binary, misleading rename paths |
| .3 | Caller test asserts keys/paths/counts/full bytes; `TestGitDiffStatusMetadataHTTP` and `TestGitDiffStatusMetadataMultiRepoHTTP` in new API `git_status_metadata_test.go` assert serialized statuses, selected repo and aggregate identity |
| .4 | `TestParseCommitDiffWithOptions_StatusMetadataBudgets` plus existing `TestGetCumulativeDiff_TruncatesLargeFile`, `_BudgetExceeded`, `_CapsFileCount`, and `TestShowCommit_NotCapped` |
| .5 | `TestShowCommit_StatusMetadataRootAndEmpty`, existing `TestShowCommit_MergeCommitUsesFirstParentDiff`, count/path/prefix controls; real tests compare before/after index and worktree state |

## End-to-end and mobile assessment

Real Git -> public operator -> registered router -> decoded HTTP response provides
end-to-end metadata evidence. No frontend code or rendered interaction changes;
mobile-parity assessment requires no new browser flow, E2E shard or ASCII preview.

## Work orders

- [x] [Task 01: Classify file status from raw metadata](task-01-classify-status.md)

## Delivery gates

The initial design turn ended at the unstaged/uncommitted handoff. The later
explicit parent INTERRUPT approved permanent tests and production edits. Parent
owns approval; no operator or model-switch prompt. Use the existing session and
profile, no other workers/tasks/sessions, and preserve others' edits/caches.

After continuation: run RED/GREEN and the work order's exact affected checks,
record actual results, normal hooks/commit/push/ready PR, all current-head required
hosted CI, authenticated configured full semantic reports and actionable
dispositions, then normal expected-head squash merge and independent remote-SHA
verification plus owned cleanup. Never mark completion at PR creation. Preserve
published SHA once CI is underway; do not rebase for a moving main. One heavy
command at a time with `GOMAXPROCS=2`, Go `-p 2`, lint concurrency 2; retain and
join every handle. No broad suites, E2E, optional polish or speculative diagnostics.

## Verification results

Design-only checks on 2026-10-02:

- `python3 scripts/list-docs.py validate`: passed, 342 decisions and 1302 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog queries for Platform requirements and designs discover both new files.
- `/home/jcfs/.nvm/versions/node/v24.18.0/bin/node --test scripts/validate-public-docs.test.mjs`: passed, 62 tests.
- `/home/jcfs/.nvm/versions/node/v24.18.0/bin/node scripts/validate-public-docs.mjs`: passed, 47 pages.
- `validateCoverage` with the real four package documents and intended
  `git_log.go` change: `ok: true`, `status: covered`, no errors. This explicitly
  exercises requirement/design/manifest/work-order links rather than accepting
  the documentation-only exemption. It is local design coverage, not hosted PR evidence.
- `git diff --check` and package status inspection: passed; all six owned docs
  remain unstaged/uncommitted, including both new work-package files.
- Cheap disposable real Git grammar controls: raw name-status matched five
  modifications, one addition, one deletion and a rename with competing marker
  paths; separately committed empty addition/deletion emitted genuine metadata
  without hunks. An initial fixture correctly exposed Git pairing simultaneous
  identical empty deletion/addition as a rename; isolating the controls resolved
  the fixture. Both commands terminated and temporary repositories were removed.

The shell initially could not resolve `node`; all three Node checks were then
completed using its existing installed executable. No install or cache change.
No production/permanent tests or Go checks were run in the design turn.

Implementation completed after the later explicit parent continuation on
2026-10-02. The work order records permanent process and HTTP RED, process GREEN,
targeted race passes (process 2.803s, API 1.879s), and zero-issue scoped lint.
One lint-only fixture correction retained all assertions; its affected real
caller race check passed in 1.193s before the lint rerun passed.
Requirement is active and design current. The completed package status records
local implementation; hosted full semantic review, required CI, expected-head
squash merge and independently verified cleanup remain separate delivery gates
in the external task/session plan. No Go argv/environment/path/count/budget,
frontend, workspace-mutation or provider-history code changed.

Post-implementation documentation gates passed again: catalog validation (342
decisions, 1302 specifications), spec-lint tests (36) and all-file lint, public
docs tests (62) and validation (47 pages), actual package coverage (`covered`,
no errors), and diff whitespace checks. Frozen pnpm installation completed
with the existing cache for normal hooks; no tracked dependency files changed.

## Risks

- Trimming a raw line can turn hunk context into false metadata.
- Scanning past file/binary/hunk boundaries can classify payload as headers.
- Overrestricting legitimate modes or pure renames can regress positive controls.
- HTTP cumulative tests must modify a path already tracked at the comparison base;
  a newly created file would legitimately report added and conceal the defect.
- Preserve native Windows portability by distinguishing grammar fixtures from
  filesystem-specific names or mode operations.

## Hosted remediation

PR #4162 at `d25a678b643cd3d7cf52eec1d32e9e0d92165c4a` exposed two
`goconst` findings in the full-backend static check (run `37020135230`, job
`110881458429`). Naming the existing added/renamed status values in the same
source file addresses the findings without behavior or scope changes. The
recovered runtime lost the previous waiter handle and temporary receipts; it
provided no terminal verdict. Delivery continues in the existing task/session
with focused remediation, current-head hosted evidence and owned cleanup.

Remediation local validation: affected parser/budget race tests
(`^TestParseCommitDiffWithOptions_StatusMetadata`) passed in 1.055s, handle
3551 joined; scoped process lint with concurrency 2 and serial runners passed
with zero issues, handle 7578 joined. A mistaken repository-relative edit
launched an unchanged focused check first (1.054s, handle 98351 joined); it was
not GREEN evidence. No caller/API or broad local checks were replayed. Local
implementation status is complete; delivery still requires current-head hosted
review/CI, expected-head normal merge and independent merge/cleanup evidence.

## Bounded hosted race-fixture remediation

At `8e26e95b93da7ddfec33f15c94130408ae6b483d`, backend shard 1/2 job
`110901597055` (run `37026200617`, artifact `11235949221`) reported a data
race only in `TestCreateTaskWithExternalIDPrepareFailureAfterStepThreeMissRecovers`.
The test replaced the task repository after the shared fixture started its
cleanup worker. The parent approved test-only construction-time injection
through the shared fixture, retaining the active worker and exact assertions.
No production service or other fixture changes are included. The exact local
pre-change race run passed ten iterations (1.472s, handle 51616 joined); it does
not negate the authoritative CI race artifact. The corrected exact ten-iteration
race regression passed in 1.431s (handle 8347 joined). No service suite, caller
replay, synthetic replay, sleeps, retries or race suppression was used.

Changed-package task service lint passed with zero issues (handle 44786 joined),
concurrency 2 and serial runners. Actual current-PR documentation coverage is
checked before the normal hooked commit; hosted delivery remains pending.
