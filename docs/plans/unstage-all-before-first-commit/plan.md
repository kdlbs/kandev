---
created: 2026-10-08
status: implemented
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
  - REQ-PLATFORM-CI-PERFORMANCE-003
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
  - ../../specs/platform/system-design/workspace-git-status.md
  - ../../specs/platform/system-design/ci-performance.md
legacy_specs: []
---

# Implementation Plan: Unstage all before the first commit

## Overview

The first-commit Unstage correction, real-Git regressions, Windows process-cohort
amendment and isolated timestamp fixture are published in READY PR4313 at frozen
`8403d17daab69882db2e7f4857e25007c13b6281`. Natural native Windows timestamp
coverage and FULL18 review passed. The process job failed an unchanged immediate
mode-transition notification fixture; its full inventory and joined cohorts were
retained without a full-suite PASS or latency-cause claim.

ROOT reviewed the four-file fixture candidate after AUTHORING_END and later released
this SAME ONE sequential order with the sole heavy lease. The bounded fixture-only
mode-transition correction now has causal wake-disabled overlay RED, affected count10
race GREEN, three unchanged paused/fast controls and full CHANGED lint PASS. No
production/shared helper/workflow or other fixture changed in this correction.
Documentation/actual19-path coverage, normal hooked publication and qualified lease
return follow. New-head hosted gates remain pending; no merge authority.

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
The amendment additionally covers only the monitor fixture's cached detail wait,
the Windows process workflow member, its fixed native Go cohort runner and focused
runner/workflow contract tests. No production tracker or Git behavior changes.
The later native dependency additionally owns only timestamp baseline setup in
`apps/backend/internal/task/service/service_repository_checkout_controls_test.go`.
It preserves all real checkout-update and event assertions without a production clock change.
Public docs remain untouched in the amendment: the Windows support guide truthfully
describes focused race-tested packages without promising an unpartitioned command.

## Technical approach

Before the initial correction, the all-files branch built `{"reset", "HEAD"}`; explicit paths build
`{"reset", "HEAD", "--", literal selectors...}`. The former ambiguous argument is
the causal difference. The published correction changed only that branch to
`{"reset", "--"}` and its explanatory comment. The separator removes revision/filename ambiguity,
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

- [x] [Task 01: Restore first-commit Unstage all and settle delivery dependencies](task-01-unstage-all.md)

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
Initial implementation, normal active-hook commit, push and READY PR publication
are complete. The amended delivery dependency is pending reviewed release; hosted
Backend remains failed. The other five required contexts passed at the frozen head.

## Amended delivery dependency: design checkpoint

The design checkpoint authorized only source/log inspection and this amendment.
ROOT subsequently reviewed and released its implementation; current results follow. The owning
Unstage AC.37/.38, production correction, four new test functions and public row
remain unchanged. [CI performance](../../specs/platform/requirements/ci-performance.md)
AC.003.4/.5 and its [design](../../specs/platform/system-design/ci-performance.md)
own complete Windows cohort selection. The existing CI-performance package keeps
its frontend adoption gates, profiling results and pending operational evidence;
this bounded follow-up does not complete or replay any of its work orders.

The fixture changes only `requireMonitorRefresh`: replace its ten-second caller
context with `tracker.cancelCtxOrBackground()` in the existing cached detail wait.
`GetGitStatusWithDetails` selects the accepted fingerprint's `job.done`, returns
worker errors and rejects supersession. The worker starts its 60-second deadline
before background admission/validation, retains one successor slot, closes each
completion after publication/index cleanup, and cancels/drains on Stop. The fixture
does not run blocking enrichment hooks. All original assertions, production
deadlines and package deadlock alarms stay intact. This is lifecycle settlement,
not a new refresh performance promise or a longer arbitrary fixture timeout.

The Windows process member gets a fixed two-cohort native Go runner. Native
`go test -race -json -timeout 25m -list '^(Test|Fuzz|Example)'` discovers the
existing entire process subtree, including `process/probe`. Sort distinct top-level
names and alternate; assign duplicate names across packages to the same cohort.
Validate complete/disjoint package-qualified selection and escaped anchored roots.
Start BOTH unchanged race/JSON/verbose/25m commands before joining either, join
every started command, retain separate raw diagnostics and actual exits, and
require exact terminal coverage. Enumeration/selection/command/coverage failures
are failures. No skipped Unicode case, assertion weakening, retry, bigger timeout,
new CI provider or configurable sharding framework. Native Windows checks stay intact.

Both exact failed logs were parsed read-only; elapsed sums count top-level terminal
events once and exclude nested events. The ordinal alternating candidate uses the
union of 718 observed names, not a claimed complete native inventory:

| Attempt / failed leaf | Completed top-level identities | Candidate cohort seconds |
| --- | --- | --- |
| 1 / 113120863590 | 715 (one active at alarm) | 828.79 / 669.93 |
| 2 / 113130914499 | 717 (one active at alarm) | 846.97 / 651.90 |

