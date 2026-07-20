---
id: "01-explicit-path-contract"
title: "Explicit local path contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/local-repositories.md"
---

# Task 01: Explicit Local Path Contract

## Acceptance

- Manual validation accepts a real Git repository outside configured discovery roots while
  automatic discovery remains bounded by those roots.
- Create and update canonicalize and validate non-empty local paths server-side and return 4xx for
  invalid paths without persisting changes.
- Pathless provider repositories retain their existing behavior.

## Verification

From `apps/backend`:

```bash
rtk go test -v ./internal/task/service -run 'Test(ValidateLocalRepositoryPath|CreateRepository|UpdateRepository|DiscoverLocalRepositories)'
rtk go test -race ./internal/task/service
```

## Files Likely Touched

- `apps/backend/internal/task/service/repository_discovery.go`
- `apps/backend/internal/task/service/repository_discovery_test.go`
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/service_test.go`
- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/backend/internal/task/handlers/repository_handlers_test.go`
- `apps/backend/internal/task/dto/dto.go`

## Dependencies

None.

## Inputs

- Spec: `What`, `API Surface`, `Failure Modes`, and the manual-selection scenarios.
- ADR: exact saved repository path is the durable grant; discovery roots govern scans only.
- Existing path canonicalization and Git metadata handling in `repository_discovery.go`.

## Output Contract

Report changed behavior, files, red/green test evidence, exact commands run, blockers, linked-worktree
risks, and update this task plus `plan.md` to done.
