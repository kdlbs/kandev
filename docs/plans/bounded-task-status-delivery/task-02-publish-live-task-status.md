---
id: "02-publish-live-task-status"
title: "Publish live task status"
status: completed
wave: 2
depends_on: ["01-persist-task-status-summaries"]
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 02: Publish Live Task Status

## Acceptance

- The projector derives lifecycle/activity, pending action, active error, Git,
  and PR aggregates from authoritative sources while filtering source events
  that cannot change the summary.
- Recoverable-error persistence, dismissal, and newer agent responses produce
  explicit projection changes; UTF-8 previews are safe and bounded to 512
  bytes.
- Each semantic change publishes one complete
  `task.status_summary.updated` revision to the owning workspace; duplicate or
  stale source occurrences produce no event.

## TDD sequence

1. RED: add table-driven derivation tests for precedence, error lifecycle,
   preview bounds, multi-repository Git, and PR attention aggregation.
2. RED: add event tests proving irrelevant stream/message updates are ignored,
   concurrent source changes converge, and one semantic revision is routed to
   the correct workspace.
3. GREEN: wire the keyed projector to task/session/activity/pending/error/Git/PR
   occurrences and add the internal/WS event types and broadcaster.
4. REFACTOR: isolate source adapters behind provider interfaces and keep event
   payloads bounded replacement snapshots.

## Verification

```bash
cd apps/backend && go test ./internal/task/statussummary ./internal/task/service/... ./internal/orchestrator/... ./internal/gateway/websocket/...
```

## Files likely touched

- `apps/backend/internal/task/statussummary/**`
- `apps/backend/internal/events/types.go`
- `apps/backend/internal/task/service/service_events.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/turn_activity.go`
- recoverable-error dismissal service/handler files
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

Task 01 supplies storage, types, and batch/rebuild interfaces.

## Parallelism

Sequential. Tasks 03 and 05 rely on the live projection behavior.

## Inputs

- Spec **Derivation rules** and the authoritative-occurrence mapping in the
  plan.
- Existing task pending-action aggregation and foreground-activity enrichment.
- Existing `persistLastAgentError`, error-dismissal, Git snapshot, and
  `GitHubTaskPRUpdated` paths.

## Risks

- Subscribing to every message update would reproduce the original traffic
  problem inside the backend; filter before rebuilding.
- Error acknowledgment local to one browser remains presentation state; only
  authoritative dismissal or a newer agent response clears the projection.
- Event publication must occur after the revision is durable.

## Verification results

- `cd apps/backend && go test ./internal/task/statussummary` — passed.
- Broad task, gateway, backendapp, lifecycle, and orchestrator integration
  packages — passed.
- Projector tests cover bounded error previews, lifecycle/activity/pending
  precedence, irrelevant message filtering, multi-repository Git aggregation,
  PR aggregation, semantic no-ops, restart state, and workspace event output.
- The projector subscribes to bounded source occurrences only; it does not
  subscribe to agent stream, shell, process, model, or MCP subjects.

## Output contract

Report source events consumed/ignored, projection semantics, revision/event
counts, focused test results, exact files changed, and unresolved domain gaps.
