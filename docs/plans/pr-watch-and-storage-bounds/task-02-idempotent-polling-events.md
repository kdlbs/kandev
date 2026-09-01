---
id: "02-idempotent-polling-events"
title: "Reconcile and publish canonical PR state"
status: pending
wave: 1
depends_on: ["01-canonical-watch-migration"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 02: Reconcile and publish canonical PR state

## Intent

Stop session-driven branch oscillation and duplicate GitHub/event work after canonical identity is available.

## Acceptance

- One branch result per task/repository drives one atomic searching-watch transition; unchanged reconciliation has no inserts or updates.
- Multiple sessions on one task/branch result in one lookup per cycle; multi-repository and multi-branch behavior remains correct.
- `GitHubPRFeedback` and `GitHubTaskPRUpdated` publish only for durable relevant state changes, coalesced by task/repository/PR/head SHA/status.

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_test.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_pr_watch_test.go`
- `apps/backend/internal/github/service_pr_watch_multi_branch_test.go`
- `apps/backend/internal/github/service_pr_sync_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential. It consumes the task-owned store contract and event semantics used by projection.

## Verification

```bash
cd apps/backend && go test ./internal/github -run 'Test.*(Poller|Reconcile|Branch|PRWatch|Sync|Feedback|MultiRepo).*' -count=1 -v
cd apps/backend && go test ./internal/github -count=1
git diff --check
```

## Output contract

Record red/green call and event counts for repeated resume, branch switch, unchanged cycles, and multi-repository cases. Update task and plan status.

## Results

Pending.

