---
status: draft
system: ui
requirements:
  - REQ-UI-QUICK-TERMINAL-001
created: 2026-11-11
owners:
  - kandev
---

# Quick Terminal render lifecycle

## Purpose and ownership

This design defines browser resource continuity within the existing Quick Terminal interaction contract.
It complements the [shared-tab design](quick-terminal.md).
The backend retains descriptor membership, host-shell identity, process lifetime, and rolling output.
[Server-owned descriptors](../../../decisions/2026-08-05-server-owned-quick-terminal-descriptors.md) remain authoritative.

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-QUICK-TERMINAL-001` | Stable Quick Terminal startup; stream ownership; lifecycle compatibility; verification |

The design covers AC-UI-QUICK-TERMINAL-001.15 and .16, with compatibility for .8, .13, and .14.

## Stable Quick Terminal startup

`QuickChatTerminalContent` keys `QuickTerminalTabView` by `tabId`.
The modal can supply a new `onStateChange` callback on each render.
That notification callback does not identify a terminal resource.

`QuickTerminalTabView.startSession` uses the existing `onStateChangeRef` and has stable callback identity.
After `startHostShell` resolves, the callback reads the latest notification handler.
The wrapper retains its existing startup notification, stable `clientId`, and descriptor-create cancellation rules.
`PtyTerminalView` already reads later notifications through `reportRef`.

The shared mount effect retains its `startSession` and `initialInput` dependencies.
These inputs can change startup behavior for login and recovery-command callers.
This repair stabilizes the Quick Terminal adapter without redefining the shared component's reset semantics.
Presentation updates, including phone controls and viewport offsets, do not become resource identities.

For one uninterrupted mounted generation, the client creates one xterm instance and one stream socket.
Descriptor persistence and unrelated store notifications do not create another generation.
Actual dismissal or tab selection can still detach the view and later attach a fresh view to the retained PTY.
Development StrictMode can replay mount setup under the existing generation and backend idempotency rules.

## Stream ownership

`mountSession` owns a cancellation token for its xterm, socket, resize observer, and input subscription.
The token reaches `openSessionWebSocket` through `attachOrStart` and `reportSession`.
Group stream callbacks in an argument object if necessary to retain the five-parameter function limit.

Each stream handler tests the token before it performs any action:

- `onmessage` ignores both binary output and JSON exit frames after cancellation.
- `onerror` cannot report an error or disarm a replacement session after cancellation.
- `onclose` cannot disarm a replacement session after cancellation.
- The one-time `open` listener cannot send `initialInput` after cancellation.

Cleanup cancels the token before it closes the socket or disposes xterm.
Guards also cover callbacks already retained or queued before cleanup.
Cancellation belongs to each mount generation, so an old socket cannot clear a newer generation's `sessionIdRef`.
Live output, exit, connection errors, and resize disarming retain their existing behavior.
The change adds no reconnect, retry, heartbeat, or keepalive mechanism.

## Lifecycle compatibility

| Consumer or condition | Preserved behavior | Evidence |
| --- | --- | --- |
| Selected Quick Terminal | Callback churn keeps one xterm and socket | Real adapter/view integration test and browser input test |
| Tab switch or dialog dismissal | Detach the browser view and retain the backend PTY | Existing PTY and desktop/phone terminal tests |
| Explicit close during startup | Pending-start cancellation still stops the resolved PTY | Existing cancellation regression |
| Detached start resolves late | Publish its session identity without creating a socket or writing to disposed xterm | Existing late-start identity regression |
| Standard login or host-shell dialog | Keep stop-on-unmount and startup-input behavior | Existing shared PTY and agent-login tests |
| StrictMode replay | Keep generation guards and per-tab host-shell idempotency | Existing race regressions |
| Genuine live stream error or exit | Report the existing localized state and disarm resize | Shared PTY event regressions |
| Task `PassthroughTerminal` | Keep its independent transport | No change to that component |

The stream cancellation guard must not suppress every late asynchronous result.
An uncancelled pending Quick Terminal start can finish after dismissal and still publish its identity.
Explicit tab close needs that identity to stop the correct PTY.
An explicitly cancelled pending start still follows `cancelPtyTerminalStart` and `stopIfLateStart`.

## Desktop and phone behavior

Both surfaces share the same adapter, cancellation token, state, and send path.
Desktop keeps the floating Quick Chat dialog.
Phone keeps the full-height dynamic-viewport dialog, fixed tab strip, and inline shortcut row.
The terminal owns vertical output scrolling, while the shortcut row owns its horizontal overflow.
Existing visual-viewport offsets and dialog safe-area padding retain keyboard clearance.
The task terminal keybar is the existing phone interaction exemplar.

Neither surface gains controls or copy.
The selected terminal retains its output and focused input while unrelated store data changes.
Phone Ctrl/Shift ownership and focus behavior remain covered by the existing shortcut tests.

## Verification

The adapter integration suite uses real `QuickTerminalTabView` and `PtyTerminalView` components.
Only xterm, WebSocket, resize observation, and API boundaries are mocked.
It covers fresh inline callbacks, state-persisting parents, and errors from closing CONNECTING sockets.
Tests count construction, start, stream, disposal, and close operations.
The callback test also resolves a deferred start after a handler replacement and checks the latest recipient.

Shared PTY tests retain old event handlers across cleanup and a replacement mount.
They prove that late error, close, output, exit, and open events have no effects.
Live-event controls prevent a guard from disabling normal terminal behavior.

Desktop and phone Playwright tests type a command across independent store updates.
They assert the original xterm node, focused textarea, one stream, and assembled command output.
Existing detach, reload, explicit close, sibling isolation, and phone-keyboard tests remain in the targeted runs.

## Implementation plan

[Quick Terminal render continuity](../../../plans/quick-terminal-render-continuity/plan.md).
