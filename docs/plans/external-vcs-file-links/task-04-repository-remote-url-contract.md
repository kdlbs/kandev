---
id: "04-repository-remote-url-contract"
title: "Repository remote URL HTTP contract"
status: superseded
wave: 3
depends_on: ["01-link-foundation", "02-toolbar-wiring"]
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 04: Repository remote URL HTTP contract

> Superseded by task 05 after security review found that a generic write-through would create an unsafe clone-target input. Only read-only DTO exposure is retained.

## Acceptance

- Repository create and update HTTP requests accept `remote_url` and pass it through the existing service request boundary.
- Repository HTTP and boot/list responses include the persisted `remote_url` through the shared repository DTO.
- Regression tests fail before the contract fix and pass afterward for create, update, and DTO serialization.
- Existing repository validation and persistence behavior is unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/task/handlers ./internal/task/dto ./internal/task/service
```

## Files likely touched

- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/backend/internal/task/handlers/repository_handlers_test.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/dto_test.go` if a focused DTO test is needed
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_resources.go`
- Existing focused service tests only if required for update pass-through coverage.

## Dependencies

- Browser RED evidence from task 03 proves the production payload omission.

## Output contract

Report the failing regression test, minimal fix, exact Go test results, changed files, and any similar HTTP/WS contract gaps discovered. Update only this task file's `status` to `in_progress` at start and `done` after targeted verification passes; do not edit `plan.md`.
