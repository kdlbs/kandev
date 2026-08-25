---
status: draft
system: ui
requirements:
  - REQ-UI-TERMINAL-RENDERING-001
---

# Terminal Rendering System Design

## Purpose and boundaries

This design defines one theme contract for every xterm constructor in the web application. The contract changes terminal presentation only. Backend shell sessions, WebSocket transport, PTY state, and stored user settings keep their current owners.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-TERMINAL-RENDERING-001` | [Shared theme contract](#shared-theme-contract), [Live theme flow](#live-theme-flow), [Responsive behavior](#responsive-behavior) |

## Confirmed fault

`getTerminalTheme` reads the current application background and foreground. It then adds one ANSI palette that was made for a dark background. Light mode therefore combines a white background with low-contrast white, yellow, cyan, blue, and magenta ANSI colors.

The consumers read the theme only when they construct xterm. An open terminal therefore keeps stale colors after the resolved theme changes.

## Components and responsibilities

- `apps/web/lib/theme/terminal-theme.ts` owns the light and dark ANSI palettes. It also owns the minimum contrast setting and the xterm theme builder.
- `apps/web/components/task/use-terminal-theme.ts` synchronizes an existing xterm instance with the resolved application theme.
- `apps/web/components/task/shell-terminal.tsx` supplies the resolved theme to development-server and shell-output terminals.
- `apps/web/components/task/use-passthrough-terminal.ts` supplies the initial theme to task and agent terminals.
- `apps/web/components/task/passthrough-terminal.tsx` keeps task, agent, desktop, tablet, and phone instances synchronized.
- `apps/web/components/settings/pty-terminal-view.tsx` uses the shared fixed-dark theme for Quick Terminal and agent-login PTYs.
- `apps/web/components/task/terminal-buffer-reader.ts` exposes bounded xterm theme data for browser tests. It removes that data during terminal cleanup.

## Shared theme contract

The theme builder receives an explicit `ResolvedTheme`. It selects a matching ANSI palette and combines it with the terminal surface colors.

Every xterm constructor sets `minimumContrastRatio` to `4.5`. Xterm can then adjust ANSI cells that do not meet the contrast requirement. The configured palette still keeps distinct ANSI hues before this final renderer adjustment.

Adaptive terminals use the application background, foreground, cursor, and selection colors. Fixed-dark terminals use the dark base and ANSI palette in both application themes.

## Live theme flow

1. `useTheme` supplies the current `resolvedTheme` to each adaptive terminal.
2. `AppThemeProvider` applies the initial root theme class in a layout effect,
   before descendant terminal construction effects run.
3. The constructor applies the matching theme before `terminal.open` shows output.
4. A synchronization hook observes later `resolvedTheme` changes.
5. The hook waits for the root theme class and CSS variables to update.
6. The hook assigns the new value to `terminal.options.theme`.
7. Xterm redraws the existing buffer without a new terminal or connection.

The synchronization hook cancels pending work during unmount. It does nothing when the terminal is not ready or is already disposed.

## Responsive behavior

This repair does not change terminal composition. Desktop Dockview, tablet panels, and `MobileTerminalPane` continue to share `PassthroughTerminal` and its state.

The nearest mobile implementation is `apps/web/components/task/mobile/mobile-terminal-pane.tsx`. The Pixel 5 test opens that existing surface and checks the same light-theme contract. The repair adds no drawer, touch target, safe-area rule, or scroll owner.

## Failure and recovery

The initial root-theme layout effect and constructor path prevent a light-theme
flash during terminal creation. The synchronization path handles later theme
changes.

If a theme update races terminal disposal, the cancellation guard prevents writes to the disposed instance. A theme update never reconnects the socket, clears the buffer, or changes terminal dimensions.

## Test contract

Unit tests cover palette selection, distinct light bright variants, the minimum
contrast value, initial root-theme ordering, and deferred hook cleanup. Browser
tests cover the theme that reaches a live xterm instance.

The desktop browser test changes the theme while one terminal remains open. It
checks that the buffer, focus, scroll position, and running command survive,
and that later shell output uses the same connection.

The mobile browser test opens the existing phone terminal in light mode. It checks the same palette and contrast contract without changing the phone composition.

## Related decisions

No architecture decision record applies. This repair keeps the existing UI theme and xterm ownership boundaries.
