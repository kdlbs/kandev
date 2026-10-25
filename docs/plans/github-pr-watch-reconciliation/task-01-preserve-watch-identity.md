---
id: "01-preserve-watch-identity"
title: "Preserve watch branch identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.1
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.2
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.3
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.4
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 01: Preserve watch branch identity

## Summary

Make refresh resolve the same per-session repository/branch targets as creation.
Prove repeated reconciliation preserves sibling targets and subsequent discovery.

## In scope

- Update the provider interface, orchestrator resolver, poller call, and mocks.
- Add the resolver and repeated-cycle regressions named in the plan.
- Prove secondary-branch discovery persists and publishes the owning task PR.

## Out of scope

Live-instance changes, new schema, frontend changes, GitLab, rate-limit policy,
bulk cleanup, and changes to workspace-group association ownership.

## Acceptance

1. Three unchanged reconciliation cycles preserve watch IDs and repository/branch
   targets without recreation, including redirected owners and multiple branches
   on the same repository. At least one searching and one numbered watch coexist.
2. Exact matches survive; unique same-repository renames update; missing/error
   and ambiguous target resolution leaves state intact. No primary-repo fallback.
3. An eligible PR on a secondary branch is linked after reconciliation with
   correct repository identity and task event; numbered links and unlink
   tombstones remain protected by existing writers and regression coverage.

## Verification

Start with a failing regression using the existing production contract before
changing it. Record the expected wrong-branch or replaced-watch failure, then
implement and run these commands from the repository root:

```bash
(cd apps/backend && go test ./internal/github ./internal/orchestrator -run 'Test(ReconcileWatches_PreservesBranchTargetsAcrossCycles|ResolvePRWatchBranchForWatch)' -count=1)
(cd apps/backend && go test ./internal/github -count=1)
(cd apps/backend && go test ./internal/orchestrator -count=1 -timeout=30m)
(cd apps/backend && go test -race ./internal/github -run 'Test(ReconcileWatches|RefreshStaleBranches)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

On the development host, the combined package run passed GitHub but exhausted
Go's default ten-minute timeout while the orchestrator suite was closing a
SQLite fixture. The final retry uses an owned temporary directory on the existing
`/dev/shm` tmpfs via both `TMPDIR` and `GOTMPDIR`. Go 1.26's `t.TempDir()`
prefers `GOTMPDIR`; an intermediate retry that left it at `/tmp` was stopped
before correcting that setting. The owned directory is removed after the run. No mount, live database, or instance
setting changes are involved. Use normal temporary storage on hosts without
this disk-I/O constraint.

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_test.go`
- `apps/backend/internal/github/poller_branch_reconciliation_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_watch_reconciliation.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_github_watch_reconciliation_test.go` (new)
- Other compile-time `TaskBranchProvider` test doubles found by symbol search.

## Dependencies

None. Implementation was explicitly authorized after the design-package handoff.

## Risks

See the plan. Reuse source-session target resolution and existing storage guards;
do not infer a rename from an arbitrary sibling or hide read errors as success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-discovery-health.md)
- [Design](../../specs/integrations/system-design/github-pr-discovery-health.md), watch target reconciliation.
- Existing `TestRefreshStaleBranches_*`, `TestListTasksNeedingPRWatch`, and
  `TestEnsureSessionPRWatchRedirectsToWorkspaceGroupOwner` fixtures.
- Plan evidence and backend `AGENTS.md`.

## Results

Completed on 2026-09-13.

- Red: the original resolver selected `primary` for a secondary-repository watch;
  repeated reconciliation grew three watches to four. Creation/refresh coverage
  also reproduced a primary repository receiving the secondary branch.
- Focused `go test ./internal/github ./internal/orchestrator -run
  'Test(ReconcileWatches_PreservesBranchTargetsAcrossCycles|ResolvePRWatchBranchForWatch)'
  -count=1`: passed both packages (GitHub 0.031s; orchestrator 79.458s).
- Full GitHub suite: passed (284.480s), as the GitHub component of the original
  combined package command.
- Full orchestrator suite with `-count=1 -timeout=30m`: passed (202.462s) using
  owned tmpfs scratch after the infrastructure timeout described above.
- `go test -race ./internal/github -run
  'Test(ReconcileWatches|RefreshStaleBranches)' -count=1`: passed (1.293s),
  using owned tmpfs scratch.
- `python3 scripts/list-docs.py validate`: passed, 267 decisions and 868 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

The source session and per-repository targets now govern both refresh and
creation. Existing numbered-watch and unlink guards remain unchanged. New
backend integration coverage proves secondary-PR persistence and task events.
Public docs need no change because the repair restores existing behavior.
No application restart, live database repair, deployment, or delegation occurred.
Owned tmpfs scratch directories were removed after their test runs.
