---
id: "03-client-state-contract"
title: "Client state contract"
status: done
wave: 3
depends_on: ["02-session-activity-ownership"]
plan: "plan.md"
spec: "../../specs/fine-grained-background-running-status-indicator/spec.md"
---

# Task 03: Client state contract

- **Acceptance:** `settled+background` remains working with instant-send;
  foreground-generating always shows the existing orange state and queue mode;
  final background completion becomes done/idle; state and activity WS updates
  cannot erase one another out of order.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/use-session-state.test.ts` plus focused session-store tests; `cd apps/web && pnpm run typecheck`.
- **Files likely touched:** `apps/web/hooks/domains/session/use-session-state.ts`
  and test, `apps/web/lib/state/slices/session/` WS reducers and tests, and shared
  backend/session types only if the event contract requires it.
- **Dependencies:** Task 02 DTO and event behavior.
- **Inputs:** existing desktop/mobile status components; no layout changes.
- **Output contract:** Report derived-flag truth table, event-ordering tests,
  files changed, commands run, and mark this task plus its plan checkbox done.

## Result

`RUNNING` without background remains busy/working; any background value remains
working but not busy even after the session settles; settled generating is
idle. State changes carry background into `WAITING_FOR_INPUT`, later activity
completion is accepted there, and terminal states reject delayed frames. The
chat status row now consumes the shared working signal. Ninety-five focused web
tests and web typecheck pass.
