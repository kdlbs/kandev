---
created: 2026-10-08
status: implemented
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
legacy_specs: []
---

# Implementation Plan: Unstage all before the first commit

## Overview

Restore Unstage all in a repository that has no first commit. One sequential work order
adds independently authored real-Git regressions, corrects the empty-list argv, updates
the causal reference wording and runs the focused checks. This design package remains
unstaged and uncommitted until ROOT reviews it and sends a later explicit implementation
interrupt to the same primary session. No local-heavy lease is granted in this turn.

## Ownership and evidence

Platform owns the shared Git mutation selection contract under
[REQ-PLATFORM-WORKSPACE-GIT-STATUS-001](../../specs/platform/requirements/workspace-git-status.md),
AC.37/.38 and the [path-details design](../../specs/platform/system-design/workspace-git-path-details.md).
Tasks retains repository/environment bindings. Clarify first-commit applicability in those
existing criteria; no incident requirement, architectural decision or framework is needed.

Starting commit is `202d48bceb50ff839e1834d928d1677337fda672`.
ROOT supplied a joined production-operator RED: before the first commit Stage all succeeds,
but empty-list Unstage returns ambiguous HEAD and leaves the index staged. Unborn selected
Unstage and committed selected/all controls PASS, with working bytes preserved. ROOT's
native handle 57083 was actually joined in receipt 42708d; wrapper exit 0 records Go exit 1,
package 0.098s and wall 9.50s. This turn accepts that supplied evidence without replay.
ROOT reports identical `git.go` bytes between proof base
`1257838968f5c92305a858427cdf723a87b0a882` and current base (edaf05 exit 0).

The protected ROOT proof at
`/tmp/kandev-root-unborn-unstage-discovery-20261008/candidate_test.go` is read-only:
regular 0400, SHA256 `fab969e96ac74d776e4f45d6635969cae2b3ed82a5edbe582229178860174998`.
Do not import, copy, replay, modify or delete it. ROOT reports original group 1381067 gone
and the exact temporary source removed. Permanent regressions will be independent.

## Scope

In scope: the empty-path branch of `GitOperator.Unstage`, real index and working-byte
evidence through production operator and registered HTTP routes, preservation of explicit
selection and committed behavior, and the matching Unstage documentation/comments.

Out of scope: HEAD probing, history/tracker changes, Discard/Revert, path admission,
command budgets/environments, transport/schema, generic Git policy, UI/copy/layout,
browser/build/E2E, PostgreSQL, installation in design, delegates and extra tasks/sessions.
Public docs remain untouched in the design turn.

## Technical approach

The all-files branch currently builds `{"reset", "HEAD"}`; explicit paths build
`{"reset", "HEAD", "--", literal selectors...}`. The former ambiguous argument is
the causal difference. Change only the all-files branch to `{"reset", "--"}`, and
update its explanatory comment. The separator removes revision/filename ambiguity,
while Git handles default HEAD and the unborn empty tree internally. No extra command,
HEAD detection, fallback, empty-tree object creation or reference policy is necessary.

This inference follows Git's [reset parser and unborn handling](https://github.com/git/git/blob/v2.43.0/builtin/reset.c).
Committed whole-reset behavior, including existing ORIG_HEAD/reflog bookkeeping, remains
the compatibility baseline. Unborn tests require no commit or refs to appear, and preserve
the symbolic HEAD and config bytes. Do not turn this fix into a guarantee that ordinary
Git index-file bookkeeping stays byte-identical. Assert membership, mode, stage and blob
content for the index; assert exact bytes and permissions for working files.

Keep explicit `reset HEAD --` paths, `literalGitPathspec`, invalid empty-entry rejection,
selected-command literal/case overrides, inherited empty-list environment, operation lock,
runner validation/admission, result errors and existing refresh exactly as owned today.

## Read-only caller audit

