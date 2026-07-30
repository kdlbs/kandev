---
id: "04-preload-recovery"
title: "Guard stale Vite preload recovery"
status: done
wave: 2
depends_on: ["01-failure-containment"]
plan: "plan.md"
decision: "../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md"
---

# Task 04: Guard Stale Vite Preload Recovery

## Acceptance

- Pure RED tests cover first failure, recent repeat, expired/corrupt marker, and
  unavailable or unverifiable `sessionStorage`.
- The listener is installed before boot-payload loading and React mounting.
- The first failure in 60 seconds writes and verifies
  `kandev.preloadRecovery`, calls `preventDefault()`, and hard-reloads once.
- A recent repeat neither reloads nor prevents the error, allowing the route or
  root boundary to render.
- Storage failure disables automatic reload rather than risking a loop.
- Listener behavior and cleanup are deterministic without importing the
  side-effectful entry module into unit tests.

## Files likely touched

- `apps/web/src/main.tsx`
- `apps/web/src/vite-preload-recovery.ts` (new)
- `apps/web/src/vite-preload-recovery.test.ts` (new)

## Dependencies

- Task 01 must land first because it also owns `main.tsx` and supplies the
  repeated-failure recovery boundary.

## Verification

```bash
cd apps/web
pnpm test -- src/vite-preload-recovery.test.ts
pnpm run typecheck
```

## Output contract

Report the marker schema/TTL, first and repeated event semantics, storage
failure behavior, RED/GREEN results, and files changed. The primary session
updates this task and `plan.md` after accepting the result.
