---
id: "05-usage-display"
title: "Built-in chat usage"
status: done
wave: 5
depends_on:
  - "04-usage-accounting"
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-001
  - REQ-COSTS-CONVERSATION-USAGE-002
  - REQ-COSTS-CONVERSATION-USAGE-003
  - REQ-COSTS-CONVERSATION-USAGE-004
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-001.1
  - AC-COSTS-CONVERSATION-USAGE-001.3
  - AC-COSTS-CONVERSATION-USAGE-001.4
  - AC-COSTS-CONVERSATION-USAGE-001.5
  - AC-COSTS-CONVERSATION-USAGE-002.1
  - AC-COSTS-CONVERSATION-USAGE-002.2
  - AC-COSTS-CONVERSATION-USAGE-002.3
  - AC-COSTS-CONVERSATION-USAGE-002.4
  - AC-COSTS-CONVERSATION-USAGE-003.1
  - AC-COSTS-CONVERSATION-USAGE-003.2
  - AC-COSTS-CONVERSATION-USAGE-003.3
  - AC-COSTS-CONVERSATION-USAGE-003.4
  - AC-COSTS-CONVERSATION-USAGE-004.1
  - AC-COSTS-CONVERSATION-USAGE-004.2
  - AC-COSTS-CONVERSATION-USAGE-004.3
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 05: Built-in chat usage

## Summary and scope

Add plugin-independent turn and session Usage controls.
Read committed backend projections and preserve the distinction between last response, turn, session, and context occupancy.
Show exact/partial tokens, estimated prices, optional provider estimates, and unavailable states.

## Exclusions

No frontend price calculator, automatic plugin removal, or model-specific wire parsing in React.

## Likely files and ownership

- New chat usage view model, API reader, turn footer, and disclosure component.
- Existing `components/task/chat/messages/message-actions.tsx`, session chrome, and `token-usage-display.tsx`.
- `lib/ws/handlers/prompt-usage.ts`, session/runtime types and store state.
- Locales, new component/view-model tests, and UI-03 E2E files.

## Acceptance

1. With Office disabled and no session-cost plugin, users see turn and session usage, plus last-response detail when observed.
2. Reload, invalidation, pending writes, fetch errors, and session changes preserve correct identity and honest completeness/provenance.
3. Phone disclosure uses a touch drawer with one scroll owner and safe-area clearance; desktop retains compact controls.

## ASCII UI preview

UI-03 from [the plan](plan.md#ui-03-usage-detail):

```text
Desktop popover             Phone inset drawer
This turn: 1,200 tokens     [Usage                Close]
Estimated cost: $0.02       This turn: 1,200 tokens
Last response: 500         Estimated cost: $0.02
Input 1,000 (cached 600)    Last response: 500
Output 200 (reasoning 80)   Input / Output breakdown
Price source               Price source
Session recorded total     Session recorded total
```

Missing price reads Cost unavailable, partial data reads Partial usage, and delayed data reads Usage pending.
The optional native-thread provider estimate is separately labeled and never added to recorded totals.
Use `useTouchDrawer` and the `MobilePickerSheet` geometry. Keep desktop 28px and phone/coarse-pointer 44px targets.

## TDD and verification

Create `conversation-usage.test.ts` and `conversation-usage-display.test.tsx` before implementation.
Unit tests cover provenance, old-version values, stale fetch guards, missing/zero, and last-response scope.
Run from the repository root:

```bash
(cd apps/web && pnpm test -- components/task/chat/conversation-usage-display.test.tsx lib/utils/conversation-usage.test.ts lib/api/domains/conversation-usage-api.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/conversation-usage.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-conversation-usage.spec.ts)
git diff --check
```

Inspect the rendered phone drawer during the existing E2E run.
Assert containment, focus return, 44px targets, and no document horizontal overflow.

## Risks

Existing prompt usage state holds only the latest counts. It cannot substitute for the committed per-turn projection.
Do not describe an API-equivalent subscription estimate as an actual charge.

## Results

The focused API, utility, and display unit tests passed (49 tests across all selected UI/API test files). Web typecheck, lint, i18n check, and new-code ratchet passed. Desktop and mobile usage E2E tests passed. The mobile capture shows the open detail drawer. The desktop capture shows the footer entry point. The desktop test also checks the open popover content.
