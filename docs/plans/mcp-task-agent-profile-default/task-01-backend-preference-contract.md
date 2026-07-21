---
id: "01-backend-preference-contract"
title: "Backend preference contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/mcp-task-agent-profile-default/spec.md"
---

# Task 01: Backend Preference Contract

## Acceptance

- User settings GET, PATCH, boot payload, and `user.settings.updated` expose `mcp_task_agent_profile_default` with only `current_task` and `workspace_default` accepted.
- Empty, legacy, or unknown stored values normalize to `current_task`; PATCH omission preserves the saved value and explicit values survive restart/round-trip.
- Invalid PATCH values return validation failure without changing the persisted preference.

## Verification

```bash
cd apps/backend && go test ./internal/user/... ./internal/backendapp/...
```

## Files Likely Touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Dependencies

None.

## Inputs

- Spec: Data model, API surface, Failure modes, Persistence guarantees, and legacy/default scenarios.
- Plan: User-settings contract and persistence.
- Patterns: `ConfirmTaskArchive` for JSON defaulting and events; ADR 0041 for backend-owned portable settings. Unlike the boolean archive preference, this field must validate and normalize an enum.

## Output Contract

Report the normalized constants/field, DTO and persistence behavior, exact tests run, files touched, blockers, and residual risks. Set this task to `done` and update its plan checkbox only after targeted verification passes.
