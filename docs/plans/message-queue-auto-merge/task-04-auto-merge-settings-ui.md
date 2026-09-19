---
id: "04-auto-merge-settings-ui"
title: "Add automatic merge switch"
status: completed
wave: 4
depends_on: ["03-auto-merge-settings"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-auto-merge.md"
---

# Task 04: Add automatic merge switch

## Acceptance

1. Frontend settings types and the Message Queue card load a default-on
   `auto_merge_enabled` draft and save/discard it independently through the
   shared settings contributor without resetting capacity or manual merge.
2. A localized **Automatically merge consecutive messages** switch explains
   same-source fallback behavior, is admin-editable even under only a capacity
   environment lock, and is read-only for members. No queue-panel control is
   added.
3. Component/API tests cover load, auto-only PATCH, dirty/discard/save,
   concurrent response reconciliation, failure, environment lock, member
   permission, and mobile-safe rendering; typecheck, focused lint, pseudo
   generation, and i18n gates pass.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/settings/system/message-queue-settings.test.tsx lib/api/domains/settings-api.test.ts && cd web && pnpm run typecheck && pnpm exec eslint --max-warnings 0 components/settings/system/message-queue-settings.tsx components/settings/system/message-queue-settings.test.tsx lib/api/domains/settings-api.ts lib/api/domains/settings-api.test.ts lib/types/system.ts && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/lib/api/domains/settings-api.test.ts`
- `apps/web/components/settings/system/message-queue-settings.tsx`
- `apps/web/components/settings/system/message-queue-settings.test.tsx`
- `apps/web/src/locales/en/system.json`
- `apps/web/src/locales/pseudo/system.json`

## Dependencies

Task 03.

## Parallelism

Sequential.

## Inputs

- Spec: `What`, `API surface`, `Permissions`, `Responsive and mobile behavior`,
  and settings/mobile scenarios.
- Plan: `Frontend` and frontend test rows.
- Mobile contract: existing Task Behavior entry point; same Message Queue card;
  one-column presentation; `settings-scroll-container` is the only scroll
  owner; touch-safe switch; shared floating save bar; desktop/mobile capability
  parity.
- Existing pattern: `MergeToggleFields`, the two-draft save coordinator, and
  `message-queue-settings.test.tsx` save-provider harness.

## Risks

- The current component is near file/function lint limits. Extract the new
  presentation block and keep draft derivation readable.
- New copy must use `t()` at render time and pass both i18n checks.
- An async save response must not overwrite a newer local toggle change.

## Output contract

Report summary, files changed, exact commands/outcomes, blockers, risks, and
update this task plus `plan.md` status in the same conversation.

## Results

- Added the third `auto_merge_enabled` draft, independent partial PATCH,
  discard, and stale-response reconciliation to the shared Message Queue save
  contributor.
- Added the localized default-on switch and same-source fallback explanation
  with a 44px mobile touch target and no new scroll owner.
- Verified 34 focused component/API tests, web typecheck, zero-warning focused
  ESLint, pseudo-locale generation, `i18n:check`, and `i18n:ratchet`.
