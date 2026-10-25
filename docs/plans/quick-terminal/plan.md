---
spec: docs/specs/ui/requirements/quick-terminal.md
created: 2026-08-03
updated: 2026-08-04
status: completed
---

# Implementation Plan: Quick Chat and Terminal Tabs

## Overview

The shipped Quick Terminal baseline already exposes the host PTY from desktop, tablet, and phone,
but it owns a separate provider/dialog and the backend permits only one host shell. This revision
keeps those launchers while moving terminal content into the existing workspace-scoped Quick Chat
tab surface. The backend first gains per-tab idempotent host-shell sessions, then the frontend
separates reusable PTY rendering from stop-on-dialog-close ownership, adds browser-local terminal
tab state, and replaces the Quick Chat plus action with a grouped creation menu.

Tasks 01–03 remain the completed record of the shipped baseline. Tasks 04–07 complete this
revision's backend, lifecycle, shared-tab, and viewport verification work.

## Backend

### Per-tab host-shell idempotency

- Extend the optional JSON body handled by
  `apps/backend/internal/agent/loginpty/handlers.go:httpStartHostShell` with
  `client_id: string`.
- Preserve the legacy singleton key when `client_id` is omitted so
  **Settings → Agents** and older clients remain idempotent exactly as they are today.
- When `client_id` is present, derive a bounded internal session key from
  `_host_shell` plus that client ID. Repeated starts for one client ID return the same running PTY;
  distinct client IDs create independent PTYs.
- Keep the public `agent_id` in the returned `Status` equal to `_host_shell`. Add an internal
  manager key/owner distinction rather than exposing the client id as an agent identity or passing
  it to the discovery-cache exit callback.
- Require a present client ID to parse as a UUID and reject malformed values with
  `400 Bad Request`; do not use a
  client-provided value as a command, path, environment variable, or log field without bounding it.
- Leave the session-ID status, stream, resize, and stop routes unchanged. Stopping one per-tab
  session must not remove another tab's session or an agent-login session.

### Manager lifecycle

- Extend `apps/backend/internal/agent/loginpty/manager.go` so uniqueness is keyed separately from
  the stable logical `AgentID`. Existing `Manager.Start(agentID, ...)` callers retain one active
  login PTY per agent.
- Store the internal key on `Session` for manager-map cleanup; continue indexing the public
  session route by the generated session ID.
- Preserve the existing idle timeout, hard timeout, 16 KB rolling output buffer, subscriber
  detach/reconnect behavior, and exit callback semantics.

## Frontend

### Host-shell client and reusable PTY view

- Extend `apps/web/lib/api/domains/settings-api.ts:startHostShell` with an optional
  `clientId`/request field serialized as `client_id`. Agent-login start requests remain unchanged.
- Extract the xterm/resize/WebSocket portion of
  `apps/web/components/settings/pty-terminal-dialog.tsx` into a reusable terminal view, likely
  `apps/web/components/settings/pty-terminal-view.tsx`.
- Give the extracted view an explicit lifecycle contract:
  - the default dialog mode starts a session and stops it on unmount, preserving every Agents/login
    caller;
  - the Quick Chat tab mode can start with a stable client ID or attach to a stored session ID,
    reports start/exit/error changes to the tab store, and only closes its WebSocket/xterm on
    unmount;
  - explicit terminal-tab close remains the owner of the stop request.
- Preserve the StrictMode mount-generation guard. A pending start that resolves after its tab was
  explicitly removed must be stopped; a view that merely detached because of a tab switch or dialog
  dismissal must not stop it.
- Reattachment uses the existing status/stream routes and buffered output. A missing session is
  reported as exited/unavailable and does not trigger an implicit replacement.
- Keep `PtyTerminalDialog` and `HostShellDialog` as wrappers for the Agents-page and login/auth
  surfaces; remove only their use as the standalone Quick Terminal presentation.

### Shared workspace tab state

- Extend `apps/web/lib/state/slices/ui/types.ts` with a discriminated
  `QuickTerminalTab` descriptor matching the spec: `tabId`, `workspaceId`, `sessionId`,
  `sequence`, `status`, optional `exitCode`, and optional `error`.
