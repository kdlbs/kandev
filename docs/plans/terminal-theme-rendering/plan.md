---
created: 2026-08-25
status: complete
requirements:
  - REQ-UI-TERMINAL-RENDERING-001
system_design:
  - ../../specs/ui/system-design/terminal-rendering.md
legacy_specs: []
---

# Implementation Plan: Terminal Theme Rendering

## Overview

The current renderer combines a light background with a dark-only ANSI palette. It also captures theme values only when xterm starts. This plan adds red regression evidence first, then implements one shared adaptive theme contract for all xterm constructors.

## Scope

### In scope

- Add separate light and dark ANSI palettes.
- Enforce a 4.5:1 minimum contrast ratio through xterm.
- Update open adaptive terminals when the resolved theme changes.
- Keep fixed-dark terminals on the shared dark palette.
- Cover desktop and phone task terminals with Playwright, and document the
  shared rendering path used by tablet task terminals.

### Out of scope

- Change terminal transport, PTY lifecycle, output buffering, or shell configuration.
- Add user-defined palettes or new appearance settings.
- Change terminal layout, mobile composition, keyboard controls, or scrolling.
- Change the visual design of Quick Terminal or agent-login dialogs.

## Technical approach

### Shared palette

Update `apps/web/lib/theme/terminal-theme.ts` to select an ANSI palette from an explicit `ResolvedTheme`. Export one minimum contrast value for every xterm constructor.

Use the shared fixed-dark result in `PtyTerminalView`. Remove its isolated background-only xterm theme.

### Reactive application

Add `apps/web/components/task/use-terminal-theme.ts`. The hook receives the xterm reference, host reference, readiness state, and resolved theme.

The hook updates `terminal.options.theme` after the root theme class commits. It cancels pending work during cleanup and never reconstructs the terminal.

`AppThemeProvider` applies the initial root theme class in a layout effect, so
descendant terminal construction effects read CSS variables for the resolved
theme on the first render.

Pass `resolvedTheme` through `ShellTerminal` and `PassthroughTerminal`. Keep their current connection, resize, input, search, and mobile behavior.

### Browser evidence

Extend the existing xterm test bridge with a read-only theme snapshot. Clear the snapshot function with the other bridge functions during cleanup.

Add desktop and mobile terminal theme specs. The desktop flow changes theme while the terminal stays mounted. The mobile flow uses the existing Pixel 5 terminal surface in light mode.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-UI-TERMINAL-RENDERING-001.1` | `terminal-theme.test.ts` selects the initial light and dark palettes. |
| `AC-UI-TERMINAL-RENDERING-001.2` | `terminal-theme.test.ts` checks the shared contrast setting. Desktop and mobile specs inspect the live xterm value. |
| `AC-UI-TERMINAL-RENDERING-001.3` | `terminal-theme.spec.ts` changes the application theme and observes the live xterm theme. |
| `AC-UI-TERMINAL-RENDERING-001.4` | `terminal-theme.spec.ts` keeps buffer content, focus, scroll position, and a running command while accepting new shell output after the theme change. |
| `AC-UI-TERMINAL-RENDERING-001.5` | Desktop and `mobile-terminal-theme.spec.ts` check the same light-theme contract. |
| `AC-UI-TERMINAL-RENDERING-001.6` | `terminal-theme.test.ts` checks the shared fixed-dark theme. |

The first work order must fail against the current implementation. The expected unit failure is `uses a readable ANSI palette in light mode`. The expected desktop failure is `updates an open terminal when the application theme changes`.

## E2E tests

- `apps/web/e2e/tests/terminal/terminal-theme.spec.ts` uses the `chromium` project for `AC-UI-TERMINAL-RENDERING-001.1` through `.4`.
- `apps/web/e2e/tests/terminal/mobile-terminal-theme.spec.ts` uses the `mobile-chrome` Pixel 5 project for `AC-UI-TERMINAL-RENDERING-001.2` and `.5`.
- Tablet task terminals use the same `PassthroughTerminal` and
  `useTerminalTheme` path as the desktop and phone surfaces. The shared-path
  evidence covers tablet rendering without adding a separate viewport test.

## Work orders

- [x] [Task 01: Add terminal theme regressions](task-01-add-terminal-theme-regressions.md)
- [x] [Task 02: Implement adaptive terminal themes](task-02-implement-adaptive-terminal-themes.md)

## Verification results

Passed on 2026-08-25:

- `cd apps/web && pnpm test -- lib/theme/terminal-theme.test.ts`
- `cd apps/web && pnpm test -- components/task/use-terminal-theme.test.ts components/theme/app-theme.test.tsx`
- `cd apps/web && pnpm run typecheck`
- Targeted ESLint for all changed terminal, theme, bridge, and E2E files with zero warnings.
- `cd apps/web && pnpm e2e:run tests/terminal/terminal-theme.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-terminal-theme.spec.ts`

## Risks

- A theme effect can read old CSS variables before the root theme class changes. The browser test must change theme through Kandev UI.
- Reconstructing xterm can clear the buffer or reconnect the shell. The implementation must mutate `terminal.options.theme` only.
- WebGL and canvas renderers use the same xterm theme contract. The mobile path keeps WebGL disabled for its existing scaling rule.
- The test bridge must remain read-only and must remove every added function during cleanup.
