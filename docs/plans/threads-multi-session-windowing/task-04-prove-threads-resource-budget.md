---
id: "04-prove-threads-resource-budget"
title: "Prove the Threads resource budget"
status: completed
wave: 4
depends_on:
  - "03-add-threads-session-switcher"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-001
  - REQ-UI-THREADS-DECK-002
  - REQ-UI-THREADS-DECK-003
  - REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001
  - REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-002
acceptance_criteria:
  - AC-UI-THREADS-DECK-001.1
  - AC-UI-THREADS-DECK-001.3
  - AC-UI-THREADS-DECK-001.4
  - AC-UI-THREADS-DECK-001.7
  - AC-UI-THREADS-DECK-002.3
  - AC-UI-THREADS-DECK-002.4
  - AC-UI-THREADS-DECK-002.6
  - AC-UI-THREADS-DECK-003.1
  - AC-UI-THREADS-DECK-003.3
  - AC-UI-THREADS-DECK-003.4
  - AC-UI-THREADS-DECK-003.5
  - AC-UI-THREADS-DECK-003.6
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.3
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.4
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.5
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.6
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-002.3
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/platform/system-design/viewport-bounded-session-delivery.md
---

# Task 04: Prove the Threads resource budget

## Summary

Add browser-level desktop and mobile proof for multi-session switching,
attention semantics, viewport activation, and full-session subscription limits.
Run the complete focused package checks against rebuilt web assets.

## In scope

- Extend desktop Threads E2E with a real primary session and sibling sessions.
- Record outgoing session subscribe/unsubscribe actions during initial load,
  tab switching, and horizontal scrolling.
- Verify same-row switch-only tabs, task/session deep links, inactive-tab status
  updates, review-ready status, and stable column positions.
- Extend mobile Threads E2E with the picker sheet, one detail owner, 44-pixel
  rows, safe-area containment, and zero document overflow.
- Run all targeted backend, frontend, i18n, typecheck, production-build, and E2E
  commands for the package.

## Out of scope

- Broad repository verification, unrelated PR checks, screenshot publication,
  or GitHub review/comment operations.

## Acceptance

- Initial browser subscriptions contain only selected sessions from the detail
  window and no unselected sibling.
- Tab switching replaces only that column's session membership; scrolling
  releases departed details and activates the newly visible selected session.
- Desktop and phone controls meet the interaction, status, containment, and
  deep-link criteria without regressing existing Threads tests.

## Verification

Run the production build and the focused package checks:

```bash
(cd apps/backend && rtk go test ./internal/task/service ./internal/gateway/websocket ./pkg/websocket -run 'PendingAction|TaskEventBroadcaster|SessionPending' -race)
(cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/session-pending-action.test.ts lib/state/slices/session/update-session-read-cursor.test.ts lib/threads/active-threads.test.ts lib/threads/thread-session-selection.test.ts lib/threads/thread-session-status.test.ts components/threads/threads-board.test.tsx components/threads/thread-column-activation.test.tsx components/threads/thread-session-switcher.test.tsx lib/links.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(make build-web)
(cd apps/web && pnpm e2e:run --project chromium tests/task/threads-view.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-view.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/task/threads-view.spec.ts`
- `apps/web/e2e/tests/task/mobile-threads-view.spec.ts`
- `apps/web/e2e/helpers/ws-traffic.ts` if the existing helper needs a Threads
  action filter
- Files from Tasks 01 through 03 only when an E2E finding exposes a scoped bug

## Dependencies

- Tasks 01 through 03 must be green before browser resource-budget assertions
  can represent the final design.

## Risks

- Use stable action and session IDs as the traffic oracle, not timing or byte
  counts.
- Wait for the settled observer/subscription state after scroll or tab changes
  before asserting membership.
- Test-harness sibling sessions must belong to a task with one real primary
  session so the contributor's production eligibility path remains exercised.

## Parallelism

`sequential`

## Results

Implemented and verified. Desktop and mobile E2E coverage records session
subscriptions during deep-link load, tab switching, and viewport scrolling.
The browser checks prove one selected conversation per active detail owner,
inactive sibling status updates, review-ready status, stable column order,
mobile picker geometry, safe-area containment, and zero horizontal overflow.

The review fix-up adds long-label desktop geometry coverage. The complete focused
package passes 33 backend race tests, 127 frontend tests, strict web lint,
typecheck, i18n validation, production web build, desktop Threads E2E (9 tests),
and mobile Threads E2E (2 tests).
