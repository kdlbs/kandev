---
id: "01-preserve-gitlink-patches"
title: "Preserve gitlink patches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
acceptance_criteria:
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.12
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
---

# Task 01: Preserve gitlink patches

## Summary and prerequisite

First admit ONLY exact `--submodule=short` in the safe flag list, with exact
positive and variant rejection tests. Then add that flag to the TWO existing
comparison patch argv in `git_log.go` after independently authored causal RED.
The dependency is confirmed absent and already scoped; no bypass, prefix
expansion, global Git policy or additional approval gate is needed.

Read the [manifest](plan.md#tests), the owner
[requirement](../../specs/platform/requirements/git-diff-file-metadata.md)
and [design](../../specs/platform/system-design/git-diff-file-metadata.md#short-parent-gitlink-patches).
Accepted ROOT evidence is recorded in the manifest. Never access, import,
copy or replay ROOT's protected proof source or fixture, including
`candidate_test.go`. Independently author the bounded permanent tests.

## In scope and owned files

- Production `apps/backend/internal/common/securityutil/git.go`: one exact
  `--submodule=short` list entry. Keep `validateGitCommandArgs` unchanged.
- Production `apps/backend/internal/agentctl/server/process/git_log.go`: one
  argument in `ShowCommit`'s patch invocation and one in `GetCumulativeDiff`.
- Existing tests `apps/backend/internal/common/securityutil/git_test.go`:
  `TestIsKnownSafeGitFlagAllowsSubmoduleShort` and
  `TestIsKnownSafeGitFlagRejectsSubmoduleVariants`.
- New tests `apps/backend/internal/agentctl/server/process/git_log_submodule_format_test.go`:
  `TestGitComparisonSubmoduleFormat` and
  `TestGitComparisonSubmoduleFormatControls`.
- New tests `apps/backend/internal/agentctl/server/api/git_submodule_format_test.go`:
  `TestGitComparisonSubmoduleFormatHTTP` and
  `TestGitComparisonSubmoduleFormatMultiRepoHTTP` using the registered router.
- This package and the existing Platform git-diff-file-metadata owner pair.

## Out of scope

Parser/framework/helper redesign, frontend/runtime/schema/history changes,
new user copy/navigation/API shapes, global config/environment/helper policy,
other Git readers/writers or broad writer audit, nested discovery/routing/base
changes, new dependencies, sibling packages/shared fixture rewrites, browser,
database/application launch, broad suites or passing-check replay.

## Acceptance

1. Independently authored native Git regressions reach BOTH public methods and
   causally fail on log-format parent gitlink loss before correction, with short
   gitlink and ordinary-file-under-log controls passing. After correction the
   manifest's bounded forward/back/add/delete/default/short/log/diff matrix
   returns explicit expected paths, complete short patch and old/new IDs,
   statuses, counts and metadata; dirty/root/empty controls remain correct.
2. Actual registered selected commit/cumulative and aggregate cumulative HTTP
   tests return distinct parent/child identities, exact per-scope bases and
   child file content under log with short controls. Selected isolation and
   parent-anchor child comparisons hold, and all owned read-only state
   snapshots are unchanged. Do not assert aggregate-empty discovery claims.
3. Exact admission passes while all altered variants fail. The final production
   diff is three flag entries. Focused first-parent/prefix/uncapped/budget and
   prior color/textconv/helper controls pass once, and document gates pass.

## Dependencies and sequence

No prior work order. Exact safe admission is an internal dependency delivered
with the two producer entries in this work order. Keep execution sequential in
the SAME primary session; no delegates or other task/session creation.

1. Await LATER ROOT's reviewed implementation INTERRUPT and exclusive GLOBAL
   LOCAL-HEAVY grant. Re-read the package, relevant scoped guidance and actual
   baseline; mark this work order `in_progress`. Refresh any moving dependency
   evidence before editing; do not rebase merely because main moved.
2. Author permanent regressions from the manifest without protected discovery
   source. Use actual Git in `t.TempDir` fixtures and existing native test
   patterns. Pin fixture environment before operator/manager capture and join
   manager teardown. No shell helper or Git shim is required. Test-only local
   submodule setup may use explicit per-command file-protocol allowance; do
   not change production Git policy.
3. Run the three anchored RED commands below serially, join each original
   handle and retain raw exit/PID/PGID receipts. RED must be causal, with
   controls passing; a fixture error is not acceptance evidence. Security
   tests must reject bare `--submodule`, abbreviations, log/diff/empty/other
   values, extra suffixes, whitespace and control-character variants.
4. Add exactly the safe-list entry plus two patch arguments after the subcommand
   and before the ref. Preserve the separate commit metadata query, all current
   safety/prefix/textconv/helper flags, bases, budgets and transport shapes.
5. Run affected GREEN plus the selected existing controls once, then scoped
   lint. Routine causal fixture/lint repairs rerun affected checks only.
   Resource/timeout/transport/unknown-cause/out-of-scope failure checkpoints
   ROOT before alternatives, retries, cache changes or foreign process kills.
6. Record actual results, reconcile the owner pair and package, mark acceptance
   `done` only after checks pass, and explicitly return LOCAL-HEAVY after fresh
   own-process absence. Persistent delivery remains incomplete until its gates.

## Verification commands after release only

Each command is rooted independently; run them SERIAL, never in parallel.
Outer GNU timeout is 6m with TERM and kill-after 10s; Go timeout is 5m.
Every Go invocation uses `-trimpath -tags fts5 -race -p=1`, GOMAXPROCS=2 and
GOMEMLIMIT=512MiB. Retain original native handle, complete raw output/exit and
actual subprocess join, cwd/argv, wrapper PID and owned PID/PGID. Join all
handles before fresh owned PID/group absence and explicit LOCAL-HEAVY RETURN.

RED (independent new tests only):

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/common/securityutil -run '^TestIsKnownSafeGitFlag(AllowsSubmoduleShort|RejectsSubmoduleVariants)$' -count=1 -timeout=5m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^TestGitComparisonSubmoduleFormat(Controls)?$' -count=1 -timeout=5m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^TestGitComparisonSubmoduleFormat(HTTP|MultiRepoHTTP)$' -count=1 -timeout=5m -v)
```

GREEN reuses the exact securityutil RED command, then these affected tests and
selected existing controls once (do not also replay the process/API RED groups):

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/process -run '^(TestGitComparisonSubmoduleFormat(Controls)?|TestGitComparisonPlainOutput(RootAndEmpty|Merge)?|TestGetCumulativeDiff_(StablePrefixesIgnoreGitDiffConfig|TruncatesLargeFile|BudgetExceeded|CapsFileCount)|TestShowCommit_NotCapped|TestGitComparisonTextconv(Controls)?|TestCumulativeDiffExternalHelpers)$' -count=1 -timeout=5m -v)
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 6m go test -trimpath -tags fts5 -race -p=1 ./internal/agentctl/server/api -run '^(TestGitComparisonSubmoduleFormat(HTTP|MultiRepoHTTP)|TestNestedSubmoduleReviewEndpointsIncludeRootAndStableChildBase)$' -count=1 -timeout=5m -v)
```

Initial bounded scoped lint uses the reviewed design baseline, refresh it with
ROOT if the implementation baseline differs. Preserve serial concurrency 2:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./internal/common/securityutil ./internal/agentctl/server/process ./internal/agentctl/server/api --new-from-rev=993f60ce889b6dcca1ca19fe98703a9159ab28c3 --concurrency=2 --allow-serial-runners --timeout=5m)
```

An ACTUAL backend PR fixup requires ONE full CHANGED lint against the refreshed
exact PR base before pushing; doc-only lint replay is not evidence. Replace
`<exact-PR-base-sha>` with the freshly verified PR base, and run under a ROOT
lease with the same serial bounds:

```bash
(cd apps/backend && env GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev='<exact-PR-base-sha>' --concurrency=2 --allow-serial-runners --timeout=5m)
```

Light document checks (also allowed during DESIGN):

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py specs --text git-diff-file-metadata --format paths
git diff --check -- docs/specs/platform/requirements/git-diff-file-metadata.md docs/specs/platform/system-design/git-diff-file-metadata.md docs/plans/git-submodule-comparison-format
git status --short -- docs/plans/git-submodule-comparison-format
```

