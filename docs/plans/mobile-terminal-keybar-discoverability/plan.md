---
created: 2026-11-22
status: complete
requirements:
  - REQ-CLI-MOBILE-PASSTHROUGH-NAV-001
system_design:
  - ../../specs/cli/system-design/mobile-passthrough-navigation.md
legacy_specs: []
---

# Implementation Plan: Mobile Terminal Key Bar Discoverability

## Overview

Phone users can move through an agent CLI menu and confirm a choice from the
first view of the key bar. [Issue #5423](https://github.com/kdlbs/kandev/issues/5423).

CLI owns the raw agent-PTY contract for this interaction. The same web control
serves the task shell and Quick Terminal, so all three surfaces gain the same
key order and swipe cue. See [requirements](../../specs/cli/requirements/mobile-passthrough-navigation.md)
and [system design](../../specs/cli/system-design/mobile-passthrough-navigation.md).

## Root cause and reproduction

`MobileTerminalKeybar` puts Ctrl, Shift, Ctrl+C, Ctrl+D, Esc, and Tab ahead
of Up and Down. Each button is at least 44 CSS pixels wide. The single
row clips overflow with a native horizontal scroller and has no scroll cue.
The shared `KEYS` list has no Enter action.

Reproduce in phone task Chat at 375 × 812 CSS pixels with a connected
passthrough session. Close Chat and Comments, then inspect the bar before a
tap or swipe. Confirm that a swipe reveals the rest of the key row.

An isolated render of the existing component and app styles at `c68b3340d72`
reproduced the clipping in headless Chromium and WebKit. The strip was
375 CSS pixels wide and measured 986 and 976 CSS pixels of scrollable content,
respectively. Up fit in both runs. Down started at x=375 in Chromium and
approximately x=370 in WebKit. Neither render had Enter or an edge cue. A
temporary DOM reorder with an Enter keycap put all three menu keys inside the
strip at 44 pixels high in both browsers. They also fit with 24 CSS pixels
reserved for the swipe icon.

## Scope

### In scope

- Prioritize menu keys, add Enter, and indicate that the row scrolls sideways.
- Cover the phone agent, task-shell footer, and Quick Terminal input paths.
- Update the [phone passthrough and Quick Terminal guide](../../public/developer-tools.md)
  with the new shortcut and swipe cue in the implementation work order.

### Out of scope

- Desktop and tablet key bars.
- PTY transport, agent-specific key mappings, and structured composer behavior.

## Technical approach

Move the navigation group to the start of the shared row. Reuse `KeybarButton`,
the shared sequence map, and each host's focus and send callbacks. Enter sends
carriage return through the same path as other bar keys. The agent bar targets
its current agent socket. Quick Terminal uses its host PTY, and the task-shell
footer uses its session-scoped sender.

Keep native horizontal scrolling and add one horizontal swipe icon beside
the strip. The row always overflows because 20 touch targets take at least
880 CSS pixels and the phone layout starts below 768 CSS pixels. The icon
can be static. The terminal owns vertical output, and the existing hosts own
keyboard and safe-area clearance.

## ASCII UI preview

UI-01. Phone task Chat with a connected agent terminal and both panels closed.
The view is 375 CSS pixels wide and the software keyboard is closed.

```text
Before, initial view
| [Ctrl] [Shift] [^C] [^D] [Esc] [Tab] [Up] [Do|

After, initial view
| [Esc] [Up] [Down] [Left] [Right] [Enter] [Tab] | <-> |
```

The inline row sits below terminal output and above the status row. The
fixed task-shell footer and inline Quick Terminal use the same key order.
The fixed icon at the right indicates that the strip scrolls sideways. The
Enter keycap uses a return glyph with a localized name. Key order and scroll
behavior are structural. Spacing is illustrative. The initial view and swipe
cue map to AC-CLI-MOBILE-PASSTHROUGH-NAV-001.9 and .10, with the shared
behavior in .11.

UI-02. Task Chat at 768 CSS pixels and above, with native terminal controls.

```text
| Agent terminal output and keyboard input         |
| Chat | Comments | Session status                 |
```

Physical keyboard input provides the desktop controls. This maps to
AC-CLI-MOBILE-PASSTHROUGH-NAV-001.8.

## Tests

- Extend the shared bar component suite to cover one carriage return through
  the selected sender.
- Extend the phone passthrough E2E for initial 375px containment, scroll
  cues, and one Enter to the agent PTY. Test containment before Playwright
  can scroll a key into view. Observe the socket URL and terminal output.
- Exercise Enter in the existing task-shell and Quick Terminal flows. Run
  the desktop passthrough scenarios for native input and the 767/768px boundary.

## Work orders

- [x] [Task 01: Expose phone terminal menu keys](task-01-phone-terminal-menu-keys.md).

Task 01 is the sole work order and has no prerequisites. It records exact
test commands and the regression to run before implementation.

## Verification results

The isolated browser reproduction and DOM layout check are design evidence.
The work order records PTY delivery and keyboard-clearance checks after
implementation.

- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.
- `python3 scripts/lint-spec-files.test.py` passed all 36 tests.
- The local PR documentation coverage check accepted the work order and
  its requirement and system-design references.
- The three focused Vitest suites passed all 42 tests. Focused Oxlint and
  `pnpm run typecheck` passed for the changed frontend files.
- `pnpm run i18n:check`, the public-doc tests and validator, spec validation,
  spec lint, and `git diff --check` passed.
- The managed E2E runner built the Go backend and Vite frontend. All 28
  selected mobile Playwright tests and both desktop compatibility tests passed.
- The initial 375px phone view and keyboard-open capture were inspected. No
  physical iOS or Android device was available for an OS-keyboard smoke check.

## Risks

- Playwright can auto-scroll a clipped key on tap. Measure each key inside
  the scroll strip before any interaction.
- The swipe icon takes a small amount of horizontal space. Measure the
  menu keys against the strip with the icon rendered.
- An emulated viewport covers geometry. Physical-device smoke checks cover
  software keyboard focus and clearance when devices are available.
