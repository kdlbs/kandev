---
id: "02-backend-ci-options-api"
title: "Backend CI options API"
status: done
wave: 2
depends_on: ["01-backend-persistence-prompts"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 02: Backend CI Options API

## Acceptance

- `GET /api/v1/github/tasks/:taskId/ci-options` returns persisted/default options, effective prompt, default/override flag, and linked PR automation states.
- `PATCH /api/v1/github/tasks/:taskId/ci-options` supports partial boolean updates and prompt override save/reset.
- API tests cover default response, partial update, override reset, and invalid payload behavior.

## Verification

```bash
cd apps/backend && rtk go test ./internal/github
```

## Files Likely Touched

- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/controller_test.go`
- `apps/backend/pkg/websocket/actions.go` if websocket update action is added
- `apps/backend/internal/gateway/websocket/task_notifications.go` if websocket notifications are added

## Dependencies

- `01-backend-persistence-prompts`

## Inputs

- Spec sections: API surface, Permissions, Failure modes.
- Plan sections: Backend > GitHub API.
- Existing patterns: `/api/v1/github/task-prs/:taskId`, action-presets endpoints, and controller tests.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 2 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
