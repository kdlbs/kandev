---
created: 2026-10-02
status: draft
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
legacy_specs: []
---

# Implementation Plan: Literal Git file selections

## Overview

Keep explicit Stage/Unstage selections and individual workspace review patches associated with
the selected literal paths. One sequential work order adds failing regressions, corrects six
command sites with one shared path representation, updates public reference wording, and runs
the exact affected checks. Implementation waits for the parent's later explicit continuation.

## Evidence and assumptions

Clean starting commit: `2f74eaf6a0671c7ae5cac8bdabf0a7f3c16e16a7`.
`GitOperator.Stage` and `Unstage` append caller paths after `--`; four per-file
`capDiffOutput` calls in `workspace_git_diff.go` do the same. Git still interprets wildcard
pathspecs after that separator. `validateGitCommandArgs` skips path arguments after `--`;
it does not disable Git's own matching. The UI forwards selected row paths unchanged.

The parent's archived `/tmp/kandev-literal-git-paths-repro.go` records real public-entry-point
failures: selected `a*.txt` also stages/unstages tracked `alpha.txt`, and its tracker review
patch includes `+unselected-marker`. The reported four-case run had three intended failures
(0.327s, exit 1); tracked Discard passed. A strengthened tracked Discard control checked both
selected restoration and unrelated byte preservation (0.078s, pass). Portable tracked
`new[ab].txt` Stage also selected `newa.txt` (0.118s, exit 1). Added-file Discard passed;
untracked bracket Stage passed before commit. These are inherited evidence, not new test runs.

This design turn read the fixture and source, then ran a disposable raw Git 2.43.0 probe.
It independently reproduced tracked bracket selection of the sibling and confirmed
`:(literal)` for multi-path Stage, actual bracket-directory descendants, leading colon/magic
and dash names, wildcard filenames, flattened patch isolation and Unstage sibling preservation.
The probe exited 0 and removed its private repository. No permanent tests or production edits
were made. The intended literal selection, empty-all and directory behavior is settled.

## Scope

In scope: selected Stage/Unstage paths, all four individual patch commands, selected/unselected
sentinel regressions through operator/tracker and existing HTTP transport, public contract wording,
and preservation of renamed/deleted paths, directory subtrees and repository scope.

Out of scope: Discard changes without new concrete evidence, new UI/API/schema, global literal
environment settings, porcelain/history/config/security redesign, branch-total changes,
extra workers/tasks/sessions, full local suites, browsers/E2E shards and broad audits.

## Technical approach

Add a small `literalGitPathspec(path string) string` helper in
`apps/backend/internal/agentctl/server/process/git_pathspec.go` that returns `:(literal)` plus
the unchanged path. Stage/Unstage build their nonempty argument lists using it; the four patch
sites use it on `entry.path`. Keep refs, options and `--` separate and preserve empty-list
commands. Avoid new flag allowlists or command/environment changes.

This is a local correction to existing path identity, not a new architectural boundary or ADR.
`.36` already owns exact patch association; its examples now explicitly include pathspec
characters. `.37` and `.38` complete the explicit mutation contract under the same owner as
mixed-path mutation `.11`. Keep PR4157's NUL/numeric/state corrections intact.
The linked completed numstat, mixed-facet and refresh packages retain their historical results.

| Boundary | Scope/identity | Behavior and evidence |
| --- | --- | --- |
| Changes to operation transport | Selected repository + raw paths | Existing forwarding; backend HTTP regression exercises the same operator. No frontend edit. |
| Git operator | One repository workDir | Literal named files or actual directory subtree; explicit, multiple and empty selection tests assert index and worktree bytes. |
| Tracker to detail transport | Exact file key + comparison layer | Real `GetGitStatusWithDetails(ctx, true)` and HTTP `details=wait` exclude unrelated patches; one focused real cached-fallback test covers the fourth command site. |
| Local/remote executors using agentctl | Existing repository-scoped process implementation | Shared command representation; no per-provider branches or capability expansion. Existing unavailable/cancellation behavior applies. |

## Tests

Planned permanent regressions use private repositories, existing helpers, bounded content,
sequential subtests and joined tracker cleanup. Snapshot selected and distinct unselected
index/worktree content before the operation; assert exact bytes afterward, not just success.
Use raw NUL filename lists or exact `git show :path` identities for assertions.
Fixture preparation uses literal selectors so setup cannot recreate the bug accidentally.

