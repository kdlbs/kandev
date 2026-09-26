---
id: "01-responsive-right-width"
title: "Separate responsive and manual right-column widths"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 01: Separate Responsive and Manual Right-Column Widths

## Acceptance

- A right column without a genuine sash-resize marker recomputes from the
  current Dockview width on reload, environment switch, and container resize.
- A genuine right-sash resize stores its raw requested width per environment;
  it survives reload and returns after a temporary narrow-screen clamp.
- Existing serialized environment layouts with no marker are treated as
  responsive defaults; mobile, tablet, layout profiles, and the global left
  sidebar are unchanged.

## TDD Sequence

1. RED: prove a saved 450px automatic right column restores at the laptop
   ratio, not at 450px, and that a container expand returns it to the wider
   ratio.
2. RED: prove a manual marker wins over the responsive target and its raw
   width returns after a laptop clamp.
3. GREEN: add the environment-scoped marker, write it only on a genuine sash
   mouseup, and route restore/enforcement/fast-switch targets through it.
4. REFACTOR: centralize the right-target selection so all Dockview entry
   points use the same automatic-versus-manual rule.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  lib/state/dockview-layout-builders-fixups.test.ts \
  lib/state/dockview-env-switch-pinned.test.ts \
  lib/state/dockview-store.test.ts
cd apps/web && pnpm e2e:run tests/layout/pane-resize-right.spec.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/local-storage.ts`
- `apps/web/lib/local-storage.test.ts`
- `apps/web/components/task/dockview-layout-setup.ts`
- `apps/web/components/task/dockview-layout-setup.test.ts`
- `apps/web/lib/state/dockview-layout-builders.ts`
- `apps/web/lib/state/dockview-layout-builders-fixups.test.ts`
- `apps/web/lib/state/dockview-env-switch.ts`
- `apps/web/lib/state/dockview-env-switch-pinned.test.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-store.test.ts`
- `apps/web/e2e/tests/layout/pane-resize-right.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. Persistence metadata, target selection, and browser coverage share
the same behavior and must land together.

## Inputs

- `docs/specs/ui/requirements/task-layout-profiles.md`, especially Persistence guarantees
  and the display-switch scenarios.
- `docs/plans/dockview-responsive-right-width/plan.md`.
- Existing user-resize persistence coverage in
  `apps/web/e2e/tests/layout/pane-resize-right.spec.ts`.

## Output contract

Report the RED failures, the automatic/manual target rules, exact files
changed, targeted unit/E2E/typecheck results, and task/plan status updates.
