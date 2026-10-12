---
id: "01-produce-plain-comparisons"
title: "Produce plain comparison patches"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-DIFF-FILE-METADATA-001
acceptance_criteria:
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.1
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.2
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.4
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.5
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.6
  - AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.7
system_design:
  - ../../specs/platform/system-design/git-diff-file-metadata.md
---

# Task 01: Produce plain comparison patches

## Summary

Make commit and cumulative patch output machine-readable under forced Git
display color. Add faithful real-Git and registered-HTTP regressions before the
two per-invocation flags, then complete the bounded checks and authorized delivery.

## In scope

- The two patch-producing argv lists in `git_log.go` and two new regression files.
- Parent-released dependency: ONLY exact `--no-color` in common/securityutil
  `git.go` with allow/variant-rejection tests in existing `git_test.go`.
- The exact matrix and branch/read-only assertions in the [plan](plan.md#tests).
- Maintain the owning requirement/design and this sequential execution record.
- Later release only: active hooks, conventional commit, push, ready PR, hosted
  gates/dispositions, normal merge and independently verified owned cleanup.

## Out of scope

Parser stripping/normalization, generic color policy, config mutation, environment
or tracker refactoring, API schemas, new dependencies, frontend/browser/build,
full backend suites, generic QA/review/audits, foreign-process/cache cleanup.

## Acceptance

1. Permanent production-method and registered-HTTP regressions first fail on
   forced-color nonempty membership; the default/disabled controls succeed.
   `--no-color` in only the two patch invocations makes them pass.
2. Positive membership/status/path/count/metadata assertions, literal ANSI content,
   binary/empty-file/root/merge/genuinely-empty controls, selected and aggregate
   independent-repository identity and read-only snapshots satisfy .1-.7.
3. Exact scoped checks and documentation coverage pass; existing execution,
   environment, comparison bases, limits and response shapes remain intact.

## Verification

Run from the repository root, one heavy command at a time. First write the
permanent tests and run the two RED commands independently before the fix;
retain each result even when the expected failure is exit 1. Then add the two
flags and run each affected package's exact GREEN command once.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p=1 ./internal/agentctl/server/process -run '^TestGitComparisonPlainOutput' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p=1 ./internal/agentctl/server/api -run '^TestGitComparisonPlainOutput' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p=1 ./internal/agentctl/server/process -run '^(TestGitComparisonPlainOutput.*|TestGitDiffStatusMetadataCallers|TestShowCommit_StatusMetadataRootAndEmpty|TestParseCommitDiffWithOptions_StatusMetadata.*|TestShowCommit_MergeCommitUsesFirstParentDiff|TestShowCommit_CountsDashAndPlusPrefixedContent|TestGetCumulativeDiff_CountsDashAndPlusPrefixedContent|TestGetCumulativeDiff_StablePrefixesIgnoreGitDiffConfig|TestGetCumulativeDiff_ModeOnlyBSlashPath|TestGetCumulativeDiff_TruncatesLargeFile|TestGetCumulativeDiff_BudgetExceeded|TestGetCumulativeDiff_CapsFileCount|TestShowCommit_NotCapped)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p=1 ./internal/agentctl/server/api -run '^(TestGitComparisonPlainOutput.*|TestGitDiffStatusMetadataHTTP|TestGitDiffStatusMetadataMultiRepoHTTP|TestHandleGitShowCommit_.*)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/common/securityutil --new-from-rev=678a4d19ad5ffd8f6f9c5f7be419bbdc02609d24 --concurrency=2 --allow-serial-runners --timeout=5m)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p=1 ./internal/common/securityutil -run '^TestIsKnownSafeGitFlag' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/git-comparison-plain-output
```

The first two commands are RED only; the later broader targeted commands provide
GREEN without a duplicate passing prefix run. If a correction affects only one
boundary, rerun its affected check only. Use `-p=1`, never `-p1`.

Run `.github/scripts/pr-docs.cjs` exported `validateCoverage` with actual changed
paths and the four package document contents. During design, include the intended
`git_log.go` runtime path so documentation-only exemption is not mistaken for
coverage. During implementation, use the actual diff. Record `covered`, no errors.

For an actual backend PR fixup, scoped AGENTS requires one full-backend changed
lint before push, using the exact current PR base SHA (resolve it first):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB golangci-lint run ./... --new-from-rev="$KANDEV_PLAIN_OUTPUT_PR_BASE" --concurrency=2 --allow-serial-runners --timeout=5m)
```

No full backend tests, Vitest or E2E. Normal commit hooks remain active.
If `apps/node_modules` is absent, install once with pinned pnpm 9.15.9 and
`pnpm install --frozen-lockfile` from `apps/` before hooks. Keep dependencies
and worktree for the parent's archive if useful; no shared-cache deletion.

## Files likely touched

- `apps/backend/internal/common/securityutil/git.go`
- `apps/backend/internal/common/securityutil/git_test.go`

- `apps/backend/internal/agentctl/server/process/git_log.go`
- `apps/backend/internal/agentctl/server/process/git_log_plain_output_test.go` (new)
- `apps/backend/internal/agentctl/server/api/git_plain_output_test.go` (new)
- `docs/specs/platform/requirements/git-diff-file-metadata.md`
- `docs/specs/platform/system-design/git-diff-file-metadata.md`
- `docs/plans/git-comparison-plain-output/plan.md`
- `docs/plans/git-comparison-plain-output/task-01-produce-plain-comparisons.md`

## Dependencies

Exact plain-output flag registration is required by the existing argument validator.
The parent released this bounded dependency after the first GREEN attempt
rejected `--no-color`; no broader validation policy is authorized.

No preceding order. Prior PR #4178 is actually merged and cleaned up. Main baseline is pinned
above; no dependency branch or planned parallel work exists.

## Risks

Use explicit positive expected files, not successful-empty equivalence. Keep
literal escape sequences intact. Build fixture configuration/environment before
manager snapshots, and bracket reads with owned config/HEAD/refs/index/status/
content snapshots. Do not suppress or weaken failures with retries/timeouts.

## Parallelism

`sequential`. This session owns all work; no agents/workers/tasks/sessions.

## Inputs

- [Requirement](../../specs/platform/requirements/git-diff-file-metadata.md), .1-.7.
- [Design](../../specs/platform/system-design/git-diff-file-metadata.md), plain output and preserved contracts.
- Existing `git_log_status_metadata_integration_test.go`, API
  `git_status_metadata_test.go`, `git_handlers_test.go`, `setupTestRepo`,
  `runGit`, `runGitAPI`, captured operator environment and registered router.
- Parent's accepted proof and [delivery gates](plan.md#delivery-gates).

## Results

Implementation released by later explicit parent message on 2026-10-03.
Permanent real-Git process RED: expected empty membership under forced color,
exit 1, package 0.652s, handle 70899 joined. Registered-HTTP RED: selected and
aggregate success-with-empty failure, exit 1, package 0.409s, handle 27281 joined.
Default/disabled controls passed. First process GREEN attempt failed, exit 1,
package 1.260s, handle 50256 joined: existing validation rejects `--no-color`.
Parent explicitly released only the exact flag allowlist dependency. Securityutil
RED failed on the expected missing flag, exit 1, package 0.011s.
One pnpm 9.15.9 frozen install completed using existing cache, handle 50175 joined;
no tracked dependency changes. ANSI content runs on every platform; Windows
excludes only the control-character filename fixture. Final scoped GREEN checks passed: process 3.067s (handle 47933), registered
API 1.867s (handle 11271), securityutil 1.012s (handle 63054); all joined.
Scoped lint including all three packages passed with zero issues (handle 39553
joined), concurrency 2, serial runners, five-minute timeout.
Catalog validation passed (343 decisions, 1321 specifications), all-file spec
lint and whitespace checks passed. Actual changed-file `validateCoverage`
returned `covered`, no errors. No full backend suite, browser/E2E or build ran.
Local implementation is complete; normal hooks and hosted delivery are tracked
in the external task plan and remain completion gates for the child.

Hosted review remediation: future scoped commands above now include `-tags fts5`
to match repository Make/CI configuration. The original RED/GREEN receipts were
run without that tag as originally reviewed. New affected API oracle regression
with disposable global forced color failed as expected on patch equality
(tagged RED, 0.246s, handle 59363 joined). The affected API race GREEN passed in 1.797s (handle 91801 joined);
unchanged passing process/securityutil checks retain their actual receipts.
The mandatory full-backend changed lint uses exact PR base
`678a4d19ad5ffd8f6f9c5f7be419bbdc02609d24`. First full changed lint
joined exit 4 for timeout (handle 27034); zero issues was not a pass. Parent
authorized one recovery with OS timeout 6m and GOMEMLIMIT=1GiB, all other flags
unchanged. Recovery joined exit 0 with zero issues (handle 15675). Local
remediation is complete; corrected-head hosted gates and merge remain pending.

## Parent-released hosted fixture remediation

The exact hosted lifecycle cancellation cleanup failure and direct-versus-shared
launch analysis are recorded in the [plan](plan.md#bounded-hosted-cancellation-fixture-correction).
The bounded added file is
`apps/backend/internal/agent/runtime/lifecycle/manager_managed_go_cache_test.go`:
remove only the cancellation fixture SessionID to use existing direct launch.
No production cache/coalescing behavior, assertion, sleep or cleanup policy changes.
Affected check, from `apps/backend`:

```bash
GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -race -trimpath -tags fts5 -p=1 ./internal/agent/runtime/lifecycle -run '^TestManagedGoCacheCancellation(PreventsLaunch|Preserved)$' -count=10
```

Corrected GREEN joined exit 0 in 1.149s (42267). Retain earlier Git and API
checks; no package/full-suite replay. Full changed-backend lint replacement is
sequential after joined test/stopped lint handles, with GNU OS timeout 6m,
GOMAXPROCS=2, GOMEMLIMIT=1GiB, exact base above, concurrency 2, serial runners
and CLI timeout 5m. Normal hooks, corrective publication, one necessary full
all-file review and one hosted monitor precede actual verified merge/cleanup.

Replacement full changed-backend lint joined exit 0 with zero issues (35060),
using the exact base and bounded flags above. Local remediation is complete;
new-head hosted review/CI and actual merge remain delivery gates.