| Criteria | Test/file | Evidence |
| --- | --- | --- |
| `.37`, `.38`, `.11` | `TestGitOperatorLiteralSelections`, `git_pathspec_test.go` | Tracked portable bracket plus sibling; native star/question/colon cases; added, deleted and renamed entries; multiple paths, actual directory subtree, empty-all; exact selected/unselected index and working bytes; independent repositories. |
| `.36`, `.9`, `.10` | `TestWorkspaceGitLiteralPatchSelection`, `workspace_git_literal_paths_test.go` | Public tracker read for unstaged, staged-only and mixed paths; flattened and facets include selected markers, exclude distinct unrelated markers, keep counts/origin and ready state. Bracket case remains portable. |
| `.36` | `TestWorkspaceGitLiteralCachedFallback`, same file | Existing staged enrichment boundary in a real repository with no prior flattened patch, proving literal selection at the fourth cached fallback call. |
| `.37`, `.38`, `.36` | `TestHandleGitLiteralSelections`, `git_literal_paths_test.go` under API | Actual Stage/Unstage JSON requests and existing detail-wait read return literal selection and correct patch identity, including selected/unselected sentinels and repository routing. |
| `.7`, `.31`, `.33` | Existing exact named tests in work order | Budget, cancellation, capped-output and ready/unavailable contracts; current PR4157 framing/numeric tests retained. |

Discard controls are preserved as evidence, not reclassified as failures. Add no production
Discard changes. Real filename tests skip only actual filesystem restrictions, never portable bracket names.

## End-to-end evidence

Real filesystem/index to public tracker read and HTTP operator/detail transport provide the
affected end-to-end boundary. Existing UI consumes the same fields and forwards the same paths.
No Playwright run or rendered UI change is needed or authorized in this package.

## Documentation impact

During implementation update only `docs/public/git-operations.md` (reference): Everyday
Operations Stage/Unstage rows and `worktree.stage`/`worktree.unstage` field descriptions shall
state literal repository-relative paths, actual directory selection and empty-all semantics.
Copy-file glob guidance is a different contract. Root README and screenshot catalog need no
change. Public docs remain untouched during this design checkpoint.

## Work orders

- [x] [Task 01: Keep selected Git paths literal](task-01-literal-path-selections.md)

## Verification results

Design checks on 2026-10-02:

- `python3 scripts/list-docs.py validate`: passed (340 decisions, 1295 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Work-order `validateCoverage` preflight on all four design artifacts plus the three prospective
  production paths: passed (`covered`, no errors).
- `git diff --check`: passed. Package status confirms unstaged spec edits and untracked
  plan/work order; production/permanent test code and public docs are unchanged.
- Disposable Git 2.43.0 selection probe: exit 0; private repository removed, command joined.

Implementation evidence on 2026-10-02:

- Permanent process RED: three selected functions failed for unintended sibling index changes
  and patch content (package 5.564s, exit 1). HTTP multi-repository RED failed for the same
  index and detail-wait patch contamination (package 0.329s, exit 1).
- Initial 15-function focused race selection: all new selection/patch tests and ten other
  selected functions passed; two existing failure injectors missed the changed argv shape
  (package 9.984s, exit 1). Three matching fault selectors were updated without changing
  assertions. Their exact three-function race run passed (package 1.728s).
- The empty-entry compatibility probe found raw empty strings rejected while `:(literal)`
  matched all. Permanent invalid-empty RED failed for both Stage and Unstage (package 0.149s).
  Empty entries now remain empty. The final three-function new-regression race selection
  passed (package 7.672s), including named/multiple/directory/empty-list and invalid-entry cases.
- HTTP race selection passed after the final production edit (package 1.625s).
- Scoped lint initially caught an unchecked new-test enrichment error; the test now checks it.
  Final scoped process/API lint passed with concurrency 2 and zero issues. Hook evidence is
  pending.

The work order records exact commands. Catalog/spec lint, public-doc validation (47 pages),
coverage preflight and whitespace checks passed. Delivery is externally pending.

## Risks and delivery constraints

Tracked bracket matching differs from untracked admission, so tracked files are the primary
regression. Test fixtures must use distinct markers and preserve real rename detection.
Literal prefixes belong only to selection sites, never raw file keys or filesystem reads.
Inherited Git pathspec environment settings retain existing handling; do not redesign them.

One local heavy command at a time: `GOMAXPROCS=2`, Go `-p 2`, lint concurrency 2; retain and
join every handle. Preserve others' work and shared branches/worktrees/caches. Normal commit
hooks, required hosted CI, authenticated substantive configured current-head full review and
verified expected-head squash merge are the completion gates. Inspect an existing automatic
CodeRabbit full report before requesting another. After any corrected candidate, request one
full review if needed; acknowledge/skipped output is not review evidence.
Preserve the published SHA as CI runs; main movement alone never warrants rebase. A cheap
isolated compatibility check and owned byte diff precede any meaningful-overlap decision.
Parent directions arrive by interrupt and supersede older queued contradictory messages.
