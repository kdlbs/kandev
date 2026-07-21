---
id: "03-e2e-and-verification"
title: "Archive preference E2E and verification"
status: done
wave: 3
depends_on: ["02-frontend-behavior"]
plan: "plan.md"
spec: "../../specs/tasks/archive-confirmation.md"
---

# Task 03: Archive Preference E2E and Verification

## Acceptance

- Desktop and mobile Playwright coverage prove confirmation-free archive from reachable archive controls.
- Focused tests, backend formatting/lint, frontend typecheck/lint, and relevant builds pass.
- The spec and plan statuses match the shipped implementation.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/task/archive-confirmation-preference.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-archive-confirmation-preference.spec.ts
make -C apps/backend fmt
make -C apps/backend lint
cd apps/web && pnpm run typecheck && pnpm run lint
```

## Files Likely Touched

- `apps/web/e2e/tests/task/archive-confirmation-preference.spec.ts`
- `apps/web/e2e/tests/task/mobile-archive-confirmation-preference.spec.ts`
- `docs/specs/tasks/archive-confirmation.md`
- `docs/plans/archive-confirmation-preference/*.md`

## Inputs

Completed backend/frontend tasks, mobile-parity skill, existing sidebar and mobile task switcher page objects.

## Output Contract

Report desktop/mobile behavior exercised, commands and results, files touched, blockers, residual risks, and final spec/plan status.
