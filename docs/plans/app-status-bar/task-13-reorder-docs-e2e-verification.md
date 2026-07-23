---
id: app-status-bar-13
title: Reorder docs, E2E, and verification
status: done
wave: 8
depends_on: [app-status-bar-12]
plan: docs/plans/app-status-bar/plan.md
spec: docs/specs/ui/app-status-bar.md
---

# Reorder docs, E2E, and verification

## Inputs

[App status bar spec](../../specs/ui/app-status-bar.md),
[plugin spec](../../specs/plugins/spec.md), task 12, public plugin docs, and the
real in-tree plugin fixture.

## Files likely touched

- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`, `docs/public/plugins.md`
- `docs/public/operations.md`, `docs/features.md`
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/web/e2e/tests/layout/app-status-bar.spec.ts`
- `apps/web/e2e/tests/plugins/plugins.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-status-drawer.spec.ts`

## Acceptance

1. Public/frozen docs explain opaque registration items, default-side slot
   semantics, Cmd/Ctrl plus mouse drag, backend persistence, disabled-plugin
   restoration, and phone order; they do not promise keyboard/touch reordering.
2. Real-host desktop E2E moves a fixture contribution across the spacer, reloads,
   restarts the backend if the fixture supports it, and verifies persisted order;
   plugin disable/enable restores position without a reload or duplicate mount.
3. Mobile E2E verifies the corresponding vertical order, 44 px rows, one scroll
   owner, no drag affordance/listener effect, no persistent bar, and existing
   navigation/focus return.
4. A 1x geometry assertion covers the separator and vertical centers. Simplify,
   strict review, docs validation, focused tests, and full verification are green.

## Verification

```sh
cd apps/web && pnpm e2e:run tests/layout/app-status-bar.spec.ts tests/plugins/plugins.spec.ts
cd apps/web && pnpm e2e:run tests/plugins/mobile-status-drawer.spec.ts -- --project=mobile-chrome
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make fmt
VITEST_MAX_WORKERS=2 TMPDIR=/tmp make typecheck test lint
```

## Dependencies

Task 12.

## Output contract

Report desktop/mobile scenarios, persistence proof, geometry evidence, docs and
review results, exact full-verification commands, and blockers. Mark this task,
the plan, and the spec shipped only when every acceptance item passes.
