---
status: current
system: cli
requirements:
  - REQ-CLI-MOBILE-PASSTHROUGH-NAV-001
---

# Mobile Passthrough Navigation System Design

## Purpose and boundaries

CLI owns raw agent-PTY navigation. Reuse the UI's `MobileTerminalKeybar` and
existing modifier transforms while keeping agent input separate from task-shell
fallback and `message.add`. This extends the raw-terminal capability excluded
from the [composer design](mobile-passthrough-composer.md); it does not change
that composer's submission contract.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-CLI-MOBILE-PASSTHROUGH-NAV-001` | Components and input ownership, shared key bar, responsive composition, lifecycle and failure, verification |

## Components and input ownership

The phone branch of `AgentTerminalInput` stays inside `PassthroughToolbar`.
`MobileAgentTerminal` owns its xterm instance, current socket, one `onData`
subscription, and terminal-specific focus/send callbacks. It uses
`PassthroughTerminal`'s `onXtermReady`, `onWsReady`, `manualInputRouting`,
and `disableWebgl` props.
`manualInputRouting` makes AttachAddon receive-only: the wrapper owns outbound
`onData` input, so modifier transformations cannot also send an unmodified copy.
The wrapper remains mounted when composer/comments visibility changes; hiding
the bar must not reconnect the agent socket.

Both bar taps and the phone terminal's `onData` call
`lib/terminal/send-pty-input.ts` with the current agent socket. The helper checks
readiness, applies `applyShellModifiers`, and sends UTF-8 bytes. It consumes
latched modifiers after sending. Modifier transforms apply while the phone
controls own input. `PtyTerminalView` uses the same helper with its host PTY.

The phone agent passes `inline`, `onSend`, and `onFocus` to
`MobileTerminalKeybar`. A tap focuses the owned xterm textarea synchronously
using `KeybarButton`'s prevent-default pointer behavior. The task-shell footer
uses the bar's session-scoped sender. Quick Terminal supplies local callbacks
through `PtyTerminalView`.

## Shared key bar

Use one key order for the inline and fixed bars. Place Esc, Up, Down, Left,
Right, Enter, and Tab first. Follow them with Ctrl, Shift, Ctrl+C, Ctrl+D,
and the existing Home, End, page, and symbol keys.

Put Enter in the shared key map with `KEY_SEQUENCES.enter` set to `"\r"`.
This matches the byte xterm sends for an unmodified physical Enter. Use the
existing key button with the return glyph and a translated accessible name.
The existing sender delivers that byte to the selected terminal. Shell
arrow taps continue through the existing cursor-mode-aware sender.

Keep the strip as a native horizontal scroller. The phone bar is shown
below 768 CSS pixels. Its 20 keys require at least 880 CSS pixels before
spacing, so the strip always overflows in phone layout.

Put one fixed horizontal swipe icon in a narrow gutter beside the strip.
The icon indicates the scroll axis at any scroll position. Hide it from
assistive technology because it is decorative. Keep the full touch area of
each key within the strip and confirm that Up, Down, and Enter fit at 375 CSS
pixels. Use the same icon and placement for the inline bar and fixed footer.

## Responsive composition

Use `useResponsiveBreakpoint().isMobile`, including narrow fine-pointer screens.
Above the phone boundary, keep the existing `PassthroughTerminal` branch.

Entry point: task Chat with a passthrough session. The nearest shipped exemplar
is `MobileTerminalPane` for terminal ownership and `PtyTerminalView` for inline
key-bar callbacks. The curated `task-layout.tsx` precedent supplies the dedicated
phone composition. A persistent inline strip fits frequent menu navigation;
a drawer would hide the keys during the interaction that needs them.

Hierarchy: session picker, flexible terminal output, inline navigation strip,
existing passthrough status row, and task bottom navigation. The strip scrolls
horizontally inside its own width, retaining 44px keys. Terminal output remains
the flexible vertical scroll owner. When composer/comments opens, omit the raw
strip and leave the existing composer/comments composition intact.

`MobilePanelArea` owns application bottom clearance. For passthrough Chat,
reserve the greater of its existing bottom-navigation plus safe-area clearance
and `useVisualViewportOffset().bottomOffset`. Replace that branch's baseline
padding rather than adding keyboard occlusion twice. Do not modify managed Chat
or shell Terminal padding. The inline bar occupies `KEYBAR_HEIGHT_PX` in flow;
do not also reserve that height as fixed-overlay padding. Keyboard close and
Visual Viewport absence restore the existing navigation/safe-area clearance.
Preserve `min-h-0`, `100dvh` application bounds, theme tokens, and font choices.

## Lifecycle and failure

Show the bar only with a session identity, ready xterm, and OPEN owned socket,
and while neither composer nor comments is expanded and Quick Chat is closed. Keep callbacks stable
across readiness changes so the connection effect does not reconnect on render.
Socket replacement updates the sender. Close handlers clear only the socket
they observed; stale close events cannot clear a replacement.

Dispose the wrapper's `onData` subscription on xterm replacement or unmount.
Reset shared modifiers on ownership activation, session change, bar hiding,
and cleanup. Use the existing single-visible-terminal model; do not register
an agent sender in the shell registry. Do not reset an unrelated terminal from
a consumer that never activated phone controls. Connecting/disconnected
terminals keep their existing overlay; the navigation strip is omitted.
Unavailable input is neither buffered nor redirected.

Opening Quick Chat releases the underlying agent's modifier ownership. The
agent can remain mounted and reconnect in the background, but it must neither
reset nor consume the overlay terminal's modifiers. Closing the overlay lets
the connected agent acquire controls with cleared modifier state.

## Persistence and security

No persisted state, migrations, runtime flags, or authentication changes.
Navigation uses the existing authenticated binary
`/terminal/session/{sessionId}?mode=agent` connection. Structured messages remain
on `message.add`; native permission choices retain the agent CLI's own semantics.

## Verification

Unit/component tests cover exact socket bytes, listener uniqueness, readiness
in either initialization order, replacement socket races, focus, cleanup,
visibility, composer coexistence, and viewport clearance. Passive Playwright
frame capture must retain the socket URL: existing `attachShellInputCapture`
reports an empty session ID for dedicated sockets and alone cannot prove agent
targeting. Scope observation to the exact agent URL, verify no task-shell frames
or new user messages, and pair wire evidence with visible native-terminal output.
Run phone and desktop regressions and inspect keyboard-open/closed captures.
Record real iPhone Safari and Android Chrome focus/keyboard smoke evidence when
available; emulated viewport changes do not prove OS keyboard behavior.

For AC-CLI-MOBILE-PASSTHROUGH-NAV-001.9 and .10, measure the three menu keys
against the strip before the first tap, which may auto-scroll in Playwright.
Check that the swipe cue is visible and a trailing key remains reachable.
Exercise Enter in each of the three terminals to cover delivery in .2 and .11.

## Related delivery

- [Implementation plan](../../../plans/mobile-passthrough-navigation/plan.md).
- [Key bar fix plan](../../../plans/mobile-terminal-keybar-discoverability/plan.md).
