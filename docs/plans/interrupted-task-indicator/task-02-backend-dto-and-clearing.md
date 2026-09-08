---
id: "02-backend-dto-and-clearing"
title: "Backend DTO exposure and marker clearing"
status: done
wave: 2
depends_on: ["01-backend-startup-marker"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/interrupted-task-indicator.md"
---

# Task 02: Backend DTO exposure and marker clearing

## Acceptance

- `v1.Task` exposes `interrupted` (boolean, `omitempty`), derived at both task
  serializers from `metadata["interrupted_at"]` presence.
- The marker clears when a session of the task transitions into `STARTING` or
  `RUNNING` through the orchestrator's session-state funnel
  (`updateTaskSessionStateWithHook` / `setSessionStarting` — verify the funnel
  covers launch, resume, prompt dispatch, and agent-ready wake paths).
- When the key was actually removed, the orchestrator publishes `task.updated`
  for that task; transitions that removed nothing publish nothing extra.
- Startup reconciliation itself never clears the marker.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/... -run 'SessionStarting|ReconcileSessionsOnStartup|Interrupted'
cd apps/backend && go test ./internal/task/dto/... ./internal/task/models/...
cd apps/backend && go build ./...
```

## Files likely touched

- `apps/backend/pkg/api/v1/task.go` — `Interrupted` field on `v1.Task`.
- `apps/backend/internal/task/dto/dto.go` — `Interrupted` field on `TaskDTO`
  (the rich wire payload) plus the derivation in `FromTaskWithSessionInfo`.
- `apps/backend/internal/task/models/models.go` — derived `Interrupted` in
  `(*Task).ToAPI`.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` — clear +
  publish in the session-start funnel.
- Orchestrator tests (`event_handlers_streaming_test.go` and related) — clear
  behavior, publish-on-remove only.

## Dependencies

Task 01 (the key must exist before the DTO derivation and clearing compile).

## Inputs

- Spec: `API surface`, `State machine` (marked → cleared), `Failure modes`,
  `Persistence guarantees`, scenarios 4–5.
- Plan: `Backend > DTO exposure` and `Backend > Clearing`.
- Existing pattern: `updateTaskSessionStateWithHook`'s publish discipline and
  the `task.updated` rich payload through `taskEvents.PublishTaskUpdated`.

## Risks

- A start path that bypasses the funnel leaves the icon after resume — assert
  the funnel covers every `STARTING`/`RUNNING` transition in the tests, and
  clear on BOTH next-states if any path lands directly in `RUNNING`.
- `RemoveTaskMetadataKey` returns `(removed bool, err)`; only publish on
  `removed == true`, and swallow/log a removal error without failing the
  session transition.
- Postgres JSON patch behavior of `RemoveTaskMetadataKey` must be exercised by
  the existing repository tests; do not add new dialect-specific SQL.

## Output contract

Report the DTO field, both serializer updates, the exact funnel site chosen for
clearing (with evidence it covers launch/resume/prompt/agent-ready), tests and
commands, then mark this task `done` and update `plan.md`.
