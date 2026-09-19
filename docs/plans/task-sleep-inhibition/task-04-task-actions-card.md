---
id: "04-task-actions-card"
title: "Task Actions sleep setting"
status: done
wave: 4
depends_on: ["03-system-api-wiring"]
plan: "plan.md"
spec: "../../specs/platform/requirements/task-sleep-inhibition.md"
---

# Task 04: Task Actions sleep setting

## Acceptance

- Task Actions renders a localized, self-documenting install-wide sleep-prevention card with saved/draft state in the shared floating-save workflow.
- Administrators can save while members are read-only; configured, active, unsupported, unavailable-service, request-failed, loading, and save-failure states are distinguishable.
- The unchanged settings-page mobile composition keeps wrapping copy and controls within its single scroll surface, with no desktop-only interaction.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
```

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/sleep-inhibition-settings.test.tsx
```

```bash
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/settings/sleep-inhibition-settings.tsx`
- `apps/web/components/settings/sleep-inhibition-settings.test.tsx`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/lib/types/system.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`

## Dependencies

Task 03.

## Parallelism

Parallel-safe only with Task 06: the frontend and public-doc file sets are disjoint and neither changes schemas, generated contracts, lockfiles, or package configuration. Execution remains sequential unless the user explicitly authorizes subagents.

## Inputs

- Spec sections: What, API surface, Permissions, Scenarios.
- Plan section: Task Actions setting card.
- Mobile exemplar: existing cards in `TaskActionsSettings` and `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`; no new overlay or navigation surface is introduced.

## Risks

- Do not place this install-wide value in the user-settings Zustand slice.
- Saving an enabled value on an unsupported host is valid; the configured switch and inactive capability warning must not contradict each other.

## Output contract

Report UI/API behavior, mobile contract, localization updates, files changed, exact tests, blockers/risks, and synchronized task/plan status.

## Results

- Added the localized install-wide Task Actions card, API wire types/client, and
  local saved/draft contributor state without mutating the shared user-settings slice.
- Members retain the configured value but cannot edit; admins participate in the
  shared floating Save changes flow. Capability, active, unavailable, unsupported,
  loading, and save-failure states are rendered from the dedicated System response.
- Focused Vitest passed (2 files, 7 tests), targeted ESLint passed with zero warnings,
  TypeScript reported no errors, and pseudo-locale/i18n checks and ratchet passed.