Run `.github/scripts/pr-docs.cjs`'s actual `validateCoverage` on these four
document contents and on the same paths plus the two planned production paths.
Require exempt and covered respectively, error-free with the correct ONE
work order and resolved owner requirement/design. This is reference coverage
only, not hosted or semantic implementation proof.

## Surface assessment

Pure comparison data repair: desktop/phone composition, copy, navigation,
scrolling and touch behavior remain unchanged. Registered HTTP and process
evidence satisfy the mobile pure-data exception; no ASCII/UI/Playwright flow.
Existing public Git operations and submodule Review guidance remains accurate;
no public guide edit. New product surface work requires a revised package.

## Later delivery gates

Inactive until ROOT release. Existing Node 24.21.0 lives at
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`; use Bash `login:false`.
After LOCAL-HEAVY admission, at most one conditional pinned pnpm 9.15.9
`install --frozen-lockfile` from `apps/` if worktree dependencies are missing
before hooked commands. No install in DESIGN. Keep normal active hooks;
no bypass. Ready PR is frozen except valid findings; no main-only rebase,
synthetic merged tests, broad passing replay or weakened checks.

Canonical caller-bound associated repository:
`16026b06-bd79-47c0-aed1-dc7ca95f63d9`. Require complete/error-free association,
exact head/PR and all FIVE automation flags false; zero relink/patch if already
correct. Unknown state is reconciled before duplicate work; no slug/taskrepo
or unattached GitLab mutation. Preserve unchecked PR template and bot additions.

HOSTED HOLD until ROOT qualifies LOCAL-HEAVY RETURN and sends later release.
Then ONE original attached all-terminal 90m collector, GNU 91m/kill-after 10s,
cadence 60s; preserve through crashes and join/prove gone before a
ROOT-authorized replacement. First bounded semantic inspection while CI runs:
authenticated configured App347564 substantive FULL CURRENT ALL paths,
source=covered/kind=reviewed. Sufficient report means ZERO requests; at most ONE
necessary actual completed-skip/gap request after inspecting. Processing/ACK
is not pass; no redundant optional wait. Require all SIX required contexts plus
actual Backend/Frontend/E2E parents SUCCESS on exact head, fresh complete and
error-free state, zero actionable visible/hidden threads, changes requested or
human gate. Ground findings; defer polish. Only an exact failed job may retry
after parent terminal, fresh OPEN head and ROOT retry budget; no blanket rerun.

MERGE NONE until separate ROOT serial static compatibility grant. Authorized
normal expected-head squash/noadmin only; verify actual SHA/tree/parent, all
owned blobs and main inclusion, then join every original and owned-only cleanup.
ROOT independently verifies/FFs/archives/ABSENT/checksum proof and releases.
Preserve managed worktree/dependencies/caches/foreign processes/refs, paused
task `a6032d95-cc1e-4db8-adca-5b643ae4138d` and unproved volume
`2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.
Callbacks are optional, never a progress gate or retry; no child-to-ROOT
interrupts. Persist checkpoints in the own task plan and current primary.

