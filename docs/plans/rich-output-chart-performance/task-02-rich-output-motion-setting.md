---
id: "02-rich-output-motion-setting"
title: "Add rich-output motion setting"
status: completed
wave: 2
parallelism: sequential
depends_on: ["01-defer-and-stabilize-charts"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 02: Add rich-output motion setting

## Acceptance

1. Appearance contains one searchable, localized `Animate rich-output charts`
   switch with concise scope/performance copy and a 44-pixel touch target.
2. The preference defaults on, previews immediately, persists only when the
   shared Save changes action succeeds, restores on Reset, survives reload, and
   is owned by the current browser/device rather than the account API.
3. Enabled charts retain the existing Recharts animation style and duration.
   Disabled charts pass `isAnimationActive={false}` to every line/bar series.
4. `prefers-reduced-motion: reduce` disables effective chart animation even
   when the saved switch is on, and a live media-query change updates future
   plots without reload.
5. Desktop and mobile settings remain contained and reachable; existing chart
   axes, tooltips, legends, replay, and file behavior are unchanged.

## TDD order

1. Add failing storage/state tests for default, malformed storage, preview,
   commit, restore, and per-device persistence.
2. Add failing Appearance tests for dirty state, shared Save/Reset, discovery,
   and explanatory copy.
3. Add failing chart tests for enabled, explicit-disabled, and OS
   reduced-motion animation props.
4. Implement storage helpers, UI-slice state/actions, Appearance card/save
   integration, effective motion hook, translations, and chart wiring.
5. Add desktop/mobile E2E for save/reload, 44-pixel geometry, no overflow, and
   actual-Recharts animation/static behavior.
6. Run focused checks and profile the animation-disabled isolated scenario.

## Files

- `apps/web/lib/settings/rich-output-motion.ts`
- `apps/web/lib/settings/rich-output-motion.test.ts`
- `apps/web/lib/settings/constants.ts`
- `apps/web/lib/state/slices/ui/rich-output-motion-actions.ts`
- `apps/web/lib/state/slices/ui/rich-output-motion-actions.test.ts`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/components/settings/rich-output-motion-settings-card.tsx`
- `apps/web/components/settings/rich-output-motion-settings-card.test.tsx`
- `apps/web/components/settings/appearance-settings-state.ts`
- `apps/web/components/settings/appearance-account-sections.tsx`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/general-settings.test.tsx`
- `apps/web/lib/settings-discovery/catalog/preferences.ts`
- `apps/web/lib/settings-discovery/catalog.test.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-block.test.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-motion.ts`
- `apps/web/components/task/chat/messages/kandev/rich-output/chart-motion.test.tsx`
- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`
- `apps/web/e2e/tests/chat/rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts`
- `docs/specs/agents/requirements/agent-rich-output.md`
- `docs/plans/rich-output-chart-performance/plan.md`
- `docs/plans/rich-output-chart-performance/task-02-rich-output-motion-setting.md`

## Targeted commands

Run from `apps/web`:

```text
pnpm exec vitest run lib/settings/rich-output-motion.test.ts lib/state/slices/ui/rich-output-motion-actions.test.ts components/settings/rich-output-motion-settings-card.test.tsx components/settings/general-settings.test.tsx lib/settings-discovery/catalog.test.ts components/task/chat/messages/kandev/rich-output/chart-block.test.tsx components/task/chat/messages/kandev/rich-output/chart-motion.test.tsx
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:raw --project=chromium e2e/tests/chat/rich-output.spec.ts e2e/tests/settings/settings-manual-save.spec.ts
pnpm e2e:raw --project=mobile-chrome e2e/tests/chat/mobile-rich-output.spec.ts e2e/tests/settings/mobile-general-settings.spec.ts
pnpm run lint:e2e-sleeps -- e2e/tests/chat/rich-output.spec.ts e2e/tests/chat/mobile-rich-output.spec.ts e2e/tests/settings/settings-manual-save.spec.ts e2e/tests/settings/mobile-general-settings.spec.ts
```

Then run `git diff --check` from the repository root.

## Risks

- A per-device setting must never leak into backend user-setting requests.
- Preview/save/discard behavior must stay consistent with theme and settings
  menu mode, including edits made while Save is in flight.
- OS reduced motion is an accessibility override, not a saved preference; the
  switch continues to represent the device preference clearly.
- User-facing text must describe only rich-output charts, not imply that every
  Kandev animation is disabled.

## Results

Added a default-on device-local Appearance preference with preview, shared
Save/Reset coordination, reload persistence, settings discovery, localized
copy, and a 44-pixel switch target. Saving only this preference does not call
the account settings API. The effective motion hook shares one live media-query
subscription and always lets `prefers-reduced-motion` override the device
choice.

Recharts lines and bars retain their native animation defaults when effective
motion is enabled. The disabled browser sample rendered the line without an
animated dash attribute and recorded 1,758 chart mutations. Unit coverage
exercises storage fallback, state transitions, OS changes, and chart props;
desktop and mobile E2E verify animation/static geometry, persistence,
containment, and touch sizing.

Final verification passed: 66 focused unit tests, TypeScript typecheck, full
workspace lint, `i18n:check`, `i18n:ratchet`, E2E sleep lint, the E2E production
build, 8 desktop E2E cases, 7 mobile E2E cases, and `git diff --check`.
