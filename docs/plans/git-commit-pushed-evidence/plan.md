---
created: 2026-10-03
status: done
requirements:
  - REQ-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001
system_design:
  - ../../specs/platform/system-design/git-commit-pushed-evidence.md
legacy_specs: []
---

# Implementation plan: Faithful pushed commit evidence

## Overview and checkpoint

Correct the bounded local pushed lookup while preserving both commit-list graph
modes and all downstream contracts. One sequential work order owns the real-Git
regression, narrow producer correction, HTTP proof, and affected checks.
The parent reviewed the complete design package and released implementation in
a later explicit continuation on 2026-10-03. Work proceeds in the same session
and profile, with no delegation. Actual hosted delivery remains a separate gate.

## Evidence and assumption check

Source HEAD and authoritative main were both
`b330ad97a8712eb7b1a8ee2863b46ba302a92acb` at design inspection.
The parent archives `/tmp/kandev-commit-pushed-reachability-repro.go` and
`/tmp/kandev-commit-pushed-reachability-recovery.log` retain the actual operator
fixture and joined exit-1 result: a local Jan 1 feature commit was falsely
pushed after merging Feb 1/Mar 1 side commits without publication. A real push
passed the positive control. The lost pre-crash diagnostic has no verdict.
Do not rerun the unchanged temporary proof or remove either parent archive.

Confirmed intent: upstream reachability, both graph modes, conservative
unavailable evidence, repository isolation, existing batch/read budgets, and
no provider or mutation-policy redesign. Verified source: first-parent range
selection is paired with a capped all-parent evidence query; the Changes merge
projection trusts true without a PR. No material product question remains.

A design-only disposable Git plumbing experiment also passed mixed reachability,
full-graph limited side membership, and upstream second-parent ancestry cases
on Git 2.43.0. Its fixture was removed. It is strategy evidence, not permanent
RED/GREEN or implementation validation.

## Scope and technical approach

- Pass base traversal context into `GitOperator.markPushedCommits` and pin the
  positive lookup root to the returned tip. Match first-parent range selection
  only for nonempty bases; preserve full-parent upstream exclusion and the N cap.
- Add `process/git_log_pushed_test.go` and `api/git_log_pushed_test.go` with real
  disposable repositories. Keep source changes in `process/git_log.go`.
- Reuse existing route, repository manager, comparison-base, and consumer paths.
  No frontend change is expected. No schema, dependency, config, or public-doc
  change is required beyond the owning Platform boundary clarification.

Excluded: provider/PR/MR history, task delivery ledger, version-resolution
authorization, file statuses, base recovery, UI presentation, broad graph/cache
architecture, uncapped history scans, and per-commit subprocess fan-out.

## Tests and traceability

All process tests below live in the new `git_log_pushed_test.go`; names are the
implementation targets. Cases can share a small fixture without making expected
truth depend on the production lookup.

| Evidence | Criteria |
| --- | --- |
| `TestGetLog_PushedFirstParentMerge`: deterministic retained topology, both local rows false; real push makes both true; exact order, parents and stats | .1, .2, .5 |
| `TestGetLog_PushedMixedFirstParent`: published older branch row plus local merge/new row, without including the side history | .1, .2 |
| `TestGetLog_PushedRecentFullGraph`: limited and default recent modes include side rows; mixed pushed/local truth and limit retained | .1, .2, .5 |
| `TestGetLog_PushedUpstreamSecondParent`: upstream contains a returned branch commit through its second parent; also full-graph side rows reachable via an upstream merge | .1, .2 |
| `TestGetLog_PushedWithoutUpstream`: no configured upstream and missing tracked ref leave usable false rows | .3 |
| `TestMarkPushedCommits_LookupFailure`: real ref lookup followed by a deliberately invalid traversal base; cancelled evidence; every row stays false | .3, .6 |
| `TestMarkPushedCommits_AnchoredLogTip`: capture rows, advance fixture HEAD, enrich original rows against their returned tip | .1, .6 |
| API `TestHandleGitLog_PushedRepositoryEvidence`: registered selected/aggregate route, same SHAs but different upstreams, repository identity, stable read snapshots | .1, .4, .5, .6 |