## Risks and parallelism

Missing exact admission, config-sensitive oracles, wrong repository selection,
missing required query binding and leaking linked-child state are the main
risks. Follow the manifest's bounded matrix and snapshot contracts.
`sequential`: one work order; no parallel agent work.

## Results

Implementation acceptance done on 2026-10-09; local delivery in progress.

Independent RED: securityutil original171a4a exit1 (exact admission missing,
variants rejected); process13956/3e78df exit1 (log loses parent gitlink in BOTH
methods; inline diff substitutes child path; default/short and ordinary/empty
controls pass); registered HTTP90918/b6687c exit1 (selected commit/cumulative
parent loss and aggregate parent loss with child files retained; short controls
pass). No fixture or unrelated failure. Production then changed exactly three
flag entries.

Affected GREEN: securityutil61210/bc1932 exit0, 1.012s; process7309/3a5c75 exit0,
20.613s including all selected compatibility controls once; API15104/32c3f4
exit0, 2.386s including stable child anchor control. Process fixture then gained
owned home isolation; only new affected cases reran as 52861/d1a3ac exit0,
2.806s. No passing compatibility replay. Scoped exact-baseline lint18997/004fb4
exit0, zero issues, --allow-serial-runners and all reviewed bounds retained.

Every original native handle and subprocess actually joined; each receipt has
fresh empty owned groups. Raw logs/argv/cwd/PID/PGID/intervals/exit receipts are
in /tmp/kandev-child95-submodule-20261009. No protected proof access, source copy,
new API/UI/schema/helper/global policy or nested comparison change. Desktop and
phone use corrected data with unchanged composition. No public guide edit.

Local publication and explicit LOCAL-HEAVY RETURN receipts are tracked in the
own versioned task plan. The one conditional frozen pnpm9.15.9 install passed
(original99633/f23146, 1.8s), with lockfile unchanged and no downloads. HOSTED/SEMANTIC HOLD and MERGE NONE remain
until ROOT's separate releases. Persistent task delivery is not complete.

Catalog validation (365 decisions/1480 specs), all-spec lint, actual nine-path
PR documentation coverage (covered, one work order, zero errors), gofmt and
whitespace checks passed after implementation.
