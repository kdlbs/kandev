---
created: 2026-10-05
status: implemented
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
legacy_specs: []
---

# Implementation Plan: Discard only selected Git files

## Overview

Correct explicit Discard filename selection across classification, index removal and tracked
restoration. ONE sequential work order adds actual-caller regressions, corrects three command
sites, verifies the registered HTTP route, and clarifies the existing public reference.
This package is design only, unstaged and uncommitted. Implementation requires a later explicit
ROOT reviewed-package IMPLEMENTATION INTERRUPT plus a global local-heavy lease.

## Owner and assumptions

Platform's existing workspace Git contract owns literal operation selection, alongside
criteria `.11`, `.37` and `.38`. Extend its existing requirement and path-details design
with `.44`; do not create a parallel workspace/UI or incident requirement.
The system boundary is unchanged and no new ADR is required: this reuses the established
literal selector and selected-command environment convention.

Confirmed: explicit files only, unselected index/worktree preservation, exact repository
routing, existing errors/refresh/locks/deadlines/admission, no directory-delete expansion.
Verified: raw filename pathspecs reach all three Discard command sites; the existing runner
captures its effective environment before admission and copies it into the child command.
No material product choice remains open. Parser, rename and transactional semantics are excluded.

## Accepted evidence, read only

Checkout: `eb589f279dc8527101b71fdaf99ce523909a5299`, branch
`feature/discard-only-the-sel-oo5`. ROOT's private actual `GitOperator.Discard` proof at
`e095ca17790d3dd0e4700b473780add77ac7a230` records ordinary `selected.txt` PASS and tracked
`:(glob)a*.txt` failure: Success=true, selected bytes still dirty, unselected `alpha.txt`
restored. Inner Go exit 1 is expected RED; wrapper 0 is receipt transport, not PASS.
ROOT handle 56044 was actually joined at 6.966s; package duration 0.153s. ROOT removed its
source and private fixtures. No replay is authorized here.

Read-only archive `/tmp/kandev-discard-literal-selection-repro_test.go` SHA256:
`f29c2244da337e39782f08d903962169edc0e9e75896b132d7826a6f4da1e8fc`.
Receipts: `/tmp/kandev-root-discard-literal-proof-receipt.json` and
`/tmp/kandev-root-discard-literal-proof.log`. Preserve them until ROOT archives them.
One source identity check found identical blobs in this checkout, proof base, and cited main
`f45fe59cf26c49dda309a88a0fbb835ed6c2185c`: `git.go`
`121c8f1996bf33ee67b90f3e36f39a58f4e3e80b`; `git_pathspec.go`
`78d8571fe018a77605a892823c76f477601842ba`.

`/tmp/kandev-discard-untracked-brackets-raw-next-candidate.json` is raw Git-only evidence:
untracked `new[ab].txt` with dirty tracked `newa.txt` produces the tracked decoy first and
selected `??` second. Current Discard classifies from the first row and can restore the decoy
while leaving the selected file. This is not an actual production test verdict; permanent
actual-caller RED is required during implementation. Ordinary tracked bracket restoration is
a passing control, not a defect. The completed work order in the
[prior literal-selection package](../git-literal-file-selections/plan.md) intentionally
excluded Discard without new evidence; preserve that package's scope and historical results.

## Scope

In scope: selected status, `rm --cached`, and `restore` calls in
`apps/backend/internal/agentctl/server/process/git.go`; real operator index/worktree assertions;
registered HTTP routing; minimal existing public guide clarification; document traceability.

Out of scope: generic environment/cache frameworks, shared policy, passing Stage/Unstage
refactors, porcelain/rename repair, transaction/rollback redesign, directory-delete expansion,
new services/schema/API fields, UI layout/copy, installs or heavy validation during design,
browsers/E2E/PostgreSQL, extra agents/tasks/sessions/tabs/model changes, shared-cache cleanup,
foreign-process termination, paused oversized child, and unproved volume.

## Technical approach