- Keep backend-restored conversation sessions in `quickChat.sessions`. Add browser-local terminal
  tabs and explicit active-kind/last-terminal state instead of widening
  `QuickChatSessionKind = "chat" | "config"`; terminal tabs have no task/session-message contract.
- Add focused UI-slice actions for:
  - reuse-or-create terminal launch for a workspace;
  - always-new terminal creation from the add menu;
  - terminal session/status updates;
  - terminal activation;
  - explicit terminal removal and adjacent same-workspace fallback, closing the modal only when no
    conversation, setup, or terminal tab remains.
- Conversation activation preserves the existing `chat` versus `config` launcher filtering
  without discarding the last active terminal. Terminal activation does not overwrite conversation
  selection. This lets each launcher restore its own matching content.
- Generate the stable tab/client ID in the browser with `crypto.randomUUID()`. Assign terminal
  sequence labels within a workspace without colliding with another open terminal.
- Update `apps/web/lib/state/slices/ui/quick-chat-sync.ts`,
  `apps/web/lib/state/default-state.ts`, and
  `apps/web/lib/state/hydration/hydrator.ts` only as needed to ensure boot/resync replacement of
  server conversation sessions preserves local terminal descriptors and active-terminal state.
  Terminal descriptors must never be sent to Quick Chat rename/delete/sync APIs.

### One Quick Chat dialog

- Update `apps/web/components/quick-chat/quick-chat-modal.tsx` to derive a shared active tab from
  the conversation and terminal state. Render:
  - `QuickChatSessionView`, `QuickChatSetup`, or `ConfigChatSetup` for conversation tabs;
  - a new `apps/web/components/quick-chat/quick-terminal-tab-view.tsx` for terminal tabs.
- Generalize `QuickChatTabItem` or introduce a sibling tab item so terminal tabs show an accessible
  terminal icon and fixed `Terminal N` label without entering chat rename/delete code.
- Terminal tab close calls the shared stop endpoint. Treat `404 session not found` as already
  stopped; retain the tab and surface an error for other failures. Closing a chat tab continues to
  use its confirmation dialog and backing-task deletion.
- Dismissing the modal removes abandoned chat setup placeholders as today but retains terminal
  descriptors and sessions. Switching tabs only detaches the selected terminal view.
- Keep the current horizontal overflow owner for the tab strip and the current resizable
  tablet/desktop and full-height phone dialog geometry. The terminal fills the content region below
  the strip and does not render a nested `Dialog`, title, description, or Done footer.

### Grouped plus menu

- Replace the direct plus-button `onNewChat` handler with a Radix
  `DropdownMenu` implemented in a focused component such as
  `apps/web/components/quick-chat/quick-tab-add-menu.tsx`.
- Mirror the task-detail add-menu information hierarchy without importing task/Dockview state:
  - **Agents** label and **New Agent**;
  - separator;
  - **Terminals** label and **New Terminal**.
- Keep existing conversation and terminal tabs selectable from the horizontal tab strip; the
  creation menu does not duplicate those rows.
- Keep the plus trigger at least 44×44 px on phone. Reuse the existing global Radix menu mobile
  treatment so the menu becomes a contained, safe-area-aware bottom sheet with 44 px rows on
  coarse/narrow viewports.

### Launcher and provider ownership

- Change `apps/web/hooks/use-quick-terminal-launcher.ts` to accept the active workspace ID and
  dispatch the reuse-or-create action into the shared tab store.
- Update the sidebar, tablet header, and phone header callers to pass their workspace ID. Keep their
  current visibility, order, test IDs, accessible labels, and hitboxes.
- Extend `apps/web/components/quick-chat/quick-chat-provider.tsx` so workspace resolution considers
  the active terminal as well as the active conversation and so one provider renders the only
  shared dialog.
- Move launcher focus capture/restoration into the shared provider/launcher contract. Preserve the
  pointer-only sidebar tooltip behavior after focus returns.
- Remove `QuickTerminalProvider` from `apps/web/src/app-shell.tsx` and
  `apps/web/app/layout.tsx`; delete the provider component/test after its focus and lazy-loading
  responsibilities are covered by the shared provider. Keep terminal code lazy enough that xterm
  does not enter the initial application bundle before a terminal tab is rendered.

### Internationalization

