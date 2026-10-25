---
id: "01-preserve-actual-byte-patches"
title: "Preserve actual-byte comparison patches"
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
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.10
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.11
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
---

# Task 01: Preserve actual-byte comparison patches

## Summary and dependencies

Add exact `--no-textconv` to the TWO existing patch-producing argument lists
in `GitOperator.ShowCommit` and `GetCumulativeDiff`. Include the confirmed
missing exact admission in `securityutil.IsKnownSafeGitFlag` in this same
work order; this is already scoped, with no later permission gate.
ROOT reviewed all four artifacts and released implementation with child36
LOCAL-HEAVY. The initial HTTP fixture failure was joined and preserved below;
ROOT explicitly regranted the lease for the minimal request correction and
continued implementation. Corrected HTTP RED joined causally before the three
production flag entries. Routine in-scope corrections rerun only affected checks.

## Scope and files

- Production: only `apps/backend/internal/agentctl/server/process/git_log.go`
  (two argument lists) and `apps/backend/internal/common/securityutil/git.go`
  (exact list entry).
- Tests: new `apps/backend/internal/agentctl/server/process/git_log_textconv_test.go`,
  new `apps/backend/internal/agentctl/server/api/git_textconv_test.go`, and
  existing `apps/backend/internal/common/securityutil/git_test.go`.
- Documents: existing Platform git-diff-file-metadata pair and this package.
- Exclude parser/count/budget/ref/environment/global helper policy/framework,
  other Git readers/writes, shared fixture rewrites, siblings' files/plans,
  UI/copy/browser/E2E/build/PG/full suites, new dependencies or agents.

## Acceptance

1. Actual-Git process regressions independently fail before production edits
   for transformed/hidden bytes and executed converters, then return explicit
   expected paths, actual old/new bytes, statuses/counts and metadata with no
   driver and configured changing/suppressing drivers. Dirty cumulative,
   converted binary, empty-file and genuinely empty controls behave correctly.
2. Registered selected/aggregate HTTP reads preserve independent repository
   identity, NUL-qualified aggregate keys, base/repository metadata and selected
   isolation. Converter execution sentinels stay absent; config/attributes,
   HEAD/refs/index/worktree bytes/status stay unchanged across reads.
3. Only exact flag admission passes, altered variants fail; targeted preserved
   first-parent/root/empty/uncapped/budget controls and document gates pass.

## Implementation sequence

