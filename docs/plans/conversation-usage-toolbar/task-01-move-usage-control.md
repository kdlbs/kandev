---
id: "01-move-usage-control"
title: "Move Usage into chat status row"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COSTS-CONVERSATION-USAGE-003
acceptance_criteria:
  - AC-COSTS-CONVERSATION-USAGE-003.3
  - AC-COSTS-CONVERSATION-USAGE-003.4
  - AC-COSTS-CONVERSATION-USAGE-003.5
system_design:
  - ../../specs/costs/system-design/conversation-usage.md
---

# Task 01: Move Usage into chat status row

## Summary

Make Usage a compact, icon-only status-row action beside transcript navigation. Remove its transcript-footer mount while retaining the current data and desktop/phone disclosures.

## In scope

- Mount `ConversationUsageDisplay` once through the composer status-row controls and preserve its selected task/session identity.
- Remove the footer mount, render only the stats icon, and keep a localized accessible name and desktop tooltip.
- Preserve desktop popover, phone drawer, focus return, pending/error states, and responsive reachability.
- Update focused component and desktop/mobile E2E coverage for the new placement and icon geometry.

## Out of scope

- Changes to usage data, APIs, pricing, feature flags, or permission approval controls.

## Acceptance

1. A selected session with usage shows exactly one icon-only Usage control in the row above the composer; no Usage control or blank space remains in the transcript footer.
2. Desktop keyboard users and phone touch users can open the existing details, close them, and return focus to the icon. Missing, loading, error, and session-switch states retain the current usage semantics.
3. The desktop icon aligns with its compact neighbors; the phone/coarse-pointer hit target is at least 44 by 44 CSS pixels, and the row has no horizontal overflow.

## ASCII UI preview

UI-01: Selected chat session with recorded usage. This excerpt matches the [full preview](plan.md#ascii-ui-preview) and `AC-COSTS-CONVERSATION-USAGE-003.3`/`.5`.

```text
Desktop:  [Threads] [start] [last] [stats] [Share] [Implement ->]
Phone:    [Threads] [start] [last] [stats] [Share]
                                      tap -> inset Usage drawer
Footer:   no Usage control
```

The bracketed labels identify icons; only the stats icon renders for Usage, with an accessible name and a desktop tooltip. Conditional neighbors can be absent. The phone row may wrap without hiding the control.

## Verification

Run from the repository root after installing `apps/` dependencies in a fresh worktree:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test -- components/task/chat/message-list-footer.test.tsx components/task/chat/chat-status-bar.test.tsx components/task/chat/conversation-usage-display.test.tsx)
(cd apps/web && pnpm run typecheck)
make build-web
(cd apps/web && pnpm e2e:run --project chromium tests/chat/conversation-usage.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-conversation-usage.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/message-list-footer.tsx`
- `apps/web/components/task/chat/chat-status-bar.tsx`
- `apps/web/components/task/chat/transcript-nav-group.tsx`
- `apps/web/components/task/chat/conversation-usage-display.tsx`
- Corresponding component tests and `apps/web/e2e/tests/chat/{conversation-usage,mobile-conversation-usage}.spec.ts`

## Dependencies

None.

## Risks

- The status row's optional controls can leave an empty wrapper or wrap poorly when the Usage component hides itself.
- Moving the mount can expose stale data during session changes if the hook's task/session identity is not preserved.

## Parallelism

`sequential`

## Inputs

- `REQ-COSTS-CONVERSATION-USAGE-003` and the [system design](../../specs/costs/system-design/conversation-usage.md#read-surface-and-presentation).
- Existing footer, status row, usage disclosure, desktop/mobile E2E, and permission-row history.

## Results

Done. The focused component suite passed 16 tests across three files. `pnpm run typecheck` and `make build-web` passed. The desktop Chromium E2E passed with an exact 24 by 24 CSS pixel trigger, accessible label, tooltip, keyboard disclosure, and focus return. A second Chromium case at a 500px fine-pointer viewport passed with a 44 by 44 target and the phone drawer. The mobile Chrome E2E passed with a trigger of at least 44 by 44 CSS pixels, drawer access, focus return, row containment, and no horizontal page overflow. The mobile screenshot confirmed the existing inset drawer and its single scroll owner. `git diff --check` passed.
