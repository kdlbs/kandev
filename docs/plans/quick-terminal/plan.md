---
spec: docs/specs/quick-terminal/spec.md
created: 2026-08-03
status: implemented
---

# Implementation Plan: Quick Terminal

## Overview

Reuse the existing host-shell start/stream/resize/stop contract and give its shared PTY dialog a
second, larger responsive presentation. A global Quick Terminal provider will own the one ephemeral
dialog instance, while the desktop sidebar and tablet/phone headers supply accessible launchers.
Focused component tests establish visibility and ordering before Playwright proves the desktop,
tablet, and phone geometry and lifecycle through the real UI.

## Backend

No backend changes. `startHostShell`, `agentLoginStreamUrl`, `resizeAgentLogin`, and
`stopAgentLogin` already provide the required ephemeral host-shell lifecycle through
`/api/v1/host-shell/start` and `/api/v1/agent-login/sessions/:id/*`.

## Frontend

### Responsive PTY presentation

- Extend `apps/web/components/settings/pty-terminal-dialog.tsx` with an explicit presentation prop
  whose default preserves the current Agents/login dialog geometry. The Quick Terminal presentation
  uses a flexible terminal body inside an `85dvh` floating dialog up to roughly `1100px` wide on
  tablet/desktop, and overrides the Radix positioning to a `100dvh` full-height surface below the
  phone breakpoint. Keep the header/footer fixed, the xterm host as the single flexible overflow
  region, and clear top/bottom safe-area insets on phone.
- Extend `apps/web/components/settings/host-shell-dialog.tsx` to forward that presentation without
  changing existing callers. Preserve the current `startHostShell` callback and cleanup semantics.

### Global launcher ownership

- Add `apps/web/components/quick-terminal/quick-terminal-provider.tsx` and
  `apps/web/hooks/use-quick-terminal-launcher.ts`. The provider owns the single open/closed state,
  lazy-loads `HostShellDialog` only when requested so xterm does not enter the initial app bundle,
  and renders it with the Quick Terminal presentation. The hook exposes a stable open callback and
  fails loudly when used outside the provider.
- Mount `QuickTerminalProvider` in the shared application chrome (`apps/web/src/app-shell.tsx` and
  the legacy/server layout in `apps/web/app/layout.tsx`) so every supported launcher opens the same
  dialog instance.

### Desktop sidebar action

- Update `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx` to add an
  `IconTerminal2` row action before the Quick Chat action, increase the New Task row's trailing inset
  for two buttons, and call `useQuickTerminalLauncher`. Show both trailing actions only when the
  sidebar is expanded and a workspace is active; retain the existing collapsed/no-workspace behavior.
- Add `quickTerminal` to `apps/web/src/locales/en/sidebar.json` and resolve the tooltip and accessible
  label at render time with `t()`.

### Tablet and phone actions

- Update `apps/web/components/kanban/kanban-header.tsx` so `TabletHeader` renders a 44px
  `Quick terminal` icon immediately before its Quick Chat button when a workspace is active.
- Update `apps/web/components/kanban/kanban-header-mobile.tsx` with the same ordered, 44px action.
  Both use the shared launcher hook and translated accessible label; neither duplicates terminal
  state or session behavior.

### Mobile design contract

- **Desktop outcome:** the expanded New Task row opens one large floating host terminal; the action
  remains hidden in collapsed rail mode.
- **Mobile entry point:** the existing Home/Tasks `KanbanHeaderMobile` action strip places Quick
  Terminal immediately before Quick Chat. Tablet uses the parallel `TabletHeader` action strip.
- **Nearest shipped exemplar:** `apps/web/components/quick-chat/quick-chat-modal.tsx` supplies the
  phone full-height versus wider floating-dialog geometry; `kanban-header-mobile.tsx` supplies the
  ordered 44px icon-action pattern.
- **Hierarchy and primary action:** the terminal is the sole content focus; its Done/close action is
  fixed outside the xterm scroll region and returns the user to the launcher.
- **Presentation rationale:** a terminal is dense, keyboard-driven content rather than a temporary
  choice, so phone uses a dedicated full-height surface. Tablet/desktop retain a floating dialog
  because the shell is a short-lived utility over the current workflow.
- **Geometry:** `100dvh` on phone, `85dvh` maximum on wider viewports, one xterm overflow owner,
  safe-area-aware padding, no document horizontal overflow, and launcher hitboxes of at least 44px.
- **Shared logic:** the provider, host-shell API, PTY state, cleanup, and errors are shared; only the
  launcher placement and responsive dialog composition differ.
- **Mobile proof:** a Pixel 5 Playwright scenario taps the action, verifies full-height containment,
  usable terminal geometry, zero document horizontal overflow, dismissal, and focus return.

## Tests

- **What:** the provider exposes one launcher, opens the lazy host-shell dialog in Quick Terminal
  presentation, and closes it without retaining local state.
  **File:** `apps/web/components/quick-terminal/quick-terminal-provider.test.tsx`.
  **How:** render a consumer under the provider, mock the host-shell dialog module, activate the
  launcher, and assert its open state and presentation props.
