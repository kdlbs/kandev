---
id: "11-e2e-strict-qa"
title: "E2E strict QA"
status: in_progress
wave: 4
depends_on: ["10-remove-zustand-server-state"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 11: E2E Strict QA

## Acceptance

- `KANDEV_E2E_WS_ASSERT=1` is meaningful and green for focused migrated paths.
- Desktop and mobile coverage exists for migrated user-facing workflows.
- Full format, typecheck, test, lint, and focused E2E commands have been run or
  blockers are documented.

## Verification

- `make fmt`
- `make typecheck test lint`
- `cd apps/web && pnpm e2e:docker --shards 3`
- `cd apps/web && pnpm e2e:docker --project mobile-chrome`
- `cd apps/web && pnpm e2e:docker --project routing`
- `cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers`

## Files Likely Touched

- `apps/web/e2e/tests/**`
- `apps/web/e2e/helpers/**`
- `apps/web/e2e/fixtures/**`
- `.github/workflows/e2e-tests.yml`
- `docs/plans/tanstack-query-server-state/plan.md`
- `docs/specs/ui/tanstack-query-server-state.md`

## Dependencies

- Task 10.

## Inputs

- `/e2e`, `/mobile-parity`, and `/verify` skill guidance.
- All domain task summaries.

## Output Contract

Update this task and the plan to `done`, list commands run, summarize residual
risks, and attach failure artifacts or exact blockers if any check cannot run.
