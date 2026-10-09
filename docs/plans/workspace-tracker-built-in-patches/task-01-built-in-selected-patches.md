---
id: "01-built-in-selected-patches"
title: "Produce built-in selected workspace patches"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.1
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.21
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.33
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.36
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.45
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.46
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
---

# Task 01: Produce built-in selected workspace patches

## Summary

Prove that executable external helpers replace actual tracker patch data, including
mixed facets and cached fallback, then suppress helper dispatch at the four selected
patch commands. Preserve current patch content, identity, readiness, resource and
read-only contracts through public tracker reads and registered HTTP transport.

## In scope

- Four exact `--no-ext-diff` options alongside existing `--no-color` in
  `process/workspace_git_diff.go`, at the sites inventoried in the manifest.
- Prospective local real-Git regressions and focused compatibility checks below.
- The four design artifacts, including actual verification results after execution.

## Out of scope

Global/shared helper policy, environment/configuration writes as the fix, validator
changes, numstat, textconv/drivers, standalone/history producers, parser/status/API/
schema/frontend changes, budget increases, production TestMain/shared-fixture
redesign, unrelated defects, broad passing tests and optional polish.
No permanent test/source edit, install or heavy command in this design turn.

## Acceptance

1. Before changing production, meaningful permanent RED proves helper execution and
   substituted patch output at every real producer under configured/env/both modes,
   with executable positive controls and passing ordinary built-in controls.
   GREEN proves exact bounded hunks, counts/status/facets/readiness and independent
   repository identity through actual tracker and registered selected/aggregate HTTP.
2. Production changes are exactly four local options. Read-only captured/process
   environments, configuration/refs/index/worktree, literal/color semantics, cache,
   snapshot ownership, admission, budgets and cancellation retain their current owners.
3. Exact affected checks and normal active hooks pass; record meaningful failures,
   actual terminal joins and results. Implementation completion is distinct from
   publication; task completion requires later verified merge and all owned joins.

## Regression matrix

Create tests only after the later ROOT implementation interrupt and exclusive heavy
lease. Load `/tdd` and its backend test reference then. Reuse `setupTestRepo`,
`seedPlainPatchRepo`, plain-patch assertion/evidence helpers and existing private API
fixtures where appropriate; keep any additional assertion/setup helpers test-local.
Use existing `newCumulativeExternalHelper` / `newExternalHTTPHelper` native test-binary
patterns or a small local specialization with unique marker, sentinel and anchored
`-test.run`. Do not add shell-script launchers, another binary build, production
TestMain behavior or shared-fixture redesign. Keep new test files below the existing
800-effective-line limit and split only local test concerns if necessary.

All fixtures use disposable repositories, isolated system/global Git settings,
explicit captured instance environment, bounded contexts, sequential subtests and
joined tracker/manager teardown. Preserve existing helper-process test entry points;
their environment gate and exact marker prevent normal test runs from exiting early.
Keep executable and sentinel paths outside repositories so status membership is not
polluted. Quote native executable paths using the existing Git helper pattern.
Verify real execution; an unavailable helper or silent control cannot count as RED.

For each producer, cover these modes:

| Mode | Repository configuration | Captured environment | Positive control before read |
| --- | --- | --- | --- |
| ordinary | No `diff.external` | No `GIT_EXTERNAL_DIFF` | Actual built-in patch with expected old/new hunk |
| configured | Helper C | No `GIT_EXTERNAL_DIFF` | Raw actual `git diff --ext-diff`, using C through repository configuration |
| environment | No helper config | Helper E | Raw actual `git diff --ext-diff`, using E through the captured environment |
| both | Helper C | Distinct helper E | Prove C and E independently first, then combine and preserve both inputs |

Each executable positive control checks exact custom output and sentinel contents,
then removes only that owned sentinel. In configured mode do not use a control that
adds `GIT_EXTERNAL_DIFF` and masks the configured helper. In both mode, the combined
control may demonstrate environment precedence, but it does not replace independent
C/E controls. After each actual producer read, require both owned sentinels absent
and helper output absent from all patches. Require exact positive built-in hunks,
not just nonempty data, totals, absence of a string or inspection of argv flags.

