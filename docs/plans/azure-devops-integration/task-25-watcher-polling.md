---
id: "25-watcher-polling"
title: "Azure watcher polling"
status: completed
wave: 12
depends_on: ["24-watcher-persistence"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 25: Azure Watcher Polling

## Acceptance

- One Azure watcher poller schedules due enabled work-item and PR watches,
  executes provider-native WIQL/PR filters, and publishes normalized events
  only after a current-generation reservation succeeds.
- Create, Run now, and scheduled polls use the same check functions. Each check
  is deterministically bounded to 100 matches; work-item hydration honors the
  Azure 200-ID batch limit and PR status defaults to active.
- Provider events contain watch ID/generation, workspace/workflow context,
  Kandev repository/base branch, profiles, prompt, cleanup/in-flight settings,
  provider identity, title, URL, and the fields needed for prompt
  interpolation. They never contain the PAT.
- Authentication, malformed WIQL/filter, timeout, rate-limit, and provider
  failures record `last_error`/`last_error_at`, update the poll timestamp, and
  publish no task event. A later successful check clears the error.
- Polling reconciles terminal state only for current-generation reservations
  that own an auto-created Kandev task. `auto`, `always`, and `never` cleanup
  cannot delete manual tasks or tasks owned by another watch/generation.
- Poller start/stop is context-aware and isolated from Azure connection-health
  polling; one failing watch does not delay another due watch.

## TDD Sequence

1. Add client/service tests for bounded WIQL and PR matching, event mapping,
   duplicate reservation rejection, and error/clear transitions; verify the
   missing check paths fail.
2. Add poller tests with a controllable clock for due scheduling, start/stop,
   independent failures, and Run now.
3. Add cleanup reconciliation tests for provider terminal states and all three
   policies, then implement the minimum polling/event code and refactor shared
   work-item/PR scheduling with the focused suite green.

## Verification

- `go test ./internal/azuredevops ./internal/orchestrator` from `apps/backend` — passed (1,455 tests).
- Coverage includes bounded checks, duplicate suppression, provider errors, terminal cleanup policies, and watcher poller scheduling.

## Files Likely Touched

- `apps/backend/internal/azuredevops/service_watches.go`
- `apps/backend/internal/azuredevops/service_watches_test.go`
- `apps/backend/internal/azuredevops/service_watch_events.go`
- `apps/backend/internal/azuredevops/poller_watches.go`
- `apps/backend/internal/azuredevops/poller_watches_test.go`
- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/mock_client.go`
- `apps/backend/internal/azuredevops/mock_controller.go`
- `apps/backend/internal/events/types.go`

## Dependencies

Task 24.

## Parallelism

Sequential. Poll scheduling, provider checks, reservations, and cleanup share
watch state.

## Inputs

- Spec: watcher behavior, failure modes, and match scenario.
- GitLab `poller.go`, `service_watches.go`, and `service_issue_watches.go`.

## Risks

- WIQL may return large existing sets. Respect top/batch limits and reservation
  checks before event publication to avoid a task burst after restart.
- Do not reserve terminal-state reconciliation as a new match; it operates only
  on existing task-owning reservations.

## Output Contract

Report scheduling and dedup behavior, RED/GREEN commands, files changed, mock
coverage, risks, and update task/plan status.
