---
id: "01-coordinate-task-pr-sync-consumers"
title: "Coordinate task PR sync consumers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.1
  - AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.2
  - AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.3
  - AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.4
  - AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.5
  - AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.6
  - AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.7
system_design:
  - ../../specs/integrations/system-design/github-task-pr-sync-coordination.md
---

# Task 01: Coordinate task PR sync consumers

## Summary

Add one shared synchronization resource for each store, workspace context, and
task. Refactor `useTaskPR` to lease that resource while preserving its current
return contract and task PR store behavior.

## In scope

- Add the task PR sync resource and its test factory.
- Share initial, retry, reconnect, and manual refresh requests.
- Publish one loaded snapshot to all consumers.
- Reference-count consumers and clean up timers, observers, and stale entries.
- Guard response writes by resource generation and active workspace context.
- Integrate the resource with `useTaskPR` without changing callers.
- Add focused resource and multi-consumer hook regression tests.

## Out of scope

- Backend or protocol changes.
- UI component changes.
- Task PR unlink or tooltip hydration redesign.
- Session Git status subscription deduplication.

## Acceptance

- Two mounted `useTaskPR` consumers for the same scoped task send one initial
  request and one request on each retry tick.
- Concurrent manual refresh calls return the same active completion and send
  one request.
- Different task or workspace-context keys remain independent.
- One consumer release preserves the lifecycle; the final release stops later
  retries and prevents stale publication.
- Permanent results stop retries. Reconnect starts one immediate request and
  one fresh bounded retry window.
- Empty results, transient failures, terminal failures, discovered pull
  requests, loaded state, and unlink behavior match the current hook contract.

## TDD sequence

### Red

- Extend `use-task-pr.test.tsx` with two consumers under one `StateProvider` and
  assert that the current implementation sends duplicate initial and retry
  requests.
- Add `task-pr-sync-resource.test.ts` cases for request joining, lease cleanup,
  scope isolation, reconnect transitions, retry terminal states, and stale
  responses. Confirm they fail before the resource exists.

### Green

- Implement the smallest store-scoped resource that satisfies the tests.
- Delegate `useTaskPR` synchronization and loaded state to the resource.
- Keep store selectors and unlink behavior unchanged.

### Refactor

- Keep resource state transitions explicit and separate from the React hook.
- Reuse `isCurrentWorkspaceContext` and the existing TaskPR scope types.
- Remove the superseded per-hook retry, request, reconnect, and loaded refs and
  effects.

## Verification

```bash
cd apps/web && pnpm exec vitest run hooks/domains/github/task-pr-sync-resource.test.ts hooks/domains/github/use-task-pr.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/github/task-pr-sync-resource.ts`
- `apps/web/hooks/domains/github/task-pr-sync-resource.test.ts`
- `apps/web/hooks/domains/github/use-task-pr.ts`
- `apps/web/hooks/domains/github/use-task-pr.test.tsx`

## Dependencies

None.

## Risks

- Strict Mode effect replay can expose cleanup and re-acquisition races.
- A stale request can write into a later workspace context if either the store
  or generation guard is missing.
- Ref-count errors can stop a needed retry or keep an orphan timer alive.
- A promise-only solution can pass concurrent-call tests while still producing
  duplicate requests on later retry ticks.

## Parallelism

`sequential`

The resource and hook integration share one lifecycle contract and one focused
regression suite.

## Inputs

- `REQ-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001` and its system design.
- The confirmed per-instance state in
  `apps/web/hooks/domains/github/use-task-pr.ts`.
- The store-scoped request registry in
  `use-task-pr-tooltip-hydration.ts` and external-resource pattern in
  `pr-commits-resource.ts`.
- The repeated `github.task_pr.sync` requests in the reported browser trace.

## Output contract

Report the files changed, the RED failure, GREEN and regression test outcomes,
typecheck and lint results, and any remaining lifecycle risk in the primary
session. Keep the requirement, design, plan, and work-order statuses in sync.

## Results

- Added the store-scoped task PR sync resource and deterministic resource tests.
- Refactored `useTaskPR` to lease the resource while preserving its return
  contract, loaded behavior, refresh operation, and unlink behavior.
- RED: the multi-consumer regression observed two initial requests from the
  previous per-hook implementation instead of one.
- GREEN: the focused resource and hook suites pass with 21 tests.
- The web typecheck and lint pass, and no rendered or mobile interaction changed.
- The resource invalidates publication when its lease is gone or its workspace
  context is no longer current. WebSocket request timeouts remain owned by the
  existing client.
- Unlink invalidates the scoped request generation after local removal, so an
  in-flight response cannot restore the removed association.