Run unstaged, staged-only and mixed dirty tracked states under each mode. Seed local
`refs/remotes/origin/main` from HEAD to make ancestry usable without network. Use
distinct original/index/worktree replacements and unchanged context lines:
single-layer replacement +1/-1; mixed flattened HEAD-to-worktree +2/-2; staged
HEAD-to-index and unstaged index-to-worktree each +1/-1. Assert `modified` status,
staged boolean, exact `Files`/modified membership, empty rename origin/skip reason,
ready snapshot and file/facet state, correct facet presence/absence and only the
appropriate changed lines. Assert headers and exact hunk bytes independently of
helper behavior. Reuse exact expectations from plain-patch tests, without deriving
expected output from the producer under test.

Use bracket names with wildcard-matching sibling and Unicode names on native Windows
as well as POSIX. Keep identical filenames in independent HTTP repositories but
distinct repository/layer markers; no cross-repository or sibling content may enter
the selected patch. Preserve `GIT_LITERAL_PATHSPECS=1` compatibility and the selected
command overrides. Include one forced local `color.diff=always` and a captured
`GIT_CONFIG_COUNT/KEY_n/VALUE_n` color setting in helper-enabled controls, along with
literal ANSI source content. Source ANSI must remain byte-for-byte; do not reject
all escape bytes or strip output. Existing full color tests remain targeted controls.
Only demonstrated unsupported filename assertions, such as a literal ESC filename
on native Windows, may be narrowly scoped. Portable content/helper/transport cases
must remain enabled; no entire new test file may be POSIX-only.

Capture helper environment before constructing the tracker/manager. After capture,
change the live process helper value to a distinct proved helper or unset it; keep
the captured configured/env/both cases unchanged. Preserve caller-owned captured
slices, tracker snapshots and `cfg.AgentEnv`; retain the existing captured-color and
literal-env compatibility evidence because suppression alone cannot prove which
environment a command used. Snapshot captured and live environment after intentional
fixture setup and before each read, then require exact equality afterward.

### Tracker and the actual fourth producer

`process/workspace_git_external_diff_test.go`:

- `TestWorkspaceGitExternalHelpers`: actual `NewWorkspaceTracker`,
  `SetGitEnvironment`, `GetGitStatusWithDetails(ctx,true)` under the complete
  mode/layer matrix above. No fake diff runner. Verify independent positive patch
  expectations, source identity/observed HEAD and all file/facet/read-only data.
- `TestWorkspaceGitExternalHelperCachedFallback`: follow
  `TestWorkspaceGitPlainCachedFallback` and `TestWorkspaceGitLiteralCachedFallback`.
  Seed real staged selected/sibling changes, initialize existing file membership with
  empty flattened `Diff` and no prior patch, and call actual `enrichWithStagedDiff`
  against captured HEAD. It executes real cached Git at the fourth site. Cover all
  helper modes and assert exact staged hunks/status/counts/readiness/sibling exclusion,
  absent helper execution and unchanged repository/env state. Do not invent a public
  API branch or empty out a live accepted snapshot to force the test.
- `TestWorkspaceGitExternalHelperCacheAndDirtyReads`: obtain an accepted ready
  helper-enabled snapshot, then actual `GetGitStatusWithDetails(ctx,false)` and
  `GetGitStatusReplay` retain the accepted files/facets/source/revision and patch.
  Mutate only fixture worktree bytes to new distinct markers; take a new read-only
  baseline and make a fresh public details read. Require the new accepted dirty
  patch/counts with current identity and no old changed-line data. Cache reads must
  not execute helpers; fresh reads cannot attach stale detail. No live background
  polling or sleeps are needed for this deterministic read sequence.
