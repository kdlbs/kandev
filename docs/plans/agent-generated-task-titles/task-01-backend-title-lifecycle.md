---
id: "01-backend-title-lifecycle"
title: "Backend preference and provisional-title lifecycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/agent-generated-titles.md"
---

# Task 01: Backend preference and provisional-title lifecycle

> Continuation note: Task 06 changes the missing/new preference default to `true` while preserving
> explicit opt-outs. This completed task records the original opt-in rollout.

## Acceptance

- The backend-owned `agent_generated_task_titles` preference defaults to false, round-trips through
  GET/PATCH, boot state, and `user.settings.updated`, and preserves omitted PATCH fields.
- `POST /api/v1/tasks` with `auto_title:true` derives the provisional title from the first six
  whitespace-normalized prompt words, uses every word for shorter prompts, rejects an empty prompt,
  and persists `agent_title_pending`; ordinary creation stays unchanged.
- Ordinary title updates clear pending state, and the task service exposes a validated one-time pending
  title mutation that publishes `task.updated` without overwriting a prior human rename.

## Verification

```bash
cd apps/backend && go test ./internal/user/... ./internal/backendapp ./internal/task/service ./internal/task/handlers -run 'Test.*(AgentGeneratedTaskTitles|AutoTitle|AgentTitlePending)'
```

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_tasks_test.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- focused task HTTP handler test file(s)

## Dependencies

None.

## Parallelism

Sequential. This task establishes shared persisted and HTTP/service contracts consumed by every later
task.

## Inputs

- Spec sections: **What**, **Data model**, **Task creation**, **Failure modes**.
- Plan sections: **Portable user setting**, **Provisional-title creation and pending state**.
- Patterns: archive-confirmation preference plumbing and existing `Service.UpdateTask` event publication.

## Risks

- Preserve user-settings omitted-field semantics and do not introduce a schema migration for the JSON
  blob.
- Do not add a normal provisional character limit or ellipsis; preserve the existing absolute
  500-character task-title safety boundary.
- Do not make the live preference a launch-time source of truth; only the task marker is durable intent.

## Output contract

Report behavior implemented, files changed, the exact test command/result, blockers or risks, and update
this task plus `plan.md` status in the same conversation.