| Boundary | Actual caller/routing | Verification responsibility |
| --- | --- | --- |
| Changes repository and global actions | `changes-panel-data.tsx` calls `git.unstage(undefined, repo)`; `useScopedStageOperations` chooses explicit/repository/global scope; `useStageDispatch` in `use-session-git.ts` fans global empty paths across repository waves | Preserve callers; operator and HTTP evidence proves the corrected shared data boundary |
| WebSocket and runtime client | `use-git-operations.ts` sends `worktree.unstage`, `paths: []`, repository scope; `GitHandlers.wsUnstage` calls runtime `GitUnstage` | Existing shapes and identity remain unchanged |
| Registered HTTP | `server.go` registers POST `/api/v1/git/unstage`; `handleGitUnstage` binds `GitUnstageRequest`, then `gitOpForRepo` / `Manager.GitOperatorFor` | Real Router requests, selected and independent repository index/bytes, truthful success/error |
| Production process | Repository-scoped `GitOperator.Unstage` in `process/git.go` | Real Git, no canned result/argv-only oracle |
| Executors using agentctl | Existing shared process implementation for local and remote agentctl | No new executor capability or provider branch; existing unavailable/error handling remains |

## Regression mapping

Use new bounded test files rather than enlarging existing large files. Fixtures must
actually start unborn when claiming first-commit coverage; existing committed helpers
cannot stand in for that condition. Initialize only private repositories, isolate local
Git configuration/identity, register cleanup immediately and join manager/tracker work.
Use distinctive selected, sibling and independent-repository index/worktree bytes.

| Criteria | Permanent test (planned) | Required evidence |
| --- | --- | --- |
| .38 | `TestGitOperatorUnstageBeforeFirstCommit` in `process/git_unstage_initial_test.go` | Production Stage all establishes index entries. Empty-list Unstage clears every entry, including nested and portable literal-name files and an addition edited after staging. Exact working bytes/permissions, absent commit/refs, symbolic HEAD and config survive |
| .37/.38 | Same operator test, selected and invalid subtests | Explicit literal selection unstages only that initial file, preserving sibling blobs/bytes; invalid empty entry fails without mutation. These are passing compatibility controls |
| .38 | `TestGitOperatorUnstageAllCommitted` in same file | Real committed repository with staged addition, tracked staged edit plus later worktree edit and staged deletion; index returns to HEAD, current worktree bytes/deletion preserved, branch/tag identity/config retained, established reset bookkeeping remains |
| .37/.38 | `TestHandleGitUnstageBeforeFirstCommit` in `api/git_unstage_initial_test.go` | Actual registered Stage/Unstage requests in root unborn repo and independent multi-repository root. Select one unborn repo via Repo, require all its entries removed, another repo untouched; explicit selection and invalid selection preserve siblings |
| .38 | `TestHandleGitUnstageAllCommitted` in same API file | Registered committed all-files control with additions and mixed edits; inspect actual index/bytes and response |
| .37/.38 | Existing `TestGitOperatorLiteralSelections`, `TestGitOperatorLiteralPathspecEnvironment`, `TestGitOperatorLiteralCaseSelection`, `TestHandleGitLiteralSelections`, `TestHandleGitStageAndUnstage` | Literal names, directory/multiple/deleted/added/renamed/empty/invalid paths and inherited literal/case controls stay passing |

The new initial/all operator and HTTP cases must fail against unchanged production for
the supplied ambiguous-HEAD cause before correction; the corresponding selected and
committed controls must pass. Do not label passing controls as defects.

## End-to-end and mobile evidence

Real Git index/filesystem through the registered Router is the affected end-to-end boundary.
Desktop and phone consume the same operation transport and existing refresh. This is the
pure backend data exception: no rendered layout, navigation, touch, copy or responsive
change. No browser, frontend build, E2E or mobile fixture is required or authorized.

## Documentation impact

The public reference `docs/public/git-operations.md`, Everyday operations Unstage row,
currently says empty paths run `git reset HEAD`. After GREEN, replace only that argv
with `git reset --` and state first-commit support with working content retained.
Audience: users operating Changes; primary page type: reference.
`docs/public/sessions-and-review.md`, root README, screenshot catalog and WebSocket
field descriptions already describe the compatible operation, so need no causal change.
Update the runtime client's stale command comment in
`apps/backend/internal/agent/runtime/agentctl/git.go`; no runtime logic change there.

