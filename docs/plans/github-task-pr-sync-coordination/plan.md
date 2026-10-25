---
created: 2026-08-27
updated: 2026-08-27
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001
system_design:
  - ../../specs/integrations/system-design/github-task-pr-sync-coordination.md
legacy_specs: []
---

# Implementation Plan: GitHub task pull request sync coordination

## Overview

Replace per-hook task pull request synchronization state with one shared,
store-scoped resource per workspace context and task. The resource coordinates
the initial request, in-flight work, retry timer, reconnect edge, loaded state,
and cleanup while `useTaskPR` preserves its current public API.

The confirmed root cause is that every mounted `useTaskPR` instance owns its
own effects and refs. A task page can mount several consumers, so it sends
several identical `github.task_pr.sync` requests and creates several retry
schedules. Backend behavior is not the source of the duplication.

## Scope

### In scope

- Add a shared task PR sync resource keyed by store, workspace ID, workspace
  generation, and task ID.
- Share automatic and manual in-flight synchronization requests.
- Give each scoped entry one retry timer and one reconnect observer.
- Reference-count consumers and clean up after the last release.
- Preserve task PR store writes, loaded outcomes, retries, reconnect recovery,
  manual refresh, and unlink behavior.
- Add deterministic resource and hook regression tests.

### Out of scope

- Backend request coalescing.
- WebSocket protocol or GitHub provider changes.
- Changes to task PR rendering, copy, or interactions.
- Refactoring tooltip hydration, workspace-wide loading, or Git status
  subscriptions.

## Technical approach

### Shared synchronization resource

Create `apps/web/hooks/domains/github/task-pr-sync-resource.ts` with a testable
factory. Keep registries in a `WeakMap` keyed by Zustand store. Key each entry by
workspace ID, workspace-context generation, and task ID.

Each entry exposes a stable snapshot and subscription for
`useSyncExternalStore`. The subscription owns a consumer lease. The first lease
starts freshness synchronization; the last release stops timers and observers
and invalidates stale completions.

The entry joins all active refresh calls to one promise. It owns one bounded
five-second retry schedule and detects one transition edge into `connected`.
Before publishing a result, it verifies the resource generation and active
workspace context.

### Hook integration

Refactor `useTaskPR` to obtain the current Zustand store API and scoped resource
entry. Replace local retry, permanent, request, reconnect, and loaded state with
the resource snapshot and delegated `refresh`.

Keep `pr` and `prs` selected from the current workspace's task PR slice. Keep
the existing `unlink` callback and return shape unchanged. No component caller
changes are expected.

## Test-first sequence

1. Add a failing multi-consumer hook regression. Two consumers for one scoped
   task must send one initial request and one request on a retry tick.
2. Add failing resource tests for concurrent manual refresh, consumer release,
   separate scope keys, reconnect recovery, permanent responses, retry
   exhaustion, and stale workspace completion.
3. Implement the shared resource and integrate `useTaskPR` until the focused
   tests pass.
4. Run the existing hook suite to prove unlink, loading, retry, and reconnect
   behavior remains unchanged.

## Tests

- `AC-INTEGRATIONS-GITHUB-TASK-PR-SYNC-COORDINATION-001.1` and `.2`: two
  consumers share initial, retry, and concurrent manual refresh requests.
- `.3`: different stores, tasks, workspaces, and workspace generations use
  separate entries.
- `.4`: one unmount preserves the shared lifecycle; the last unmount cancels
  future retries and observers.
- `.5`: empty, transient, permanent, discovered-PR, exhausted, and reconnect
  cases retain their current outcomes.
- `.6`: a stale context response cannot write associations or publish loaded
  state for the active context.
- `.7`: all consumers observe the same loaded result; existing unlink tests
  remain green.

## Verification commands

Run from the repository root unless a command changes directory:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/github/task-pr-sync-resource.test.ts hooks/domains/github/use-task-pr.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
git diff --check
```

If `apps/node_modules` is absent in a fresh worktree, first run
`cd apps && pnpm install --frozen-lockfile`.

## Work orders

- [x] [Task 01: Coordinate task PR sync consumers](task-01-coordinate-task-pr-sync-consumers.md)

## Implementation waves and parallel candidates

Wave 1 is sequential:

- Task 01 owns the resource, hook integration, and shared regression boundary.

There are no parallel-safe work orders. Splitting the resource from the hook
would require both tasks to edit the same lifecycle contract and tests.

## Risks

- React Strict Mode can subscribe, release, and subscribe again during mount.
  The resource must rejoin an unresolved entry instead of sending a second
  freshness request.
- A workspace can be reloaded with the same ID but a new generation. Omitting
  the generation from the key could apply stale synchronization state.
- Stopping only the in-flight request duplication would leave duplicate retry
  schedules. The shared resource must own timers and reconnect transitions.
- Cleanup must not let one consumer stop retries required by another consumer.
- The change normalizes data ownership only. It must not alter desktop or
  mobile rendering, touch targets, or navigation behavior.

## Open questions

None.
