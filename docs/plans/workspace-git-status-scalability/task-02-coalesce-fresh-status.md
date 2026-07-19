---
id: "02-coalesce-fresh-status"
title: "Coalesce live workspace status observations"
status: done
wave: 2
depends_on:
  - "01-linear-diff-enrichment"
plan: "plan.md"
spec: "../../specs/platform/workspace-git-status.md"
---

# Task 02: Coalesce live workspace status observations

## Acceptance

- Fresh reads, empty-cache fallback, polling, and explicit refresh share at most one underlying observation per workspace tracker.
- Different workspace trackers may observe repositories in parallel.
- Each caller can return on its own context cancellation without cancelling or poisoning other waiters.
- The shared observation is rooted in tracker lifetime, has an independent bounded deadline, and stops when the tracker stops or the deadline expires.
- Cancelled shared work returns no partial success and does not update cached status.
- Fresh reads remain non-cache-owning; existing poll and explicit-refresh overlap policy remains intact.

## Verification

```bash
cd apps/backend
rtk go test ./internal/agentctl/server/process -run 'Test(GetGitStatus|WorkspaceTracker.*Concurrent|WorkspaceTracker.*Cancel)'
rtk go test -race ./internal/agentctl/server/process
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_concurrency_test.go`

## Dependencies

- `01-linear-diff-enrichment`

## Inputs

- Existing `updateMu` polling and explicit-refresh policy in `workspace_tracker.go`.
- Existing detached `singleflight.DoChan` cancellation pattern in `apps/backend/internal/agent/handlers/git_handlers.go`.
- Existing fresh-cache regression and workspace-overlap tests.

## Output contract

Report the shared-context lifetime and deadline, all observation entry points routed through the flight, cache-write ownership, deterministic concurrency test results, changed files, and remaining risks. Do not make the first caller's context own shared work.
