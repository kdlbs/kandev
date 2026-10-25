---
id: "01-faithful-pushed-lookup"
title: "Match bounded pushed evidence to returned history"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001
acceptance_criteria:
  - AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.1
  - AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.2
  - AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.3
  - AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.4
  - AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.5
  - AC-PLATFORM-GIT-COMMIT-PUSHED-EVIDENCE-001.6
system_design:
  - ../../specs/platform/system-design/git-commit-pushed-evidence.md
---

# Task 01: Match bounded pushed evidence to returned history

## Summary

Make the optional local Git lookup faithfully cover the already returned rows.
Prove the correction with actual operators and registered selected/aggregate HTTP
routes, keeping the same API and consumer policy.

## In scope

- All seven named process tests and the registered HTTP producer test in the
  [plan's test matrix](plan.md#tests-and-traceability).
- Base context and captured positive tip in `markPushedCommits`, conditional
  first-parent inclusion, full-parent exclusions, original cap and failure behavior.
- Update only misleading production comments about traversal/evidence; comments
  state the invariant without regression-history or AC narration.
- Accurate work-order/plan results and spec status when later implementation is released.

## Out of scope

Provider history and mutation policy, UI markup/copy/navigation, new fields,
N+1 Git ancestry processes, removing/enlarging the evidence cap, generalized
graph/cache infrastructure, network reads, broad test/audit work, or other tasks.

## Acceptance

1. The retained merged-side topology fails permanent RED on the older local
   first-parent row's pushed truth, then passes GREEN with the positive real push,
   mixed states, both graph modes, and upstream merge ancestry controls.
2. Actual registered HTTP reads preserve selected/aggregate repository identity
   and opposite pushed values for shared SHAs, all existing fields, and stable
   refs/HEAD/index/status. Unavailable evidence stays false and usable.
3. Enrichment uses the same bounded three-command maximum, captured returned tip,
   correct graph/range context, existing execution seam and unchanged consumers.

## Implementation sequence

1. Await a later explicit parent interrupt release, then mark this order and the
   plan in progress. Remain in this session/profile; no delegation. Read `/tdd`
   and its backend fixture reference before creating permanent tests.
2. Seed an owned deterministic local/bare fixture and immediately register cleanup.
   Pin base and merge dates too. The existing process `runGit` and API `runGitAPI`
   strip all `GIT_*`; a small test-local date command must append explicit author
   and committer dates after filtering, disable signing, and operate only in its
   temporary repository. Avoid `t.Setenv` date assumptions, tied timestamps,
   wall-clock ordering, and parallel environment-sensitive fixtures. Prefer
   `t.TempDir` so setup failures cannot leak manually allocated repositories.
3. Add `TestGetLog_PushedFirstParentMerge` first, run only it, and record expected
   RED. Its two rows must both be false before push, and both true afterward.
   Assert exact SHA membership/order, first parent, and existing inline statistics.
4. Apply the narrow helper/signature correction described by the paired design.
   Keep log selection/parsing and API routing source untouched.
5. Add remaining controls in small sibling files. For failure, call the actual
   helper with initially false rows and a nonexistent base after a valid local
   upstream resolves; actual rev-list must fail conservatively. A cancelled
   context also stays false. Do not mock the command runner/coordinator.
6. For HTTP, use two disposable clones with identical graph SHAs, distinct bare
   origins and upstream tips: one stays at base, the other publishes the graph.
   Configure existing per-repo bases, then construct the real manager/server
   after fixture/environment setup and register manager teardown immediately.
   Both selected queries and aggregate `GET /api/v1/git/log` must prove each
   repository's own value. Use the registered router and decoded producer output.
7. Run the affected checks below serially, record results, and synchronize package
   status. Proceed to authorized normal delivery only after these checks pass.

## Verification

From the repository root, initial permanent RED (also rerun this exact case for GREEN):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB mise exec -- go test -trimpath -p 1 ./internal/agentctl/server/process -run '^TestGetLog_PushedFirstParentMerge$' -count=1)
```

After correction and all cases, affected checks, one at a time:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB mise exec -- go test -trimpath -p 1 -race ./internal/agentctl/server/process -run '^(TestGetLog_.*|TestMarkPushedCommits_.*)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB mise exec -- go test -trimpath -p 1 -race ./internal/agentctl/server/api -run '^(TestHandleGitLog_.*|TestComputeMergeBase_.*|TestGitReviewEndpointsCorrectStaleBase|TestMultiRepoReviewEndpointsUseStoredBaseBranches|TestMultiRepoReviewEndpointsCorrectStaleBases)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB mise exec -- golangci-lint run ./internal/agentctl/server/process/... ./internal/agentctl/server/api/... --new-from-rev=b330ad97a8712eb7b1a8ee2863b46ba302a92acb --concurrency=2 --allow-serial-runners --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If PR remediation changes backend code, scoped backend guidance additionally
requires the following named full lint once per correction before fixup push,
using the actual PR base SHA, not moving main; this is not a broad test audit:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB mise exec -- golangci-lint run ./... --new-from-rev='<actual-pr-base-sha>' --concurrency=2 --allow-serial-runners --timeout=5m)
```

