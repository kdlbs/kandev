---
id: "02-responsive-close-feedback"
title: "Responsive optimistic terminal close"
status: done
wave: 2
depends_on: ["01-nonblocking-close-feedback"]
plan: "plan.md"
spec: "../../specs/ui/requirements/terminal-close-feedback.md"
---

# Task 02: Responsive optimistic terminal close

## Intent

Apply the shared immediate-removal contract to tablet and phone without changing their established
navigation or terminal-picker composition.

## Acceptance

- Tablet tabs derive directly from the optimistically filtered terminal list and expose no teardown
  spinner or pending metadata.
- Phone rows disappear with their mounted terminal content before transport settles.
- Phone close remains a visible touch target at least 44px in each active dimension.
- Busy phone close morphs within the target row into touch-sized Cancel and Close terminal actions;
  no nested modal, popover, or drawer is added.
- The final-terminal auto-create guard remains background success bookkeeping and never keeps the
  row visible.

## Files

- `apps/web/components/task/task-right-panel.tsx`
- `apps/web/components/task/mobile/mobile-terminal-pane.tsx`
- `apps/web/components/task/mobile/mobile-terminals-section.tsx`
- `apps/web/components/task/mobile/mobile-terminal-row.tsx`
- Focused tests alongside those files

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- --run components/task/mobile/mobile-terminal-row.test.tsx
cd web && pnpm run typecheck
```

## Results

- Teardown-pending presentation was removed from shared tabs and phone rows.
- Phone retains its 44px close action and existing picker, scroll, safe-area, and selection model;
  required confirmation morphs that action in place.
- Typecheck passed; responsive tests are included in the 4-file, 12-test focused run.
