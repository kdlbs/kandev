---
id: app-status-bar-07
title: Prove mobile Status drawer parity
status: done
wave: 4
depends_on: [app-status-bar-04, app-status-bar-06]
plan: docs/plans/app-status-bar/plan.md
---

# Prove mobile Status drawer parity

## Inputs

[Spec scenarios](../../specs/ui/app-status-bar.md#scenarios); mobile UI language; tasks 04 and 06; configured Pixel 5 project.

## Files

- `apps/web/e2e/tests/plugins/mobile-status-drawer.spec.ts`
- Existing mobile helpers/page objects only when strictly needed.

## Acceptance

1. Mobile E2E installs fixture, opens Status from Home, task bottom navigation, and PageTopbar route, then sees built-ins plus plugin contribution with `mobile-drawer` props.
2. Test proves dismiss/focus return, 44 px trigger, safe-area clearance, one internal scroll owner, zero document horizontal overflow, and no persistent second footer.
3. Existing task navigation still works after drawer dismissal.

## Verification

```sh
make build-web
cd apps/web && pnpm e2e:run tests/plugins/mobile-status-drawer.spec.ts -- --project=mobile-chrome
```

## Output contract

Report mobile assertions, screenshot/rendered check, build/E2E result, blockers, and task status.
