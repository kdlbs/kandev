---
id: "01-exact-dirty-path-refresh"
title: "Restore exact dirty-path refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.42
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.14
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.15
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
system_design:
  - ../../specs/platform/system-design/workspace-dirty-path-monitor.md
---

# Task 01: Restore exact dirty-path refresh

## Summary

Detect repeated edits to every supported dirty tracked filename using raw
NUL-framed Git paths and the exact file's mtime. Prove the production tick
refreshes cached file data, attempts status refresh, and emits the existing
repository-qualified file refresh, while unchanged ticks remain quiet.

## In scope

1. Mark this order in progress after the later explicit parent interrupt.
   Add meaningful real-Git tests in `workspace_monitor_dirty_paths_test.go`
   before changing production code. Record RED for
   `TestWorkspaceStateDirtyExactPaths` and `TestMonitorTickDirtyExactPaths`.
   Do not restore/replay the parent's temporary probe merely for acknowledgement.
2. Commit tracked baselines before two dirty writes, with fixed mtimes ten seconds
   apart and unchanged index mtime. Cover ordinary control, leading/trailing
   space, tab, newline, double quote and Unicode under default/true/false
   quotePath. Use individually justified native shape skips only. Seed similar
   real paths with distinct sentinel mtimes: changing a decoy without changing
   the monitored path must not supply the monitored evidence. Preserve exact
   identities across mixed paths, including newline-like/quoted representations.
3. Capture direct `monitorTick` events with a buffered subscriber. Cover initial
   dirty, second edit, and settled no-op; assert accepted basic status identity
   and updated cached file-list timestamp/membership plus the `refresh` operation, empty path,
   and repository name. Use existing detail completion/barrier seams for no-op
   silence; `Stop` joins all enrichment. Do not start a monitor/app/DB/browser.
4. Cover dirty tracked deletion with missing-file stat evidence, staged rename
   followed by two edits at its destination, and a tracked `node_modules` path.
   Retain deletion/rename metadata and stable controls; do not change parsers
   for full status/renames. Prove unsafe traversal remains rejected via the
   existing containment boundary rather than inventing path normalization.
5. Change only the tracked monitor query/framing/fingerprint in
   `workspace_monitor.go`. Preserve exact bytes through stat and use unambiguous
   NUL record separators; retain paths when stat fails. Preserve polling,
   failures, index/untracked behavior, environment and optional locks.
6. Run the exact focused checks sequentially, record actual results, synchronize
   this order/plan, then deliver through active hooks, ready PR, hosted gates,
   substantive authenticated current-head FULL semantic review of every changed
   file, actionable dispositions, expected-head normal squash merge, independent
   merged-SHA read and joined owned cleanup.

## Out of scope

UI/payload/copy changes, hashes/content scanning, new frameworks or production
test seams, untracked fingerprint refactor, unrelated Git parser/mutation work,
broad suites/audits, sleeps/slow polling loops, extra workers/tasks/sessions,
optional polish, and rebasing solely for advancing main.

## Acceptance

- Tests fail before correction for the supplied path causes, and pass after the
  smallest repair without relaxed assertions. No-op and exact-decoy controls
  would reject a stat of the wrong path or an index-only change.
- Actual controlled ticks publish refreshed status/file evidence and exact
  repository-scoped refresh payloads. Existing limits and containment hold.
- All listed relevant checks and normal hosted gates pass; completion requires
  actual merge SHA and owned cleanup. Plan handoff alone is not task completion.

## Verification

From repository root, run one heavy command at a time and retain/join every
handle. The first command is RED before the production edit. GREEN uses the
second command once for the final source; rerun only after affected changes.

