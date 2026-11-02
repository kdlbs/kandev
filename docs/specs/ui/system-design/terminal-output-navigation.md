---
status: current
system: ui
requirements:
  - REQ-UI-TERMINAL-OUTPUT-NAVIGATION-001
---

# Terminal Output Navigation System Design

## Purpose and boundaries

This is a client-side viewport action shared by task shell and dev-server output. The existing [touch scrolling](terminal-touch-scrolling.md) and [terminal rendering](terminal-rendering.md) contracts remain independent. No new server, persistence, transport, or terminal lifecycle is introduced.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-TERMINAL-OUTPUT-NAVIGATION-001` | Viewport state, Scope and composition, Verification |

## Viewport state

Use the current xterm instance as authority: normal `buffer.active.viewportY < buffer.active.baseY` means there is later scrollback. Recompute after initialization, xterm scroll, parsed output, resize and active-buffer changes, using xterm's public events; verify event ordering against the installed xterm types and behavior during implementation. Reuse the hosts' initialized readiness, current refs and event lifetimes; do not introduce speculative lifecycle infrastructure. Do not poll or change the write path to follow output. xterm normally retains the scrolled viewport as output arrives, subject to scrollback eviction; prove this with real xterm. Only the explicit action calls `scrollToBottom()` on the current instance. Recompute when it moves. Dispose the subscriptions with the existing instance lifecycle and avoid stale callbacks into a prior instance.

Hide the action before readiness, when the host is inactive or covered, during search, and in the alternate buffer. A normal buffer returning from alternate is re-evaluated. `PassthroughTerminal` uses `computeTerminalPaneState`, workspace restoration and `search.isOpen` to gate a task shell. `ShellTerminal` gates its read-only dev-server output on readiness and search; its process stopping indicator is not a blocking overlay. A future or other caller must not gain this action accidentally. Avoid depending solely on `mode="shell"` if a non-task caller shares that mode: explicitly scope the task hosts as needed.

## Scope and composition

`terminal-panel.tsx`, `bottom-terminal-panel.tsx`, `task-right-panel-tab-contents.tsx` and `mobile/mobile-terminal-pane.tsx` are task-shell entry points. `terminal-panel.tsx` and `task-right-panel-tab-contents.tsx` render `ShellTerminal` with `processOutput` for dev-server output. Keep agent, Quick Terminal, login, and unrelated interactive `ShellTerminal` uses excluded. Share viewport tracking and an accessible localized button where practical, but keep eligibility with each host.

The action sits inside the terminal's lower trailing edge, above any key row and within its clipped viewport. It does not create another scroll owner. Desktop uses the ordinary 28px control; phone and coarse pointers use at least 44px in both dimensions. The existing `MobileTerminalPane` supplies the full-height phone terminal, picker and key-row context; the safe-area-aware `mobile-fab.tsx` is the geometry exemplar, not a new global fixed element. Account for dynamic viewport and keyboard through the host's existing available bounds. The button is visible without hover, has a localized accessible name and supports keyboard activation. Do not unconditionally focus the terminal on activation or steal focus from another control. Hide it while search or a full-pane status overlay is shown rather than allowing overlapping controls.

## Verification

Host integration coverage renders the existing terminal hosts with controlled xterm state and proves the missing action in RED, then visibility, scroll activation, no input send, focus behavior and cleanup in GREEN. An unresolved new module or symbol is not behavioral RED evidence. Browser coverage uses real routed task surfaces, actual xterm and real isolated backend fixtures: desktop shell and dev-server output plus phone shell via `MobileTerminalPane`. Observe viewport and content before and after output and activation; do not replace the routed surface with a test harness. The [work order](../../../plans/terminal-output-navigation/task-01-jump-to-latest.md) defines commands and geometry checks.
