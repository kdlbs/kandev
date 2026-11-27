---
id: "02-session-reconciliation-client"
title: "Skip unchanged session publication"
status: complete
wave: 2
depends_on:
  - "01-conditional-session-response"
plan: "plan.md"
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-001
acceptance_criteria:
  - AC-UI-SESSION-REFRESH-EFFICIENCY-001.1
  - AC-UI-SESSION-REFRESH-EFFICIENCY-001.2
  - AC-UI-SESSION-REFRESH-EFFICIENCY-001.3
  - AC-UI-SESSION-REFRESH-EFFICIENCY-001.4
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
---

# Task 02: Skip Unchanged Session Publication

## Summary

Teach the session-specific client to handle 304 without parsing or publishing
a session. Keep the existing bounded reconciliation loop and its protection
against stale HTTP responses and missed WebSocket state.

## In scope

- Add a typed conditional fetch in the session API domain. Leave generic
  `fetchJson` response handling unchanged.
- Retain a validator only in the current store/session reconciliation owner.
- Cover changed responses, 304, failed reads, server-without-ETag fallback,
  reconnect, and consumer handoff.
- Add desktop and phone E2E state-regression coverage and record a clean
  before/after session trace.

## Out of scope

- A new backend endpoint, changing the session poll window, and environment
  status read sharing.

## Acceptance

- An unchanged conditional response causes no `setTaskSession` call or
  subscribed session render.
- A changed full response still updates the view without overwriting a newer
  WebSocket state. Reconnect and error recovery retain their current behavior.
- Desktop and phone display the same current state and pending action after
  an authoritative change.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/api/domains/session-api.test.ts hooks/domains/session/use-session.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-refresh-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-refresh-efficiency.spec.ts)
```

## Files likely touched

- `apps/web/lib/api/domains/session-api.ts`
- `apps/web/lib/api/domains/session-api.test.ts`
- `apps/web/hooks/domains/session/session-state-reconciler.ts`
- `apps/web/hooks/domains/session/use-session.test.ts`
- `apps/web/e2e/tests/session/session-refresh-efficiency.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-session-refresh-efficiency.spec.ts` (new)

## Dependencies

Task 01 supplies the conditional response contract.

## Risks

- Client-side cache substitution or a stale validator can make a 304 look
  current after connection or ownership changes. Force a full read when no
  matching local full session exists.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/session-refresh-efficiency.md)
- [System design](../../specs/ui/system-design/session-refresh-efficiency.md)
- `apps/web/hooks/domains/session/use-session.test.ts`

## Results

The typed conditional client, reconciliation no-publication path, local-session
fallback, reconnect invalidation, and focused tests are implemented. The
desktop and phone browser scenarios verify 200-to-304 revalidation and that a
held old full response cannot replace a newer cancelled session state.
Verification passed:

```bash
(cd apps/web && pnpm exec vitest run lib/api/client.test.ts lib/api/domains/session-api.test.ts hooks/domains/session/use-session.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-refresh-efficiency.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-refresh-efficiency.spec.ts)
```
