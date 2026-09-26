---
spec: docs/specs/ui/requirements/quick-terminal.md
created: 2026-08-05
status: complete
---

# Implementation Plan: Quick Terminal Refresh Persistence

## Overview

Quick Terminal currently loses its tab descriptors on a full page reload because they exist only
in the in-memory Zustand UI slice. This repair adds a backend-owned descriptor lifecycle scoped to
the authenticated user and workspace, returns those descriptors through boot and resync, and
reattaches a live host-shell session through the existing PTY status/stream routes. The PTY remains
ephemeral across backend restarts; a stale descriptor is retained as exited/unavailable and never
starts a replacement implicitly.

The work is sequential: the descriptor storage and backend contract must exist before frontend
hydration can consume it, and the browser/E2E proof depends on both sides of that contract.

## Confirmed root cause

`quick-terminal-actions.ts` creates `QuickTerminalTab` descriptors only in
`quickChat.terminalTabs`. `StateProvider` creates a new store on reload, while
`addQuickChatState` restores only task-backed conversation sessions. The host-shell manager may
still hold the detached PTY, but no `tabId`/`sessionId` descriptor is booted back into the frontend,
so the shared dialog has no terminal tab to render or attach.

## Backend

### Descriptor storage and service

- Add a `quickterminal` backend package with model, SQLite repository, service, and focused tests.
- Create a durable `quick_terminal_tabs` table with `tab_id`, `user_id`, `workspace_id`, nullable
  `session_id`, positive workspace-local `sequence`, lifecycle status, nullable exit code/error,
  and created/updated timestamps. Enforce user/workspace/tab and user/workspace/sequence
  uniqueness, index user/workspace listing, and allocate sequences atomically without reuse.
- Resolve the authenticated user from `authn.IdentityFromContext`; preserve the synthetic default
  identity when authentication is disabled. Every read and mutation must authorize the workspace
  through `taskService.AuthorizeWorkspaceAccess` and scope the repository query by user ID.
- Reconcile rows against the `loginpty.Manager`: a live host-shell manager entry keyed by the
  descriptor's UUID supplies the current session and running state; a missing entry clears the
  session association and yields exited/unavailable without creating a PTY.
- Bound and validate all IDs and lifecycle fields at the service boundary. A client may update only
  its own descriptor and may associate only the manager session whose bounded host-shell key
  matches that descriptor.

### API and boot contract

- Register `GET`, `POST`, `PATCH`, and `DELETE /api/v1/quick-terminal-tabs` routes in a dedicated
  handler/controller layer. `POST` is idempotent for a user/tab UUID; `DELETE` stops the associated
  live host shell best-effort before removing the row and treats a missing row as already closed.
- Keep `/api/v1/host-shell/start` and the existing agent-login session status, stream, resize, and
  stop routes compatible. Preserve the legacy singleton host-shell behavior when `client_id` is
  omitted and keep `_host_shell` as the public agent identity.
- Expose the minimal manager lookup/key helper needed by the descriptor service without changing
  `Manager.Start` callers or exit-callback semantics.
- Wire the repository/service into `backendapp/storage.go`, `backendapp/types.go`, route
  registration, and `boot_state.go`. `quickChat.terminalTabs` must be present for the resolved
  workspace and use the same reconciliation service as HTTP resync.

## Frontend

### API and wire mapping

- Add `apps/web/lib/api/domains/quick-terminal-api.ts` and the matching HTTP descriptor types.
  Export list/create/update/delete calls through the API barrel; do not send descriptors through
  Quick Chat task rename/delete/sync APIs.
- Map the backend snake-case descriptor payload into the existing `QuickTerminalTab` shape and
  include terminal descriptors in the boot `HydrationState` and workspace resync response.

### State and hydration

- Extend the UI slice with server-descriptor adoption/reconciliation actions that preserve local
  active-kind and last-terminal selection, keep setup/chat placeholders local, and never discard a
  sibling terminal during conversation resync.
- Make new terminal creation persist the browser-generated UUID before starting a shell, and make
  lifecycle/session updates durable without replacing a server descriptor with an empty local
  array. Failed descriptor writes remain visible as an understandable terminal error and do not
  affect siblings.
- On boot, reload, workspace switch, and socket reconnect, reconcile by `tabId` and workspace.
  Preserve backend ordering/sequence, restore a live `sessionId`, and mark stale sessions
  exited/unavailable without implicit replacement.

### Terminal lifecycle and shared dialog

- Update `use-quick-chat-modal.ts` and the terminal tab view to persist start/reattach/exit/error
  state and to delete the server descriptor on explicit close. A stop/delete 404 is already
  stopped; other failures retain the descriptor and surface the localized error.
- Ensure a restored `exited`/`error`/unavailable descriptor does not pass through the PTY view's
  start-on-mount path. Only a newly created `connecting` descriptor may start a shell without a
  session; a restored live descriptor attaches by `sessionId`.
- Preserve the existing detach-on-unmount and late-start ownership guarantees: a detached start
  that resolves still records its session and a later explicit close stops it; switching tabs or
  dismissing the dialog never stops it.
- Keep the existing one-dialog desktop/tablet/phone geometry, lazy xterm loading, focus return,
  fixed terminal labels, and touch-safe controls unchanged.

