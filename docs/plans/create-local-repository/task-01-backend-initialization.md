---
id: "01-backend-initialization"
title: "Backend local repository initialization"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/create-local-repository/spec.md"
---

# Task 01: Backend Local Repository Initialization

## Acceptance

- The workspace-scoped endpoint creates an absent `<parent>/<name>` repository with unborn branch
  `main`, no files, and no commits, then returns its persisted repository DTO.
- Invalid names/parents, unknown workspaces, and existing targets fail with the specified status and
  do not mutate the target or persist a repository.
- Git or persistence failure leaves no repository row and applies the request-owned cleanup contract.

## Verification

```bash
make -C apps/backend fmt
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers)
make -C apps/backend lint
```

## Files Likely Touched

- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/local_repository_initialization.go`
- `apps/backend/internal/task/service/local_repository_initialization_test.go`
- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/backend/internal/task/handlers/repository_handlers_test.go`

## Dependencies

None.

## Inputs

- Spec: What, API Surface, Permissions, Failure Modes, and Persistence Guarantees.
- Existing exact-path validation: `apps/backend/internal/task/service/repository_discovery.go`.
- Existing persistence/event path: `Service.CreateRepository` in
  `apps/backend/internal/task/service/service_resources.go`.
- Security boundary: `docs/decisions/2026-07-20-explicit-local-repository-trust.md`.

## Output Contract

Report endpoint/service behavior, rollback strategy, tests run, files touched, blockers, and residual
filesystem race risks. Mark this file `done` and update the plan entry only after targeted tests pass.
