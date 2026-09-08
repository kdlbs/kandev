---
id: "02-preflight-task-repository-selections"
title: "Preflight task repository selections"
status: done
wave: 2
depends_on:
  - "01-add-server-repository-inspection"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.1
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.3
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.5
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.6
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.7
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.8
system_design:
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 02: Preflight Task Repository Selections

## Summary

Wire plugin inspection into task creation. Resolve every first-use plugin
selection before persistence, ignore mutable browser descriptor values, and
map typed failures to safe transport responses.

## In scope

- Start with failing task-service tests that prove the current code inserts a
  task before plugin URL resolution fails.
- Add a narrow `RepositorySelectionResolver` interface and optional service
  setter for focused tests.
- Wire a production adapter from the task service to the plugin service.
- Preserve workspace authorization and external-ID deduplication before any
  plugin action.
- Resolve all eligible plugin selections into an in-memory trusted list before
  `createTaskWithCapacity` or repository persistence.
- Ignore request host, owner, name, clone URL, and default branch when applying
  the authoritative plugin result. Check immutable pinning hints when present.
- Reuse `validateTrustedRemoteRepository`, canonical repository identity, and
  the existing trusted persistence path.
- Add typed task-service errors and stable error codes for invalid, not-found,
  and unavailable resolution outcomes.
- Map the errors in REST and WebSocket handlers. Confirm the existing frontend
  submit path keeps the dialog open and displays the safe message.

## Out of scope

- Refactoring all task-create side effects into one database transaction.
- Changing task-create payload fields.
- Adding a new dialog control or translation key unless testing proves the
  existing failed-create message cannot express the error.
- Resolving already-persisted repository IDs through a plugin.

## Acceptance

- A first-use plugin repository is persisted and attached through the existing
  canonical trusted-descriptor path.
- Tampered request descriptors cannot change the provider host, clone URL,
  owner, name, default branch, or credential destination.
- One failed plugin selection in a multi-repository request produces no task,
  repository, task-repository, event, or last-used write.
- An already-settled external-ID retry returns its task without an inspect call.
- Built-in URLs, repository IDs, and authenticated Host task creation keep
  their existing behavior.
- Client errors are bounded and classified without raw plugin response text.

## Verification

Create write-spy regressions first and confirm the failure occurs after a task
insert on the old path. Then run:

```bash
# From apps/backend:
rtk go test ./internal/task/service -run 'CreateTask.*Plugin|RepositorySelection|TrustedRemote' -race
rtk go test ./internal/task/handlers -run 'CreateTask.*Repository|TaskCreate.*Error' -race
rtk go test ./internal/backendapp -run 'Plugin.*Repository|CodeHost' -race
```

## Files likely touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_tasks_test.go`
- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/errors_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers_test.go`
- `apps/backend/internal/task/handlers/task_ws_handlers_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/plugins_test.go`

## Dependencies

- Task 01 supplies the normalized inspection result and typed plugin errors.

## Risks

- Preflight before external-ID lookup would break idempotent retries.
- Preflight after task insertion would preserve the reported bug.
- Parent-repository inheritance and Office identifier allocation share the
  current preparation function. Keep provider preflight before task insertion
  and pin any unavoidable non-row ordering with tests.
- A nil resolver in focused tests must fail closed only for plugin selections,
  while unrelated creation tests remain isolated.

## Parallelism

`sequential`

## Inputs

- Task 01 normalized descriptor and error contract.
- Existing `TrustedProviderDescriptor` and Host `Tasks.Create` adapter.
- External-ID idempotency create sequence.
- `ADR-2026-08-26-server-owned-plugin-repository-task-resolution`.

## Results

Implemented the task-service resolver seam and backend adapter. First-use
plugin selections are resolved after authorization, parent inheritance, and
external-ID deduplication, then validated in memory before task or repository
writes. Browser identity fields are discarded, trusted Host task creation is
preserved, and invalid, not-found, and unavailable failures map to bounded
REST and WebSocket responses.

Verification passed:

```text
go test ./internal/task/service -run 'CreateTask.*Plugin|RepositorySelection|TrustedRemote' -race
go test ./internal/task/handlers -run 'CreateTask.*Repository|TaskCreate.*Error' -race
go test ./internal/backendapp -run 'Plugin.*Repository|CodeHost' -race
```
