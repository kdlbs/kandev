---
id: "02-navigation-e2e"
title: "Integration card navigation E2E coverage"
status: complete
wave: 2
depends_on: ["01-card-navigation"]
plan: "plan.md"
spec: "../../specs/integrations/clickable-integration-cards.md"
---

# Task 02: Integration card navigation E2E coverage

## Acceptance

- Desktop Playwright coverage clicks a native integration card body and reaches
  the workspace-scoped integration settings page.
- Mobile Playwright coverage taps the same card body with the configured
  `mobile-chrome` project and reaches the same destination.
- The tests use stable card or route selectors and do not regress the existing
  switch-isolation coverage.

## Verification

From the repository root, the managed runner must build the production
frontend before each project run:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run --project chromium tests/integrations/integrations-index-card-navigation.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-integrations-index-card-navigation.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/integrations/integrations-index-card-navigation.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-integrations-index-card-navigation.spec.ts`

## Dependencies

Task 01.

## Parallelism

`sequential`.

## Inputs

- `docs/specs/integrations/clickable-integration-cards.md`, Scenarios.
- `docs/plans/clickable-integration-cards/plan.md`, E2E Tests.
- Existing fixtures from `apps/web/e2e/fixtures/test-base.ts`.
- Existing layout and toggle tests in `apps/web/e2e/tests/integrations/`.

## Output contract

Report both exact managed-runner commands, discovered test counts, pass/fail
results, build and teardown evidence, and synchronized task and plan status.

## Results

Added desktop and mobile card-body navigation coverage. Both tests click or tap
the lower card body away from the title, description, and native switch, then
assert the workspace-scoped integration URL and destination heading.

Managed verification passed:

```text
pnpm e2e:run --project chromium tests/integrations/integrations-index-card-navigation.spec.ts
1 passed

pnpm e2e:run --project mobile-chrome tests/integrations/mobile-integrations-index-card-navigation.spec.ts
1 passed

pnpm e2e:run --project chromium tests/integrations/integrations-index-enabled-toggle.spec.ts
1 passed
```