Use normal pre-commit and commit-msg hooks. Before hooks/package commands in a
fresh worktree, install once via `(cd apps && mise exec -- pnpm install --frozen-lockfile)`;
do not change the lockfile. No Vitest run is needed for unchanged consumers.
If a necessary pure consumer test is added, update its scope and run only the
named Changes test files with `--maxWorkers=1` after that installation. No new
Playwright, browser build, or UI preview is required for this source-data repair.

Retain every returned handle and join it before continuing/replacing a command.
Interrupted/lost commands have no verdict until reconciled. Never delete parent
proof archives, kill foreign processes, or clear shared caches.

## Files likely touched and ownership

- `apps/backend/internal/agentctl/server/process/git_log.go`
- New `apps/backend/internal/agentctl/server/process/git_log_pushed_test.go`
- New `apps/backend/internal/agentctl/server/api/git_log_pushed_test.go`
- This plan/work order, paired Platform requirement/design, and Platform README.

Existing `api/git.go`, `process/git_log_test.go`, and Changes consumer tests are
context and controls; changes there require a concrete necessity. Preserve
foreign edits in the shared machine/codebase.

## Dependencies and parallelism

None. `sequential`. No workers, native agents, persistent tasks, or extra sessions.

## Risks and inputs

Use the paired design's traversal proof and failure/consumer boundaries, the
plan's retained evidence pointers, scoped backend/agentctl/API guidance, and
the backend fixture reference. Negative ancestry and no-base topology are the
material regression risks. Pin the returned tip without adding a ref-read.

Public docs assessment: internal docs only; intended user guidance is unchanged.
Mobile assessment: shared data only; no layout/touch/navigation changes. Preserve
provider overlays, merge-detail/navigation contracts, and all JSON fields.

## Results

The parent released implementation in a later explicit continuation after
reviewing all five package files. Permanent focused RED failed on the expected
older local commit's falsely true pushed flag (exit 1, 0.147s); membership,
parents, stats, and the real-push positive control were intact. Focused GREEN
passed after correction (exit 0, 0.160s).

The exact process race command above passed (exit 0, 2.321s) and the exact API
race command passed (exit 0, 2.489s). All handles joined and owned fixtures
cleaned up. The exact scoped golangci-lint command passed (exit 0, zero issues).
Catalog validation, all specification lint, whitespace checks, and local
actual-file PR coverage preflight passed. The single frozen pnpm install passed
without lockfile changes. Normal hooks and publication are pending externally.
Task completion remains gated on actual hosted review/checks and
independently verified merge. Parent owns archival and retained proof archives.
