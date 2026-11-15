---
status: current
system: ui
requirements:
  - REQ-UI-QUICK-TERMINAL-001
created: 2026-11-11
updated: 2026-11-11
owners:
  - kandev
---

# Quick Terminal Shell Recovery

## Purpose and boundaries

This design extends the existing [Quick Terminal interaction contract](quick-terminal.md).
It defines explicit recovery inside a persisted tab after shell loss.
The backend descriptor service and `loginpty.Manager` retain their existing ownership.

The [descriptor decision](../../../decisions/2026-08-05-server-owned-quick-terminal-descriptors.md)
requires an explicit start after shell loss. A visible recovery action satisfies that constraint.
Automatic restart on mount remains excluded because shell exit can be intentional.
The interaction adds no endpoint, schema, runtime flag, or durable history.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| `REQ-UI-QUICK-TERMINAL-001` | [Recovery flow](#recovery-flow), [Attempt ownership](#attempt-ownership), [Responsive presentation](#responsive-presentation) |
| `AC-UI-QUICK-TERMINAL-001.15`, `001.16` | [Recovery flow](#recovery-flow) |
| `AC-UI-QUICK-TERMINAL-001.14`, `001.17`, `001.18`, `001.20` | [Attempt ownership](#attempt-ownership) |
| `AC-UI-QUICK-TERMINAL-001.8`, `001.19` | [Responsive presentation](#responsive-presentation) |

## Components and responsibilities

- `QuickTerminalTabView` shows recovery beside the selected tab's lifecycle status.
  It owns the terminal mount for one admitted recovery attempt.
- `useQuickChatModal` supplies terminal lifecycle callbacks and preserves descriptor identity.
  A focused terminal lifecycle hook can contain request ordering and recovery preparation.
- `PtyTerminalView` keeps its shared attach/start behavior and `detach-on-unmount` policy.
  Its initial session ref belongs to one mount and must not carry an exited shell into recovery.
- `quickterminal.Service.List` reconciles durable descriptors against the live PTY manager.
  `BindHostShellSession` records a successful start under the same descriptor.
- `loginpty.Handlers.httpStartHostShell` and `Manager.StartWithKey` retain per-tab idempotency.
  A repeated start reuses an active manager entry rather than terminating it.

## Recovery flow

1. An exited or unavailable selected tab shows **Start new shell**.
   Initial restoration, selection, and dialog reopening never invoke recovery.
2. Activation immediately admits one attempt and disables its action.
   The attempt retires callbacks from the previous terminal mount.
3. Pending lifecycle writes for that tab settle before recovery starts a replacement.
   A rejected older write must not prevent a fresh descriptor read.
4. Read `listQuickTerminalTabs(workspaceId)` and locate the existing `tabId`.
   A missing descriptor or denied read fails visibly without creating a descriptor or PTY.
5. If reconciliation returns a live session, attach to it.
   This covers another client recovering the tab before the read completes.
6. Otherwise, prepare a local connecting state with no session, exit code, or previous error.
   Reuse the confirmed descriptor instead of the new-tab descriptor-create path.
   In particular, do not restore an old exited `sessionId` from an idempotent create response.
7. Mount a fresh `PtyTerminalView` and call `startHostShell` with `clientId = tabId`.
   The existing host-shell handler binds its result before it returns.
8. Publish the returned session through the existing lifecycle path.
   Keep `tabId`, workspace, sequence, saved order, and active selection unchanged.
   Restore ordinary terminal focus and the phone shortcut controls.

The action remains available after a failed recovery, including after reload.
An unbound error tab can use the same action when its persisted descriptor still exists.
A transport error attached to a live shell does not authorize stopping that shell.
An idempotent start that discovers a live shell reuses it.

## Attempt ownership

Recovery preparation and lifecycle writes share an owner for each tab in the active client.
No queue or pending marker applies to sibling tabs.
The owner survives terminal presentation detach long enough to settle admitted work.
`QuickChatProvider` keeps the same `QuickChatModal` instance mounted while a terminal descriptor remains available.
The lifecycle hook therefore retains its request owner when the user switches to a conversation and dismisses the dialog.

The owner invalidates previous callback generations before a new attempt starts.
It drains already-issued lifecycle writes before the replacement bind.
Ignoring an old PATCH response alone is insufficient because that PATCH can still overwrite the server row.
Current-generation writes remain ordered and only their current results update the UI.
Older callbacks cannot enqueue writes after retirement.

Recovery uses a fresh mount generation for the inner PTY view.
Changing only the `sessionId` prop cannot reset `initialSessionIdRef`.
The mount key changes on an explicit recovery attempt, not on every returned session update.
This prevents ordinary running-state updates from starting another terminal mount.

Existing workspace sync revisions and the local connecting guard remain authoritative.
An earlier descriptor read cannot cancel a pending recovery or restore its previous shell.
A current live descriptor can attach through the same stable tab key.
StrictMode can replay an effect, but the backend key still permits only one active PTY.

Closing the tab begins a per-tab close fence at the currently published generation.
The fence cancels pending preparation but does not retire that generation while DELETE is pending.
Callbacks are ignored during the fence, and the selected view neither creates a descriptor nor mounts a PTY.
If DELETE fails, the fence clears without changing the generation, so the retained tab can receive later shell events or an explicit retry.
If DELETE succeeds, the tab is removed and its generation is retired while the fence remains set for any stale render.
Closing the tab invalidates pending preparation before any host-shell request.
If a start already escaped cancellation, the existing pending-start owner stops its late result.
Closing removes the descriptor through the existing delete path.
Recovery never recreates a descriptor deleted during preparation.
Switching tabs or dismissing the dialog merely detaches presentation.
A late successful start still records its session for reattachment or explicit close.

## Responsive presentation

The lifecycle region shows status first and one labelled recovery button.
During preparation and startup, the status announces progress through `role="status"`.
The button keeps its accessible name and remains disabled until the attempt settles.
Errors use the existing alert region and permit an explicit retry.
All new copy uses `t()` and the complete locale catalogs.

Desktop uses the current floating Quick Chat dialog and a 28px ordinary button.
Phone uses the current full-height dialog reached through `AppNavSheet`.
The recovery button occupies its own full-width row inside the selected terminal panel.
Phone and coarse-pointer targets measure at least 44px in both dimensions.
The width-based phone rule also applies with a fine pointer.

`QuickChatModal` and `QuickTerminalTabView` are the nearest shipped surface exemplars.
`MobileTerminalKeybar` supplies the existing keyboard clearance pattern.
This short action stays inline because it needs no picker or intermediate sheet.
The fixed tab strip and lifecycle region surround the terminal's existing content region.
The terminal remains the sole output scroll owner.
The dialog retains safe-area padding and the terminal panel retains its visual-viewport offset.
The phone action remains reachable above an open keyboard without document horizontal overflow.

## Verification

Component coverage exercises restored unavailable and naturally exited tabs, retained old session IDs,
manual recovery, failure and retry, rapid activation, and StrictMode.
Deferred-promise tests cover old lifecycle writes across conversation dismissal, stale descriptor reads, failed close followed by shell exit, and close during recovery preparation or startup.
The stale-write case must inspect request order as well as the final UI state.

Desktop and phone Playwright coverage uses real worker-owned shells and `backend.restart()`.
It verifies a new session in the same descriptor after both restart and `exit`.
It proves executable output with a marker assembled by the shell, rather than echoed command text.
A mixed-state case retains one live sibling while another tab recovers.
Rendered checks measure desktop density, touch targets, viewport containment, and keyboard clearance.

Delivery and exact commands belong to the
[shell recovery package](../../../plans/quick-terminal-shell-recovery/plan.md).
