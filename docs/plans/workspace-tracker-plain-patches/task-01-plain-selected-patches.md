---
id: "01-plain-selected-patches"
title: "Produce plain selected workspace patches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
---

# Task 01: Produce plain selected workspace patches

## Summary

Prove that forced Git presentation color corrupts actual workspace patch data, then
disable color at all four selected tracker patch commands. Preserve actual literal
ANSI content, exact filename association, layered state and snapshot ownership.
ROOT accepted the package and released implementation in this same primary session
after the design turn ended.

## In scope

- Four `--no-color` additions at the sites inventoried in the plan.
- Causal tracker, cached-fallback and registered selected/aggregate HTTP regressions
  specified below. Existing process/API helpers and real Git remain authoritative.
- Focused compatibility checks and synchronized requirement/design/plan results.

## Out of scope

Output stripping; Git configuration writes; numstat/porcelain/history redesign;
extra external-diff/textconv policy; standalone comparisons; UI/API/schema;
output-budget changes; broad writers/adjacent defects; delegates or model switches.
No production/permanent-test edit, install, heavy check, commit/push/PR in design.

## Acceptance

1. Before production edits, all four real patch sites have causal behavioral RED
   evidence under forced color and positive ordinary controls. GREEN proves exact
   patch content/count/status/readiness/facets, literal ANSI preservation, unchanged
   config/refs/index/worktree and repository isolation through actual transport.
2. Production edits are limited to the four selected tracker option additions.
   Existing literal argv/env, captured index/HEAD/environment, repository routing,
   capping, budgets, admission, cancellation and ready/unavailable semantics hold.
3. Exact affected checks and normal hooks pass and actual results replace Pending.
   Work-order completion records implementation only; task completion requires
   verified merge and all owned handle joins under separate ROOT delivery gates.

## Regression matrix

Use `isolateTestGitEnv` / existing private API fixture environment isolation. Seed
tracked files using literal setup selectors with distinct selected and sibling
content; compare old/index/worktree contents with independently stated expectations.
No global configuration writes and no `t.Parallel` with environment fixtures.

Run each actual producer under ordinary unset color, explicit `color.ui=false` plus
`color.diff=false`, local `color.diff=always`, local `color.ui=always` with diff unset,
and UI always plus explicit diff false. Include captured-instance command environment
`GIT_CONFIG_COUNT/KEY_n/VALUE_n` forcing UI/diff color, with contradictory live
process values after capture, so selected commands must use the retained environment.
Verify captured and process environments are unchanged. No flag-string assertions.

`TestWorkspaceGitPlainPatches` in prospective `workspace_git_plain_patches_test.go`:

- Actual `NewWorkspaceTracker` and `GetGitStatusWithDetails(ctx,true)` on unstaged,
  staged-only and mixed tracked paths. Seed `origin/main` from fixture HEAD to make
  ancestry ready without network. Check complete membership and detail readiness,
  exact path/modified status and staged boolean, empty skip reason, exact additions
  and deletions, and correct presence/absence and status/counts of both mixed facets.
- ASCII old/index/worktree sentinels make flattened HEAD-to-worktree, staged
  HEAD-to-index and unstaged index-to-worktree expectations distinct. Assert exact
  hunk/line content and expected patch headers, not merely nonempty output or totals.
  For ASCII source/name cases reject Git ANSI presentation bytes; assert selected
  patches exclude sibling and other-layer changed-line markers.
  Single-layer replacement controls expect +1/-1. For mixed controls, HEAD has
  `base-stage\nbase-worktree\n`, index has `index-stage\nbase-worktree\n`, and
  worktree has `index-stage\nworktree-line\n`: flattened is +2/-2, each facet
  is +1/-1, with modified status and only its own replacement lines. Add a fixed
  shared context line if needed; it does not change these expectations.
- Portable bracket filename plus matching sibling constrains literal pathspecs.
  Retain current `GIT_LITERAL_PATHSPECS=1` and captured-env controls; no command
  reconstruction from the archived diagnostic. Include ordinary Unicode names.
- Separate literal ANSI text in removed/added lines survives byte-for-byte with
  normal patch prefixes and exact line counts in each representation. Expected
  raw patch lines prove absence of extra presentation wrappers without treating
  every ESC as corruption. A POSIX-supported literal ESC filename retains exact
  `Files` key and Git's normal quoted header representation. On native Windows
  only that demonstrated invalid control-character filename is excluded; content,
  ordinary/bracket/Unicode and all non-filename tests remain enabled.

`TestWorkspaceGitPlainCachedFallback` in the same prospective file:

- Follow `TestWorkspaceGitLiteralCachedFallback`: real repository, staged selected
  and unrelated sibling changes, empty flattened `FileInfo.Diff`, actual
  `enrichWithStagedDiff` boundary and captured HEAD. No synthetic patch output.
