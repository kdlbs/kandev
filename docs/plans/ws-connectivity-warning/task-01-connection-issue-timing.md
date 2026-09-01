---
id: "01-connection-issue-timing"
title: "Connection issue timing"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/ws-connectivity-warning.md"
---

# Task 01: Connection issue timing

## Acceptance

- One transient `ConnectionIssueSeverity` in the canonical frontend store remains `none` before
  3,000 ms offline, becomes `unstable` at 3,000 ms, and becomes `lost` at 10,000 ms.
- Switching among non-`connected` raw statuses preserves the original interval; `connected` clears
  severity immediately and starts the next outage from a fresh grace period.
- The single `useWebSocket` lifecycle owns and disposes the timers without changing retry,
  subscription, queue, or raw status/error behavior.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/ws/connection-issue-monitor.test.ts lib/ws/use-websocket.test.tsx lib/state/store.test.ts
cd apps/web && pnpm run typecheck
```

## Completion report

- Timing/state behavior: the monitor preserves one outage clock, reports `unstable` at 3,000 ms
  and `lost` at 10,000 ms, clears a visible warning on reconnect or lifecycle disposal, and cancels
  pre-threshold timers without emitting a redundant state change.
- Focused tests: passed — 3 test files and 11 tests.
- Typecheck: passed — `tsc --noEmit` completed with no errors.
- Blockers: none.
- Remaining risk: browser timer throttling can delay a warning, but cannot shorten either grace
  period or leave a surfaced warning behind after monitor replacement.

## Files likely touched

- `apps/web/lib/types/connection.ts`
- `apps/web/lib/state/slices/ui/types.ts`
- `apps/web/lib/state/slices/ui/ui-slice.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/ws/connection-issue-monitor.ts`
- `apps/web/lib/ws/connection-issue-monitor.test.ts`
- `apps/web/lib/ws/use-websocket.tsx`
- `apps/web/lib/ws/use-websocket.test.tsx` if direct lifecycle wiring needs a focused boundary test
- `apps/web/lib/state/store.test.ts`

## Dependencies

None.

## Parallelism

Sequential. Every later surface consumes this shared state contract.

## Inputs

- Spec: `What`, `State machine`, `Failure modes`, and the first five scenarios.
- Plan: `Frontend > Canonical issue timing`.
- Existing patterns: `apps/web/lib/ws/use-websocket.tsx`,
  `apps/web/lib/state/slices/ui/ui-slice.ts`.

## Risks

- React remounts or retry status churn must not create multiple clocks.
- Cleanup order must prevent an intentional unmount disconnect from starting a new timer.

## Output contract

Report the timing/state behavior, files changed, exact commands and results, blockers/risks, then
mark this task `done` and update its checkbox in `plan.md`.
