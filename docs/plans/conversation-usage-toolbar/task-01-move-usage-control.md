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

Make Usage a compact, icon-only status-row action beside transcript navigation. Remove its transcript-footer mount while retaining the current data and desktop/phone disclosures, including access from an archived transcript's read-only banner.

## In scope

- Mount `ConversationUsageDisplay` once through the composer status-row controls and preserve its selected task/session identity.
- Remove the footer mount, render only the stats icon, and keep a localized accessible name and desktop tooltip.
- Preserve the desktop popover, phone drawer, focus return, pending/error states, and responsive reachability. Close the tooltip while the desktop popover is open.
- Retain Usage access in the read-only archived transcript banner beside Jump to latest.
- Keep the proceed action right-aligned when Usage returns no control.
- Update focused component and desktop/mobile E2E coverage for the new placement, archived access, and icon geometry.

## Out of scope

- Changes to usage data, APIs, pricing, feature flags, or permission approval controls.

## Acceptance

1. A selected session with usage shows exactly one icon-only Usage control in the row above the composer; an archived transcript retains the control in its read-only banner. No second control or empty gap appears in the transcript footer.
2. Desktop keyboard users and phone touch users can open the existing details, close them, and return focus to the icon. The desktop tooltip closes while its popover is open. Missing, loading, error, and session-switch states retain the current usage semantics.
3. The desktop icon aligns with its compact neighbors; the phone/coarse-pointer hit target is at least 44 by 44 CSS pixels; the proceed action stays right-aligned when Usage is empty; and the row has no horizontal overflow.

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
(cd apps/web && pnpm test -- components/task/chat/chat-status-bar.test.tsx components/task/chat/conversation-usage-display.test.tsx)
(cd apps/web && pnpm run typecheck)
make build-web
(cd apps/web && pnpm e2e:run --project chromium tests/chat/conversation-usage.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-conversation-usage.spec.ts)
(cd apps/web && pnpm run lint)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/message-list-footer.tsx`
- `apps/web/components/task/chat/chat-status-bar.tsx`
- `apps/web/components/task/chat/transcript-nav-group.tsx`
- `apps/web/components/task/chat/conversation-usage-display.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
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

Initial implementation validation passed. The focused component suite passed 16 tests across three files; web typecheck and `make build-web` passed. Desktop Chromium verified the 24px desktop trigger, accessible tooltip, keyboard disclosure, and focus return. The 500px fine-pointer case verified the 44px phone drawer. Mobile Chrome verified the touch-sized trigger, drawer, focus return, row containment, and no horizontal overflow. `git diff --check` passed.

PR fixup validation passed: the focused component suite passed 11 tests across two files, including the shared coarse-pointer drawer policy and proceed alignment when Usage returns null. Web typecheck and lint passed, as did `make build-web`. Desktop Chromium passed all three tests, covering the desktop popover, 500px fine-pointer drawer, keyboard and pointer tooltip dismissal, and archived access. Mobile Chrome passed both tests, including archived drawer access. `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed. Desktop and mobile screenshots were recaptured and visually inspected.
