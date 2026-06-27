---
id: "11-e2e-strict-qa"
title: "E2E strict QA"
status: done
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

## Verification Completed

- `rtk make fmt` passed.
- `rtk make typecheck test lint` passed.
- `rtk pnpm --dir apps/web e2e:docker --shards 3` passed with strict WS
  accounting:
  - shard 1: 363 passed / 1 flaky retry
  - shard 2: 370 passed
  - shard 3: 369 passed
- `rtk pnpm --dir apps/web e2e:docker --project mobile-chrome` passed 78
  mobile Docker tests with strict WS accounting.
- `rtk pnpm --dir apps/web e2e:docker --project routing` passed 7 routing
  Docker tests with strict WS accounting.
- `rtk env KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e --project=containers`
  passed 99 container-backed tests / 1 skipped after building the Linux
  `mock-agent` helper required by the Docker/SSH executor project.

Focused regression checks run during final QA:

- `rtk pnpm --dir apps/web e2e:docker e2e/tests/pr/pr-multi-popover.spec.ts`
  passed 3 desktop Docker tests.
- `rtk pnpm --dir apps/web e2e:docker e2e/tests/pr/pr-detail-auto-show.spec.ts e2e/tests/pr/pr-detail-dedup.spec.ts e2e/tests/pr/pr-detail-manual-open.spec.ts e2e/tests/pr/pr-detection.spec.ts e2e/tests/pr/pr-multi-popover.spec.ts`
  passed 13 desktop Docker tests.
- `rtk env KANDEV_E2E_CONTAINERS=1 pnpm --dir apps/web e2e --project=containers e2e/tests/ssh/recovery.spec.ts:97`
  passed after the metadata smoke test was changed to wait for the SSH
  `ExecutorRunning` row it asserts instead of waiting on unrelated chat-session
  completion.

Residual risks:

- No unresolved Task 11 blockers. The containers project keeps one destructive
  Docker recovery test skipped by design.

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
