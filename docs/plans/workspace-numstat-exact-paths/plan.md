---
created: 2026-10-02
status: completed
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
  - ../../specs/platform/system-design/workspace-git-path-details.md
legacy_specs: []
---

# Implementation Plan: Exact workspace numstat paths

## Overview

Repair workspace diff association in one sequential work order. The parent reviewed the complete
design package and explicitly authorized implementation on 2026-10-02. Task 01 is complete.
Parent owns coordination; no extra workers, tasks, or sessions were used.

## Confirmed root cause and evidence

Inspected checkout and actual remote main: `6979c7642d2e73f6c765155a417713c6c05ac088`.
`workspace_git_diff.go` requests human-readable numstat in all three per-file enrichment phases.
`parseNumstatEntry` trims real whitespace and leaves C-quoted paths encoded.
`resolveNumstatPath` treats literal ` => ` inside a filename as a rename.
These keys miss the actual `Files` entry, silently retaining an empty ready diff and zero additions.

The parent's reusable `/tmp/kandev-workspace-numstat-paths-repro.go` was read and repeated through
a temporary test on this checkout. All six `GetGitStatusWithDetails(ctx, true)` cases failed:
`café.txt`, `literal => name.txt`, and `trailing.txt `, each staged and unstaged.
Targeted Go package elapsed 0.700s, exit 1. The temporary test was removed and its process joined.
No app, browser, or database was started.

A disposable raw subprocess probe on Git 2.43.0 confirmed ordinary NUL records and actual rename
old/new records for HEAD, cached, and index-to-worktree comparisons. Human-readable simple and
brace-style renames both become the same explicit old/new framing under `-z`. Pure cached renames
produce `0<TAB>0<TAB><NUL>old<NUL>new<NUL>`; binary records produce `-<TAB>-<TAB>path<NUL>`.

## Scope and assumptions

Confirmed scope: exact path association across all three representations, genuine rename destinations,
normal names, binary and content-unchanged records; preserve lifecycle and resource contracts.
The defect violates existing mixed-facet criterion `.9`; criterion `.36` makes the previously implicit
exact-name contract explicit. No material unresolved choice remains.

Excluded: UI/API/schema changes, watcher/status/history parser redesign, matcher dependencies,
non-UTF-8 wire serialization changes, pathspec redesign, and branch-total format changes.

## Technical approach

Add a small workspace-specific NUL parser in `workspace_git_numstat.go`; reuse `numstatEntry`.
Change only the three path-bearing numstat commands and iteration in `workspace_git_diff.go`.
Retain `parseNumstatEntry`/`resolveNumstatPath`: `git_log.go` still consumes text rows through
`numstatByPath`. C-unquoting alone leaves literal arrows ambiguous; disabling `core.quotePath`
does not preserve control-character framing. NUL records resolve both problems without guessing.

Keep existing per-file patch execution, membership lookup, comparison bases, state propagation,
budgets, admission, and cancellation. No new ADR is needed for a local format correction that
preserves the established public identity and ownership contracts.

The primary design is already 32,221 bytes, close to the 32 KiB limit. A linked focused design
records this parsing capability without expanding or restructuring unrelated lifecycle sections.
Completed mixed-facet, progressive-refresh, and scalability packages retain their historical results;
this work neither reopens their work orders nor claims new browser validation.

## Tests

New focused files: `workspace_git_numstat_test.go` and `workspace_git_diff_paths_test.go` in
`apps/backend/internal/agentctl/server/process/`.

| Criteria | Planned test | Evidence |
| --- | --- | --- |
| `.36` | `TestParseWorkspaceNumstatZ` | Exact normal, Unicode, quote/backslash, arrow/brace, leading/trailing-space, tab/newline bytes; ordinary and rename records; multiple adjacent records; binary `-`, zero counts, empty and incomplete framing. |
| `.36` | `TestWorkspaceGitDetailsPreservesExactPaths` | Public details-wait tracker read; staged and unstaged modifications for each filesystem-supported name, correct key/count/patch and ready state. |
| `.9`, `.36` | `TestWorkspaceGitDetailsPreservesExactMixedPaths` | Flattened +2, staged +1 and unstaged +1, independent layer patch markers, same exact path identity. |
| `.9`, `.36` | `TestWorkspaceGitDetailsPreservesRenameDestinations` | Real simple and directory/brace-rendered renames, rename origin metadata, modified and content-unchanged variants, staged-only and rename-plus-unstaged mixed states. |
| `.36` | `TestWorkspaceGitDetailsPreservesBinaryPaths` | Binary numstat zero counts do not suppress an existing binary patch; correct exact key and ready state. |
| `.7`, `.31`, `.33` | Existing exact budget, cancellation, carry-forward and failed-diff leaves | Preserve existing semantics while command flags change. |

Use private repositories and existing helpers, explicit `core.quotePath=true`, bounded fixture content,
and tracker cleanup. Native Windows skips only invalid filesystem names, never the parser matrix.
Zero-line mode-only parser coverage is portable; any real chmod regression must be Unix-scoped.

## End-to-end evidence

The real repository to public details-wait tracker path is the affected end-to-end boundary.
No new Playwright test or rendered UI change is needed; the existing UI consumes identical status fields.
Local backend/E2E full suites and browser startup are excluded by the caller's resource constraints.

## Documentation impact

Internal requirement/design and this package change. Public Changes guidance in
`docs/public/tasks-and-workflows.md`, `docs/public/use-kandev.md`, root README, and screenshot catalog
needs no correction: this restores existing counts/diffs for valid filenames without new user controls,
configuration, terminology, or API shape. No public-doc validators are needed without public edits.

## Work orders

- [x] [Task 01: Preserve workspace numstat paths](task-01-preserve-numstat-paths.md)

## Verification results

Design checks on 2026-10-02:

- `python3 scripts/list-docs.py validate`: passed (340 decisions, 1295 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `git diff --check`: passed; package status confirmed untracked plan/work-order and unstaged spec edits.
- `.github/scripts/pr-docs.cjs` exported `validateCoverage` on the complete package plus prospective
  production path: passed (`covered`, no errors). Used the installed Node 24.21.0 executable because
  the initial shell omitted Node from PATH. The work order includes that toolchain path.

Implementation checks on 2026-10-02:

- Permanent narrow RED: all six original staged/unstaged cases failed for the expected zero-count/empty-patch defect (package 0.676s).
- Focused GREEN command in Task 01 with `-race -p 2`, `GOMAXPROCS=2`: passed (19 selected test functions; package 7.110s). Includes expanded exact-path, mixed-facet, real rename, binary, framing, and existing lifecycle/resource regressions.
- Focused process-package lint command in Task 01, concurrency 2: passed, zero issues.
- Catalog validation, full specification lint, and diff whitespace checks: passed.

Normal commit hooks and external delivery evidence are recorded in the task plan. Hosted CI/review and verified merge remain external delivery gates; work-order completion does not imply merge.

## Delivery constraints and risks

One local heavy command at a time: `GOMAXPROCS=2`, Go `-p 2`, lint concurrency 2.
Retain and join every process handle. Preserve shared branches/caches and others' edits.
Potential accidental history-parser migration and rename-record desynchronization are constrained by focused tests.
Rename fixtures must preserve enough common content for Git to detect an actual rename.
Preserve the published candidate SHA during CI; main movement alone never authorizes rebase.
Use normal hooks and required CI plus substantive authenticated current-head semantic review.
Actual verified squash merge SHA is the completion gate; no progress declaration substitutes for it.
