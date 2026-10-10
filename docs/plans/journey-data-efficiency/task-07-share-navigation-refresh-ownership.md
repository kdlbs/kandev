---
id: "07-share-navigation-refresh-ownership"
title: "Share task and board refresh ownership"
status: complete
wave: 7
depends_on:
  - "05-bound-route-boot-data"
  - "06-hydrate-turns-by-message-window"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-JOURNEY-LOADING-003
acceptance_criteria:
  - AC-PLATFORM-JOURNEY-LOADING-003.1
  - AC-PLATFORM-JOURNEY-LOADING-003.2
  - AC-PLATFORM-JOURNEY-LOADING-003.3
  - AC-PLATFORM-JOURNEY-LOADING-003.4
  - AC-PLATFORM-JOURNEY-LOADING-003.5
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 07: Share task and board refresh ownership

## Summary

Multiple task/session-list and board consumers issue one request per refresh generation, including one shared boot-to-live gap repair where necessary.

## In scope

Move compact task-session and workflow snapshot reads behind store-scoped resource owners. Reuse TaskNavigationReads and existing resource patterns rather than process-global ID maps.
Seed initial values/freshness from boot. Coalesce initial gap repair, reconnect, foreground, and mutation invalidations across route and hook consumers. Keep one in-flight request and at most one trailing refresh per key.
Remove duplicate fetch ownership from route enrichment and per-hook reconnect effects. Retain account/workspace generations, abort semantics, live-event revision merges, and bounded navigation recovery.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- Multiple task/session-list and board consumers issue one request per refresh generation, including one shared boot-to-live gap repair where necessary.
- Deferred-response tests prove final release cancellation, other-consumer continuity, no cross-store cache sharing, and no stale task resurrection.
- Boot and SPA navigation preserve existing error/retry and archived/saved-view behavior on desktop and phone.

## ASCII UI preview

[UI-01: full composition and states](plan.md#ascii-ui-preview). Applies to the acceptance IDs in this work order.

```text
UI-01 desktop: [Sidebar page <=100 rows] | [Selected board OR task detail]
UI-01 phone:   [Task/workflow picker] -> [One focused board OR task detail]
               [Existing loading / empty / Retry state in its own region]
```

Existing controls and scroll ownership remain. Detail demand follows visibility, not mounting or keyboard focus. Preview spacing is illustrative.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/web && pnpm exec vitest run lib/state/task-navigation-reads.test.ts hooks/use-task-sessions.test.ts hooks/use-workflow-snapshot.test.ts hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts lib/ssr/session-page-state.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/task/journey-boot-loading.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-journey-boot-loading.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/state/task-navigation-reads.ts lib/state/task-navigation-reads.test.ts hooks/use-task-sessions.ts hooks/use-task-sessions.test.ts hooks/use-workflow-snapshot.ts hooks/use-workflow-snapshot.test.ts hooks/domains/kanban/use-all-workflow-snapshots.ts lib/ssr/session-page-state.ts e2e/tests/task/journey-boot-loading.spec.ts e2e/tests/task/mobile-journey-boot-loading.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the web typecheck and targeted ESLint for every changed TS/TSX file after the listed tests. If localized copy changes, run `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web`. Managed E2E commands build fresh assets and enforce resource limits.

## Files likely touched

- `apps/web/lib/state/task-navigation-reads.ts`
- `apps/web/lib/state/task-navigation-reads.test.ts`
- `apps/web/hooks/use-task-sessions.ts`
- `apps/web/hooks/use-task-sessions.test.ts`
- `apps/web/hooks/use-workflow-snapshot.ts`
- `apps/web/hooks/use-workflow-snapshot.test.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/lib/ssr/session-page-state.ts`
- `apps/web/e2e/tests/task/journey-boot-loading.spec.ts`
- `apps/web/e2e/tests/task/mobile-journey-boot-loading.spec.ts`

## Dependencies

[Task 05](task-05-bound-route-boot-data.md), [Task 06](task-06-hydrate-turns-by-message-window.md)

## Risks

Suppressing all initial repairs can miss the boot-to-socket gap. A hook cannot independently abort a request still owned by another consumer.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Implemented store-scoped session-list and workflow-snapshot owners. Route and
hook consumers share requests, initial gap repair, and bounded refreshes.
Deferred-response tests passed for cancellation, other-consumer continuity,
separate stores, retired generations, revision merges, and tombstones. The
combined 45-file run passed 665 tests, including route startup and recovery.
Desktop and phone board boot and workflow navigation tests passed. See
[implementation evidence](implementation-evidence.md#task-07-shared-navigation-and-board-reads)
for commands and results.

### Review correction

Shared reads reject obsolete success and error responses after scope changes, disposal, or final release, including loaders that ignore cancellation. Mounted task-session owners guard state, error, and loading commits, preserving successor attempts and live joined consumers. Task-entry enrichment requests lightweight workflow steps instead of a complete snapshot. Board ownership follows visible or adjacent expanded desktop lanes and the focused phone board. New browser cases use real client navigation through a large workflow and twenty-board demand controls. See the [review correction results](implementation-evidence.md#review-remediation).

### PR fixup

PR #4404 fixup keeps response publication and loading settlement under the retained shared-read scope when the initiating hook leaves. Success, failure, retry, final release, and scope-retirement regressions passed. Waiting callers now receive the final queued refresh failure instead of an earlier successful value. See [fixup results](implementation-evidence.md#pr-4404-fixup).


## PR #4404 browser CI remediation

A sibling added during an older membership request triggers a direct compact follow-up before settlement. The regression omits any extra test render and covers the repository-label repair observed on phone.

Task detail and cached route projection consume canonical task records. A live move retains the selected detail after its source board releases the row. Sidebar edit, link, detach, and reparent actions use bounded page records. Task mentions search one authorized page of up to 50 tasks and reject obsolete responses.