- `TestWorkspaceGitExternalHelperBudgets`: actual real-Git per-file >256 KiB patch
  and a bounded set exceeding the shared 2 MiB threshold, in helper-enabled
  configured/env/both cases, with ordinary controls. Require per-file cap and
  `truncated`, preserved full membership, `budget_exceeded` for skipped patches,
  ready/skipped state, positive built-in hunk content for emitted data and absent
  helper execution. Count every flattened/facet representation against the existing
  budget; allow its existing final-representation overshoot and do not depend on
  map iteration order or timestamps. Reuse current mixed-budget tests as additional
  accounting evidence. Do not use a production hook that fabricates patch bytes.

### Registered HTTP transport

`api/git_status_external_diff_test.go`, `TestGitStatusHTTPExternalHelpers`:

- Actual manager and `NewServer` registration, `getGitAPI` requests through its
  router, independent `selected` and `other` repositories with distinct helper
  sentinels/configuration and content. Set `BaseBranches` and fixture local refs;
  preserve configured/env/both modes per repository, not merely one helper globally.
- Both `/api/v1/git/status?repo=selected&fresh=true&details=wait` and
  `/api/v1/git/status/multi?fresh=true&details=wait` cover all helper/layer controls.
  Decode actual JSON to file/facet types. Assert HTTP 200, success, complete ready
  membership/details, exact selected/aggregate repository set and keys, status/
  counts/facets, expected hunks, literal ANSI preservation and helper nonexecution.
  No direct handler call, mocked tracker result or cumulative-diff substitute.
- After ready reads, selected/aggregate `mode=replay` retains the same accepted
  built-in patch data and repository identity. Dirty fixtures and both repositories
  remain unchanged by reads; use fresh reads again only for intentional dirty-byte
  changes when exercising existing cache/ordering behavior.
- Capture both repositories before/after each selected and aggregate read, not just
  after the whole matrix; retain `cfg.AgentEnv` and live-environment equality.
  Register joined `manager.StopForTeardown` cleanup before checking final helper
  sentinels; teardown must complete and active leak assertions must remain enabled.

### Read-only evidence and compatibility

Read-only snapshots include raw local config, HEAD, all refs, index entries/OIDs/
per-file index blobs, working-tree bytes and dirty/staged porcelain classification.
Include raw index bytes from each disposable repository as well as semantic index
content; reads may not rewrite them. Snapshot probes must themselves be lockless
and explicitly built-in/plain (`--no-optional-locks`, `--no-ext-diff`, `--no-color`),
with isolated helper environment so the measurement cannot execute helpers or mutate
the index. Include unrelated sibling paths and both HTTP repositories. Compare
evidence only across reads; intentional fixture configuration/dirty writes happen
before a new baseline. Helper sentinels are independently checked outside this state.

Keep cancellation, transient-detail failure, retained index/HEAD, snapshot replacement,
literal argv/environment and color semantics through the anchored existing checks.
Any fault selector affected by the extra option may receive a routine causal local
fixture correction, preserving its failure/state assertions. No weakening a real
failure, widening production or altering shared fixtures to make a check pass.
No frontend/browser/build/E2E work: this is a pure shared-data producer correction
with no rendered or viewport-dependent interaction change.

## Verification