- Assert real returned cached patch exact markers/hunks, +/− counts, modified/staged
  identity, ready state, no skip reason, and no sibling content for the same color
  controls. Include literal ANSI content preservation. This reaches the fourth
  site even when normal staged-only reads already populated flattened data.

`TestGitStatusHTTPPlainPatches` in prospective `api/git_status_plain_patches_test.go`:

- Create an actual manager and `NewServer`; issue requests through registered router
  using `getGitAPI`. Use two repositories with independent markers/configuration,
  explicit `BaseBranches`, fixture local comparison refs and joined teardown.
- Selected `status?repo=selected&fresh=true&details=wait` and aggregate
  `status/multi?fresh=true&details=wait` cover the color control matrix and actual
  unstaged/staged/mixed results. Decode JSON to typed file/facet data; assert HTTP
  200, success, complete/ready state, exact repository set, keys, statuses, staged
  identity, positive counts/hunks/facets and decoded literal ANSI content. No fake
  tracker result, direct handler invocation, flag inspection or cross-repo mixing.
- Snapshot local config bytes, HEAD and raw ref inventory, each selected/unselected
  index blob and worktree bytes before reads; require equality afterward in tracker,
  fallback and HTTP fixtures. Do not demand raw index-file byte equality: optional
  Git stat-cache mechanics are separate from unchanged index content. Preserve
  manager/tracker instance environment and existing snapshot cleanup behavior.

Any existing fault injection needing changed argv accommodation must retain its
causal failure and state assertions. Only routine in-scope fixture/assertion/lint
corrections and their affected reruns are authorized; checkpoint broader blockers.

## Verification

Run from repository root only after explicit ROOT implementation release and the
ONE GLOBAL LOCAL-HEAVY lease. Mark `in_progress`, write the tests, run the first
two commands for RED, make the four option additions, and run the relevant GREEN
commands. All expressions are anchored; no whole-package or full-suite test replay.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^(TestWorkspaceGitPlainPatches|TestWorkspaceGitPlainCachedFallback)$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^TestGitStatusHTTPPlainPatches$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^(TestWorkspaceGitLiteralPatchSelection|TestWorkspaceGitLiteralCachedFallback|TestWorkspaceGitLiteralPathspecEnvironment|TestWorkspaceGitLiteralCapturedEnvironment|TestWorkspaceTrackerGitStatusCaptureRecoveryRetriesOneEvidenceChange|TestWorkspaceTrackerGitStatusCaptureRecoveryUsesReplacementIdentity|TestWorkspaceTrackerDetailsWaitCanCancelWithoutCancelingEnrichment|TestWorkspaceTrackerRetriesOnlyFailedDiffAfterTransientGitFailure|TestCapDiffOutput_Truncation|TestDiffBudget_TracksReplacedContentAtBoundary|TestEnrichWithDiffData_CanceledContextReturnsErrorForTrackedFiles|TestDiffBudgetAndCarryForwardHonorCancellation)$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^(TestHandleGitLiteralSelections|TestGitStatusHTTPReturnsRecoveredEnrichedCapture|TestGitStatusMultiRetriesOnlyTheRepositoryWithChangedEvidence)$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run --concurrency=2 --allow-serial-runners --timeout=5m ./internal/agentctl/server/process/... ./internal/agentctl/server/api/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workspace-tracker-plain-patches
```

Reference coverage preflight, from repository root (prospective production/test
paths intentionally included during design):

```bash
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'NODE'
const fs = require('node:fs');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const docs = ['docs/specs/platform/requirements/workspace-git-status.md', 'docs/specs/platform/system-design/workspace-git-path-details.md', 'docs/plans/workspace-tracker-plain-patches/plan.md', 'docs/plans/workspace-tracker-plain-patches/task-01-plain-selected-patches.md'];
const future = ['apps/backend/internal/agentctl/server/process/workspace_git_diff.go', 'apps/backend/internal/agentctl/server/process/workspace_git_plain_patches_test.go', 'apps/backend/internal/agentctl/server/api/git_status_plain_patches_test.go'];
const result = validateCoverage({changedFiles: [...docs, ...future], fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]))});
console.log(JSON.stringify(result));
if (!result.ok) process.exitCode = 1;
NODE
```

Cheap design checks use catalog/spec-lint, reference coverage and whitespace only.
The installed Node/pnpm path is `/home/jcfs/.nvm/versions/node/v24.18.0/bin`;
cached pinned pnpm is `/home/jcfs/.cache/node/corepack/v1/pnpm/9.15.9/bin/pnpm.cjs`.
Do not reinstall the runtime. If apps dependencies are missing at later release,
one lease-owned frozen install is allowed, once, before normal hooks:

```bash
(cd apps && PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH node /home/jcfs/.cache/node/corepack/v1/pnpm/9.15.9/bin/pnpm.cjs install --frozen-lockfile)
```

No concurrent install/test/lint/typecheck/heavy hook across sibling children. Retain
every session/handle, join to its actual exit, and release the lease only after
owned process trees terminate. A resource/timeout/transport/unknown failure is a
ROOT checkpoint; do not retry or raise budgets automatically.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_plain_patches_test.go`
- `apps/backend/internal/agentctl/server/api/git_status_plain_patches_test.go`
- Existing in-scope fault-injection fixtures only if the added option changes their
  causal selector; retain original assertions and record actual affected reruns.
