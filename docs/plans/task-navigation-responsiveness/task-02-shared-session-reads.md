---
id: "02-shared-session-reads"
title: "Share session read ownership"
status: done
wave: 2
depends_on:
  - "01-overview-hydration"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 02: Share Session Read Ownership

## Summary

Make concurrent shell, commit, and cumulative-diff consumers share outstanding
reads and initialization state. Preserve legitimate refresh and context changes.

## In scope

- Store-scoped coordination with typed resource keys, promise ownership, dirty
  generations, one retry owner, and bounded inactive bookkeeping.
- Migration of the three hooks and all cumulative-diff invalidation callers.
- Existing domain result stores, readiness/terminal behavior, and shared loading
  state; retain last-known diff behavior inside the correct store scope.

## Out of scope

New backend APIs, message history polling, UI layout/copy, dependency additions,
and a general query-cache migration.

## Acceptance

1. Two or more same-scope consumers issue one initial read and observe shared
   loading/result state. Later mounting alone does not refetch; failed reads
   remain retryable and settled inactive bookkeeping is bounded (`.2`).
2. Invalidation bursts during a read produce one necessary follow-up. Unmounting
   one consumer or settling an obsolete promise cannot release another owner's
   guard; terminal/readiness and authoritative-empty semantics remain correct.
3. Different stores, auth identities, workspace generations, connection epochs,
   sessions, or shell request arguments cannot share or publish stale data.
   Include a same-store transition and reversed response order (`.5`).

## Implementation sequence

1. Mark in progress. Mount multiple real-store consumers with deferred transport
   responses. First prove current shell/commit duplication and diff guard reset
   failures through the actual hooks.
2. Implement the narrowly scoped coordinator following preview-feedback's
   `WeakMap<AppStore, ...>` pattern, with identity-checked finalization. Domain
   adapters retain request argument and result handling details.
3. Move successful initialization out of per-hook refs and remove subscriber
   resets of shared in-flight state. Preserve explicit refresh generations.
4. Scope cumulative-diff cache/listeners/timers and change invalidation callers
   to pass their store. Update affected test mocks in the same pass.
5. Cover environment-only to task-scoped shell hydration, session changes in one
   environment, reconnect on the same client, last-consumer departure, timeout,
   mid-flight invalidation, late old completion, and authoritative empty commits.
6. Run targeted checks and record RED/GREEN results. Task 04 counts browser reads.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/state/session-read-coordinator.test.ts hooks/domains/session/use-user-shells.test.tsx hooks/domains/session/use-session-commits.test.ts hooks/domains/session/use-cumulative-diff.test.ts lib/state/slices/session-runtime/user-shells.test.ts)
(cd apps/web && pnpm exec eslint lib/state/session-read-coordinator.ts lib/state/session-read-coordinator.test.ts hooks/domains/session/use-user-shells.ts hooks/domains/session/use-user-shells.test.tsx hooks/domains/session/use-session-commits.ts hooks/domains/session/use-session-commits.test.ts hooks/domains/session/use-cumulative-diff.ts hooks/domains/session/use-cumulative-diff.test.ts lib/ws/handlers/git-status.ts components/task/base-branch-picker.tsx)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Also run an existing caller's focused test if changing its invalidation wiring
changes behavior; record the exact path and result. Do not drop an existing
assertion merely to accommodate the new coordinator signature.

## Files likely touched

- `apps/web/lib/state/session-read-coordinator.ts` and `.test.ts` (new)
- `apps/web/hooks/domains/session/use-user-shells.ts` and `.test.tsx` (new test)
- `apps/web/hooks/domains/session/use-session-commits.ts` and `.test.ts`
- `apps/web/hooks/domains/session/use-cumulative-diff.ts` and `.test.ts`
- `apps/web/lib/ws/handlers/git-status.ts`
- `apps/web/components/task/base-branch-picker.tsx`
- `apps/web/hooks/domains/session/use-session-read.ts` (new)
- `apps/web/hooks/domains/session/session-read-test-helpers.tsx` (new)
- `apps/web/AGENTS.md` for the shared ownership convention
- `apps/web/hooks/domains/session/use-session-changes-count.test.ts`
- Existing test mock factories for the changed invalidation signature

## Dependencies

Task 01 precedes this work in the sequential delivery order. Its code is not a
technical dependency of the coordinator.

## Risks

Do not let an environment-only shell response overwrite a later task-scoped
result. Do not suppress legitimate commit snapshots merely because events
populated the store. Keep a dirty key dirty while it has no subscribers, without
running a detached refresh loop. Inactive eviction must not evict an active
promise into duplicate requests.

## Parallelism

`sequential`

## Inputs

- [Requirements `.2`, `.5`](../../specs/ui/requirements/task-navigation-responsiveness.md)
- [Shared session reads](../../specs/ui/system-design/task-navigation-responsiveness.md#shared-session-reads)
- `hooks/domains/comments/use-preview-feedback.ts`
- `lib/state/workspace-context.ts` and `lib/ws/connection.ts`
- Existing commit/diff hook tests for terminal and invalidation contracts

## Results

- RED: all three real-store multi-consumer tests made two reads instead of one;
  older shell responses also overwrote newer task/workspace results.
- GREEN: 50 tests across coordinator, shell/commit/diff hooks, shell reducers,
  and git-status handlers passed. Coverage includes scope retirement, 32-key
  inactive eviction, shared retries, terminal snapshots, and invalidation drain.
- Replaced selector mocks in the existing commit/diff suites with real-store
  tests. The teardown test now actually clears the snapshot; invalidation races
  assert a serialized follow-up and reject the obsolete result.
- Targeted ESLint, frontend typecheck, and whitespace checks passed.
- Added `use-session-read.ts` and `session-read-test-helpers.tsx` to share the
  scoped subscription and real-provider fixtures without duplicating adapters.

- The full frontend run exposed the Changes badge test's selector-only store
  mock missing `useAppStoreApi`. Its seven counting/isolation cases now use the
  real store through the shared fixture and pass without weakening assertions.

- Final ownership audit added two failing environment-rebinding cases: a late
  commit snapshot overwrote the replacement environment, and an old diff was
  retained after the session moved. Session reads now validate the current
  session-to-environment mapping before publication. All 51 affected coordinator,
  hook, Changes badge, and git-status handler tests pass with this guard.
