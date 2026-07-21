---
id: "02-watcher-dispatch"
title: "GitLab watcher task dispatch"
status: done
wave: 2
depends_on: ["01-workspace-connection"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 02: GitLab Watcher Task Dispatch

## Acceptance

- Review and issue matches flow through `WatcherDispatchCoordinator` and create
  exactly one task with the selected workflow step, profiles, prompt, optional
  repository/base branch, and auto-start parameters.
- Dispatch failure releases the reservation; deleted profiles/repositories and
  max-inflight violations disable or defer the watch using shared coordinator
  semantics instead of creating an orphan/duplicate task.
- Workspace-scoped create/edit/pause/run/reset/delete APIs validate watch
  dependencies. Reset preview drives the shared best-effort delete/clear/rerun
  contract; delete best-effort removes owned tasks and never reruns the watch.

## Verification

```bash
cd apps/backend && rtk go test ./internal/orchestrator/... ./internal/gitlab/...
cd apps/backend && rtk go test -run 'Test.*GitLab.*(Dispatch|Watch|Reset|SelfHeal)' ./internal/orchestrator/... ./internal/gitlab/...
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/source_gitlab.go` (new)
- `apps/backend/internal/orchestrator/source_gitlab_test.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_gitlab.go`
- `apps/backend/internal/orchestrator/event_handlers_gitlab_test.go`
- `apps/backend/internal/orchestrator/watcher_dispatch_wiring.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/gitlab/watch_models.go`
- `apps/backend/internal/gitlab/store.go`
- `apps/backend/internal/gitlab/store_watches.go`
- `apps/backend/internal/gitlab/store_watches_test.go`
- `apps/backend/internal/gitlab/service_reservations.go`
- `apps/backend/internal/gitlab/service_watches.go`
- `apps/backend/internal/gitlab/service_watches_test.go`
- `apps/backend/internal/gitlab/service_issue_watches.go`
- `apps/backend/internal/gitlab/service_cleanup.go`
- `apps/backend/internal/gitlab/controller_watches.go`
- `apps/backend/internal/gitlab/controller_watch_reset.go` (new)
- `apps/backend/internal/gitlab/controller_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/adapters.go`

## Dependencies

Task 01 must provide workspace client resolution before a poll can dispatch
against the correct host/account.

## Inputs

- Spec: automation-watch `What`, Task and watch records, Automation watch state
  machine, watch failure modes, and watch scenarios.
- Pattern: `apps/backend/internal/orchestrator/source_jira.go`,
  `source_linear.go`, `source_sentry.go`, and `watcher_dispatch.go`.
- Pattern: GitHub/Jira reset preview and reset controller/service tests.
- Lifecycle contract: mirror GitHub code-host watches exactly for reset/delete;
  cleanup policy only governs terminal-item cleanup, not explicit reset/delete.
- Constraint: do not add another integration-specific task-creation pipeline or
  keep the nullable `GitLabIssueTaskCreator`/`GitLabReviewTaskCreator` seam.

## Output Contract

Report both `WatcherSource` mappings, dedup lifecycle, validation/self-heal
behavior, reset policy, files changed, tests run, blockers, and poller risks.
Mark this task `done` and update `plan.md` after verification.