Reuse `literalGitPathspec` at precisely three selected-command boundaries. Preserve each
path as one argument after `--`; leave invalid empty strings empty. Use the existing
`runGitCommandWithEnvironment` seam with `GIT_LITERAL_PATHSPECS=0` and
`GIT_ICASE_PATHSPECS=0` for these selected subprocesses only. Keep the original raw path for
`filepath.Join` / `os.Remove`. Track status with the same existing classification and
fallback; collect errors and refresh exactly as today.

| Boundary | Identity | Behavior and verification |
| --- | --- | --- |
| Actual operator | Repository workDir + explicit filenames | Exact classification, index removal and restoration; independent byte oracle before/after. |
| Registered HTTP | `GitDiscardRequest.Repo` to `gitOpForRepo` / `Manager.GitOperatorFor` | Select a named child repository from a non-Git workspace root; identical filenames in other repo stay untouched; invalid route scope fails without mutation. |
| Effective Git environment | Existing captured operator/provider environment | Selected child overrides only; inherited literal/case/glob controls tested through actual Git; provider/process environment unchanged. |
| Existing Changes actions | Same repository and raw filename across desktop/phone | Shared backend selection correction and existing refresh/result contract; no rendered change. |

`POST /api/v1/git/discard` is registered in `api/server.go`. The existing handler rejects an
empty list, resolves the repository, calls Discard and returns its existing result. No handler
production edit is planned. No additional Git processes or repository enumeration is needed.

## Tests

All tests use disposable real repositories and call `GitOperator.Discard` or the registered
HTTP route. Snapshot exact index membership/blob bytes and worktree existence/bytes separately.
Use different HEAD, staged and worktree sentinels for selected and unselected files; never
assert only success or helper-generated argv. Fixture/oracle Git must use a clean private
environment and literal selectors, exact NUL membership, and raw `git show :path` output.
Do not trim content bytes. Existing raw Git test helpers retain their output bytes.

| Criteria | Planned test/file | Required evidence |
| --- | --- | --- |
| `.44` | `TestGitOperatorDiscardLiteralSelections`, process `git_discard_literal_paths_test.go` | Ordinary tracked, tracked bracket control, ordinary untracked/added; portable untracked bracket vs dirty tracked decoy; added bracket with independently staged decoy; native magic names; multiple selected files; invalid empty/list; exact unselected index/worktree preservation. |
| `.44` | `TestGitOperatorDiscardLiteralEnvironment`, same file | Inherited literal/case settings from copied provider env and ambient fallback; case-distinct sibling where supported; supported glob/noglob controls separately; unchanged provider/process env. |
| `.44` | `TestHandleGitDiscardLiteralSelections`, API `git_discard_literal_paths_test.go` | Independent HTTP fixture/assertions, two real repositories with same names but repository-distinct sentinels, selected/multiple/mixed files, inherited instance env, invalid repo and empty request. |
| `.44` | Existing `TestHandleGitDiscard_RestoresTrackedFile` in work order | Ordinary registered Discard remains green. Prior `.37`/`.38` Stage/Unstage context is retained without rerunning untouched suites. |

Portable bracket cases always run, including native Windows. Only explicit native-invalid
names (colon/star/question names) skip Windows, with reason; case-only sibling test skips
only after `os.SameFile` proves the filesystem cannot distinguish them. Tests are sequential.
No new broad rename or directory-discard scenarios belong to this work order.

## End-to-end evidence and mobile assessment

Actual GitOperator plus the registered HTTP route cover the selected file from request to
repository/index/filesystem mutation. The independent HTTP fixture detects routing to the
wrong repository and avoids sharing the operator test's expected-result implementation.
This is a pure data-selection correction; no layout, touch, navigation, scrolling, responsive
state, or copy changes. Per mobile-parity's no-UI rule, shared actual-caller tests satisfy the
same desktop/phone outcome. No browser/E2E or ASCII UI preview is required or authorized.

## Documentation impact

Audit found `docs/public/git-operations.md` (reference) owns Discard behavior. During
implementation minimally clarify its Everyday Operations Discard row and `worktree.discard`
field description: named literal files in the selected repository, nonempty list, tracked
restoration and added/untracked deletion. Keep the irreversible-operation warning. Stage,
Unstage, copy-file glob instructions, root README, screenshot catalog and public API shapes
need no update. Public docs remain untouched in this design turn.

