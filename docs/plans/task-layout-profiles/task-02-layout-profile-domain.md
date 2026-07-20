---
id: "02-layout-profile-domain"
title: "Layout profile domain"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 02: Layout Profile Domain

## Acceptance

- Pure helpers define built-in templates, validate reusable layouts, resolve the effective default, and perform immutable profile mutations.
- Editor-created layouts allow exactly one Agent and at most one of each reusable optional panel, with no empty groups.
- Invalid or legacy layouts are identified without mutating their stored payloads.

## Files likely touched

- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`
- `apps/web/lib/state/layout-manager/constants.ts`
- `apps/web/components/task/layout-preset-selector.tsx`

## Dependencies

None.

## Inputs

- Spec `What`, `Data model`, and legacy failure-mode requirements.
- Existing `LayoutState`, `PANEL_REGISTRY`, `getPresetLayout`, and `SavedLayout` types.

## Verification

```bash
pnpm --filter @kandev/web test -- --run lib/layout/layout-profiles.test.ts
```

Run from `apps`.

## Output contract

Report helper contracts, tests run, files changed, blockers, and risks; mark this task and its plan item done.
