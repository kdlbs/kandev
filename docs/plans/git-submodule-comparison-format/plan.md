---
created: 2026-10-09
status: implemented
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
legacy_specs: []
---

# Implementation plan: Preserve submodule changes in Git comparisons

## Overview and prerequisite

One sequential work order first tests and admits exact `--submodule=short` in
`securityutil.IsKnownSafeGitFlag`, then adds that argument to ONLY the existing
patch producers `GitOperator.ShowCommit` and `GitOperator.GetCumulativeDiff`.
The exact flag is absent at baseline. Its admission is part of this package;
do not widen a safe prefix, bypass validation, or change global Git policy.

Platform owns the shared comparison file/patch contract. Reconcile the existing
[requirement](../../specs/platform/requirements/git-diff-file-metadata.md)
and [design](../../specs/platform/system-design/git-diff-file-metadata.md)
with minimal missing .12/.13 criteria. Preserve existing .1-.11 behavior.
This design package does not authorize implementation.

## Accepted baseline and causal evidence

Actual clean workspace HEAD at design start:
`993f60ce889b6dcca1ca19fe98703a9159ab28c3`, matching ROOT's qualified head.
Only `qualified-proof.json` and `future-design-brief.md` in
`/tmp/kandev-root-submodule-format-discovery-20261009` were read for discovery.
The protected candidate/source/fixtures must never be accessed, copied,
imported, replayed, changed or removed by this task.

ROOT original native session **36794**, chunks **d5ecf3 / 23c3e4**, actually
joined exit **1**. Original wrapper PID **439117**, subprocess PID/PGID
**439140**, bounded 280s plus kill-after 10s, cwd ROOT's `apps/backend`;
native and subprocess joins are true, with fresh owned group/wrapper absence.
One causal `diff.submodule=log` case lost the parent `vendor/lib` file from BOTH
actual public comparison maps, despite `Success=true`. Short gitlink and
ordinary-file-under-log controls passed; HEAD/index/status were preserved.
This is accepted RED evidence, reused without a discovery replay. Independent
permanent regressions still need their own causal RED after release.

Concrete impact: `CommitRowFiles` consumes `useCommitDetail` through
`requestCommitDetail` -> `requestCommitDiff` -> `session.commit_diff` ->
`GitHandlers.wsCommitDiff` -> runtime agentctl `Client.GitShowCommit` ->
registered `/api/v1/git/commit/:sha` -> `GitOperator.ShowCommit`. An empty
successful map renders `task:noFilesInThisCommit` for a dependency pointer
advance. Child cumulative scopes are separately compared, so this evidence
does not establish that the entire aggregate comparison is empty.

## Scope

- Production: two patch argument lists in
  `apps/backend/internal/agentctl/server/process/git_log.go` and one exact list
  entry in `apps/backend/internal/common/securityutil/git.go`.
- Independent tests: new `git_log_submodule_format_test.go` in process, new
  `git_submodule_format_test.go` in registered API tests, and scoped
  `securityutil/git_test.go` admission/variant tests.
- Documents: existing Platform owner pair, this manifest and one work order.

Exclude parser redesign, new helpers/frameworks, global configuration or
environment changes, frontend/copy/navigation, runtime/schema/history changes,
new enums or API shapes, recursive comparison/routing changes, other Git
producers/writers, broad writer audit, dependency changes and sibling-package
edits. No new architecture boundary requires an ADR.

## Technical approach

Keep `validateGitCommandArgs` and managed `runGitCommand` in use. Add exact
`--submodule=short` after the subcommand and before the ref in both existing
patch argv. Do not change the separate no-patch commit metadata query. Preserve
first-parent/root/empty bases, fixed prefixes, `--no-color`, `--no-textconv`,
cumulative `--no-ext-diff`, captured environment, cancellation/admission, counts,
uncapped commit detail, and cumulative byte/file budgets and skip reasons.

Short output supplies existing `diff --git` sections with `Subproject commit`
identities. The existing parser handles them; parsing Git submodule logs or
inline child diffs would mix parent and child identities. Initialized children
retain their own comparison anchors and repository operators under the
[accepted nested-scope decision](../../decisions/2026-08-05-nested-submodules-as-repository-scopes.md).
Existing [Review behavior](../../specs/ui/requirements/submodule-review.md),
including parent-row suppression when child file diffs are available, remains.

