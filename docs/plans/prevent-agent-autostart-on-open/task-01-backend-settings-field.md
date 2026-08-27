---
id: "01-backend-settings-field"
title: "Backend user-settings field"
status: done
wave: 1
parallelism: sequential
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 01: Backend user-settings field

## Acceptance

- `UserSettings` gains `PreventAutoStartAgentOnOpen bool` with json tag
  `prevent_auto_start_agent_on_open`; the value round-trips through
  `FromUserSettings`, `UpdateUserSettingsRequest`, the service apply path,
  `publishUserSettingsEvent`, and the SQLite settings blob
  (`marshalUserSettingsPayload` / `scanUserSettings`).
- `PATCH /api/v1/user/settings` with `{"prevent_auto_start_agent_on_open": true}`
  persists the value across a settings reload; omitting the key leaves it
  unchanged (pointer semantics).
- The SSR boot payload exposes it as `preventAutoStartAgentOnOpen`.
- No DB migration: the settings blob already accepts new JSON fields.

## Verification

```bash
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/... -race)
```

```bash
(cd apps/backend && make lint)
```

## Files Likely Touched
- `apps/backend/internal/user/models/models.go` (`UserSettings` struct)
- `apps/backend/internal/user/dto/dto.go` (`UserSettingsDTO` + `FromUserSettings` at `:238`, `UpdateUserSettingsRequest` at `:105`)
- `apps/backend/internal/user/dto/dto_test.go`
- `apps/backend/internal/user/service/service.go` (service `UpdateUserSettingsRequest` at `:52`, `applyTaskActionPreferences` at `:346`, `publishUserSettingsEvent` at `:773`)
- `apps/backend/internal/user/controller/controller.go` (`UpdateUserSettings` mapping at `:61`)
- `apps/backend/internal/user/store/sqlite.go` (`marshalUserSettingsPayload` at `:519`, `scanUserSettings` payload struct at `:707`)
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go` (boot-payload map at `:459`)

## Dependencies

None.

## Inputs

- Spec "Data model" and "API surface" sections.
- Existing pattern: `ConfirmTaskArchive` plumbing across the same files,
  including the SQLite blob serialization in
  `internal/user/store/sqlite.go` (`marshalUserSettingsPayload` writes
  `confirm_task_archive`; `scanUserSettings` reads it via a `*bool` payload
  field with a nil-guarded assignment).

## Output Contract

The field exists end to end on the backend: model → DTO → service apply →
controller → store blob → boot payload, with pointer-based PATCH semantics.
Tests pin the round-trip, the omitted-key behavior, and the store reload
(legacy JSON without the key loads the default `false`).