Read the [manifest](plan.md#test-design) and paired design. Execute the single
work order sequentially under the explicit release and lease. Add small real-Git tests using guarded native
test-binary helpers, positive execution controls and converter-disabled raw
oracles/snapshots, following existing external-diff patterns without changing
shared fixtures. Run anchored RED commands and join every handle, then add the
single exact allowlist entry and two flags. Run the same affected commands GREEN
once; additional checks require a new change, failure or unresolved concern.
Synchronize work-order/manifest results and status honestly. Normal hooked
delivery/ROOT merge lease follows the manifest; local done is not task COMPLETE.

## Verification after release and lease

Run from repo root, sequentially; each command independently roots its package.
Retain stdout/stderr, exit and every returned handle; ACTUALLY JOIN before the
next command. No automatic retries or cache cleanup after a resource failure.
Use Go's valid `-p=1` spelling for the requested one-package concurrency
(`-p1` is invalid, as recorded by the prior plain-output package). RED runs
only new regressions, sequentially after the LOCAL-HEAVY grant:

```bash
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=5m ./internal/common/securityutil -run '^(TestIsKnownSafeGitFlagAllowsNoTextconv|TestIsKnownSafeGitFlagRejectsNoTextconvVariants)$')
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/process -run '^(TestGitComparisonTextconv|TestGitComparisonTextconvControls)$')
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/api -run '^TestGitComparisonTextconvHTTP$')
```

After joined meaningful RED and the three scoped production edits, GREEN uses
these same securityutil/API commands once. The process GREEN replaces its RED
command with the following, including necessary existing compatibility controls
only once:

```bash
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=5m ./internal/agentctl/server/process -run '^(TestGitComparisonTextconv|TestGitComparisonTextconvControls|TestGitComparisonPlainOutputRootAndEmpty|TestGitComparisonPlainOutputMerge|TestCumulativeDiffExternalHelperBudgets|TestShowCommit_NotCapped)$')
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=1GiB golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/common/securityutil --new-from-rev=dd7dfa81634236cfeb0df6fd7fac4e005d08d2f3 --timeout=5m --concurrency=2 --allow-serial-runners)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Initial scoped lint uses the exact reviewed checkout base above. Full CHANGED
`./...` is reserved for an actual backend-code PR fixup as specified below.

Run the lightweight PR-doc `validateCoverage` preflight from
`.github/scripts/pr-docs.cjs` with actual changed paths and all four documents
before publication. If `apps/node_modules` is absent, perform ONE frozen
`pnpm@9.15.9` install from `apps/` after lease, join, then normal active hooks.
For an actual backend-code PR fixup, resolve and record the PR's exact base SHA
as `KANDEV_TEXTCONV_PR_BASE_SHA`, then run the required full CHANGED lint ONCE:

```bash
: "${KANDEV_TEXTCONV_PR_BASE_SHA:?set the exact current PR base SHA from PR metadata}"
(cd apps/backend && timeout --kill-after=10s 6m env GOMAXPROCS=2 GOMEMLIMIT=1GiB golangci-lint run ./... --new-from-rev="$KANDEV_TEXTCONV_PR_BASE_SHA" --timeout=5m --concurrency=2 --allow-serial-runners)
```

## Inputs, risks and parallelism

Inputs: paired design's actual-byte section, manifest evidence/test matrix,
`git_log_external_diff_test.go`, API `git_external_diff_test.go`, scoped backend
and agentctl guidance, `/tdd` backend-tests reference. No proof replay.
Main risks: helper execution during observation, false empty equality,
wrong repository SHA, native path quoting and stale captured environments.
Parallelism: `sequential`; no new agents/tasks/sessions/model changes.
The pure-data mobile exception applies. Public-guide audit requires no edit.

## Results

Authored process/native-helper and controls, registered independent HTTP, and
exact securityutil allow/reject regressions. All ran with the specified bounded
race settings, sequentially and actually joined:

- Securityutil RED: exit 1, package 0.012s, exact flag rejected as expected;
  altered variants passed. `/tmp/kandev-child36-textconv-securityutil-red.log`.
- Process RED handle 63254: exit 1, package 25.867s. No-driver controls passed;
  configured converters changed/suppressed patches and executed as expected.
  `/tmp/kandev-child36-textconv-process-red.log`.
- HTTP RED handle 76166: exit 1, package 13.726s. Selected no-driver controls
  passed, configured selected reads showed causal textconv failures. Both
  aggregate controls unexpectedly returned HTTP 400 because the new fixture
  omitted the required `base` query parameter. This is a test-authoring error,
  not a production verdict for aggregation. `/tmp/kandev-child36-textconv-api-red.log`.

Initial checkpoint was WAITING ROOT under the actual-failure barrier. Guidance for
the bounded repair: change only this new test's aggregate request to
`/api/v1/git/cumulative-diff?base=` plus `repos[0].base`, as in existing independent
repository HTTP tests; retain the independent per-repository raw oracles.
After ROOT direction and a new lease, the next command is HTTP RED above,
without replaying already joined securityutil/process RED. No automatic retry.
Production is unchanged. No GREEN/lint/install/hooks/commit/push/PR has run.
All local handles joined; lease returned, zero live owned handles. No callback.


### Regrant and causal corrected RED

ROOT regranted child36 LOCAL-HEAVY for the minimal aggregate request correction
(`?base=` plus `repos[0].base`), preserving independent bases and raw oracles.
Corrected HTTP RED handle 71117 joined exit 1, package 15.798s: all no-driver
controls passed, configured selected and aggregate reads failed causally on
suppressed/transformed patches and executed sentinels. Receipt:
`/tmp/kandev-child36-textconv-api-red-corrected.log`. Earlier RED was not replayed.

Applied only the two patch-producer flags and exact allowlist entry. Securityutil
GREEN handle 14163 joined exit 0 (1.014s); process GREEN handle 48364 joined
exit 0 (13.477s) including required existing compatibility/budget controls once;
HTTP GREEN handle 96081 joined exit 0 (6.049s). Initial three-package lint
handle 11977 joined exit 1 with one gocritic ifElseChain finding in the new API
helper. Its dispatch became switch; only affected HTTP GREEN/API lint are rerun.
No assertion, timeout or policy was weakened.


### Completed local implementation checks

After the routine helper dispatch correction, affected HTTP GREEN handle 89295
joined exit 0 (6.043s), and API-only exact-base lint handle 46297 joined exit 0
with zero issues. Earlier three-package lint identified no process/securityutil
issues; those unchanged passing checks were not replayed. Initial lint and
fixture failure receipts remain archived honestly. No full-backend initial lint.

Catalog and all-spec lint passed. Actual changed-path documentation coverage
is covered with no errors and exactly one work order, recorded in
`/tmp/kandev-child36-textconv-doc-coverage.json`. Git whitespace and gofmt checks
are clean. Backend-only source data correction uses the pure-data mobile
exception; existing public read-only comparison guidance needs no edit.

Local implementation/acceptance is done. Normal hooked ready publication and
hosted FULL current-head review/terminal CI remain delivery gates, with a
separate ROOT merge lease and verified actual merge required for persistent
task completion. Managed worktree/dependencies and ROOT proof remain preserved.