- Add render-time translations for **Agents**, **Terminals**, **New Agent**, **New Terminal**,
  `Terminal {{count}}`, terminal-tab accessible labels, and terminal lifecycle errors in the
  appropriate English namespace.
- Do not translate tab IDs, backend session IDs, or enum discriminants. Use `count` for numbered or
  plural copy and include the changed paths in the i18n ratchet check.

### Mobile design contract

- **Desktop outcome:** the sidebar terminal launcher opens the shared large Quick Chat dialog on the
  last terminal or a first new terminal; the plus menu creates either content kind while the tab
  strip switches between existing content.
- **Mobile entry point:** the existing 44 px Home/Tasks terminal and Quick Chat actions remain. The
  terminal action selects terminal content inside the same full-height dialog as Quick Chat.
- **Nearest shipped exemplars:** `quick-chat-modal.tsx` owns the full-height phone/floating wider
  surface and horizontal tab strip; `dockview-add-panel-items.tsx`,
  `session-reopen-menu.tsx`, and `terminal-reopen-menu.tsx` supply the grouped menu hierarchy;
  the global Radix menu treatment supplies the phone bottom sheet.
- **Hierarchy and primary action:** the fixed strip identifies the active tab; the selected chat or
  terminal is the sole content focus. The plus control is the visible creation overflow and the
  phone close control remains fixed.
- **Presentation rationale:** conversations and terminals are frequently revisited dense content,
  so they share a full-height phone workspace rather than nesting another dialog or temporary
  drawer. Only the short creation choice rises in the menu bottom sheet.
- **Geometry:** `100dvh` on phone, current bounded `85vh` floating geometry on wider viewports,
  one selected-content scroll owner, safe-area clearance, a horizontally scrollable tab strip, and
  no document horizontal overflow.
- **Shared logic:** terminal identity, lifecycle, workspace filtering, active selection, and API
  behavior are shared across viewports; only launcher placement and existing responsive dialog/menu
  presentation differ.
- **Mobile proof:** Pixel 5 coverage taps both launchers, opens the grouped menu, creates and switches
  terminal tabs, verifies 44 px controls/menu rows, terminal containment, internal scrolling,
  dismissal, focus return, and zero document horizontal overflow.

## Tests

- **What:** a host-shell client ID is idempotent for one tab while distinct IDs create independent
  sessions and legacy starts remain singleton.
  **Files:** `apps/backend/internal/agent/loginpty/manager_test.go` and a focused
  `apps/backend/internal/agent/loginpty/handlers_test.go`.
  **How:** table-driven manager and `httptest` handler tests cover same/different/missing/invalid
  keys, independent stop, stable public agent ID, and unchanged agent-login uniqueness.
- **What:** standard PTY dialogs stop on unmount while Quick Chat terminal views detach, reattach,
  and stop only after explicit tab close.
  **Files:** new focused tests beside
  `apps/web/components/settings/pty-terminal-view.tsx` and
  `apps/web/components/quick-chat/quick-terminal-tab-view.tsx`.
  **How:** mock xterm, WebSocket, ResizeObserver, and API calls; exercise StrictMode replay, pending
  start removal, detached reattachment, exit, 404 stop, and sibling isolation.
- **What:** store actions implement reuse-or-create versus always-new behavior, per-workspace last
  selection, deterministic close fallback, and server reconciliation that preserves terminal tabs.
  **Files:** `apps/web/lib/state/slices/ui/quick-chat-actions.test.ts`,
  `apps/web/lib/state/slices/ui/quick-chat-sync.test.ts`, and
  `apps/web/lib/state/hydration/hydrator.test.ts`.
  **How:** pure Zustand/helper tests with two workspaces, several chat kinds, several terminal IDs,
  and empty server resync.
- **What:** the shared modal renders mixed tabs and a grouped creation menu, while both launcher
  kinds select the intended last tab and restore focus; conversation tabs also expose context-menu
  rename.
  **Files:** `apps/web/components/quick-chat/quick-chat-modal.test.ts`, a new menu component test,
  `apps/web/hooks/use-quick-chat-launcher.test.ts`, the replacement terminal-launcher/provider
  tests, and existing sidebar/mobile-header component tests.
  **How:** mocked store/API/component tests assert menu grouping, creation callbacks, context-menu
  rename, accessible labels, workspace gating, focus restoration, and tooltip behavior.

