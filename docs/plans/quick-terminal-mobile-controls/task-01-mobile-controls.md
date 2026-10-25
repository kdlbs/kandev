---
id: "01-mobile-controls"
title: "Add Quick Terminal mobile controls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-001
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-001.8
  - AC-UI-QUICK-TERMINAL-001.13
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
---

# Task 01: Add Quick Terminal mobile controls

## Summary

Expose the task terminal's shortcut set on phone Quick Terminals with local
PTY input and focus ownership. Validate interrupt and modifiers in a real shell.

## In scope

Shared keybar callbacks, inline geometry, host PTY input transforms, phone
keyboard clearance, focused regressions, screenshots, and public documentation.

## Out of scope

Backend changes, terminal persistence, and authentication terminal controls.

## Acceptance

- The active running phone Quick Terminal has the complete shared shortcut row.
- Input and refocus target its own xterm/socket and preserve modifier semantics.
- Controls clear keyboard/safe areas; desktop and shared task terminals pass checks.

## ASCII UI preview

UI-01: Quick Terminal, launched from the phone app menu, running.

```text
Phone before              Phone after
[Terminal tabs | + | X]   [Terminal tabs | + | X]
[host shell output     ]  [host shell output     ]
[OS keyboard           ]  [Ctrl Shift ^C ^D Esc Tab ->]
                          [OS keyboard           ]

Desktop: [Terminal tabs | + | X]
         [host shell output     ]
```

The shortcut row is inline, horizontally scrollable, and clears safe areas.
Terminal output owns vertical scrolling. The phone keyboard reduces terminal
space; controls retain focus. Connecting/exited/error terminals omit the row.
Control order and phone-only composition are required; spacing is illustrative.
Maps to AC-UI-QUICK-TERMINAL-001.13.

Full preview: [plan](plan.md#ascii-ui-preview).

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/settings/pty-terminal-input.test.ts components/settings/pty-terminal-view.test.tsx components/task/mobile/mobile-terminal-keybar.test.tsx components/quick-chat/quick-terminal-tab-view.test.tsx components/task/mobile/session-mobile-layout.test.tsx)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/terminal/mobile-quick-terminal.spec.ts e2e/tests/terminal/mobile-terminal-keybar.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build e2e/tests/terminal/quick-terminal.spec.ts -- --retries=0)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/web/components/task/mobile/mobile-terminal-keybar.tsx
- apps/web/components/task/mobile/mobile-terminal-keybar-helpers.tsx
- apps/web/components/settings/pty-terminal-view.tsx
- apps/web/components/settings/pty-terminal-input.ts
- apps/web/components/settings/pty-terminal-input.test.ts
- apps/web/components/quick-chat/quick-terminal-tab-view.tsx
- apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts
- docs/public/developer-tools.md

## Dependencies

None.

## Risks

Do not route host input through the active task terminal registry.

## Parallelism

sequential

## Inputs

Quick Terminal requirement/design, existing task keybar and host PTY renderer.

## Results

- Unit command: 5 files, 69 tests passed after review remediation. Behavioral RED reproduced raw Ctrl/Shift bytes before the input fix.
- Mobile command built the production assets and backend. All 15 shared task-keybar cases passed; the final changed Quick Terminal spec reran with `pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/terminal/mobile-quick-terminal.spec.ts -- --retries=0`: 2 passed. The pre-fix phone check failed because Control was absent.
- Desktop command: 2 passed, including fine-pointer widths 767/768 and shell continuity.
- Typecheck, changed-file ESLint, i18n checks, public documentation validators, documentation coverage preflight, specification catalog/lint, and diff checks passed.
- Phone screenshots captured and inspected with keyboard closed and simulated keyboard occlusion. Desktop omits the controls, so no desktop screenshot is required.
- Public how-to guidance updated in `docs/public/developer-tools.md`.


## Review remediation

- Modifier ownership regression failed before the correction. Enabled host PTY controls now reset modifiers on activation, owner change, and cleanup. Standard PTYs preserve unrelated modifiers.
- Corrected the three task terminal padding assertions to 58px/358px and added the phone controls section to the requirement/design mapping.
- Final focused unit command passed 69 tests in 5 files. Mobile Quick Terminal spec passed 2 tests, including Ctrl/Shift cleanup across tab selection and dismissal/reopening.
- Typecheck, changed-file ESLint, public documentation validation, specification catalog/lint, and diff checks passed.

Final desktop Quick Terminal rerun passed both desktop/tablet scenarios after the ownership fix.
