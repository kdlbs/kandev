---
created: 2026-10-04
status: done
requirements:
  - REQ-UI-QUICK-TERMINAL-001
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
legacy_specs: []
---

# Quick Terminal mobile controls

## Overview

Quick Terminal does not mount the task terminal shortcut bar and sends raw input
to its host-shell socket. Reuse the bar with local focus/input callbacks and
cover real shell behavior before publishing the fix. The user requested
unattended execution through merge, so this package proceeds in this session.

## Scope

Phone shortcut parity, keyboard placement, local input routing, regression tests,
and public how-to guidance. No backend or terminal lifecycle changes.

## Technical approach

Extend MobileTerminalKeybar with inline presentation and callback overrides.
PtyTerminalView owns its sender and focus callback; modifier transforms are
opt-in for Quick Terminal only. QuickTerminalTabView reserves keyboard space
using the existing visual viewport hook. Shared touch buttons meet 44px.

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

## Tests

`pty-terminal-input.test.ts` proves exact socket bytes, modifier consumption,
closed-socket retention, and standard PTY behavior. Existing PTY lifecycle and
keybar tests cover detach and keyboard retention.

## E2E tests

`mobile-quick-terminal.spec.ts` covers shortcut rendering, real interrupt,
modifiers, keyboard geometry, and selected-tab isolation (001.8, 001.13).
`mobile-terminal-keybar.spec.ts` guards the shared task surface.
`quick-terminal.spec.ts` guards desktop host terminals.

## Work orders

- [x] [Task 01: Add mobile controls](task-01-mobile-controls.md)

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

## Verification results

- Unit command: 5 files, 69 tests passed after review remediation. Behavioral RED reproduced raw Ctrl/Shift bytes before the input fix.
- Mobile command built the production assets and backend. All 15 shared task-keybar cases passed; the final changed Quick Terminal spec reran with `pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/terminal/mobile-quick-terminal.spec.ts -- --retries=0`: 2 passed. The pre-fix phone check failed because Control was absent.
- Desktop command: 2 passed, including fine-pointer widths 767/768 and shell continuity.
- Typecheck, changed-file ESLint, i18n checks, public documentation validators, documentation coverage preflight, specification catalog/lint, and diff checks passed.
- Phone screenshots captured and inspected with keyboard closed and simulated keyboard occlusion. Desktop omits the controls, so no desktop screenshot is required.
- Public how-to guidance updated in `docs/public/developer-tools.md`.


## Risks

Host PTYs must never use the task shell-input fallback or steal focus from a
background task. Desktop authentication PTYs must keep raw-input behavior.

## Review remediation

- Modifier ownership regression failed before the correction. Enabled host PTY controls now reset modifiers on activation, owner change, and cleanup. Standard PTYs preserve unrelated modifiers.
- Corrected the three task terminal padding assertions to 58px/358px and added the phone controls section to the requirement/design mapping.
- Final focused unit command passed 69 tests in 5 files. Mobile Quick Terminal spec passed 2 tests, including Ctrl/Shift cleanup across tab selection and dismissal/reopening.
- Typecheck, changed-file ESLint, public documentation validation, specification catalog/lint, and diff checks passed.

Final desktop Quick Terminal rerun passed both desktop/tablet scenarios after the ownership fix.
