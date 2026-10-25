---
id: "01-post-restore-session-reconciliation"
title: "Repair missing Agent panels at restore completion"
status: done
wave: 1
depends_on: []
parallelism: sequential
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 01: Repair Missing Agent Panels at Restore Completion

## Acceptance

- A completed desktop environment restore with an active session always has a
  live Agent panel.
- An empty center group receives and activates Agent before restoration ends.
- A valid saved active tab remains active while Agent is restored beside it.
- Existing stale-session replacement remains atomic and sibling session tabs
  remain inactive.
- The desktop task-switch regression never displays the empty-group watermark.

## TDD sequence

1. RED: reproduce missing Agent restoration with an empty center-group fake.
2. RED: prove restoring beside active Plan must not activate Agent.
3. GREEN: add the minimum post-restore reconciliation and call it from stale
   session replacement.
4. REFACTOR: centralize target-group and activation decisions without changing
   existing handoff behavior.
5. E2E: assert the visible desktop task-switch outcome.

## Files

- `apps/web/lib/state/dockview-env-switch.ts`
- `apps/web/lib/state/dockview-env-switch.test.ts`
- `apps/web/e2e/tests/layout/saved-layout-session-isolation.spec.ts`
- `docs/specs/ui/requirements/task-layout-profiles.md`

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/dockview-env-switch.test.ts
cd apps/web && pnpm e2e:run tests/layout/saved-layout-session-isolation.spec.ts
make fmt
make typecheck test lint
```

## Result

- A normal slow-path environment restore now repairs a missing active Agent
  panel synchronously before restoration completes.
- Empty center groups activate the repaired Agent; a saved active Plan remains
  active.
- Saved maximize overlays intentionally excluding Agent remain unchanged.
- Unit regressions, the focused production Playwright spec, formatting,
  typecheck, the full test suite, and lint passed on 2026-07-28.