From repository root, only after explicit later ROOT release and ONE GLOBAL
LOCAL-HEAVY lease. Mark the work order `in_progress`; write permanent tests; run the
first two commands for meaningful RED; add four options; run affected GREEN and
the compatibility commands once. Do not run an initial broad passing replay.
One heavy command at a time; retain every handle/PID/log and actually join before
starting the next command. A timeout/resource/transport/unknown/out-of-scope failure
checkpoints ROOT without automatic retry or raised limits.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^(TestWorkspaceGitExternalHelpers|TestWorkspaceGitExternalHelperCachedFallback|TestWorkspaceGitExternalHelperCacheAndDirtyReads|TestWorkspaceGitExternalHelperBudgets)$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^TestGitStatusHTTPExternalHelpers$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^(TestWorkspaceGitPlainPatches|TestWorkspaceGitPlainCachedFallback|TestWorkspaceGitLiteralPatchSelection|TestWorkspaceGitLiteralCachedFallback|TestWorkspaceGitLiteralPathspecEnvironment|TestWorkspaceGitLiteralCapturedEnvironment|TestWorkspaceTrackerGitStatusCaptureRecoveryRetriesOneEvidenceChange|TestWorkspaceTrackerGitStatusCaptureRecoveryUsesReplacementIdentity|TestWorkspaceTrackerDetailsWaitCanCancelWithoutCancelingEnrichment|TestWorkspaceTrackerDetailsWaitRejectsSupersededSnapshot|TestWorkspaceTrackerStopCancelsSharedObservationWithoutCaching|TestWorkspaceTrackerRetriesOnlyFailedDiffAfterTransientGitFailure|TestCapDiffOutput_Truncation|TestDiffBudget_TracksReplacedContentAtBoundary|TestMixedChangeFacetDiffBudgetCountsAllRepresentations|TestCarryForwardMixedChangeFacetsRespectsBudget|TestEnrichWithDiffData_CanceledContextReturnsErrorForTrackedFiles|TestDiffBudgetAndCarryForwardHonorCancellation)$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^(TestGitStatusHTTPPlainPatches|TestHandleGitLiteralSelections|TestGitStatusHTTPReturnsRecoveredEnrichedCapture|TestGitStatusMultiRetriesOnlyTheRepositoryWithChangedEvidence)$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/common/securityutil -run '^(TestIsKnownSafeGitFlagAllowsNoExtDiff|TestIsKnownSafeGitFlagRejectsNoExtDiffVariants)$' -count=1 -timeout=2m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 6m golangci-lint run --concurrency=2 --allow-serial-runners --timeout=5m ./internal/agentctl/server/process/... ./internal/agentctl/server/api/... ./internal/common/securityutil/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workspace-tracker-built-in-patches
```

Cheap design preflight (prospective production/test paths intentionally included):

```bash
PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = ['docs/specs/platform/requirements/workspace-git-status.md', 'docs/specs/platform/system-design/workspace-git-path-details.md', 'docs/plans/workspace-tracker-built-in-patches/plan.md', 'docs/plans/workspace-tracker-built-in-patches/task-01-built-in-selected-patches.md'];
const future = ['apps/backend/internal/agentctl/server/process/workspace_git_diff.go', 'apps/backend/internal/agentctl/server/process/workspace_git_external_diff_test.go', 'apps/backend/internal/agentctl/server/api/git_status_external_diff_test.go'];
const result = validateCoverage({ changedFiles: [...docs, ...future], fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])) });
console.log(JSON.stringify(result));
if (!result.ok) process.exitCode = 1;
NODE
```

Design uses only cheap catalog/spec-lint/reference coverage/whitespace/source checks
and the existing Node path. No install or runtime edits. If apps dependencies are
absent at later release, perform one pinned frozen install under the heavy lease
before package commands or normal hooks; do not reinstall Node:

```bash
(cd apps && PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH node /home/jcfs/.cache/node/corepack/v1/pnpm/9.15.9/bin/pnpm.cjs install --frozen-lockfile)
```

Retain all joined results and preserve managed worktree/dependencies/shared caches,
foreign refs/processes, ROOT proof, paused oversized task and unproved volumes.
If a later actual finding requires frontend checks, first checkpoint scope with
ROOT; use Node 4 GiB, one Vitest worker and exact anchored scoped tests, changed
lint/typecheck/i18n/docs/coverage and normal active hooks without bypass. These
conditional constraints do not authorize frontend expansion or checks now.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_diff.go`
- Prospective `apps/backend/internal/agentctl/server/process/workspace_git_external_diff_test.go`
- Prospective `apps/backend/internal/agentctl/server/api/git_status_external_diff_test.go`
- Existing causal fault selector only if necessary for the added option; record it.
- Existing owning requirement/design and this manifest/work order, four artifacts.

## Dependencies

None. Color predecessor is already merged at the actual base recorded in the plan.
Later ROOT implementation interrupt and exclusive heavy lease are operational
gates, not another task or work order. Keep work in this same primary session.

## Risks

