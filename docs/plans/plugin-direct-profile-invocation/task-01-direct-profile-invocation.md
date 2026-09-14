---
id: "01-direct-profile-invocation"
title: "Direct profile invocation and settings rendering"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-DIRECT-PROFILE-INVOCATION-001
acceptance_criteria:
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.1
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.2
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.3
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.4
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.5
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.6
  - AC-PLUGINS-DIRECT-PROFILE-INVOCATION-001.7
system_design:
  - ../../specs/plugins/system-design/plugin-direct-profile-invocation.md
---

# Task 01: Direct Profile Invocation and Settings Rendering

## Summary

Add the direct `agent-profile` config format to Settings > Plugins, render
only eligible platform profiles through the shared picker, and route
`Host.InvokeUtilityAgent` to `ExecuteProfilePrompt` on the stored profile ID
with typed configuration errors and bounded legacy fallback.

## In scope

- Add the `agent-profile` renderer, picker-mode eligibility filtering, and
  unavailable-selection replaceable rendering.
- Register the companion Notes settings picker via the daemonized target
  server and the platform agent registry.
- Keep the plugin config form's hydration merge and the store profile-list
  revision guard so profile/config updates do not overwrite each other.
- Route direct selections to `ExecuteProfilePrompt`; return typed
  `FailedPrecondition` for missing/ineligible selections before any runner
  starts.
- Adapt host-utility manager errors to gRPC codes with not-found/config
  classes; pass non-not-found store failures through unchanged.
- Bound legacy `utility_agent` fallback to trusted declared-selector
  configurations per ADR 0048.
- Cover the paths with backend and frontend focused tests, docs updates, and
  PR validation evidence.

## Out of scope

- Notes-side manifest, webhook translation, packaging, and QA evidence (Task
  02).
- `Host.InvokeUtilityAgent` signature or gate semantic changes.
- Utility-agent behavior for manifests that declare only the legacy format.

## Acceptance

- Notes's Utility Agent settings field lists eligible profiles, not utility
  or custom-prompt rows, on desktop and mobile widths.
- Selecting a profile persists its stable ID, marks the shared settings
  contributor dirty, saves, and reloads showing the same display label.
- Disabled, deleted, workspace-scoped, and CLI-passthrough profiles cannot be
  newly selected; previously saved unavailable selections render replaceably.
- `Host.InvokeUtilityAgent` with a declared, selected profile executes that
  profile directly with no utility/custom-prompt lookup; missing or
  ineligible selections return typed `FailedPrecondition`, and
  non-`agent_invoke` plugins are denied before profile lookup.
- Direct selection takes precedence over a coexisting legacy value;
  legacy-only behavior is unchanged.

## Verification

```bash
cd apps/backend
go test ./internal/plugins/... ./internal/backendapp/...
cd ../apps && pnpm --filter @kandev/web test -- --run \
  lib/plugins/config-schema.test.ts \
  components/settings/utility-agent-profile-picker.test.ts \
  components/settings/plugins/plugin-config-form.agent-profile.test.tsx \
  hooks/domains/settings/use-settings-data.test.tsx \
  lib/state/hydration/hydrator.test.ts \
  lib/state/slices/settings/types.test.ts \
  lib/ws/handlers/agents.test.ts
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/plugins/config-schema.ts`
- `apps/web/components/settings/utility-agent-profile-picker.tsx`
- `apps/web/components/settings/plugins/plugin-config-form.tsx`
- `apps/web/hooks/domains/settings/use-settings-data.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/lib/ws/handlers/agents.ts`
- `apps/backend/internal/plugins/host_utility.go`
- `apps/backend/internal/plugins/host.go`
- `apps/backend/internal/plugins/service.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/pkg/pluginsdk/host.go`
- `docs/decisions/0048-plugin-host-utility-agent-invoke.md`
- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `docs/screenshots/pr-validation/2870/*`

## Dependencies

None.

## Risks

- Legacy fallback must remain available for stored configs that still expect
  it; explicit bounds and tests prevent silent removal.
- Concurrent hydration of config and profile snapshots can cause lossy
  overwrites without merge and revision guards.
- Eligibility criteria must match the agents system's rules, so the shared
  registration provider is anchored to the same store used by the settings
  API.

## Parallelism

`sequential`

## Inputs

- `docs/specs/plugins/requirements/plugin-direct-profile-invocation.md`
- `docs/specs/plugins/system-design/plugin-direct-profile-invocation.md`
- `docs/specs/agents/requirements/utility-agent-profiles.md`
- `docs/decisions/0048-plugin-host-utility-agent-invoke.md`
- `apps/web/lib/plugins/config-schema.ts`
- `apps/web/components/settings/utility-agent-profile-picker.tsx`
- `apps/backend/internal/plugins/host_utility.go`

## Results

- Added the `agent-profile` renderer and shared-picker `agentRegistration`
  mode with utility-execution eligibility (enabled, global, non-CLI,
  inference-capable) and replaceable unavailable-selection rendering.
- Registered the daemonized target server so Notes renders platform profiles
  with correct labels, persisted the saved stable ID, and surfaced the
  profile name in the enhancement preview.
- Preserved legacy `utility_agent` fallback for trusted declared-selector
  configs, bounded per ADR 0048 and regression-tested.
- Wired hydration merges and store revision guards so profile snapshot and
  plugin-config updates do not clobber each other; profile WS updates retry
  when a render would go stale.
- Verification: focused backend `go test` and web vitest/lint/typecheck all
  passing at the delivery head, with PR validation screenshots added.
