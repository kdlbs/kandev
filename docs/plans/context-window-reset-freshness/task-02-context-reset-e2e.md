---
id: "02-context-reset-e2e"
title: "Cover reset freshness end to end"
status: done
wave: 2
depends_on: ["01-invalidate-reset-context-usage"]
plan: "plan.md"
spec: "../../specs/ui/requirements/context-window-reset-freshness.md"
---

# Task 02: Cover reset freshness end to end

## Intent

Prove through the real reset UI and mock-agent runtime that the old ring disappears after reset and only a fresh agent usage report makes it return.

## Acceptance

- The browser regression starts with a visible seeded context-window ring, completes the existing reset confirmation flow, and observes that the ring is absent while the fresh conversation has no report.
- A later mock-agent command that emits `usage_update` makes the ring visible with the new conversation's reading.
- The reset control is available again once the reset settles and the session returns to idle; it remains hidden during the busy reset turn.
- The existing reset divider, idle recovery, and post-reset message flow remain covered.

## TDD sequence

1. Extend the existing reset-context recovery scenario with a seeded stale ring and
   fresh-report assertion.
2. Run the focused scenario against a fresh production build after Task 01; it
   passes with the reset invalidation and fresh usage propagation in place.
3. Refactor only shared seeding/page-object code needed to keep the scenario readable.

## Files likely touched

- `apps/web/e2e/tests/session/session-recovery.spec.ts`
- `apps/web/e2e/pages/session-page.ts` only if a stable context-ring locator is not already adequate

## Dependencies

Task 01.

## Parallelism

`sequential` — this test depends on both backend and frontend invalidation behavior.

## Mobile parity

This change normalizes shared session-runtime state and does not alter composition, controls, touch behavior, scrolling, or breakpoints. Both toolbars consume the same `TokenUsageDisplay`; existing `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts` covers the mobile rendering and touch disclosure. No new mobile-specific scenario is required.

## Verification

- `cd apps/web && pnpm e2e:run --host tests/session/session-recovery.spec.ts -- --grep "reset context hides stale usage"`

## Inputs

- Task 01 completed behavior.
- Existing reset flow in `apps/web/e2e/tests/session/session-recovery.spec.ts`.
- Mock-agent `/background 1ms` usage update path in `apps/backend/cmd/mock-agent/handler.go`.

## Verification result

The focused Playwright regression passed against a fresh backend and Vite build:

```text
cd apps/web && pnpm e2e:run --host tests/session/session-recovery.spec.ts -- --grep "reset context hides stale usage"
1 passed (9.4s)
```

No browser artifacts or blockers were produced. The regression uses the existing
reset dialog/divider flow, verifies the control returns at idle, and uses the
mock agent's `/background 1ms` usage update.
