---
id: session-hostname-resolution-01
title: Portable resolve-session-hostnames setting contract
status: pending
wave: 1
depends_on: []
plan: docs/plans/session-hostname-resolution/plan.md
spec: docs/specs/session-hostname-resolution/spec.md
---

# Portable resolve-session-hostnames setting contract

## Inputs

The [session-hostname-resolution spec](../../specs/session-hostname-resolution/spec.md)
(User setting + Contracts sections) and the existing
`app_status_bar_enabled` user-settings round trip as the pattern.

## TDD sequence

1. Add failing backend tests: missing stored value and initial payload
   default to `false`; explicit `true` survives store read → DTO response →
   boot mapping → event payload; PATCH omission preserves the current value;
   PATCH explicit `false` is durable.
2. Add failing frontend mapper/WS-handler tests: default `false`, explicit
   `true`, omission preserving current state, revision-ordered snapshots.
3. Add the smallest model, DTO, controller, service, store, boot, HTTP type,
   state, and mapper changes that make those tests pass.
4. Refactor only repeated mapping code exposed by the tests. No UI or lookup
   behavior in this task.

## Implementation

- Add `ResolveSessionHostnames bool` to
  `apps/backend/internal/user/models/models.go` (json
  `resolve_session_hostnames`).
- Add the response field and pointer PATCH field to
  `apps/backend/internal/user/dto/dto.go` (including `FromUserSettings`).
- Thread the PATCH pointer through
  `apps/backend/internal/user/controller/controller.go` and
  `apps/backend/internal/user/service/service.go`
  (`UpdateUserSettingsRequest` + `applyBasicSettings`).
- Apply an explicit value only when the pointer is non-nil. Publish
  `resolve_session_hostnames` in the complete `user.settings.updated` event
  (`publishUserSettingsEvent`).
- In `apps/backend/internal/user/store/sqlite.go`:
  - default `false` in `defaultUserSettings` (zero value; no entry needed);
  - include `"resolve_session_hostnames"` in `marshalUserSettingsPayload`;
  - decode it as `*bool` in the `scanUserSettings` payload struct and
    overwrite the default only when present.
- Add `"resolveSessionHostnames": settings.ResolveSessionHostnames` to
  `apps/backend/internal/backendapp/boot_state_routes.go`
  (`mapUserSettingsState`).
- Add `resolve_session_hostnames?: boolean` to response and PATCH shapes in
  `apps/web/lib/types/http-user-settings.ts`.
- Add `resolveSessionHostnames: boolean` to
  `apps/web/lib/state/slices/settings/types.ts` (`UserSettingsState`).
- Initialize it to `false` in `createDefaultUserSettings()` and map it
  through the shared path in `apps/web/lib/ssr/user-settings.ts`; the common
  mapper must preserve the current value when the field is absent.
- **Controller mapping regression test (required):** the controller maps a
  long request literal field-by-field; an omitted `ResolveSessionHostnames`
  entry silently drops `{resolve_session_hostnames: true}` before the
  service/store while DTO/service/store tests all pass. Add
  `apps/backend/internal/user/controller/controller_test.go` coverage:
  a PATCH request with the pointer field set reaches the service request
  (and the response setting) as `true`, an omitted field stays nil (no
  change), and an explicit `false` is preserved.
- The existing `user.settings.updated` WS handler already flows through the
  shared mapper; do not add a second field-specific handler.

## Files likely touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/controller/controller_test.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/service/service_test.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/store/sqlite_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/boot_state_user_settings_test.go`
- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/ws/handlers/users.test.ts`

## Acceptance

1. A new user, an old settings JSON blob without the field, and an initial
   payload that omits it all resolve to `false`.
2. Stored and PATCHed `true` remains true through controller → repository
   read, DTO response, boot state, frontend mapping, and event mapping.
3. An omitted PATCH field leaves the existing setting unchanged (pinned at
   the controller layer too).
4. The complete `user.settings.updated` event payload includes
   `resolve_session_hostnames` (pin the key in a service test).
5. No UI, sessions payload, or lookup behavior changes yet.

## Verification

```sh
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/...)
(cd apps/web && pnpm exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts)
git diff --check
```

The backend run must include the controller mapping regression test
(`internal/user/controller/controller_test.go`).

## Dependencies

None. This task creates the portable setting consumed by Task 02.

## Risks

- A default that remains `true` in either mapper enables resolution for old
  rows with no preference. Pin default-false at store, boot, and frontend
  mapper boundaries.
- The complete event map can silently omit the new field even when HTTP
  works. Pin the event key in service tests.
