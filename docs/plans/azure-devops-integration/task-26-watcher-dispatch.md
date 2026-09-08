---
id: "26-watcher-dispatch"
title: "Azure watcher dispatch"
status: completed
wave: 12
depends_on: ["25-watcher-polling"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 26: Azure Watcher Dispatch

## Acceptance

- Provider-specific work-item and PR watcher sources build Kandev task requests
  with Azure URL/identity metadata, repository/base branch, workflow context,
  selected profiles, and interpolated prompt.
- Work-item tasks write `azure_devops_work_item_watch_id`,
  `azure_devops_project_id`, `azure_devops_work_item_id`, and
  `azure_devops_work_item_url`. PR tasks write
  `azure_devops_pr_watch_id`, `azure_devops_project_id`,
  `azure_devops_repository_id`, `azure_devops_pull_request_id`, and
  `azure_devops_pull_request_url`. Each source's `WatchMetadataKey` returns its
  exact watch ID key.
- Shared dispatch reserves before creation, enforces `max_inflight_tasks`,
  attaches task ID generation-safely, releases retryable failures, and
  treats generation ownership loss as terminal.
- Missing workflow, workflow step, Kandev repository, agent profile, or
  executor profile self-disables the owning watch with the dependency error.
  Other retryable task-creation failures release the reservation for the same
  generation.
- Backend startup registers the sources, event handlers, poller lifecycle, task
  dependency checks, workspace cleanup, and watcher metadata keys used by
  open-task counting. Azure initialization remains non-fatal.

## TDD Sequence

1. Add source unit tests for request mapping, prompt interpolation, exact
   metadata, reserve/release/attach calls, max in-flight value, and terminal
   ownership loss; run red before implementing sources.
2. Add event-handler/shared-dispatch tests for both event kinds, throttle
   counting, retry release, and every self-heal dependency.
3. Add backend wiring/lifecycle tests, then register sources, handlers, poller,
   and cleanup. Refactor provider-common mapping only after focused tests pass.

## Verification

- `go test ./internal/azuredevops ./internal/orchestrator` from `apps/backend` — passed (1,455 tests).
- Coverage includes exact Azure metadata keys, prompt interpolation, throttling, reservation attach/release, dependency self-healing, and startup wiring.

## Files Likely Touched

- `apps/backend/internal/orchestrator/source_azuredevops.go`
- `apps/backend/internal/orchestrator/source_azuredevops_test.go`
- `apps/backend/internal/orchestrator/event_handlers_azuredevops.go`
- `apps/backend/internal/orchestrator/event_handlers_azuredevops_test.go`
- `apps/backend/internal/orchestrator/watcher_dispatch_wiring.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/task/repository/count_open_watcher_tasks_test.go`
- `apps/backend/internal/events/types.go`

## Dependencies

Task 25.

## Parallelism

Sequential. Registration and shared dispatch wiring are package-wide seams.

## Inputs

- Spec: watcher task-creation, dedup, self-heal, and reset scenarios.
- `source_gitlab.go` for separate review/issue sources.
- Shared `watcher_dispatch.go` and throttle contract.

## Risks

- The metadata key written by each source must exactly match
  `WatchMetadataKey`; otherwise the configured in-flight cap silently does
  nothing.
- The provider's Azure repository filter is event metadata, while the Kandev
  repository ID selects task materialization; never substitute one for the
  other.

## Output Contract

Report dispatch lifecycle, metadata keys, RED/GREEN commands, files changed,
self-heal evidence, risks, and update task/plan status.