- **What:** the expanded New Task row shows Quick Terminal immediately before Quick Chat, invokes
  the launcher, and hides both actions when collapsed or workspace-less.
  **File:** `apps/web/components/app-sidebar/app-sidebar-new-task-item.test.tsx`.
  **How:** mock the shared launcher hook and assert click behavior, DOM order, and gating.
- **What:** the phone header shows Quick Terminal immediately before Quick Chat and invokes the
  shared launcher only with an active workspace.
  **File:** `apps/web/components/kanban/kanban-header-mobile.test.tsx`.
  **How:** mock the launcher hook and assert accessible button behavior, absence, and DOM order.

No unit test is planned for the CSS-only dialog geometry; real rendered measurements belong in the
Playwright scenarios below. The existing host-shell PTY E2E remains the integration proof for
start, command I/O, idempotent start, and stop.

## E2E Tests

- **Scenario:** an expanded desktop sidebar opens a floating terminal larger than the Agents-page
  baseline and closing it returns focus to the sidebar action.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`.
  **What to verify:** action order, dialog and terminal bounding boxes, viewport containment,
  successful PTY mount, dismissal, and focus return.
- **Scenario:** the 700px tablet header exposes Quick Terminal immediately before Quick Chat and
  opens the same contained floating presentation.
  **File:** `apps/web/e2e/tests/terminal/quick-terminal.spec.ts`.
  **What to verify:** tablet action order, 44px hitbox, floating dialog containment, and dismissal.
- **Scenario:** the Pixel 5 Home header opens a full-height terminal without horizontal document
  overflow and returns focus after dismissal.
  **File:** `apps/web/e2e/tests/terminal/mobile-quick-terminal.spec.ts`.
  **What to verify:** touch entry, action order/hitbox, dynamic-viewport containment, terminal body
  height, document `scrollWidth`, safe dismissal, and focus return.
- **Scenario:** the reused host-shell lifecycle still starts, carries command input/output, uses one
  idempotent active session, and stops cleanly.
  **File:** `apps/web/e2e/tests/settings/host-shell-pty.spec.ts`.
  **What to verify:** retain and rerun the existing integration scenarios alongside the new UI path.

## Repair: StrictMode PTY startup race

### Confirmed root cause

The SPA renders under React StrictMode, which replays the terminal effect during development. Two
idempotent `/host-shell/start` requests can therefore return the same session ID. The stale effect's
cancelled branch then calls `stopAgentLogin` for that shared ID, killing the session owned by the
current effect. The backend removes the session, while the surviving `ResizeObserver` continues
posting `/resize`, producing the observed 404s.

### Repair scope

- Track a mount generation so stale effect cleanup cannot stop a newer mount's session.
- Clear the session identity when the PTY stream reports exit/close so resize requests stop.
- Keep sidebar row-action tooltips pointer-driven so restored accessibility focus does not leave a
  tooltip open after the dialog closes.
- Add a regression scenario that opens Quick Terminal, waits through startup, executes a command, and
  proves the surface remains interactive without stale resize failures.

## Verification Results

- Focused component tests: 3 files, 25 tests passed, including restored-focus tooltip coverage.
- Web typecheck, targeted ESLint, i18n ratchet, and i18n checks passed.
- Managed Quick Terminal Chromium E2E: 2 tests passed, including shell prompt and command input/output.
- Managed host-shell lifecycle E2E: 2 tests passed.
- Managed Pixel 5 E2E: 1 test passed.
- Backend `internal/agent/loginpty` tests: 5 passed.
- The managed runner rebuilt backend, Vite assets, and the fixture plugin for each run and cleaned up
  each worker-scoped backend/session.

## Implementation Tasks

Wave 1:

- [x] [Task 01: Build the shared Quick Terminal UI](task-01-quick-terminal-ui.md) (`done`)

Wave 2:

- [x] [Task 02: Prove Quick Terminal across viewports](task-02-quick-terminal-e2e.md) (`done`)

Wave 3:

- [x] [Task 03: Fix Quick Terminal startup race](task-03-fix-quick-terminal-startup-race.md) (`done`)

Execution is sequential in the primary conversation. No subagent delegation is planned or
authorized.

## Risks

- `PtyTerminalDialog` also serves agent-login and auth-recovery flows. The new presentation must be
  opt-in so their dimensions and session behavior remain unchanged.
- xterm measures its container at mount and through `ResizeObserver`; the flexible large layout must
  have a non-zero settled height before `fit()` and remain stable when mobile browser chrome or the
  on-screen keyboard changes the dynamic viewport.
- The sidebar root is always mounted on desktop. Lazy-loading the provider's terminal body is needed
  to avoid adding xterm and its CSS to the initial application bundle.
- Host-shell start is intentionally idempotent. A stale or leaked session would make later launches
  reconnect unexpectedly, so close/cancel cleanup and the existing lifecycle E2E are required.
