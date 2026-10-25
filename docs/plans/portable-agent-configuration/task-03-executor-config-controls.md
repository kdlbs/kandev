---
id: "03-executor-config-controls"
title: "Add executor configuration controls"
status: done
wave: 2
depends_on: ["01-portable-config-catalog"]
plan: "plan.md"
spec: "../../specs/agents/requirements/portable-agent-configuration.md"
---

# Task 03: Add executor configuration controls

## Acceptance

- Local Docker, SSH, and Sprites profile forms show each agent's independent bundle checkboxes inside that agent's expanded row and persist `agent_config_bundles` through the shared save action.
- Visible copy and a warning icon explain secrets, hooks, commands, paths, target replacement, and the SSH shared-home risk.
- Fine pointers use a tooltip, coarse pointers use a drawer, and every touch control meets the 44 CSS pixel target.

## TDD scenarios

1. RED: Add API-client and parser tests for the bundle response and saved JSON list.
2. RED: Add component tests for independence, dirty state, unavailable files, saved missing files, and serialization.
3. RED: Add pointer-mode tests for tooltip focus and coarse-pointer drawer behavior.
4. GREEN: Add the API types, profile state, save integration, controls, and translated copy.
5. REFACTOR: Extract the warning control and bundle list from the existing authentication card.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/profile-edit lib/api/domains`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check`
- `cd apps/web && pnpm run i18n:ratchet`
- `cd apps && pnpm --filter @kandev/web lint`
- `git diff --check`

## Files likely touched

- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/components/settings/profile-edit/executor-profile-baselines.ts`
- `apps/web/components/settings/profile-edit/remote-credentials-card.tsx`
- `apps/web/components/settings/profile-edit/remote-credentials-card.test.tsx`
- `apps/web/components/settings/profile-edit/profile-runtime-sections.tsx`
- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/app/settings/executors/new/[type]/page.tsx`
- `apps/web/src/locales/en/executors.json`
- `apps/web/src/locales/pseudo/executors.json`
- `apps/web/src/locales/pt-pt/executors.json`
- `apps/web/src/locales/zh-cn/executors.json`
- `apps/web/src/locales/zh-hk/executors.json`
- `apps/web/src/locales/zh-tw/executors.json`

## Dependencies

- Task 01 supplies the HTTP response contract and stable bundle IDs.

## Parallelism

Parallel-safe with Task 02 after Task 01.
This task owns frontend settings files and locale catalogs.

## Inputs

- The catalog response from Task 01.
- The existing remote-credentials card.
- The settings save coordinator.
- `SleepInhibitionInfoTooltip` and `useTouchDrawer` as interaction examples.

## Output contract

Report desktop behavior, mobile behavior, accessibility, translations, RED evidence, GREEN evidence, and test results.

## Risks

- The existing remote-credentials card is close to the frontend file-size limit.
- A tooltip-only warning does not meet the visible settings guidance.
- A saved missing source must remain removable.

## Results

Implemented independent authentication and configuration controls, persistent
bundle IDs, unavailable-source hints, fine-pointer tooltip/coarse-pointer
drawer behavior, and translated copy. Configuration choices are now rendered
inside the matching expanded agent row instead of a separate global section.
Focused frontend tests passed with 25 tests. Web typecheck, lint, i18n check,
and i18n ratchet passed.
