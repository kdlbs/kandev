---
id: "02-settings-tree-filter"
title: "Filter disabled profiles from the settings left panel"
status: done
wave: 2
depends_on: ["01-hide-setting-hook-and-toggle"]
plan: "plan.md"
spec: "../../specs/agents/requirements/hide-disabled-profiles-nav.md"
---

# Task 02: Filter disabled profiles from the settings left panel

## Acceptance

- The Settings left panel's Agents tree (`AgentsGroup`) hides a profile
  whose `enabled === false` when `useHideDisabledAgentProfilesInNav()`'s
  `hideDisabled` is on.
- Enabled profiles and legacy profiles without the `enabled` field are
  always listed, with the existing `DisabledBadge` convention unchanged.
- The office sidebar's `AgentsSection` is untouched (does not consume the
  hook).

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- \
  components/app-sidebar/sections/settings/settings-tree-render.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/app-sidebar/sections/settings/agents-group.tsx`
- `apps/web/components/app-sidebar/sections/settings/settings-tree-render.test.tsx`
  (add a hoisted `hideAgentProfiles` mock for the new hook, default `false`,
  plus the new filter cases)

## Dependencies

Task 01's hook (`useHideDisabledAgentProfilesInNav`).

## Parallelism

Sequential. Task 03's E2E asserts this filter's behavior.

## Inputs

- Spec: `What` (filter bullet), `Failure modes` (legacy-absence rule),
  and the second/third/sixth scenarios.
- Plan: `Frontend > 3. Settings-tree filter`.
- Existing patterns: `workspaces-group.tsx`'s `WorkspaceIntegrationItems`
  filter (`WORKSPACE_INTEGRATIONS.filter(([slug]) => !hideDisabled || toggleEnabled[slug])`),
  the `hideDisabled` mock pattern in `settings-tree-render.test.tsx`, and the
  existing "SettingsTree agents group" tests.

## Risks

- Filter with `(p.enabled ?? true)`, never `p.enabled` — `undefined` must
  stay visible.
- Keep the `DisabledBadge` (`labelSuffix`) logic intact: it only renders on
  profiles that are actually listed.
- Do not import the hook into `agents-section.tsx` (office sidebar) — office
  rows have no `enabled` field and must not be filtered.

## Output contract

Report the filter behavior, files changed, exact commands and results,
blockers/risks, then mark this task `done` and update its checkbox in
`plan.md`.

## Completion report

- `AgentsGroup` now consumes `useHideDisabledAgentProfilesInNav` and filters
  `agent.profiles` with `!hideDisabled || (p.enabled ?? true)`; legacy
  profiles without the flag stay listed; `DisabledBadge` logic untouched;
  office `AgentsSection` untouched.
- Tests: extended `settings-tree-render.test.tsx` with a hoisted
  `hideAgentProfiles` mock (default `false`) and three filter cases; the
  suite (25 tests) passes, including the pre-existing badge test.
- Blockers: none.