These partial profiles total about 1500 seconds, split about 55/45 and 57/43.
Largest observed tests include monitor dirty paths 78.82/86.05s, plain patches
65.69/72.70s, external helpers 63.81/63.85s and discard environment 56.02/42.72s.
Alphabetical alternation is sufficient for this measured candidate without a
historical weight map. Native enumeration may add names and change assignments;
unexecuted tail times, contention and complete-cohort wall times remain UNKNOWN.
Do not certify a 25m bound from these sums or claim a comparative speedup.

Both attempts ran Backend Tests37718308043 at frozen a5ebc322..., with two real
25m package failures. Attempt2 additionally failed default/unicode at the fixture
wait; the different final active tests had run only one second. Both owned Unstage
functions passed twice. Original2193 joined95322a/143 under the exact stop grant,
replacement22075 joined21ab0f/1 terminal with 59PASS/2FAIL/0PENDING. All diagnostic
reads joined and owned groups are gone. Raw logs/metadata, exact receipts, canonical
association complete/errors[]/five flags false and exhausted retry budget remain
under `/tmp/kandev-child77-unstage-20261008/`. No active observer or merge authority.

Later local checks are limited to helper TDD, the registered workflow contract,
the changed monitor fixture plus focused bounded/cancellation/Stop controls, one
full CHANGED lint against exact PR base, and documentation/coverage gates. Existing
passing Unstage/API controls are not locally replayed. See Task 01 for exact commands.
Natural current-head hosted Windows cohorts must prove complete actual native
coverage. END design now; no implementation/test/lint/install/code/head/push or
new collector is authorized until ROOT's later reviewed release.

## Retained delivery constraints

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


## Amended local result: blocked on lint timeout

Reviewed implementation is written. Workflow GREEN passed 13 contracts; final
helper GREEN passed six functions (race, 2.037s); changed monitor plus five
lifecycle controls passed (race, 6.707s). Initial workflow/helper behavioral REDs
established missing wiring and helper behavior. No original Unstage/API replay.

The ONE full CHANGED lint exactbase202d48 hit GNU6m, original48453 joined a319cb
exit124 with no output/verdict and group2232641 absent. All original local commands
joined; fresh owned-group audit empty. Global local-heavy lease EXPLICITLY RETURNED.
Docs/coverage gates, hooks, commit/push and hosted collector not run. Same order
blocked pending ROOT's specific resource decision; no automatic retry or fallback.
Frozen published HEAD and both Windows failures/retry exhaustion remain intact.
Resource/receipt proof: `/tmp/kandev-child77-unstage-20261008/amend-resource-checkpoint.json`.


## Authorized recovery result

ROOT granted one identical full CHANGED warm-cache recovery. It joined7359/325135
exit1 after255.857s with six owned runner diagnostics, not another resource timeout.
Minimal listing extraction, checked closes/write and event constants corrected them.
Only affected helper tests (six/race2.037s) and helper-only lint (zero issues/.771s)
followed and passed; every original joined/groupsgone. The original124 remains
failed/no verdict/cause unproved, and the one recovery allowance is exhausted.
No original passing product checks, full lint or cache reset were repeated.
Documentation/coverage gates and normal publication now follow under ROOT release.


## Local implementation complete; hosted evidence pending

Catalog363/1437, spec tests36 and all-spec lint passed. After the one missing
manifest design reference was added, affected actual17-path coverage and diff
checks passed (covered/errors[]). Task01 local implementation is done; normal
hooks/fixup publication and actual new-head hosted native coverage, six required
contexts, three parents and substantive full review remain pending delivery gates.
No complete Windows cohort pass, performance improvement or merge-ready claim.

## Native timestamp dependency: released validation

The process amendment passed active hooks and was published normally at `7bcbec5b...`.
All 39 retained local originals were joined and fresh owned groups were absent before
the explicit heavy-lease return. Current authenticated CodeRabbit App347564 FULL ALL17
review completed with a substantive assessment and no actionable findings; it becomes
historical after any later fixup. Original hosted collector57475 remains live with its
original deadlines; no replacement or hosted retry is authorized.

Current-head native Windows job113152567200/run37728353083/attempt1 failed
`TestRepositoryCheckoutDefaultsPresence` at line42: strict `UpdatedAt.After` was false.
That test and production update were unchanged from base202d48. The update stamps
`time.Now().UTC()` without a logical monotonic-clock promise. Actual failed timestamps
were not logged; the raw log proves the assertion failure, not a clock-resolution cause.
ROOT reviewed the source/log and authorized the minimal private fixture baseline.

Before each existing payload iteration, parameterized `ExecContext` sets only the
private row's persisted `updated_at` to 2000-01-01 UTC, requires exactly one affected
row, and the canonical repository read confirms that instant. All original payload,
presence, field, real-update, strict timestamp and event assertions remain intact.
No sleep, looser comparison, production clock seam or companion-test change.

The fixture and SAME manifest/order are unstaged and uncommitted. No validation
result is claimed yet. ROOT's later sole-heavy release is active after child78's
qualified return; Task01 records the anchored count10/race test under three minutes,
ONE full CHANGED lint and normal documentation/actual-path coverage gate. Before
corrected push, stop/join ONLY owned observer57475 and prove its group absent;
preserve historical evidence without waiting for incomplete old CI or cancelling jobs.
Natural new-head Windows success must prove the correction; no same-head rerun,
performance claim or merge authority. A single new-head observer follows qualified
local publication, all original joins and explicit heavy return.

