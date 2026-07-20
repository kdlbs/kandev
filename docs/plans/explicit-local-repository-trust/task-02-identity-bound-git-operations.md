---
id: "02-identity-bound-git-operations"
title: "Identity-bound Git operations"
status: done
wave: 2
depends_on: ["01-explicit-path-contract"]
plan: "plan.md"
spec: "../../specs/workspaces/local-repositories.md"
---

# Task 02: Identity-Bound Git Operations

## Acceptance

- Saved repositories outside discovery roots support branch listing and refresh through repository
  ID.
- Fresh-branch mutation resolves the repository persisted for the task before executing Git and
  preserves dirty-file consent and compensation behavior.
- Raw paths remain limited to read-only validation, branch, and status probes.

## Verification

From `apps/backend`:

```bash
rtk go test -v ./internal/task/service -run 'Test(RefreshRepositoryBranches|PerformFreshBranch|ListBranches)'
rtk go test -v ./internal/task/handlers -run 'Test.*FreshBranch|Test.*RepositoryBranches'
rtk go test -race ./internal/task/service ./internal/task/handlers
```

## Files Likely Touched

- `apps/backend/internal/task/service/repository_discovery.go`
- `apps/backend/internal/task/service/repository_fetch.go`
- `apps/backend/internal/task/service/fresh_branch.go`
- `apps/backend/internal/task/service/fresh_branch_test.go`
- `apps/backend/internal/task/service/repository_discovery_test.go`
- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`

## Dependencies

- Task 01 provides the canonical explicit-path validation boundary.

## Inputs

- Spec: identity-bound refresh and destructive-operation scenarios.
- Existing post-create fresh-branch compensation in `task_http_handlers.go`.
- Existing repository and task-repository persistence interfaces.

## Output Contract

Report identity resolution changes, preservation of destructive-operation safeguards, red/green test
evidence, files, commands, blockers, and update this task plus `plan.md` to done.
