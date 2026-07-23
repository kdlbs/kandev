---
id: app-status-bar-06
title: Prove live plugin slots on desktop
status: done
wave: 3
depends_on: [app-status-bar-01, app-status-bar-03]
plan: docs/plans/app-status-bar/plan.md
---

# Prove live plugin slots on desktop

## Inputs

[Spec: plugin slots](../../specs/ui/app-status-bar.md#plugin-slots); tasks 01 and 03; plugin fixture and existing plugin E2E setup.

## Files

- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/web/e2e/tests/plugins/plugins.spec.ts`
- `apps/web/e2e/pages/session-page.ts`
- Supporting fixture assertions only when needed.

## Acceptance

1. Fixture registers visible left and right contributions and exposes presentation plus active-context values for host tests.
2. Desktop E2E proves both contributions appear, receive `bar` props, disappear after disable, and reappear after enable without reload.
3. Page object gains an app-status locator; existing chat-status locator semantics remain unchanged.

## Verification

```sh
make build-web
cd apps/web && pnpm e2e:run tests/plugins/plugins.spec.ts
```

## Output contract

Report fixture behavior, E2E result/build command, locator compatibility, blockers, and task status.
