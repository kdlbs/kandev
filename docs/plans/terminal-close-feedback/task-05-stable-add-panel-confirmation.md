---
id: "05-stable-add-panel-confirmation"
title: "Stable compact add-panel confirmation"
status: done
wave: 5
depends_on: ["04-consistent-confirmation-entrypoints"]
plan: "plan.md"
spec: "../../specs/ui/requirements/terminal-close-feedback.md"
parallelism: sequential
---

# Task 05: Stable compact add-panel confirmation

## Root cause

Radix dropdown items take focus on pointer movement. The nested non-modal popover interpreted that
owning-menu focus as outside interaction and closed. Separately, the row's in-flow 40px close button
expanded a standard 28px desktop menu row to 48px.

## Intent

Keep localized confirmation reachable across its parent menu and restore standard desktop menu
density without weakening the coarse-pointer touch target.

## Acceptance

- Pointer movement across the open add-panel menu does not dismiss its terminal confirmation.
- Cancel and Close terminal remain reachable after that pointer movement.
- Focus or interaction outside both surfaces still dismisses the non-modal popover.
- Fine-pointer terminal rows match adjacent standard menu-row height.
- Coarse-pointer terminal rows and their close controls retain a 44px active dimension.

## Files

- `apps/web/components/task/close-terminal-confirm-popover.tsx`
- `apps/web/components/task/terminal-reopen-menu.tsx`
- `apps/web/e2e/tests/terminal/terminal-dockview-ui.spec.ts`

## Verification

```bash
cd apps/web
pnpm e2e:run --host --no-build --shards 1 tests/terminal/terminal-dockview-ui.spec.ts -- --grep "destroy button on a reopen-menu row"
cd ../..
make build-web
cd apps/web
pnpm e2e:run --host --no-build --shards 1 tests/terminal/terminal-dockview-ui.spec.ts -- --grep "destroy button on a reopen-menu row"
```

## Results

- RED reproduced popover action detachment after pointer-driven menu focus; the focused production
  E2E passed after the owning-menu focus boundary was added.
- RED measured the terminal row at 48px beside a 28px standard row; absolute close-control
  positioning restored both to 28px on fine-pointer layouts.
- Isolated Firefox inspection confirmed the popover remains visible after pointer travel and the
  compact row geometry is aligned.
- Production web build, typecheck, targeted ESLint, i18n ratchet, E2E sleep ratchet, and focused
  Chromium E2E passed.
