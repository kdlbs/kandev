---
spec: docs/specs/agents/requirements/profile-disable.md
created: 2026-08-01
status: implemented
---

# Implementation Plan: Disable an Agent Profile

## Overview

Add an `enabled` flag to `agent_profiles` (kanban-flavour profiles only),
expose it through the profile DTO/PATCH API, carry it into the frontend
profile type and the store's `AgentProfileOption`, and filter it out of every
profile *selection* surface (new task, new subtask, new session, handoff,
quick chat). Two settings toggles: one in the profile page header (dirty/save
flow), one in the `/settings/agents` profile list (immediate save). Existing
sessions and labels are untouched because filtering happens only where
profiles are offered as choices — the store keeps disabled profiles.

Order: backend persistence/contracts → frontend types + selection filtering →
settings UI → E2E. Each layer is independently testable.

## Backend

### Schema & store (`apps/backend/internal/agent/settings/store/sqlite.go`)

- Add migration `r.migrate.Apply("agent_profiles.enabled", ALTER TABLE agent_profiles ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1)` beside the other `agent_profiles.*` migrations in `migrate()`.
- Add `enabled INTEGER NOT NULL DEFAULT 1` to the `CREATE TABLE IF NOT EXISTS agent_profiles` DDL (both the original table and the `recreateAgentProfilesWithoutModelCheck` new-table DDL, and include the column in that migration's copy lists so a legacy DB recreation preserves it).
- `CreateAgentProfile` INSERT: add `enabled` column + bind (`dialect.BoolToInt(profile.Enabled)`).
- `UpdateAgentProfile` UPDATE: add `enabled = ?` to the SET list + bind.
- `agentProfileSelectColumns`: add `enabled`; `scanAgentProfile`: scan into `var enabled int` and set `profile.Enabled = enabled == 1`.

### Model (`apps/backend/internal/agent/settings/models/models.go`)

- Add `Enabled bool json:"enabled" db:"enabled"` to `AgentProfile` (near `CLIPassthrough`/`UserModified`).

### DTO (`apps/backend/internal/agent/settings/dto/dto.go`)

- Add `Enabled bool json:"enabled"` to `AgentProfileDTO`.

### Controller (`apps/backend/internal/agent/settings/controller/profile_crud.go`)

- `CreateProfile`: set `Enabled: true` on the new profile (Go zero value is false; the DB default only applies when the column is omitted).
- `UpdateProfileRequest`: add `Enabled *bool`.
- `UpdateProfile`: apply `if req.Enabled != nil { profile.Enabled = *req.Enabled }`.
- `toProfileDTO`: map `Enabled: profile.Enabled`.

### Handler (`apps/backend/internal/agent/settings/handlers/handlers.go`)

- `updateProfileRequest`: add `Enabled *bool json:"enabled,omitempty"`; pass `Enabled: body.Enabled` into the controller call.

## Frontend

### Types & client

- `apps/web/lib/types/agent-profile.ts`: add `enabled?: boolean` to `AgentProfile` (identity section) and to `AgentProfilePayload`.
- `apps/web/lib/api/domains/agent-profile-normalize.ts`: `normalizeAgentProfile` picks `enabled` with fallback `true`; `toAgentProfilePayload` sets `enabled`.
- `apps/web/app/actions/agents.ts`: `updateAgentProfileAction` payload gains `enabled?: boolean`.
- `apps/web/lib/state/slices/settings/types.ts`: `AgentProfileOption` gains `enabled?: boolean`; `toAgentProfileOption` maps `enabled: profile.enabled ?? true` (every call site — settings routes, use-settings-data, WS handlers, SSR, agent pages — picks it up automatically).

### Selection filtering (hide disabled profiles from choices only)

- Define one shared `isSelectableAgentProfile(profile)` predicate that returns
  `profile.enabled !== false`, preserving the enabled-by-default behavior for
  legacy payloads. Every selector path below must use this helper rather than
  duplicating the enabled check.

- `apps/web/components/task-create-dialog-options.tsx`: `useAgentProfileOptions` filters selectable profiles — covers new task, new subtask, new session, and quick-chat option lists.
- `apps/web/components/task-create-dialog-computed.ts`: `useExecutorProfileCompat`'s `compatibleAgentProfiles` memo filters disabled profiles — feeds autopick (`decideAgentProfileAutopick` last-used / workspace-default / first candidates) and the `noCompatibleAgent` submit gate.
- `apps/web/components/task/handoff-profile-menu-items.tsx`: `useHandoffProfiles` filters disabled profiles.
- `apps/web/components/task/new-session-dialog.tsx`: `useNewSessionDialogState` keeps the raw current-session profile for existing-session labels and selector visibility, while the new-session initial fallback picks the first selectable profile.
- `apps/web/components/quick-chat/quick-chat-setup.tsx`: only apply the workspace-default `default_agent_profile_id` when that profile is enabled, else start unselected.

### Settings UI

- `apps/web/components/settings/agent-profile-page.tsx`:
  - `ProfileEditorHeader` gains an `enabled` prop and renders a labeled `Switch` (`@kandev/ui/switch`) + helper text ("Disabled profiles stay available for existing sessions but won't be offered when creating new tasks or agents").
  - `useProfileEditorState` `isDirty` includes `draft.enabled !== savedProfile.enabled`.
  - `useProfileSave` PATCH payload includes `enabled: draft.enabled`.
  - Wire the switch through `ProfileEditor` → header.
- `apps/web/app/settings/agents/page.tsx`:
  - `ProfileListItem` renders an `enabled` `Switch` beside the profile name; clicking it calls `updateAgentProfileAction(profile.id, { enabled: next })`, then syncs the store (`setSettingsAgents` with the updated profile + `setAgentProfiles` rebuilt via `toAgentProfileOption`) — the same shape as `useSyncAgentsToStore` in the profile page. Keep the switch as a sibling outside the row's navigation link so toggling never navigates.

## Tests

- **What:** store persists/reads `enabled` (default true, update to false, scan round-trip). **File:** `apps/backend/internal/agent/settings/store/sqlite_profile_enabled_test.go` (or extend `sqlite_migration_test.go`). **How:** in-memory sqlite repo; create → expect `Enabled == true`; update → `false`; re-read → `false`.
- **What:** migration backfills existing rows to enabled. **File:** store migration test. **How:** create DB at pre-migration schema, insert a row without the column, run migrations, read `enabled == 1`.
- **What:** PATCH `/api/v1/agent-profiles/:id` with `{"enabled": false}` persists and returns `enabled: false`. **File:** `apps/backend/internal/agent/settings/controller/profile_crud_test.go` (+ handler test in `handlers/`). **How:** controller test with fake repo; assert DTO carries `Enabled` and repo receives it.
- **What:** `toAgentProfileOption` carries `enabled`. **File:** `apps/web/lib/state/slices/settings/types.test.ts` (or existing settings slice test). **How:** unit test.
- **What:** `normalizeAgentProfile` defaults missing `enabled` to `true` and maps `false`. **File:** `apps/web/lib/api/domains/agent-profile-normalize.test.ts`. **How:** unit test.
- **What:** `useAgentProfileOptions` omits disabled profiles. **File:** `apps/web/components/task-create-dialog-options.test.tsx`. **How:** render hook with mixed list, expect only enabled options.
- **What:** autopick never resolves to a disabled profile (last-used / workspace default / first fallback). **File:** `apps/web/components/task-create-dialog-effects.test.ts` (exercises `decideAgentProfileAutopick` via the compat-filtered list). **How:** feed a compat list that excludes the disabled profile; assert decision picks an enabled id or "no-compatible".
- **What:** compatibility filtering removes disabled profiles before autopick. **File:** `apps/web/components/task-create-dialog-computed.test.ts` (or the closest dialog-computed test). **How:** feed mixed raw profiles through the executor compatibility path and assert the returned list contains only enabled profiles.
- **What:** `useHandoffProfiles` omits disabled profiles. **File:** `apps/web/components/task/handoff-profile-menu-items.test.ts`. **How:** unit test with mixed store items.
- **What:** profile page header toggle drives dirty state and PATCH payload. **File:** `apps/web/components/settings/agent-profile-page.test.tsx`. **How:** render with a profile, flip the switch, assert `isDirty` and the save action payload include `enabled`.
- **What:** `/settings/agents` list toggle calls the update action and refreshes the row. **File:** `apps/web/app/settings/agents/page.test.tsx` (or component test for `ProfileListItem`). **How:** mock `updateAgentProfileAction`, click the switch, assert the action payload and store sync.

## E2E Tests

- **Scenario:** GIVEN an existing profile, WHEN the user disables it from the profile settings page and opens the new-task dialog, THEN it is absent from the agent profile selector. **File:** `apps/web/e2e/tests/settings/agent-profile-disable.spec.ts` (pattern from `agent-profile-acp.spec.ts`). **What to verify:** toggle persists (page reload keeps it off), the selector in the task-create dialog does not list the profile, and the profile still appears under "Manage existing profiles by agent" with the toggle off; toggling it back on restores it to the selector.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-backend-enabled-column](task-01-backend-enabled-column.md)

Wave 2:

- [x] [task-02-frontend-types-and-selection-filtering](task-02-frontend-types-and-selection-filtering.md)

Wave 3:

- [x] [task-03-settings-toggle-ui](task-03-settings-toggle-ui.md)

Wave 4:

- [x] [task-04-e2e](task-04-e2e.md)

## Risks

- Go zero-value pitfall: a fresh profile must default to `enabled = true` — the controller must set it explicitly, otherwise every new profile would be created disabled.
- The store's three read paths share `agentProfileSelectColumns`; adding the column there plus `scanAgentProfile` keeps all reads consistent (the file already documents this as the rule).
- The `recreateAgentProfilesWithoutModelCheck` migration rebuilds the table for legacy DBs; forgetting the new column there would silently drop `enabled` for upgraded installs.
- Disabled + workflow-locked profile: the selector shows "No compatible agent profiles" and blocks submit until the user re-enables the profile or changes the workflow — intended per spec, but worth confirming in review.
