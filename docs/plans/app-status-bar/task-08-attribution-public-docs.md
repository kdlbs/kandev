---
id: app-status-bar-08
title: Ship attribution and public records
status: done
wave: 3
depends_on: [app-status-bar-03]
plan: docs/plans/app-status-bar/plan.md
---

# Ship attribution and public records

## Inputs

[Spec: attribution](../../specs/ui/app-status-bar.md#attribution); exact public slot contract; Orca revision `d9d939a33b5858495ffb33489a952f1ac9293610` and MIT license.

## Files

- `docs/public/plugins-authoring.md`, `docs/public/plugins.md`, `docs/public/operations.md`, `docs/features.md`
- `docs/decisions/0017-resource-metrics-sampling.md`
- `apps/web/components/app-status-bar/app-status-bar.tsx`
- `apps/web/scripts/generate-licenses.ts`, `apps/web/lib/types/system.ts`, generated `apps/web/generated/licenses.json`
- `apps/web/e2e/tests/system/licenses-page.spec.ts`

## Acceptance

1. Public authoring docs show both slot registrations, exact props, one-presentation lifecycle, mobile adaptation, and full-bleed trigger responsibility.
2. Operations/features and decision 0017 name Status bar/drawer semantics and phone drawer-only metric subscription.
3. Shipped licenses manifest contains one pinned Orca source notice with full MIT text; implementation has one focused source comment; license E2E finds it.

## Verification

```sh
cd apps && pnpm --filter @kandev/web licenses:gen
make build-web
cd apps/web && pnpm e2e:run tests/system/licenses-page.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Output contract

Report documentation/decision changes, regenerated artifact status, attribution proof, tests, blockers, and task status.