- The four artifacts in this package. No public documentation or frontend changes.

## Dependencies

None. Later explicit ROOT release and the global local-heavy lease are operational
gates, not additional work orders.

## Risks

Fallback coverage can be missed by normal staged-only reads. ANSI-wide stripping or
rejection destroys user data. Environment fixtures must be isolated and teardown
must join tracker jobs. Native Windows filename restrictions do not justify skipping
portable content or transport cases. Preserve current snapshot/admission code.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md): `.7`,
  `.9`, `.31`, `.33`, `.36`, `.45` under `REQ-PLATFORM-WORKSPACE-GIT-STATUS-001`.
- [Path-details design](../../specs/platform/system-design/workspace-git-path-details.md),
  especially Plain selected patches and Preserved execution and quality contracts.
- Accepted ROOT proof and receipt as recorded in the plan; read-only, no replay/removal.
- Current `workspace_git_diff.go`, `workspace_git_literal_paths_test.go`, registered
  status handlers, `git_literal_paths_test.go`, `git_status_helpers_test.go` and
  `git_capture_recovery_test.go`.

## Delivery gates

Use normal active commit hooks without bypass. Later backend-code PR fixup gets
ONE bounded full CHANGED lint at the exact PR base, only when actual corrective
findings require it; no passing broad replay. Record an actual full base SHA before:

```bash
(cd apps/backend && test -n "$KANDEV_PR_BASE_SHA" && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run --new-from-rev="$KANDEV_PR_BASE_SHA" --concurrency=2 --allow-serial-runners --timeout=5m ./...)
```

Require six actual required SUCCESS contexts and terminal successful product parent
workflows, fresh complete error-free exact-head snapshot/resolver including hidden
actionable findings, and configured authenticated CodeRabbit App 347564 substantive
FULL all-file current-head coverage. Sufficient automatic FULL coverage is accepted;
no duplicate optional review request/wait, and ACK is not a review. Disposition every
actual finding with grounded evidence, no speculative polish. Keep a ready PR frozen
except actual corrective findings; no moving-main-only rebase or synthetic merge test.

Exactly one retained normal all-terminal collector per PR; all old handles must be
joined and owned PIDs gone before a ROOT-authorized replacement. CI retries and
resource recovery require bounded ROOT direction with budgets keyed by workflow/job
NAME; no whole/passing replays or duplicate unknown mutations.

Merge requires the separate serial ROOT MERGE lease, normal expected-head squash,
no admin bypass, independent actual MERGED SHA/tree/owned blob and authoritative
remote verification, and only-owned cleanup/joins. Preserve managed worktree,
dependencies, shared caches, foreign processes/refs and accepted ROOT proof until
ROOT independently verifies and archives. Durable checkpoints support recovery;
there is no machine-crash guarantee. Task completion means actual verified merge
and all-owned-handle joins, never PR publication alone.

## Results

Reviewed commands above were run sequentially after the exclusive ROOT lease:

- New process RED: exit 1 (3.467s). All four real producer sites exhibited presentation
  ANSI under forced color; ordinary/disable controls passed. New HTTP RED: exit 1
  (5.834s), actual selected and aggregate decoded patch corruption.
- Four production option additions only. New process GREEN: exit 0 (5.033s), 24
  cases; new HTTP GREEN: exit 0 (7.641s), 18 cases for both registered routes.
  HTTP fixtures also preserve Unicode/ESC filename identity; native Windows excludes
  only the invalid control-character filename. Literal ANSI content is never skipped.
- Existing exact 12-function process controls: exit 0 (5.907s). Existing exact
  three-function HTTP controls: exit 0 (2.090s). No existing fixture changes needed.
- Exact scoped lint: exit 0, zero issues. Catalog (351 decisions/1368 specifications),
  full spec lint, seven-file reference coverage (`covered`, no errors) and whitespace
  passed. All test/lint handles actually joined before their successors.

The authorized single pinned frozen apps install passed (pnpm 9.15.9, 933 packages
reused, zero downloads, unchanged lockfile). The implementation commit's normal
active pre-commit and commit-msg hooks passed with no bypass, including scoped Go
lint, architecture/catalog/specification checks, gofmt and commitlint. Implementation
is complete. Publication, hosted evidence and the later ROOT merge remain delivery
gates. Actual log paths, handle IDs/terminal exits, leases, full hook and external
receipts are maintained in the existing durable child Kandev plan. Task completion
is not claimed here.