The existing literal-selection package and other linked tracker/Discard/refresh packages
remain historical evidence. Their scope and recorded results do not become failures or
require re-execution for this repair.

## Work orders

- [x] [Task 01: Restore first-commit Unstage all](task-01-unstage-all.md)

Execute sequentially in the same primary session only after later explicit ROOT release.

## Verification results

Design validation on 2026-10-08:

- Catalog validation passed: 363 decisions and 1437 specifications (f152f9, exit 0).
- Specification-linter self-tests passed: 36 tests (0c36e3, exit 0).
- Full specification lint passed (10b4fb, exit 0).
- Repository delivery coverage preflight passed with the actual four design documents
  and prospective owned production/test/doc paths: covered, errors empty (19ffd1, exit 0).
- Diff whitespace check passed; Git index is empty and the manifest/order are untracked,
  with only the two owning specification files modified (a4bd7a, exit 0).

Every design command completed on its original invocation; no running process handle
remains. No production checks, permanent tests, installation or commits occurred.
Task 01 records exact later RED/GREEN, scoped lint and documentation commands.

Implementation validation after ROOT's later explicit release:

- Permanent anchored RED: expected exit 1, process 0.200s / HTTP 0.522s. Only unborn
  all-files cases failed with ambiguous HEAD and staged entries retained. Explicit,
  invalid-selection and committed controls passed, with working bytes preserved.
- Anchored nine-function race GREEN: exit 0, process 3.823s / HTTP 2.412s.
- Initial scoped lint found only QF1003 in the new HTTP test. A switch correction
  was verified by the affected HTTP test alone (race, 1.309s, exit 0) and API lint
  alone (0 issues, exit 0). No passing process checks were replayed.
- Documentation gates passed: catalog 363/1437, 36 specification-linter tests,
  full specification lint, 62 public-doc validator tests, 47 published pages,
  actual owned-path delivery coverage (covered, no errors), and diff whitespace.
- The one conditional frozen workspace install used pnpm 9.15.9, reused cached
  dependencies, changed no lockfile and passed. Active hooks remain in use.

Task 01 records original native handles, actual joins and PID/group/deadline receipts.
Implementation is complete; commit/publication and hosted evidence remain pending.

## Risks and delivery constraints

The requirement file is near its 20 KiB limit; keep AC clarifications minimal. A success
response alone cannot certify the fix: index and working-byte assertions are mandatory.
Avoid fixture commits that hide the initial state and avoid byte-comparing mutable
index-file stat bookkeeping.

After ROOT release, obtain the ONE GLOBAL LOCAL-HEAVY lease, retain and actually join
each original process handle with exact PID/group/UTC cutoff receipts, and return the lease
explicitly before the hosted collector. Resource, timeout, transport, unknown or scope
failures checkpoint ROOT without automatic retry. Active hooks, no bypass/amend.

Normal READY PR delivery follows local checks. Preserve canonical repository association,
all five automation flags FALSE, author body and bounded managed regions. Freeze published
SHA absent a valid finding. One all-terminal 90m collector, GNU91m/kill10/60s cadence,
must actually join before any ROOT-authorized replacement. Obtain substantive automatic
authenticated CodeRabbit App347564 current-head/all-files review; one necessary request
only for an actual skip/gap. No optional repeat review/polish or hosted rerun without grant.
Every actual finding is grounded, fixed or dispositioned. Full CHANGED backend lint once
against exact PR base is required after backend fixup; docs-only fixups do not replay it.
Six required contexts and exact-head Backend/Frontend/E2E parents must succeed with
current terminal evidence and no actionable findings, changes-requested or human gate.
Stop merge-ready until separate serial ROOT grant. Preserve worktree/dependencies,
foreign processes/refs/caches and protected proof. Task-plan recovery notes hold identities,
remaining gates, resource receipts and next action across turns.
