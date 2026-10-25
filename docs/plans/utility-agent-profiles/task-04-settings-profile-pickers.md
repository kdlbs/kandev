---
id: "04-settings-profile-pickers"
title: "Replace utility settings pickers"
status: completed
wave: 2
depends_on: ["01-persist-profile-bindings"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 04: Replace utility settings pickers

## Intent

Replace default/action/custom agent-family and model controls with eligible agent-profile pickers
while preserving shared Settings save/discard behavior and intentional phone composition.

## Acceptance

- The default card and each built-in override select profile IDs only; save, discard, reload, dirty
  indicators, unavailable saved selections, and the `unconfigured` binding state behave
  deterministically.
- Custom create/edit has one required profile picker, and custom rows show the selected profile or
  localized repair copy for a stale binding.
- Desktop and phone layouts keep all controls reachable, use localized copy, preserve one scroll
  owner, and introduce no horizontal document overflow.

## Files likely touched

- `apps/web/lib/api/domains/utility-api.ts`
- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/user-settings-mapper.ts`
- `apps/web/components/settings/utility-agents-section.tsx`
- `apps/web/components/settings/utility-agents-section.test.ts`
- `apps/web/components/settings/utility-sections.tsx`
- `apps/web/components/settings/utility-sections.test.tsx`
- `apps/web/components/settings/utility-dirty.ts`
- `apps/web/components/settings/utility-agent-dialog.tsx`
- `apps/web/components/settings/utility-agent-dialog.test.tsx`
- `apps/web/hooks/domains/settings/use-settings-data.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/zh-cn/settings.json`
- `apps/web/src/locales/pseudo/settings.json`

## Dependencies

Task 01 wire fields and validation contract.

## Parallelism

Parallel-safe with task 02 after task 01: owned files are frontend-only and do not overlap the
runtime implementation. Sequential execution remains the default.

## Inputs

- Spec: Settings-related `What`, API fields, UI failure modes, and desktop/mobile scenarios.
- Plan: `Settings data and API types`, `Utility Agents settings surface`, and `Mobile design
contract`.
- Existing patterns: `ConfigChatAgentSection`, `useHealthyAgentProfiles`,
  `toAgentProfileOption`, the shared Settings save coordinator, current utility card composition,
  and Radix Select phone treatment.

## Verification

Bootstrap once if needed:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run:

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/utility-agents-section.test.ts components/settings/utility-sections.test.tsx components/settings/utility-agent-dialog.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
```

## Output contract

Report picker eligibility and stale-state behavior, desktop/mobile composition, locale artifacts,
files changed, exact test results, blockers, risks, and synchronized task/plan status. Do not change
backend runtime or public docs.

## Results

Replaced utility agent/model controls with eligible profile pickers, added stale/unconfigured repair states, profile-backed custom CRUD, localized copy, and responsive stacked rows. Verified with utility/profile dialog tests, typecheck, lint, i18n check, and ratchet.
