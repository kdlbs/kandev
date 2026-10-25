---
id: "05-archived-sidebar-e2e"
title: "Prove archived sidebar flows"
status: done
wave: 4
depends_on: ["04-integrate-archived-rows"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-archived-filter.md"
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
cd apps/web && pnpm e2e:run --host tests/task/sidebar-filter.spec.ts -- --grep "offers archived as a filter dimension"
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "offers archived as a filter dimension"
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

- Desktop regression `sidebar-filter.spec.ts` passes: Archived is available as
  a filter dimension, only archived rows remain, a live archive event adds one
  row, and selecting the archived row opens detail with Unarchive.
- Mobile regression `mobile-sidebar-views.spec.ts` passes: the same filter and
  archived row/badge are usable from the task-switcher drawer; drawer and
  popover geometry stay inside the Pixel 5 viewport, the task list remains the
  scroll owner, and document horizontal overflow is zero.
- Verification:
  - `pnpm e2e:run --host --no-build tests/task/sidebar-filter.spec.ts -- --grep "offers archived as a filter dimension"` — 1 passed.
  - `pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-sidebar-views.spec.ts -- --grep "offers archived as a filter dimension"` — 1 passed.
  - `pnpm --filter @kandev/web build` and the managed backend build used by the
    host E2E runner — passed (only existing bundle/codesign warnings).