Companion inventory: status metadata, plain output and built-in cumulative
packages are completed; textconv's task is done while its manifest retains
delivery-pending `in_progress`. Those packages own prior criteria/results.
This package adds no work to them and selects only necessary compatibility tests.

## Tests

Future test names below are an authorship contract, not evidence of tests run.
All fixtures use actual native Git in disposable repositories, no shell helper,
and no replacement of the Git executable. Assert expected positive values and
raw short-patch oracles, not equality between potentially empty production maps.

| Evidence | Bounded scenarios | Criteria |
| --- | --- | --- |
| `TestIsKnownSafeGitFlagAllowsSubmoduleShort`, `TestIsKnownSafeGitFlagRejectsSubmoduleVariants` in `securityutil/git_test.go` | Exact positive admission; bare/abbreviated flag, log/diff/empty/other values, suffixes, whitespace and control-character variants rejected | .12 implementation prerequisite |
| `TestGitComparisonSubmoduleFormat` in process `git_log_submodule_format_test.go` | Forward gitlink update: unset/short/log/diff, BOTH methods; backward/add/delete: log plus short controls, BOTH methods; ordinary file under log | .2, .3, .12 |
| `TestGitComparisonSubmoduleFormatControls` in the same file | Dirty tracked gitlink cumulative read; root gitlink addition; empty commit and empty cumulative read; bracket every read with owned state snapshots | .5, .12, .13 |
| `TestGitComparisonSubmoduleFormatHTTP` in API `git_submodule_format_test.go` | Registered selected commit and cumulative reads, independently seeded parents with the same gitlink path and different child IDs/bases; log and short controls | .3, .12, .13 |
| `TestGitComparisonSubmoduleFormatMultiRepoHTTP` in the same file | Registered aggregate cumulative read, parent NUL-qualified gitlink identities plus initialized child file diffs anchored to original parent gitlinks; log and short controls | .3, .12, .13 |
| Existing focused process controls | First-parent merge, root/empty, stable prefixes, uncapped commit, per-file/total/file-count budgets, color/textconv/helper behavior | .4-.11 compatibility |
| Existing `TestNestedSubmoduleReviewEndpointsIncludeRootAndStableChildBase` | Root and child scopes with stable parent-recorded anchor and scope metadata | .13 compatibility |

Avoid multiplying every operation by every display setting. Forward updates
establish all four display preferences; log/short operation controls establish
backward/add/delete semantics. Check exact path, full expected old/new commit
IDs, whole patch, modified/added/deleted status, 1/1, 1/0 or 0/1 line counts,
commit metadata/totals and cumulative base/head/commit count. Raw oracles use
explicit short format, no color/external diff/textconv and fixed prefixes;
snapshot commands must not execute configured converters/helpers either.

API fixtures configure each parent's stored base before manager construction.
Every cumulative URL includes the required `base` parameter, including
aggregate requests; aggregate scopes still resolve their own stored bases.
Selected calls assert only their selected repository. Aggregate parent keys
are `<parent>\0vendor/lib`; child keys are `<parent>/vendor/lib\0<child-path>`.
Check repository-relative `path`, `repository_name`, exact `base_ref`, child
`is_submodule=true` and its omission on ordinary parent entries. Bracket reads
with config/HEAD/refs/index/status/content snapshots for parents and children;
resolve actual child Git paths because `.git` can be a file. Independent IDs,
bytes and bases prevent wrong-selection equality from passing.

## End-to-end and surface assessment

Registered router tests exercise real HTTP dispatch through the manager and
actual Git without opening a port or starting an application/database/browser.
This is the necessary producer-to-consumer seam for .12/.13. The existing
desktop and phone surfaces consume the same corrected data. No composition,
copy, navigation, touch, scrolling or breakpoint behavior changes; the
mobile-parity pure-data exception applies, with no ASCII preview or new
Playwright test. Any later UI change requires a revised package.

Public audit searched `docs/public/**`, README and screenshot catalog.
`git-operations.md` already describes faithful metadata and read-only Git
actions; `sessions-and-review.md` and `feature-status.md` describe independent
submodule scopes and unavailable-child gitlink fallback. WebSocket guidance
retains its current API contract. These remain accurate; no gratuitous public
guide edit is required. Internal docs are updated by this package.

