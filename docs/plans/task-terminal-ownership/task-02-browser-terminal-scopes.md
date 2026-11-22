---
id: "02-browser-terminal-scopes"
title: "Isolate browser terminal scopes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-TERMINALS-002
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-TASKS-TASK-TERMINALS-002.2
  - AC-TASKS-TASK-TERMINALS-002.5
  - AC-TASKS-TASK-TERMINALS-002.7
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 02: Isolate browser terminal scopes

## Summary

Give ordinary results and shell publication writers the same task/environment
identity. Migrate every producer and consumer in one compatible pass. Keep
unmanaged terminals shared by environment.

## In scope

- Introduce `UserShellScope` and one tuple-key helper. Add `byScope` entries
  for ordinary rows, dismissal tombstones, and settlement flags.
- Retain environment maps for unmanaged rows only. Partition successful lists.
  Reject adoption of environment-only ordinary rows into a task scope.
- Update `useUserShells`, loading/failure handling, and read publication groups.
  Distinct task writers must coexist. Preserve same-scope request sharing,
  unmount survival, invalidation coalescing, scope retirement, and retention bounds.
- Expose successful current-list evidence separately from settled `isLoaded`.
  A failed initial read supplies no creation authority. Preserve same-scope
  cached rows on refresh failure without treating the failure as empty success.
- Migrate all shell action signatures, direct selectors, and asynchronous
  publishers. Capture task identity at admission, including MCP authentication.
- Update environment cleanup, session-to-environment migration, initial-state
  handling, fixtures, and frontend guidance for the new result namespaces.

## Out of scope

- Gateway changes, stale-panel repair, new controls, and new retry loops.
- Changing commit/diff ownership, general cache frameworks, or unrelated slices.

## Acceptance

1. Permanent hook regressions named `keeps task A shells while returning from
   task B` and `publishes concurrent shared-environment tasks independently`
   fail first. Both then pass in either response order. Same-tuple consumers
   still share one request, including after the first consumer unmounts.
2. Deferred taskless, failure, reconnect, workspace, and restoration cases never
   publish ordinary rows or flags into another task. Scoped mutations and
   tombstones preserve siblings. Fixed/script rows remain visible to all same-
   environment consumers. Cleanup retains live references and removes every
   owned scope after the final environment reference disappears.
3. Every production shell selector/writer uses the correct namespace. Late
   create, rename, resume, destroy, and authentication results target their
   captured scope. Updated real-consumer tests cover desktop and phone hooks.

## Verification

Dependencies already exist in this investigation worktree. In a fresh worktree,
first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/session-runtime/user-shells.test.ts lib/state/slices/session-runtime/migrate-env-keyed-data.test.ts lib/state/slices/session-runtime/purge-session.test.ts lib/state/session-read-coordinator.test.ts hooks/domains/session/use-user-shells.test.tsx hooks/domains/session/use-terminals-shell-state.test.tsx hooks/domains/session/use-ensure-default-terminal-ordinary.test.ts hooks/domains/session/use-mobile-terminals.test.ts components/task/terminal-tab.test.tsx components/task/agent-mcp-prepare-actions.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
rg -n 'userShells|setUserShells|addUserShell|removeUserShell|updateUserShell' apps/web/hooks apps/web/components apps/web/lib
git diff --check
```

The `rg` inventory is a manual completeness check. Ordinary access under
`byEnvironmentId` is a defect. Valid unmanaged access remains allowed.
Add tests for newly affected logic beside its current suite and run them too.

## Files likely touched

- `apps/web/lib/state/user-shell-scope.ts` (new), `session-read-coordinator.ts`,
  `app-state-types.ts`, and `default-state.ts`.
- `apps/web/lib/state/slices/session-runtime/types.ts` and
  `session-runtime-slice.ts`, plus their shell, migration, and purge tests.
- `apps/web/hooks/domains/session/use-user-shells.ts`, `use-session-read.ts`,
  `use-terminals.ts`, `use-mobile-terminals.ts`, and
  `use-ensure-default-terminal-ordinary.ts`.
- `apps/web/components/task/terminal-tab.tsx`, `terminal-reopen-menu.tsx`,
  `dockview-header-actions.tsx`, `dockview-watermark.tsx`,
  `dockview-layout-setup.ts`, and `agent-mcp-prepare-actions.tsx`.
- The listed coordinator, hook, and component suites, their shared fixtures,
  and `apps/web/AGENTS.md`.

This inventory is the known closure of production shell accesses at the
investigation head. Use the verification search to catch additional references.

## Dependencies

None. Execute after Task 01 by plan order. Task 03 requires this result model.

## Risks

Changing only the cache key leaves the coordinator writer collision intact.
Moving script rows into task scope breaks shared environment behavior. Returning
fresh arrays from a Zustand selector can cause repeated rendering. Keep stable
empty values and memoize merged ordinary/unmanaged results.

## Parallelism

`sequential`

## Inputs

- [Browser scope design](../../specs/tasks/system-design/task-terminal-persistence.md#browser-result-scopes).
- [Shared read design](../../specs/ui/system-design/task-navigation-responsiveness.md#shared-session-reads).
- [Task requirements](../../specs/tasks/requirements/task-terminal-persistence.md)
  and [read requirements](../../specs/ui/requirements/task-navigation-responsiveness.md).
- Existing `session-read-test-helpers.tsx` and deferred shell-read tests.
- `apps/web/AGENTS.md`, `/mobile-parity`, and `/tdd`.

## Results

Completed. `lib/state/user-shell-scope.ts` owns the tuple key and the active
scope guard. Ordinary rows, tombstones, loading, loaded, and listed flags live
in task/environment scopes; `byEnvironmentId` holds only fixed, script, and
unmanaged rows. Shell reads publish to that tuple, so tasks sharing an
environment no longer replace each other's writer. Environment-shared rows are
ordered by request generation, so a delayed older list cannot remove a newer
script terminal. A pending read withdraws
older successful-list authority. Create, script, resume, and authentication
callbacks keep results in their captured cache and publish panels or local
selection only while that scope is still active.

Verification executed from `apps/web` (plus the repository-level checks):

```bash
pnpm exec vitest run lib/state/slices/session-runtime/user-shells.test.ts lib/state/slices/session-runtime/migrate-env-keyed-data.test.ts lib/state/slices/session-runtime/purge-session.test.ts lib/state/session-read-coordinator.test.ts hooks/domains/session/use-user-shells.test.tsx hooks/domains/session/use-terminals-shell-state.test.tsx hooks/domains/session/use-ensure-default-terminal-ordinary.test.ts hooks/domains/session/use-mobile-terminals.test.ts components/task/terminal-tab.test.tsx components/task/agent-mcp-prepare-actions.test.tsx
pnpm run typecheck
pnpm run lint
rg -n 'userShells|setUserShells|addUserShell|removeUserShell|updateUserShell' apps/web/hooks apps/web/components apps/web/lib
git diff --check
```

The focused suites passed (105 tests at the Task 02 boundary), typecheck and
lint passed, and the `rg` inventory contains no ordinary-environment access.