Released count10/race fixture check passed (package1.577s, wall90.435s), original75293
joinedaade8a/0/group2382648 absent. ONE full CHANGED lint passed zero issues,
original52635 joined1377ed/0/group2389256 absent, wall99.131s. No passing test replay
or production clock change. ROOT-authorized old observer57475 was ownership-verified,
stopped and actually joined26aa11: collector143/no verdict, wrapper1 on empty-report
summary parsing after recording the true terminal receipt; exact owned group/wrapper
are absent. Historical35PASS/1FAIL/25PENDING remains unqualified for delivery. Old
process job was still running; no available complete coverage report or hosted cancellation.
Catalog363/1437, all-spec lint, actual18-path coverage (covered/errors[]) and diff
whitespace passed, original7210 joined415baa/0/group2401993 absent. Normal active
hooks/publication follow; new-head native
fixture and complete cohort coverage, required gates/parents/review remain pending.

## Immediate mode-transition dependency: authoring only

At frozen8403, the natural native Windows timestamp test passed and authenticated
App347564 FULL ALL18 review completed without actionable findings. Windows process
job113159794671/run37730879950/attempt1 failed an unchanged monitor mode-transition
notification assertion. The configured wait was TWO seconds, not the whole case's
5.28-second duration. Process767/probe16 inventory, disjoint392/391 cohorts and exact
783 terminal roots are proven; both original cohorts joined exits0/1, no package
timeout observed, reportComplete=false. Own Unstage functions passed. Cause remains
unproved and Windows retry budget exhausted; no merge-ready claim.

ROOT first authorized authoring and subsequently released this fixture-only correction. It keeps
two-second immediate admission, joining real scan completion separately before the
unchanged two-second real notification assertion. Existing monitorRunning/completed
MonitorTickStats catch long or short ticks; stale tickDone is drained after paused
initialization, then the actual admitted completion is received. Both fast timers
remain30 seconds, preventing the timer from satisfying admission. No shared helper,
production, policy, workflow or other fixture is changed.

The accepted90-second completion guard uses status observation60 plus three
Git command budgets10 each. Queue wait and process cleanup are additional; this is
an owned fixture failure/cancellation guard, not a production whole-tick guarantee.
ROOT qualified this boundary and later released SAME-order implementation with the
sole global local-heavy lease. Task01 records the disposable wake-disabled overlay
RED, exact affected positive/control commands and one corrected-source CHANGED lint.
Results are pending until each original execution is actually joined.

SAME one order is in_progress for this authoring dependency. Four candidate paths
(test, owning CI design, manifest and Task01) are unstaged/uncommitted. Original64810
and deadlines remain intact; joined diagnostic receipts/raw native evidence retained.
AUTHORING_END preceded ROOT's later explicit sole-heavy implementation release.
No hosted retry or MERGE authority follows; callback/queue is never a progress gate.


### Mode-transition released local results

ROOT later reviewed the exact four-file candidate and accepted90 seconds solely as
an owned TEST failure/cancellation guard, not a production aggregate deadline or
queue/cleanup/failure-cause assurance. Its receipt is
`/tmp/kandev-root-child77-mode-fixture-review-20261008.json`; the later same-primary
release granted sole local-heavy implementation. No production/shared helper/workflow
or other fixture source changed.

ONE disposable Go-overlay negative/count1 failed the exact two-second admission
assertion: case2.08s/package2.126s, original72826 joined719e15/exit1/group2890115
absent, wall10.237s. It retained timer drain/fast mode check but returned before
CAS/tick; this is causal wake-property RED, not a compiler/cleanup/90s-timeout result.
Only owned overlay JSON/source were removed after join; production bytes preserved.
Affected positive/count10 passed race/package2.187s, original74101 joined610362/0,
group2892230 absent/wall9.806s. Exactly the three planned unchanged paused/fast
controls passed race/package2.499s, original86763 joined87e467/0/group2894939 absent,
wall4.157s. The single exact-base full CHANGED lint passed zero issues, original84190
joined044456/0/group2896471 absent/wall12.814s. No passing Unstage/API/helper/clock
replay or package/job timeout increase occurred. Natural new-head Windows evidence
is still required; local checks cannot prove the hosted failure's latency cause.

Documentation/actual19-path coverage, normal active hooks/publication and original
observer64810 join precede heavy return. Current8403/full18/native clock PASS and
failed process evidence become historical after correction publication. Required
contexts, three parents, full actual-file review and complete native cohort coverage
remain delivery gates; no hosted rerun or merge authority.


Documentation gates passed: catalog363/1437, all-spec lint, actual19-path delivery
coverage (covered/errors[]) and whitespace, original25938 joined6b4490/0,
group2901835 absent/wall1.401s. No unchanged validator/public/product test replay.
Public docs need no change for this fixture-only synchronization correction. Normal
active-hook publication follows; all hosted outcomes remain separately qualified.
