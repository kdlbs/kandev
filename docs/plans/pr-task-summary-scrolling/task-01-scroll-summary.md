---
id: "01-scroll-summary"
title: "Make long PR summaries scrollable"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
acceptance_criteria:
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.21
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.22
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.23
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.24
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.25
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
---

# Task 01: Make long PR summaries scrollable

## Summary

Keep a long GitHub task summary inside the viewport.
Make every entry reachable with pointer, keyboard, and the existing phone drawer.

## In scope

- Desktop shell sizing and one internal scroll body.
- Opt-in trigger/content pointer and focus continuity through the shared tooltip-state alias.
- Accessible tooltip description, named scroll region, visible focus indicator, and E2E selectors for the wrapped content.
- Focused unit, component, and rendered desktop/phone regression evidence.

## Out of scope

- Provider state, APIs, polling, association changes, and global Tooltip behavior.
- New mobile surfaces or new localized copy.

## Acceptance

1. Five wrapped PR entries and automation detail fit within a bounded shell and remain reachable through scrolling (AC 001.21).
2. Real pointer transfer, wheel input, keyboard focus/scroll keys, and Escape work without duplicate hydration (AC 001.22).
3. The existing phone drawer reaches the final entry with one scroll owner and no viewport overflow (AC 001.23).
4. The desktop Tooltip description includes rendered PR numbers, titles, authors, and statuses, and its scroll region has a localized accessible name (AC 001.24).
5. Keyboard focus on the desktop scroll region has a visible indicator (AC 001.25).

## ASCII UI preview

See the [full UI-01 and UI-02 previews](plan.md#ascii-ui-preview).

```text
UI-01 Desktop                UI-02 Phone
+----------------------+     +----------------------+
| PR entries         # |     | 5 pull requests      | fixed
| PR 5 after scroll    |     | PR entries         # | scroll
| Automation           |     | PR 5 / Automation    |
+----------v-----------+     +----------------------+
Task [PR 5]                  Task picker -> PR drawer
```

The desktop body owns scrolling. The phone header stays fixed above its existing body.
Both final entries clear their viewport edges (AC 001.21 through 001.23).

## Implementation sequence

1. Add a five-PR geometry/wheel regression to the existing desktop E2E file.
2. Run it before production edits and record the defect-specific failure.
3. Add unit evidence for opt-in hover/focus continuity and unchanged defaults.
4. Implement the bounded shell and local pointer behavior with the existing hover helper.
5. Add keyboard, accessibility, short-summary, and phone scenarios. Keep existing E2E selectors aligned with the scroll body. Run all commands below.
6. Record results and mark this work order and the plan complete.

## Verification

From the repository root, install dependencies if this worktree lacks them:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Run these commands from the repository root. E2E commands rebuild production assets.
Run the two E2E commands sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/task/use-task-icon-tooltip-state.test.ts components/github/pr-task-icon.render.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/use-task-icon-tooltip-state.ts components/github/pr-task-icon.tsx components/github/pr-task-icon-disclosure.tsx components/github/pr-task-icon.render.test.tsx e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts e2e/tests/pr/pr-status-badge.spec.ts e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium -- e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host --project chromium -- e2e/tests/pr/pr-status-badge.spec.ts --grep "shows sidebar automation indicators and refreshes them for active PRs|renders readable task PR summary and compact trailing actions" --workers=1 --retries=0)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts --workers=1 --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/github/pr-task-icon-disclosure.tsx`
- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/task/use-task-icon-tooltip-state.ts`
- `apps/web/components/task/use-task-icon-tooltip-state.test.ts`
- `apps/web/components/github/pr-task-icon.render.test.tsx`
- `apps/web/e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`

The alias `components/integrations/use-change-request-task-tooltip-state.ts`
already re-exports the shared hook and needs no duplicated implementation.

## Dependencies

None. Use the existing `useHoverPopover` helper without changing its public behavior.

## Risks

Pointer-events and focus changes can expose close-order races across the portal gap.
Radix height includes shell padding, so the body needs correct flex sizing.
Shared defaults must remain stable for other indicators.

## Parallelism

`sequential`

## Inputs

- [Summary requirements](../../specs/ui/requirements/pr-task-status-summary.md), AC 001.21 through 001.23.
- [Summary design](../../specs/ui/system-design/pr-task-status-summary.md#disclosure-scrolling).
- `apps/packages/ui/src/tooltip.tsx` and `apps/web/components/integrations/use-hover-popover.ts`.
- Existing desktop hydration and mobile sidebar automation E2E scenarios.

## Results

Implemented the bounded desktop tooltip and its single scroll body. Pointer and focus can move from the trigger into the content without closing it. Escape closes the disclosure and restores trigger focus. Other task-icon callers retain the shared hook's default behavior. The existing phone drawer reaches all five PR entries and automation detail. The Tooltip description is derived from its rendered, localized summary text, so it includes PR identities and statuses without duplicating the focusable scroll body inside Radix's hidden description node. The visible scroll region has a localized name and keyboard focus indicator.

A review follow-up fixed stale Escape suppression when Escape is handled on an already-focused trigger. The hook now arms the suppression only when focus must return to the trigger. The regression confirms the first deliberate refocus reopens the tooltip; content-to-trigger Escape dismissal and default-hook behavior remain covered.

The five-PR desktop E2E regression failed before the production change because the tooltip began above the 420px viewport (top coordinate -806). It passes after the fix.

The PR fixup also corrected two stale desktop E2E selectors that still expected the pre-scroll-wrapper DOM structure, removed a duplicate mobile fixture field, and captured a fresh desktop screenshot with keyboard focus visible.

Verification passed:

- Targeted Vitest: 2 files, 34 tests.
- Web typecheck, targeted ESLint, Prettier, and `pnpm run i18n:ratchet`.
- Managed Chromium E2E: long-summary viewport, keyboard focus, scrolling, and Escape test passed (1); both previously failing PR status badge tests passed (2).
- Managed mobile-chrome E2E: 2 passed.
- Specification catalog validation: 337 decisions and 1271 specifications; linter tests: 36 passed; full specification lint and `git diff --check` passed.