Every criterion in this table belongs to
`AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001`.
Existing `TestGetLog_NoBaseReturnsFullGraph`,
`TestGetLog_FirstParentSkipsMergedInCommits`, stale-base controls, route limit/since
controls, and multi-repository comparison tests stay in the affected runs.
The registered HTTP producer test supplies end-to-end evidence without an app,
browser, server port, database, provider, or network service.

Consumer assessment: `changes-panel.test.ts` already tests no-PR mixed backend
flags and provider overlays; `changes-panel-remote.test.ts` covers repository
identity and divergence. Read and retain these tests. Do not add a frontend test
that merely repeats their existing assertions. If actual projection changes
become necessary, amend this package and apply the pure state/data mobile note.

## Work orders

- [x] [Task 01: Match bounded pushed evidence to returned history](task-01-faithful-pushed-lookup.md) (`done`, sequential; no dependencies).

## Validation and delivery

Exact commands and RED/GREEN sequence are in the work order. Use one local
heavy command at a time with GOMAXPROCS=2, GOMEMLIMIT=512MiB, `-trimpath`, and
`-p 1`; retain/join every handle. No broad Go/Vitest/E2E audit is planned.
Normal active hooks run; a fresh worktree requires exactly one frozen pnpm
installation from `apps` before hook/package commands. Do not alter lockfiles,
bypass hooks, amend commits, kill foreign processes, or clear shared caches.

After later release and successful affected checks, commit/push/open ready PR
under existing authorization. Use one managed `scripts/pr-await` for hosted
gates, join it before any replacement, and require actual required terminal
checks plus substantive authenticated full all-file semantic review of the
current SHA. Inspect coverage/body, not an ACK or skipped check; disposition
every finding. A completed full CodeRabbit report is sufficient; request one
necessary full review only when a corrected head lacks full coverage. Preserve
the published SHA when main drifts. Merge normally with the expected head,
independently verify actual merged SHA, remote and owned content, and clean only
owned resources. Completion requires actual merge. Parent owns archival and
next-child release; do not create workers, tasks, or sessions.

## Risks

- Applying first-parent mode to the no-base query loses side-history evidence.
- Narrowing negative ancestry mislabels commits published through an upstream merge.
- A moving positive root can crowd out an already returned local row; use its captured tip.
- Fixture helpers strip date variables. Pin every relevant date in an isolated
  command environment after stripping ambient `GIT_*`; register cleanup early.
- Tracking refs are cached local evidence, not an assertion of current server state.

## Verification results

Design-only strategy experiment passed. Documentation gates passed:
`python3 scripts/list-docs.py validate` (343 decisions, 1317 specifications),
`python3 scripts/lint-spec-files.test.py` (36 tests),
`python3 scripts/lint-spec-files.py --all`, `git diff --check`, and the local
`validateCoverage` preflight from `.github/scripts/pr-docs.cjs` against the new
package plus the prospective production path (`covered`, no errors). The
catalog discovers both new Platform documents. The preflight uses current
workspace files and does not claim hosted coverage or implemented source.

The design checkpoint ended without production/permanent tests, staging,
commits, or a PR. After the later explicit release, permanent
`TestGetLog_PushedFirstParentMerge` failed for the expected older local row's
false-pushed assertion (exit 1, package 0.147s); the real-push control passed.
After the bounded correction, its focused GREEN passed (exit 0, 0.160s).

The work-order process race command passed (exit 0, 2.321s), including all seven
new process cases and existing GetLog controls. Its API race command passed
(exit 0, 2.489s), including registered selected/aggregate repository evidence,
unchanged read snapshots, and the existing limit/base controls. The exact scoped
golangci-lint command passed (exit 0, zero issues). All local implementation
checks passed. The final catalog/specification/whitespace gates and actual-file
local PR coverage preflight passed; the single frozen pnpm install completed
without lockfile changes. Normal hooks and hosted publication/review/merge remain pending
and are recorded in the external task plan. Package status records local work
completion, not an actual merge verdict.