## Work orders

- [x] [Task 01: Make Discard filename selections literal](task-01-literal-discard.md)

## Verification results

Design validation on 2026-10-05:

- `python3 scripts/list-docs.py validate`: passed (351 decisions, 1355 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `validateCoverage` preflight with all four design documents and prospective `git.go`:
  `covered`, `ok:true`, no errors. Default shell lacked Node; the existing verified
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node` ran the lightweight preflight.
  Work-order commands explicitly use that existing PATH; no install was performed.
- Catalog confirms the requirement and referenced existing design; `.44` was absent before
  this edit. Coverage validates the work-order requirement/criterion/design/manifest chain.
- `git diff --check` and the package's file status checks passed. Final validation is recorded
  in the own platform task plan. Exactly two existing specs and two new plan documents are
  unstaged/uncommitted. Source, permanent tests and public guide remain unchanged.

The design checkpoint above ran no Go/Node tests, product lint/typecheck/build, installs,
hooks, browser/E2E or PostgreSQL. ROOT subsequently reviewed all four artifacts and granted
implementation and the global local-heavy lease. Task 01 is done after the scoped implementation checks. Go validation
uses GOMAXPROCS=2, GOMEMLIMIT=512MiB, -trimpath, -tags fts5, -race and -p=1; only new Discard
callers and the existing ordinary HTTP Discard control run locally. `.37`/`.38` remain
historical context. Initial lint stays scoped; full CHANGED lint is conditional on actual
backend-code PR fixup. No merge lease has been granted.


Implementation results on 2026-10-05:

- Actual permanent process selection RED: expected exit 1, package 0.974s; ordinary and
  original tracked-bracket/empty-input controls PASS, portable mixed bracket/native magic/
  multiple selections fail on exact selected/unselected bytes.
- Independent registered HTTP RED: expected exit 1, package 5.599s, proving selected-repository
  wrong-file behavior with independent index/worktree and other-repository assertions.
- Actual operator environment RED: expected exit 1, package 2.298s.
- After the three scoped call corrections, exact new process Discard GREEN passes 4.196s;
  exact new HTTP Discard routing plus existing ordinary Discard control passes 6.798s.
  All commands use -trimpath/-tags fts5/-race/-p=1, GOMAXPROCS=2, GOMEMLIMIT=512MiB.
- Initial scoped process/API lint passes with zero issues; no full backend scan absent fixup.
- Catalog 351 decisions/1355 specifications, specification lint, 62 public-doc tests,
  47-page live public-doc validation, actual changed-path coverage and diff checks PASS.
- One pinned pnpm 9.15.9 frozen install passes 2.2s; package manifest and lockfile unchanged.
  Every local handle actually joined. Normal hooks/publication/external delivery receipts
  remain in the own platform task plan. Local work-order done does not imply merged.

Only the three selected Discard commands changed in production; no parser, Stage/Unstage,
manager, API, schema or frontend production edits. The prior Stage/Unstage suites did not
rerun and are not claimed as coverage. Minimal public reference clarification is included.

## Risks and delivery boundaries

Classification and both mutation commands must be corrected together. Passing tracked bracket
controls alone miss the mixed untracked/tracked and explicit magic failures. Keep empty entries
unprefixed to avoid a whole-tree selector. Preserve partial-error behavior rather than adding
rollback. Correct index assertions must see absence as well as bytes.

Execute the exact work-order commands only after both ROOT admissions. One heavy operation at
a time, resource caps, all handles retained and ACTUALLY JOINED. Resource failure checkpoints;
no automatic retry, cache wipe or foreign kill. Delivery remains separate from work-order done:
ready PR, immutable head except grounded corrections, complete terminal exact-head CI evidence,
substantive authenticated full CodeRabbit App 347564 review, and separate ROOT merge lease.
The own platform task plan retains identities, leases, live handles and detailed completion gates.
Parent callback queue remains full; no callbacks/questions/retries. End design WAITING.
