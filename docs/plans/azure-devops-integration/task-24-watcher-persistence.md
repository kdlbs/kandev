---
id: "24-watcher-persistence"
title: "Azure watcher persistence"
status: completed
wave: 12
depends_on: ["22-task-work-item-links"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 24: Azure Watcher Persistence

## Acceptance

- Separate work-item and PR watcher stores implement workspace-scoped CRUD,
  enabled/due listing, error timestamps, deleting state, reset preview/reset,
  and cleanup-policy deletion. Common fields include workflow/step, Kandev
  repository/base branch, agent/executor profiles, prompt, poll interval,
  cleanup policy, optional in-flight cap, generation, last poll/error, and
  timestamps.
- Work-item filters persist Azure project ID and WIQL. PR filters persist Azure
  project ID, optional `azure_repository_id`, status, creator, and reviewer;
  the Azure filter is never confused with the Kandev `repository_id`.
- Reservation uniqueness is `(watch, generation, project, work item)` or
  `(watch, generation, project, Azure repository, PR)`. Reserve, attach, and
  release all require the caller's generation; ownership loss is a terminal
  result, not a retry against the new generation.
- Controllers require workspace authorization for list/create and authorize the
  watch's owner before ID-based mutations without leaking another workspace's
  existence.
- Poll interval defaults to 300 seconds and clamps values below 60 seconds.
  `max_inflight_tasks` uses omitted/zero/positive semantics shared by other
  watches.
- `GET /:id/reset/preview` returns only tasks owned by the current watch
  generation. Reset increments generation before shared cleanup and removes
  prior reservations afterward. Delete marks the watch deleting and disabled
  before cleanup. Workspace deletion removes watches and reservations.

## TDD Sequence

1. Add migration/store tests for both watch kinds, restart persistence,
   reservation uniqueness, generation ownership loss, and workspace cleanup;
   run them red before adding schema/store methods.
2. Add service/controller tests for validation, cross-workspace ID access,
   error recording, reset preview, cleanup policies, and deleting state; run
   them red before adding routes.
3. Implement schema, stores, reset service, and controllers. Refactor shared
   watch-kind helpers only after both suites are green.

## Verification

- `go test ./internal/azuredevops ./internal/orchestrator` from `apps/backend` — passed (1,455 tests).
- Coverage includes workspace isolation, restart persistence, generation-aware reservations, reset preview/reset, deleting state, and workspace cleanup.

## Files Likely Touched

- `apps/backend/internal/azuredevops/watch_models.go`
- `apps/backend/internal/azuredevops/store_watches.go`
- `apps/backend/internal/azuredevops/store_watches_test.go`
- `apps/backend/internal/azuredevops/store_watch_reset.go`
- `apps/backend/internal/azuredevops/store_watch_reset_test.go`
- `apps/backend/internal/azuredevops/service_watches.go`
- `apps/backend/internal/azuredevops/service_watches_test.go`
- `apps/backend/internal/azuredevops/service_watch_reset.go`
- `apps/backend/internal/azuredevops/service_watch_reset_test.go`
- `apps/backend/internal/azuredevops/controller_watches.go`
- `apps/backend/internal/azuredevops/controller_watch_reset.go`
- `apps/backend/internal/azuredevops/controller_test.go`
- `apps/backend/internal/azuredevops/lifecycle.go`

## Dependencies

Task 22.

## Parallelism

Sequential. Both watcher kinds share schema, reset, cleanup, and authorization.

## Inputs

- Spec: Azure watches data model/API/state guarantees.
- GitLab watch models/store/reset as the closest two-kind provider pattern.
- `internal/watchreset` shared cleanup helper.

## Risks

- Generation must be included in every reservation write/update condition so a
  reset cannot let an old dispatch attach a task to new ownership.
- ID-addressed routes must not trust a caller-supplied workspace ID before
  loading and authorizing the watch's persisted owner.

## Output Contract

Report schema and state transitions, RED/GREEN commands, files changed,
authorization evidence, risks, and update task/plan status.