## E2E Tests

- **Scenario:** first terminal launch creates one tab; dismissing and reopening via the sidebar
  selects the same session and replays its marker output.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`.
  **What to verify:** shared Quick Chat dialog identity, one tab after reopen, same terminal marker,
  no standalone Quick Terminal dialog/footer, focus return, and no stale tooltip.
- **Scenario:** the plus menu creates a second independent terminal and switches existing chats and
  terminals without duplication.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`.
  **What to verify:** grouped labels/rows, distinct terminal labels and command output, terminal
  shortcut reuse, Quick Chat shortcut selection, tab close stopping only one session, and fallback.
- **Scenario:** the low-level host-shell API supports same-key idempotency, different-key
  concurrency, independent command streams, independent stop, and omitted-key compatibility.
  **File:** `apps/web/e2e/tests/settings/host-shell-pty.spec.ts`.
  **What to verify:** response/session identities and live WebSocket command markers for two PTYs.
- **Scenario:** phone users can open the shared surface, invoke the add menu as a touch-safe bottom
  sheet, create/switch terminals, dismiss, and return to the launcher.
  **File:** `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`.
  **What to verify:** 44 px controls/menu rows, dynamic-viewport containment, terminal height,
  safe-area/internal scrolling, same-session reopen, and zero document horizontal overflow.
- Re-run the existing Quick Chat desktop/mobile happy paths affected by the shared dialog:
  `apps/web/e2e/tests/chat/quick-chat.spec.ts` and the relevant
  `apps/web/e2e/tests/chat/mobile-quick-chat-entry.spec.ts` scenarios.

## Verification Results

Completed. Tasks 04–06 record the backend, lifecycle, state, UI, typecheck, lint, and i18n
results. Task 07 records the final managed desktop run (20 passed) and Pixel 5 run (5 passed),
including backend/Vite rebuilds and worker teardown evidence.

## Implementation Waves And Parallel Candidates

Historical shipped baseline:

- [x] [Task 01: Build the shared Quick Terminal UI](task-01-quick-terminal-ui.md) (`done`)
- [x] [Task 02: Prove Quick Terminal across viewports](task-02-quick-terminal-e2e.md) (`done`)
- [x] [Task 03: Fix Quick Terminal startup race](task-03-fix-quick-terminal-startup-race.md) (`done`)

Revision wave 4:

- [x] [Task 04: Add per-tab host-shell sessions](task-04-per-tab-host-shell-sessions.md)
  (`done`)

Revision wave 5:

- [x] [Task 05: Extract detachable PTY lifecycle](task-05-detachable-pty-lifecycle.md)
  (`done`)

Revision wave 6:

- [x] [Task 06: Unify Quick Chat and terminal tabs](task-06-unified-quick-tabs.md)
  (`done`)

Revision wave 7:

- [x] [Task 07: Prove shared tabs across viewports](task-07-shared-tabs-e2e.md)
  (`done`)

Tasks 04–07 are sequential because each consumes the contract established by the preceding task and
the frontend tasks share PTY/dialog files. The task files are suitable handoff packets, but this
plan does not itself authorize subagent execution.

## Risks

- The login PTY manager enforces one login process per agent for credential safety. Per-tab host
  shells must add a separate idempotency key without relaxing that invariant or changing the public
  agent identity delivered to exit callbacks.
- A hidden terminal is intentionally alive without a subscriber. Its output is recoverable only
  from the existing 16 KB rolling buffer, and it can expire under the existing 10-minute idle or
  30-minute hard timeout.
- Start, detach, explicit close, StrictMode replay, and late async completion can race. Stable
  client IDs plus explicit ownership tests are required to avoid leaked or accidentally stopped
  sibling sessions.
- Backend Quick Chat hydration/resync is authoritative only for conversation sessions. A naive array
  replacement can erase local terminal tabs or activate a tab from another workspace.
- xterm measures its mounted container. Reattachment and the selected-tab layout must fit only after
  the content region has non-zero dimensions and must tolerate phone browser chrome and the
  on-screen keyboard.
- Quick terminals execute with the backend host user's permissions. The shared dialog must not imply
  task-worktree isolation or route terminal descriptors through conversation persistence APIs.
