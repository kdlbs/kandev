---
id: "01-backend-duplicate-endpoint"
title: "Add the backend profile duplicate endpoint"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/profile-duplicate.md"
---

# Task 01: Backend profile duplicate endpoint

## Acceptance

- `POST /api/v1/agent-profiles/:id/duplicate` (interlock-protected) creates a
  new profile named `<source name> Copy` with a distinct ID and
  `user_modified: true`, copying model, fallback model, auto-fallback, mode,
  config options, allow-indexing, auto-approve, CLI passthrough, CLI flags,
  env vars (secret refs included), command prefix, enabled state, and the
  source's MCP config row (enabled + servers + meta) when one exists.
- Runtime state is not copied: the copy is idle, no pause reason, no last-run
  timestamp, zero consecutive failures.
- A disabled source produces a disabled copy; an enabled source produces an
  enabled copy.
- The copy is committed atomically (row + MCP config in one repository
  transaction, inserted with the source's enabled state): a failure leaves no
  partial profile and a disabled source never becomes briefly selectable.
- Unknown or soft-deleted source ID → `ErrAgentProfileNotFound` → HTTP 404,
  and nothing is created.
- An office-scoped source (non-empty `workspace_id`) → `ErrAgentProfileNotFound`
  → HTTP 404 (existence hidden), rejected before MCP config is read or any
  row is written, with zero broadcasts on any channel (global or
  workspace-scoped).
- Success returns the new `AgentProfileDTO` and broadcasts the
  `agent.profile.created` WS notification with `{"profile": <dto>}`.
- The route is added to the interlock 403 route list.

## Verification

- RED/GREEN controller:
  `cd apps/backend && go test -run 'TestDuplicateProfile' ./internal/agent/settings/controller`
- Interlock list:
  `cd apps/backend && go test -run 'TestRegisterRoutesProtectsEveryStateChangingAgentSettingsRoute' ./internal/agent/settings/handlers`
- Full package after refactor:
  `cd apps/backend && go test ./internal/agent/settings/...`

## Files likely touched

- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/internal/agent/settings/controller/profile_duplicate_test.go` (new)
- `apps/backend/internal/agent/settings/controller/reconciler_test.go` (extend shared `fakeStore` with MCP config methods + `DuplicateAgentProfile`, additively)
- `apps/backend/internal/agent/settings/store/store.go` (add `DuplicateAgentProfile` to the repository interface)
- `apps/backend/internal/agent/settings/store/sqlite.go` (atomic `DuplicateAgentProfile` in one transaction; shared insert/upsert helpers)
- `apps/backend/internal/agent/settings/store/sqlite_duplicate_test.go` (new)
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/interim_settings_interlock_test.go`
- Optional: `apps/backend/internal/agent/settings/handlers/profile_duplicate_handlers_test.go`

## Dependencies

None.

## Parallelism

Sequential by default; file-disjoint from Tasks 02–03 and may run in parallel
only with explicit user authorization.

## Inputs

- Spec duplicate scenarios
- `profile_crud.go` existing `DeleteProfile` not-found mapping and
  `CreateProfile` field list; `store.Repository` interface; sqlite
  `CreateAgentProfile` / `GetAgentProfileMcpConfig` /
  `UpsertAgentProfileMcpConfig`; `ws.ActionAgentProfileCreated` broadcast
  pattern from `httpCreateProfile`.

## Output contract

Report RED and GREEN results, the response/error contract, changed files, the
enabled-state and MCP-copy behavior, and update this task plus `plan.md`
status.