False configured-helper proof, missed cached fallback, overbroad Windows skips,
environment leakage, nondeterministic budget order and unjoined tracker teardown
can produce false confidence. Preserve independent positive controls and assertions.
No source or archive replay; current captured environment/index code is authoritative.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/platform/requirements/workspace-git-status.md), especially
  `.46` and all preservation criteria declared in frontmatter.
- [Path-details design](../../specs/platform/system-design/workspace-git-path-details.md),
  Built-in selected patches and Preserved execution and quality contracts.
- [Manifest](plan.md), actual base/source blob inventory and accepted ROOT proof.
- Existing native `git_log_external_diff_test.go` / `api/git_external_diff_test.go`,
  plain/literal patch tests, registered `api/git.go` / `api/server.go`, manager
  captured environment and current snapshot/cancellation/budget tests.

## Later delivery and completion gates

Standing ROOT delivery remains authorized only after reviewed implementation
release and resource leases; design ends now without publication. Use normal ready
PR and normal hooks. Actual backend-code PR fixup gets ONE full changed lint at the
exact live PR base, with no broad passing replay. Record the full base SHA first:

```bash
(cd apps/backend && test -n "$KANDEV_PR_BASE_SHA" && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --kill-after=10s 6m golangci-lint run --new-from-rev="$KANDEV_PR_BASE_SHA" --concurrency=2 --allow-serial-runners --timeout=5m ./...)
```

Freeze the PR head except actual corrective findings. No moving-main-only rebase,
synthetic tests, optional polish or redundant reviewer requests/waits. Early inspect
authenticated configured automatic CodeRabbit App347564 substantive FULL exact-head
review of ALL changed files: source=covered=head and kind=reviewed; ACK/progress is
not semantic coverage. At most one necessary full request follows an actual
completed automatic skip/gap. Ground every finding/disposition in actual evidence.

Use one retained normal all-terminal collector, recording actual PID/start/cutoff/
log/handle. Actually join it and prove PID gone before a ROOT-authorized replacement.
No CI retry without a bounded ROOT grant keyed workflow/job NAME: terminal parent,
fresh frozen OPEN head, exact failed jobs plus normal dependents only, no whole or
passing replay and no budget reset. Require SIX actual required SUCCESS and successful
product-parent workflows, fresh complete error-free exact-head snapshot, resolver
visible/hidden/actionable unresolved=0, no changes requested or human gate. Known
nonrequired auxiliary failures need explicit grounded exceptions and truthful counts,
never a product bypass. Resources/timeouts/transport/unknown/out-of-scope failures
checkpoint ROOT without automatic recovery.

A separate serial ROOT MERGE lease is required. Use normal expected-head squash,
no admin bypass. Cheap authoritative base/static compatibility may be checked;
prove actual MERGED SHA, tree, ALL owned blobs and remote inclusion independently.
Retain/join cleanup only for proven owned scratch/processes. Preserve all managed
and foreign resources and ROOT evidence. Completion means actual merge plus joins,
not PR publication. ROOT independently verifies/archives, removes this slot and
refills. Durable receipts support recovery; no guarantee against machine crash or
promise of unattended continuation after stopping.

## Results

Design validation passed as recorded in the manifest. ROOT reviewed the four
artifacts and later released implementation in this same primary session with the
sole global local-heavy lease. The one pinned frozen apps install passed; its
session59732 was actually joined, PID321807 is gone, and the lockfile is unchanged.

Two permanent native real-Git regression files were written and gofmt applied.
Production remains unchanged. Process RED session56174 was actually joined exit1,
PID337807 gone, log `/tmp/kandev-child43-process-red.log`, receipt
`/tmp/kandev-child43-process-red.receipt`: package 120.045s, Go panic
`test timed out after 2m0s`, while `TestWorkspaceGitExternalHelperCacheAndDirtyReads/configured`
was running. Completed main/fallback cases show actual executable controls followed
by helper execution and `CUSTOM DIFF OUTPUT` replacing built-in hunks. The main
matrix took 90.61s; some mixed reads also reached their 15s context deadlines.
The first complete permanent RED gate is therefore not claimed satisfied.

