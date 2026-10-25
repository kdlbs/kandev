---
id: "03-shutdown-and-reattach-verification"
title: "Shutdown wiring, reconcile coverage, and reload E2E"
status: draft
wave: 2
depends_on: ["01-loginpty-lifetime-policy"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 03: Shutdown wiring, reconcile coverage, and reload E2E

## Acceptance

- Graceful backend shutdown stops all live quick-terminal (host-shell) PTYs by invoking
  `loginMgr.StopAll()` through the existing cleanup chain, so no-timeout sessions do not leak.
- `reconcileTab` keeps a still-running host-shell session as `running` with its `sessionId` after a
  simulated long idle (session no longer reaped), and only clears/marks unavailable when the manager
  genuinely has no live entry for the descriptor.
- After a page reload of a still-running quick terminal (including one idle past the old 10m idle
  window), the same tab and `sequence` reattach to the same PTY and no new numbered terminal is
  created.
- If any residual client path starts a fresh shell for a restored `running` descriptor, it is fixed
  so a restored live descriptor attaches by `sessionId` only.

## Verification

```bash
(cd apps/backend && go test ./internal/backendapp ./internal/quickterminal/... -count=1)
(cd apps/web && pnpm vitest run components/quick-chat lib/state/slices/ui)
(cd apps/web && pnpm e2e -- terminal/quick-terminal.spec.ts)
```

## Files likely touched

- `apps/backend/internal/backendapp/` (cleanup registration for `loginMgr.StopAll()`) and its test.
- `apps/backend/internal/quickterminal/service_test.go`
- `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`
- Frontend reconcile / terminal tab view only if a residual fresh-start path is found.

## Dependencies

Depends on Task 01 (`StopAll` and the no-timeout policy must exist first).

## Parallelism

Sequential after Task 01. Independent of Task 02.

## Inputs

- Task 01 policy and `Manager.StopAll()`.
- `backendapp` `addCleanup`/`runCleanups` graceful-shutdown pattern (mirrors the lifecycle manager
  cleanup already registered).
- `service.go` `reconcileTab` / `List` reconciliation and `ownsSession` guard.
- Existing reattach E2E in `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`.

## Output contract

Report the cleanup wiring, reconcile test additions, any frontend change made, and exact command
results including the E2E run. Prefer a test-only shortened policy hook or fake clock over a real
10-minute idle wait in E2E. Update this task and the parent plan only after backend, frontend, and
the targeted E2E pass.

## Results

_To be filled in during implementation._
