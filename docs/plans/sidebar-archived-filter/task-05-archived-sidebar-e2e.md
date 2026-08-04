---
id: "05-archived-sidebar-e2e"
title: "Prove archived sidebar flows"
status: pending
wave: 4
depends_on: ["04-integrate-archived-rows"]
plan: "plan.md"
spec: "../../specs/ui/sidebar-archived-filter.md"
---

# Task 05: Prove archived sidebar flows

Add browser regressions for archived browsing and live projection updates on
the shipped desktop and phone compositions.

## Acceptance

- Desktop coverage proves Archived: Show lists archived tasks only, an archived
  row opens archived detail with Unarchive, and a later archive event adds one
  live row without duplication.
- Mobile coverage proves the same filter and navigation value from the existing
  task-switcher drawer.
- Mobile assertions cover drawer/popover containment, the internal task-list
  scroll owner, and zero document horizontal overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/task/sidebar-filter.spec.ts -- --grep "archived tasks"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "archived tasks"
```

## Files likely touched

- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/sidebar-filter-popover.ts`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`

## Dependencies

Task 04.

## Parallelism

`sequential` — verifies the completed backend, store, WebSocket, and responsive
UI vertical slice.

## Inputs

- Spec desktop/mobile, live event, and archived navigation scenarios.
- Plan **E2E Tests** and **Mobile design contract**.
- Existing `SidebarFilterPopoverPage`, `apiClient.archiveTask`, archived detail
  top-bar test, and managed E2E runner patterns.

## Risks

- The test must wait for the archived-only request to complete before asserting
  an empty or filtered result.
- The live archive assertion must key by task ID/title and verify a single row,
  so the synthetic archived placeholder cannot mask duplication.
- Rebuild once for the desktop run and use `--no-build` only for the immediately
  following mobile run against the same artifacts.

## Output contract

Report red/green commands, discovered test counts, geometry assertions,
failure artifacts, teardown/cleanup evidence, exact files changed, blockers,
and update this task plus `plan.md` status/results.

## Results

Pending.
