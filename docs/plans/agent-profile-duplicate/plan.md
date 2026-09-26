---
spec: docs/specs/agents/requirements/profile-duplicate.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Agent Profile Duplicate

## Overview

Add a one-click "duplicate" for agent profiles on the settings agents surface
(`/settings/agents` profile list and the per-profile settings page). A new
interlock-protected endpoint `POST /api/v1/agent-profiles/:id/duplicate`
copies the source profile's full configuration (model, mode, config options,
CLI flags, env vars, launcher prefix, auto-approve flags, enabled state, MCP
config row) into a fresh row named `<source> Copy`, broadcasts the existing
`agent.profile.created` WS event, and returns the new profile DTO.

## Backend

### Controller: `DuplicateProfile`

Add to `apps/backend/internal/agent/settings/controller/profile_crud.go`:

- `type DuplicateProfileRequest struct { ID string }`.
- `func (c *Controller) DuplicateProfile(ctx, req) (*dto.AgentProfileDTO, error)`:
  1. `repo.GetAgentProfile(ctx, req.ID)`; map the "agent profile not found"
     error to `ErrAgentProfileNotFound` exactly like `DeleteProfile` does
     (the sqlite store wraps `sql.ErrNoRows` in a message containing
     "agent profile not found").
  2. Build a fresh `models.AgentProfile` copying every configuration field:
     `AgentID`, `AgentDisplayName`, `Model`, `FallbackModel` (trimmed),
     `AutoFallback`, `Mode`, `ConfigOptions` (through
     `profileconfig.SanitizeConfigOptions` on a copied map),
     `AllowIndexing`, `AutoApprove`, `CLIPassthrough`, `CLIFlags` (deep
     copy of the slice), `EnvVars` (deep copy of the slice, keeping
     `SecretID` refs), `CommandPrefix`, `UserModified: true`, and the office
     enrichment configuration fields (`WorkspaceID`, `Role`, `Icon`,
     `ReportsTo`, `SkillIDs`, `DesiredSkills`, `MaxConcurrentSessions`,
     `CooldownSec`, `SkipIdleRuns`, `FailureThreshold` (copied pointer),
     `ExecutorPreference`, `BudgetMonthlyCents`, `Settings`, `Permissions`).
     Do NOT copy runtime state: `Status`, `PauseReason`, `LastRunFinishedAt`,
     `ConsecutiveFailures`, `DeletedAt`, `MigratedFrom`, `CustomPrompt`,
     `DangerouslySkipPermissions`.
  3. Name: `strings.TrimSpace(source.Name) + " Copy"`. The name is persisted
     data, not UI copy (same convention as seeded executor/repository names),
     so the suffix is a plain string in Go.
  4. Read the source's MCP config row when it has one (`repo
     .GetAgentProfileMcpConfig(ctx, source.ID)`; tolerate `sql.ErrNoRows` /
     nil); deep-copy `Servers` and `Meta` into a fresh
     `models.AgentProfileMcpConfig` (ProfileID filled by the repo). A source
     without a row leaves the copy without one — the default-config
     semantics and boot `EnsureDefaultMcpConfig` cover MCP-supporting
     agents.
  5. `repo.DuplicateAgentProfile(ctx, store.DuplicateAgentProfileInput{...})`
     — NEW atomic repository operation (adversarial review round 1): one
     transaction inserts the row with the caller-provided `Enabled` state
     (NOT forced true like `CreateAgentProfile`, so a disabled source never
     becomes briefly selectable) and upserts the MCP config row. A failure
     rolls back, leaving no partial copy. Rollback makes each attempt
     atomic, but the endpoint is not HTTP-idempotent: a retried request
     creates another row (the UI guards against double-clicks). The store
     assigns the fresh UUID and a single `CreatedAt`/`UpdatedAt` pair,
     which the returned DTO reflects (no stale timestamp after a second
     write).
  6. **Consistent snapshot (adversarial review round 5):** inside the
     transaction the repository re-reads the source profile and MCP rows and
     verifies their `updated_at` still match the revisions the copy was
     built from (`ErrProfileChanged` / `ErrSourceProfileNotFound` otherwise);
     WAL snapshot isolation aborts the write with a busy error if a
     concurrent writer commits between the verification and the insert. The
     controller retries (bounded, `maxDuplicateRetries = 2`) on
     `ErrProfileChanged` / `ErrSourceProfileNotFound` / busy errors with a
     fresh source read, so the copy always reflects one consistent snapshot.
  7. Return `toProfileDTO(clone)`.

Env-var secret refs are copied verbatim; they were validated when the source
was created/updated, so no re-validation is needed.

### Handler + route

In `apps/backend/internal/agent/settings/handlers/handlers.go`:

- Register `api.POST("/agent-profiles/:id/duplicate", h.interlock,
  h.httpDuplicateProfile)` next to the other `/agent-profiles/:id` routes.
- `httpDuplicateProfile`: require `:id`, call
  `controller.DuplicateProfile`, map `ErrAgentProfileNotFound` → 404, other
  errors → 500; on success broadcast
  `ws.ActionAgentProfileCreated` with `{"profile": resp}` (same payload
  shape as `httpCreateProfile`) and return the DTO with 200.

### Tests (backend)

- Controller, new file
  `apps/backend/internal/agent/settings/controller/profile_duplicate_test.go`
  (using the existing `newTestController` + `newFakeStore`):
  - full-config copy: source with model/fallback/mode/config options/CLI
    flags/env vars/command prefix/auto-approve/cli-passthrough duplicates
    with equal fields, new ID, `Default Copy` name, `user_modified` true;
  - disabled source → disabled copy stored in the single atomic write (no
    follow-up enabled flip, timestamps match the stored row);
  - MCP config row copied to the new profile ID with equal enabled/servers/meta
    (extend the shared `fakeStore` in `reconciler_test.go` with an
    `mcpConfigs` map + working `GetAgentProfileMcpConfig` /
    `UpsertAgentProfileMcpConfig` + `DuplicateAgentProfile`, additively —
    other tests only observe the existing nil behaviour);
  - store failure propagates and leaves no partial copy;
  - unknown ID → `ErrAgentProfileNotFound`;
  - empty source name (`""` → `" Copy"` is acceptable; source names are
    non-empty by validation, but pin the suffix behaviour).
- Store, new file `apps/backend/internal/agent/settings/store/sqlite_duplicate_test.go`:
  sqlite `DuplicateAgentProfile` round-trip — fresh ID, full configuration
  preserved, disabled copy stays disabled (not forced enabled), MCP row
  copied, source untouched; no MCP row created when none passed.
- Handler, `apps/backend/internal/agent/settings/handlers/interim_settings_interlock_test.go`:
  add `{method: POST, path: "/api/v1/agent-profiles/profile-1/duplicate"}`
  to the route list that must return 403 without the interlock token.
- Handler test (mandatory, adversarial review round 9): an office-scoped
  source (`WorkspaceID != ""`) returns 404 with zero broadcasts on any
  channel — global or workspace-scoped — and the office profile is rejected
  before MCP config is read or written. Also assert 404 mapping and the
  `agent.profile.created` broadcast for kanban profiles, following
  `agent_update_handlers_test.go`'s controller-stub pattern.

## Frontend

### API action

In `apps/web/app/actions/agents.ts` add:

```ts
export async function duplicateAgentProfileAction(profileId: string): Promise<AgentProfile> {
  const raw = await agentSettingsRequest<unknown>(
    `${apiBaseUrl}/api/v1/agent-profiles/${profileId}/duplicate`,
    { method: "POST" },
  );
  return normalizeAgentProfile(raw);
}
```

### List page: per-row Duplicate button

- `apps/web/app/settings/agents/profile-list-item.tsx`: accept an
  `onDuplicate: (profile: AgentProfile) => void` prop; render an icon-only
  button (copy icon, e.g. `IconCopy` from `@tabler/icons-react`) outside the
  link, next to the enabled switch, with `aria-label` / tooltip via
  `t("agents:duplicateProfileNamed", { name: profile.name })` and
  `data-testid={`duplicate-profile-${profile.id}`}`.
- `apps/web/app/settings/agents/page.tsx`: thread `onDuplicate` through
  `AgentProfilesSection`; add a `handleDuplicateProfile` wired like
  `useProfileEnabledToggle` (POST via the action, then merge the returned
  profile into `settingsAgents` + `agentProfiles` store slices atomically via
  `useAppStoreApi().setState`, appending the new profile to its agent's
  `profiles`; toast success/failure via `agents:duplicateProfileSuccess` /
  `agents:failedToDuplicateProfile`). The `agent.profile.created` WS handler
  already upserts the same profile; the direct merge keeps the UI consistent
  even if WS is delayed. The merge (`applyProfileDuplicated` in
  `use-profile-duplicate.ts`) dedupes by ID, preserves WS-delivered options
  whose owning agent is absent, never lets a stale duplicate response clobber
  a newer `agent.profile.updated` version (newer `updatedAt` wins), and
  always rebuilds the copy option with the known agent metadata.

### Profile settings page: header Duplicate button

- `apps/web/components/settings/agent-profile-page.tsx`: add a Duplicate
  button to `ProfileEditorHeader` (copy icon + `t("agents:duplicate")`,
  `data-testid="duplicate-profile-header"`, disabled + `aria-busy` while a
  duplicate is in flight). The flow lives in the extracted
  `useProfileDuplicateAction` hook
  (`components/settings/agent-profile-duplicate-action.ts`, unit-tested):
  a ref-based in-flight guard (two rapid clicks can never issue two
  copies), then save EVERY dirty contributor via the shared settings save
  coordinator (`useSettingsSaveCoordinator().saveAll` — the profile editor
  AND its MCP card; `saveAll` respects each contributor's `canSave`, and the
  abort toasts the blocking contributor's `invalidReason` — MCP error or
  profile-name validation), POST the duplicate, merge the copy into the
  store, toast `agents:duplicateProfileSuccess`, cancel any pending
  navigation intent the dirty guard had blocked, and SPA-navigate via
  `runWithNavigationBlockerBypassed(() => router.push(...))` — never
  `window.location.assign`, so the success toast survives and the
  dirty-settings blocker is already resolved by the save. The row-level
  duplicate hook guards against double-clicks (per-profile in-flight set).
  The profile save path (`syncAgentsToStore`) reconciles the options slice
  by ID (`reconcileAgentProfileOptions`), preserving WS-delivered orphan
  options exactly like the duplicate merge. The merge keeps a newer
  WS-delivered orphan option (revision carried on the option as `updatedAt`)
  over a stale duplicate HTTP response, matching the newer-wins rule used
  when the owning agent is present.

### i18n

Add to `apps/web/src/locales/en/agents.json`:

- `"duplicate": "Duplicate"`
- `"duplicateProfile": "Duplicate profile"`
- `"duplicateProfileNamed": "Duplicate {{name}}"`
- `"duplicateProfileSuccess": "Profile duplicated"`
- `"failedToDuplicateProfile": "Failed to duplicate profile"`

Regenerate the pseudo locale (`pnpm --filter @kandev/web i18n:pseudo`). Real
locales (`zh-cn`, `pt-pt`) fall back to en; key-parity is a warning there,
missing en keys are a hard error.

### Tests (frontend)

- `apps/web/app/settings/agents/profile-list-item.test.tsx`: extend with a
  case asserting the Duplicate button renders and calls `onDuplicate` with
  the profile (mirroring the existing toggle tests).
- `apps/web/lib/api/domains/agent-profile-normalize.test.ts` or the actions
  layer: only if an existing action-test pattern exists; otherwise the
  component test plus E2E covers the contract.

## E2E

New `apps/web/e2e/tests/settings/agent-profile-duplicate.spec.ts` modeled on
`agent-profile-delete.spec.ts`:

- Create a source profile via `apiClient.createAgentProfile(agent.id,
  "Dup Me", { model: agent.profiles[0].model, cli_flags: [...], command_prefix: ... })`.
- `testPage.goto("/settings/agents")`, wait for the profile list, click the
  duplicate button on the "Dup Me" row (`duplicate-profile-<id>`).
- Expect a "Dup Me Copy" row to appear without reload.
- Navigate to the copy's profile page and assert the copied model/cli flag
  values render.
- `finally`: delete both profiles via `apiClient.deleteAgentProfile(..., true)`.

## Implementation Waves

Execution is sequential in the primary conversation; no subagents are
authorized.

Wave 1:

- [x] [Task 01: Backend duplicate endpoint](task-01-backend-duplicate-endpoint.md)

Wave 2:

- [x] [Task 02: Frontend duplicate UI](task-02-frontend-duplicate-ui.md)

Wave 3:

- [x] [Task 03: E2E duplicate flow](task-03-duplicate-e2e.md)

## Risks

- `DuplicateAgentProfile` inserts the copy with the caller-provided enabled
  state inside one transaction, unlike `CreateAgentProfile` which forces
  `Enabled=true` for other callers — a store regression that forces the
  duplicate enabled is pinned by `sqlite_duplicate_test.go` (disabled copy
  stays disabled) and by the controller test asserting no follow-up write.
- `GetAgentProfileMcpConfig` returns different "absent" shapes across the
  sqlite store (`sql.ErrNoRows`) and the shared fake store (`nil, nil`);
  the controller must tolerate both.
- The list-page direct store merge and the WS `agent.profile.created`
  broadcast can both insert the copy; the WS handler upserts by ID so this
  is safe (verify while implementing).
