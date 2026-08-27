---
id: "01-silence-launcher-focus"
title: "Silence automatic launcher focus"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-001.9
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
---

# Task 01: Silence Automatic Launcher Focus

## Summary

Keep focus on the launcher after Escape closes Quick Chat. Hide the focus appearance for this one
automatic return. Restore the normal appearance after focus leaves.

## In scope

- Add and remove the transient marker in the shared focus helper.
- Add the scoped silent-focus selector.
- Add unit coverage for marker lifecycle and disconnected launchers.
- Add Chromium coverage for silent return and later normal keyboard focus.
- Run the current Pixel 5 touch-close scenario as the mobile rendered check.

## Out of scope

- Global focus-style changes.
- New application state or persisted settings.
- Layout, copy, tooltip, or mobile interaction changes.

## Acceptance

- Escape closes the dialog and keeps focus on its launcher without visible focus decoration.
- The marker is removed on blur. Later keyboard focus uses the normal focus decoration.
- Shortcut and command-palette origins regain focus without the silent marker.
- Pointer, tooltip, disconnected-launcher, and mobile touch-close behavior do not change.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web exec vitest run components/quick-chat/quick-chat-focus.test.ts components/quick-chat/quick-chat-provider-focus.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx)
(cd apps && pnpm --filter @kandev/web exec vitest run components/global-commands.test.tsx)
(cd apps && pnpm --filter @kandev/web exec eslint components/quick-chat/quick-chat-focus.ts components/quick-chat/quick-chat-focus.test.ts components/quick-chat/quick-chat-provider-focus.test.tsx components/app-sidebar/app-sidebar-new-task-item.tsx)
(cd apps/web && pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "returns focus without a visible indicator")
(cd apps/web && pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "returns focus to a shortcut origin without a silent marker")
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-quick-chat-entry.spec.ts -- --grep "opens from the home header and closes with the touch control")
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-focus.ts`
- `apps/web/components/quick-chat/quick-chat-focus.test.ts`
- `apps/web/components/global-commands.tsx`
- `apps/web/components/global-commands.test.tsx`
- `apps/web/hooks/use-quick-chat-launcher.ts`
- `apps/web/components/quick-chat/quick-chat-provider-focus.test.tsx`
- `apps/web/app/globals.css`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`
- `docs/specs/ui/requirements/quick-terminal.md`
- `docs/specs/ui/system-design/quick-terminal.md`
- `docs/plans/quick-chat-silent-focus-return/plan.md`
- `docs/plans/quick-chat-silent-focus-return/task-01-silence-launcher-focus.md`

## Dependencies

None.

## Risks

- The marker cleanup must run on every blur path.
- The CSS selector must override utility focus styles without affecting other buttons.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-QUICK-TERMINAL-001` and `AC-UI-QUICK-TERMINAL-001.9`.
- The launcher focus-return section in the Quick Chat and Terminal Tabs system design.
- Existing unit tests in `apps/web/components/quick-chat/quick-chat-focus.test.ts`.
- Existing focus and tooltip assertions in `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`.

## Results

- Added a transient `data-quick-chat-silent-focus` marker to automatic launcher focus restoration.
- Removed the marker on the launcher's first blur and skipped marking disconnected launchers.
- Added an explicit non-launcher focus option for global keyboard shortcuts and command-palette
  actions, preserving their focus origin without applying silent styling.
- Added a scoped focus-visible style that hides only the automatic return indicator.
- Unit tests: 34 passed across the four targeted Vitest files.
- ESLint: passed for all changed frontend source and test files.
- Chromium E2E: 2 passed for launcher and shortcut-origin focus return.
- Pixel 5 E2E: 1 passed for the existing mobile touch-close scenario.
- Specification lint and `git diff --check`: passed.