## Tests

- **What:** descriptor schema, atomic sequence allocation, user/workspace isolation, idempotent
  create, live-session reconciliation, stale-session clearing, and close/delete semantics.
  **Files:** `apps/backend/internal/quickterminal/repository/sqlite_test.go`,
  `apps/backend/internal/quickterminal/service/service_test.go`, and
  `apps/backend/internal/quickterminal/handlers/handlers_test.go`.
  **How:** real SQLite repository tests plus table-driven service and `httptest` handler tests;
  include two users, two workspaces, duplicate tab IDs, missing PTYs, and stop/delete 404s.
- **What:** boot payload and workspace resync return persisted terminal descriptors without
  erasing conversations or terminals from another workspace.
  **Files:** `apps/backend/internal/backendapp/helpers_test.go`,
  `apps/web/lib/state/hydration/hydrator.test.ts`, and
  `apps/web/lib/state/slices/ui/quick-chat-sync.test.ts`.
  **How:** boot fixtures with a live and stale descriptor, then pure state reconciliation tests
  with empty server conversation lists and multiple workspaces.
- **What:** browser API mapping and store actions persist create/update/remove while retaining
  active terminal selection and preventing an exited descriptor from starting a replacement.
  **Files:** `apps/web/lib/api/domains/quick-terminal-api.test.ts`,
  `apps/web/lib/state/slices/ui/quick-chat-actions.test.ts`,
  `apps/web/components/quick-chat/use-quick-chat-modal.test.ts`,
  `apps/web/components/quick-chat/quick-terminal-tab-view.test.tsx`, and
  `apps/web/components/settings/pty-terminal-view.test.tsx`.
  **How:** mocked API, xterm, WebSocket, ResizeObserver, and StrictMode tests; cover reload
  reattachment, stale-session no-start, detached late start followed by close, natural exit, 404
  close, and sibling isolation.

## E2E Tests

- **Scenario:** GIVEN a running terminal with marker output, WHEN the user reloads the page and
  opens Quick Chat, THEN exactly one terminal tab with the same sequence reconnects to the same
  PTY and replays the marker without a second shell.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`.
  **What to verify:** shared dialog identity, tab count/label, marker output, no standalone
  terminal dialog/footer, and cleanup that explicitly closes every surviving terminal in `finally`.
- **Scenario:** GIVEN a persisted descriptor after the backend has restarted, WHEN the user opens
  Quick Chat, THEN the tab is visible as exited/unavailable and **New Terminal** is required for a
  new shell.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts` (or a focused backend fixture
  helper if a process restart cannot preserve the test database).
  **What to verify:** accessible lifecycle status, no implicit new PTY, explicit replacement, and
  workspace filtering.
- **Scenario:** GIVEN a phone viewport, WHEN the user reloads and reopens the shared surface, THEN
  the persisted terminal remains contained in the dynamic viewport and the terminal strip/menu
  retains its touch-safe controls and zero document horizontal overflow.
  **File:** `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`.
  **What to verify:** same-session marker, terminal panel height, 44px controls/menu rows, focus/
  dismissal, and teardown of all surviving tabs.

## Verification Results

- Backend: `go test ./internal/backendapp ./internal/quickterminal/... ./internal/agent/loginpty/... -count=1` passed (279 test cases/subcases); `make -C apps/backend lint` passed with 0 issues.
- Frontend: focused Vitest passed (9 files, 124 tests); web typecheck, changed-file ESLint, i18n ratchet, and i18n checks passed. The i18n check retained its existing advisory 657 zh-cn catalog issues and 5 orphan English keys.
- E2E: managed desktop Chromium `quick-terminal.spec.ts` passed 2/2; managed Pixel 5 `mobile-quick-terminal.spec.ts` passed 1/1. Both runs included reload/reattachment and explicit terminal cleanup.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Persist Quick Terminal descriptors](task-01-persist-descriptors.md) (`done`)

Wave 2:

- [x] [Task 02: Expose descriptor API and boot state](task-02-descriptor-api-boot.md) (`done`)

Wave 3:

- [x] [Task 03: Reconcile descriptors in the web app](task-03-web-descriptor-reconciliation.md) (`done`)

Wave 4:

- [x] [Task 04: Prove refresh reattachment](task-04-refresh-reattachment-e2e.md) (`done`)

All tasks are sequential. The backend schema, host-shell manager lookup, boot payload, frontend
hydration, and E2E fixtures share the same lifecycle contract, so no task is parallel-safe.

## Risks

- A durable row can outlive its in-memory PTY. Reconciliation must clear stale session IDs and must
  not turn a refresh into an implicit new shell.
- Create, start, detach, late completion, exit, and explicit close can race. Stable UUIDs and
  server-side idempotency must prevent duplicate sessions and orphaned descriptors.
- Multiple authenticated clients can mutate sibling tabs concurrently. Repository updates must be
  row-scoped and sequence allocation must not overwrite unrelated descriptors.
- Host-shell permissions remain those of the backend user and are not task-worktree isolated.
- The existing rolling output buffer remains bounded and non-durable; a refresh after output has
  aged out may restore the tab without restoring all prior text.
