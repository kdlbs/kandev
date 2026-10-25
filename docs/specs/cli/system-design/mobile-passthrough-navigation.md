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
| `REQ-CLI-MOBILE-PASSTHROUGH-NAV-001` | Components and input ownership; Responsive composition; Lifecycle and failure; Verification |

## Components and input ownership

Main at recovery includes `AgentTerminalInput` and a fixed footer bar from
PR #4521. Preserve that component's desktop branch and the toolbar's ended-session
recovery rendering. Replace its phone branch with the recovered local-socket
implementation and remove the duplicate passthrough footer bar. Task-shell
footer navigation remains unchanged.

The phone branch stays inside `PassthroughToolbar`.
It owns its xterm instance, current socket, one `onData` subscription, and
terminal-specific focus/send callbacks. Use `PassthroughTerminal`'s existing
`onXtermReady`, `onWsReady`, `manualInputRouting`, and `disableWebgl` props.
`manualInputRouting` makes AttachAddon receive-only: the wrapper owns outbound
`onData` input, so modifier transformations cannot also send an unmodified copy.
The wrapper remains mounted when composer/comments visibility changes; hiding
the bar must not reconnect the agent socket.

Move the existing `sendPtyInput` helper and tests from
`components/settings/pty-terminal-input.*` to `lib/terminal/send-pty-input.*`,
then update `PtyTerminalView` to import that same implementation. The helper
already checks socket readiness, applies `applyShellModifiers`, encodes UTF-8,
and consumes latched modifiers after send. Preserve its current behavior.
Both bar taps and the phone terminal's `onData` use this helper. Apply modifiers
only while the phone controls own input. Hidden controls leave native text unchanged. Use the actual current socket, never `sendShellInput`,
`useShellKeySender`, or the global active-shell registry. No backend changes.

Pass `inline`, `onSend`, and `onFocus` to `MobileTerminalKeybar`. Its fallback
sender is never selected for this consumer. Focus the owned xterm textarea
synchronously on tap, using the existing prevent-default pointer behavior.
Keep `KEY_SEQUENCES`, control order, translated labels, and shared styling.
Do not add a second key map, modifier store, or bar component.

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

## Related delivery

- [Implementation plan](../../../plans/mobile-passthrough-navigation/plan.md).
