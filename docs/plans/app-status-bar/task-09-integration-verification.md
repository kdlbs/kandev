---
id: app-status-bar-09
title: Integrate and verify app status surface
status: done
wave: 5
depends_on: [app-status-bar-05, app-status-bar-07, app-status-bar-08]
plan: docs/plans/app-status-bar/plan.md
---

# Integrate and verify app status surface

## Inputs

Tasks 01–08; [feature spec](../../specs/ui/app-status-bar.md); task reports and changed-file map.

## Files

No planned source file. Update task/plan status and only fix verified integration defects in their owning files.

## Acceptance

1. Desktop/tablet bar, phone drawer, connection, preference-gated metrics, plugin hot-toggle, chat-local status, fixed overlays, and mobile navigation meet spec scenarios together.
2. No duplicate metrics subscription or plugin mount occurs across breakpoint/drawer transitions.
3. Formatting, typecheck, tests, lint, focused desktop/mobile E2E, public-doc validation, and license proof pass in required order.

## Verification

```sh
make fmt
make typecheck test lint
cd apps/web && pnpm e2e:run tests/layout/app-status-bar.spec.ts tests/plugins/plugins.spec.ts
cd apps/web && pnpm e2e:run tests/plugins/mobile-status-drawer.spec.ts -- --project=mobile-chrome
cd apps/web && pnpm e2e:run tests/system/licenses-page.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Output contract

Report all commands/results, manual viewport matrix, integration fixes, residual risks, and final task/plan statuses.