## Work orders

- [x] [Task 01: Preserve gitlink patches](task-01-preserve-gitlink-patches.md)

## Execution and delivery barriers

End the actual DESIGN turn with these four uncommitted artifacts. Await a
LATER ROOT reviewed-package implementation INTERRUPT in the SAME primary
session AND exclusive GLOBAL LOCAL-HEAVY. No production/permanent test edits,
Go/install/staging/commit/PR now; no delegates/tasks/sessions/tabs/model switches.
No approval or model-switch prompt. Exact scoped commands, process receipt
requirements and later delivery gates are in the work order and live task plan.
LOCAL-HEAVY must be explicitly returned after all original handles are joined
and fresh owned PID/PGID absence is proven. HOSTED HOLD continues until a
separate ROOT release. MERGE NONE until ROOT's separate serial static
compatibility grant. Autopilot and artifact creation release none of these gates.

## Verification results

Product verification pending explicit implementation release. Accepted ROOT
RED above is historical evidence, not independently executed child tests.
Design-only checks on 2026-10-09, all original tool calls terminal exit 0:

- Catalog validation: 365 decisions and 1480 specifications, chunk `1be591`.
- Specification linter tests: all 36 pass, chunk `965806`.
- All-spec lint: all files pass, chunk `7b119f`; owner requirement/design are
  below their 20/32 KiB limits.
- Whitespace: `git diff --check` passes, chunk `3966e9`; actual content checks
  also validate both untracked plan files' trailing whitespace/final newlines.
- Actual `.github/scripts/pr-docs.cjs` `validateCoverage`, using existing Node
  24.21.0 with Bash `login:false`: the four document paths are `ok=true`,
  `status=exempt`; the same actual contents plus both planned production paths
  are `ok=true`, `status=covered`, errors empty, exactly this ONE work order and
  the Platform design/requirement resolved, chunk `059ced`. This is local
  reference coverage, not product, hosted CI or semantic implementation proof.
- Catalog discovery resolves the owning pair. Package inventory contains one
  manifest and ONE work order. Backend and staged diffs are empty; all four
  artifacts remain uncommitted. No Go/install/permanent test/production edit,
  staging/commit/PR or heavy command occurred. No running native handle remains.

DESIGN handoff: work order pending. Next action is to await LATER ROOT reviewed
implementation INTERRUPT AND exclusive GLOBAL LOCAL-HEAVY in this SAME primary
session, then start the work order's independent test authoring and anchored
securityutil RED. LOCAL-HEAVY has not been acquired; HOSTED HOLD and MERGE NONE
remain in force.

## Risks

- A producer flag without exact admission fails before Git runs.
- A config-sensitive or parser-derived oracle can repeat the defect and pass
  empty maps; explicit positive gitlink identities and raw patches are required.
- Identical repositories or omitted cumulative `base` can disguise routing
  defects or create a fixture-only failure; use independently configured bases.
- Treating inline child output as parent patches violates existing nested scope
  boundaries. Keep child comparisons and parent-row presentation separate.

## Implementation release

ROOT reviewed all four artifacts and released implementation in this SAME primary
session with exclusive GLOBAL LOCAL-HEAVY on 2026-10-09. All four SHA256 values
in /tmp/kandev-root-child95-design-review-release-20261009.json matched before
Task01 became in_progress. Both lint command templates now include the required
--allow-serial-runners, with baselines and resource/time bounds preserved.
HOSTED HOLD and MERGE NONE remain; no delegation or protected proof access.

## Local implementation results

Task01 acceptance is done. Independent security/process/registered HTTP causal
RED ran and joined before exactly three production flag entries. All affected
GREEN and selected compatibility controls passed once; the new process fixtures
then gained hermetic home isolation with only their affected tests rerun.
Exact-baseline scoped lint returned zero issues. Full actual receipts and test
counts/results are recorded in the work order and own task plan.

Implementation is complete. Local publication, normal active-hook evidence,
canonical association/body readback and explicit GLOBAL LOCAL-HEAVY RETURN
receipts are tracked in the own versioned task plan. The single conditional
frozen pnpm9.15.9 install passed without lockfile changes or downloads.
HOSTED/SEMANTIC HOLD and MERGE NONE remain until separate ROOT release/grant.