The timeout was checkpointed without automatic retry or increased bounds. ROOT
then explicitly authorized bounded continuation in this same primary, retaining
the sole local-heavy lease (merge lease remains none). The completed cached
fallback ran 20.20s: configured/environment/both positive controls passed, while
exact patch headers/hunks and helper suppression failed. It will not be replayed.
Session56174 is actually terminal/joined; wrapper337807 and owned fixture processes
are gone, with `/tmp/kandev-child43-process-red-terminal.json` retained.

ROOT authorizes this narrower HTTP RED once, followed by only the four flags if
causal HTTP RED succeeds, then the full unchanged GREEN matrices above:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --kill-after=10s 4m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^TestGitStatusHTTPExternalHelpers$/^(ordinary|configured)$/^mixed$' -count=1 -timeout=2m)
```

This exception avoids repeated race-instrumented helper executions during RED;
it does not replace permanent final coverage. No GORACE, runtime, assertion,
timeout, resource or shared-helper changes are authorized. Unknown/resource
failures still checkpoint ROOT without automatic recovery.

HTTP narrow RED session11127/PID368123 joined exit1, package35.826s, without
timeout. Ordinary mixed passed; configured mixed executable controls passed and
registered selected/aggregate independent repositories failed helper suppression
and exact built-in hunks. Logs/receipts: `/tmp/kandev-child43-http-red.log`,
`/tmp/kandev-child43-http-red.receipt`, `/tmp/kandev-child43-http-red-terminal.json`.
Wrapper/owned fixture processes are gone. The four reviewed producer-local
`--no-ext-diff` options are now applied; full permanent GREEN is next.

## Implementation validation

Production is exactly four `--no-ext-diff` additions beside existing `--no-color`
options; a byte comparison against design-base source proves no other changes.
Permanent test matrices and native helper controls remain intact. No global
configuration, validator, shared fixture, runtime, parser, API or frontend change.

| Check | Actual joined handle / wrapper PID | Result |
| --- | --- | --- |
| Full process GREEN | 31625 / 373253 | PASS, package 37.780s |
| Full registered HTTP GREEN | 98165 / 379686 | PASS, package 33.953s |
| Original process compatibility | 95160 / 387307 | PASS, package 9.032s |
| Original HTTP compatibility | 31827 / 393151 | PASS, package 8.687s |
| Exact security flag/variant compatibility | 55221 / 399217 | PASS, package 1.012s |

All ran sequentially using the exact commands above: trimpath, fts5, race, p=1,
GOMAXPROCS=2, GOMEMLIMIT=512MiB, Go2m/GNU4m/kill10. Every handle is terminal and
actually joined, each wrapper gone before the next heavy operation. Receipts and
logs are `/tmp/kandev-child43-{process-green,http-green,process-compat,http-compat,security-compat}.{log,receipt}`.
No passing replay, bounds increase, GORACE adjustment or reinstall was used.
The failed partial process RED timeout remains preserved and is not a passing
suite. The ROOT-authorized narrow HTTP RED proves the transport regression.

Catalog validation passed (351 decisions, 1369 specifications); all-spec lint
passed. Actual seven-file documentation coverage passed, covered/errors0, receipt
`/tmp/kandev-child43-docs-coverage.json`. gofmt and whitespace checks passed.
Public Git guide audited: no public controls/API/copy/workflow change justifies an
edit. Backend patch data preserves desktop/mobile consumers; no browser, build,
E2E, typecheck or frontend test is required for this pure-data fix.
Scoped three-package lint passed with zero issues, original concurrency2/serial/
CLI5m/GNU6m/GOMAX2/GOMEM512 bounds: session26580/PID399917 actually joined exit0
at04:38:48Z, wrapper/owned descendants gone. Log/receipt:
`/tmp/kandev-child43-scoped-lint.log`, `/tmp/kandev-child43-scoped-lint.receipt`.
Normal active hooks and publication follow. Exclusive local-heavy lease is retained
through joined publication, then returned before the sole hosted collector.
Task 01 remains in progress until actual separately authorized merge and all joins.