```bash
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -p 2 ./internal/agentctl/server/process -run '^Test(WorkspaceStateDirtyExactPaths|MonitorTickDirtyExactPaths)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -trimpath -race -p 2 ./internal/agentctl/server/process -run '^Test(WorkspaceStateDirtyExactPaths|MonitorTickDirtyExactPaths|WorkspaceStateDirtyPathTransitions|GetUntrackedFilesID_ExcludesNodeModules|ParseGitUntrackedOutput|GetGitStatus_ExcludesUntrackedNodeModules|MonitorLoop_OverlapGuard_SkipsTick|RunGitOutput_PollingVariantReleasesThrottleOnTimeout|WorkspaceGitAdmissionWaitDoesNotConsumeTimeout|WorkspaceGitEnvironmentSnapshotsAreDetached|GitPollSnapshot_DetectsModifiedFiles)$' -count=1)
(cd apps/backend && GOMAXPROCS=2 golangci-lint run --concurrency=2 --new-from-rev=9fffff9c4ed1aea5d0a1189b0e1877146f7aacfd --timeout=5m ./internal/agentctl/server/process)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/workspace-dirty-path-monitor
```

Run `.github/scripts/pr-docs.cjs`'s exported `validateCoverage` locally with the
actual changed paths and complete referenced file contents; do not publish a
synthetic hosted status. At design checkpoint use the planned source/test paths
as an explicitly simulated coverage input, since production is still untouched.
Required hosted CI remains authoritative; promptly diagnose any failed leaf.
An actionable PR lint finding requires the scoped AGENTS full new-revision lint
command with concurrency two, rather than weakening the gate.

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_monitor.go`
- `apps/backend/internal/agentctl/server/process/workspace_monitor_dirty_paths_test.go`
- `docs/specs/platform/requirements/workspace-git-status.md`
- `docs/specs/platform/system-design/workspace-git-status.md`
- `docs/specs/platform/system-design/workspace-dirty-path-monitor.md`
- `docs/plans/workspace-dirty-path-monitor/plan.md`
- `docs/plans/workspace-dirty-path-monitor/task-01-exact-dirty-path-refresh.md`

## Dependencies

None. Existing merged exact-path/count/literal/metadata repairs are baseline.
No companion package requires reopening or changing its completed results.

## Risks

Use existing test Git environment isolation, literal-safe fixture setup, and
immediate cleanup registration. Native unsupported names, asynchronous
enrichment noise and index/decoy mtime drift require explicit controls. Full
subscriber queues and timestamp-preserving writes retain current limitations.

## Parallelism

`sequential`; no delegation is authorized.

## Inputs

- [Workspace Git Status requirement](../../specs/platform/requirements/workspace-git-status.md): `.42`, `.13` through `.15`, `.31`.
- [Monitor supplement](../../specs/platform/system-design/workspace-dirty-path-monitor.md).
- `workspace_monitor.go`, `workspace_monitor_test.go`, `workspace_overlap_test.go`,
  `workspace_git_status_progressive_test.go`, `workspace_stream.go`,
  `workspace_git_poll.go`, `workspace_git_untracked.go`, and tracker fixture helpers.
- Parent's joined proof/source archive and clean baseline recorded in `plan.md`.
- Backend/agentctl AGENTS and `/tdd` backend test reference.

## Results

Implementation verified on 2026-10-02 after the explicit parent continuation.
The named RED command failed as expected (joined exit 1, package 5.023s),
including actual missing tick refresh/status publication and stale cache/diffs.
The named focused GREEN race command passed (package 11.523s).
The exact scoped lint command passed with zero issues. Catalog validation
(343 decisions/1309 specs), full spec lint, actual changed-path PR-documentation
coverage (`covered`, no errors), `git diff --check` and package status checks
passed. All owned handles joined; no running app or polling loop was used.

The minimum production repair adds `-z`, splits NUL records and frames exact
path/mtime evidence with NUL. The cached-list assertion uses capture timestamp
and tracked membership, matching its existing producer contract; published
status proves second-edit content. No viewport or public documentation changed.

Normal hooked commit, ready PR, hosted CI/review and actual merge/cleanup remain
task-level delivery gates tracked in the persistent task plan. `done` records
completed implementation and local verification, not an assertion of merge.
