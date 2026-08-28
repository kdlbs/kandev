---
id: "01-compact-searchable-picker"
title: "Implement the compact searchable picker"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 01: Implement the compact searchable picker

Replace the always-visible native 50-option selector with compact latest and
Kandev-default quick choices plus an explicit searchable browser. Keep the
existing target preview, default reset, approval, and job lifecycle callbacks.

## Acceptance

- Opening the dialog with a long catalogue shows the status summary and quick
  choices, not the full version history.
- Browsing exposes a searchable list with latest, active, and default markers;
  selecting a result previews that exact target.
- Desktop and mobile preserve the same selection behavior, with one contained
  scroll owner and touch-sized rows on mobile.
- Existing locales remain complete and the focused desktop/mobile tests pass.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/agent-runtime-update-control.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run --project chromium e2e/tests/settings/agent-runtime-update.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/settings/mobile-agent-runtime-update.spec.ts
```

## Files likely touched

- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/components/settings/runtime-version-picker.tsx`
- `apps/web/components/settings/agent-runtime-update-control.test.tsx`
- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `apps/web/src/locales/en/agents.json`
- `apps/web/src/locales/pt-pt/agents.json`
- `apps/web/src/locales/zh-cn/agents.json`
- `apps/web/src/locales/zh-hk/agents.json`
- `apps/web/src/locales/zh-tw/agents.json`
- `docs/specs/agents/requirements/runtime-updates.md`
- `docs/plans/managed-runtime-version-picker/plan.md`
- `docs/plans/managed-runtime-version-picker/task-01-compact-searchable-picker.md`

## Dependencies

None.

## Parallelism

Sequential.

## Inputs

- Option 1 selected by the user: compact summary, quick latest/default
  choices, explicit browse action, searchable complete catalogue.
- Existing `AgentRuntimeUpdateControl` callbacks and desktop/mobile surface.
- Mobile parity contract: one drawer body scroll owner and 44px touch rows.

## Output contract

Update the picker, localized copy, focused component/E2E coverage, and these
plan results. Report exact commands, changed files, and any remaining browser
verification blockers.

## Results

- Implemented the compact quick-choice picker and on-demand searchable version browser.
- Preserved preview/default callbacks and updated desktop/mobile selectors, rollback, reset, and status assertions.
- Added localized copy and regenerated the pseudo catalogue.
- Component test, typecheck, i18n check, changed-file lint, desktop E2E (15), mobile E2E (5), and `git diff --check` all pass.
- Traditional Chinese generation was blocked by the pre-existing `dynamicProfileSettings` residual warning; equivalent generated values were added manually and the catalog checks pass.
- Follow-up compactness refinement keeps the initial desktop dialog and mobile
  drawer free of body overflow, while long streamed output remains scrollable.
- Fresh desktop and mobile screenshots were captured, inspected, mapped in the
  PR asset manifest, and compressed before publication.
