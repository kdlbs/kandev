---
status: current
system: ui
requirements:
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-001
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002
---

# Task Agent Tab Reconciliation System Design

## Purpose and boundaries

This design makes desktop Agent-tab reconciliation depend on both task session
state and Dockview readiness. The task system remains authoritative for session
membership and the normal active-session fallback. The desktop workbench owns
the user's current Agent-tab selection and projects it into task state.

## Requirement mapping

| Requirement                                | Design section                                                                                                                                  |
| ------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-TASK-AGENT-TAB-RECONCILIATION-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [Responsive behavior](#responsive-behavior) |
| `REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002` | [Inline rename event boundary](#inline-rename-event-boundary), [Responsive behavior](#responsive-behavior), [Verification design](#verification-design) |

## Components and responsibilities

- `task-detail-route.tsx` loads the selected task and its session list for the
  first client render.
- `StateHydrator` merges that payload into the application store before the
  task workbench settles.
- `DockviewDesktopLayout` publishes the live `DockviewApi` through the Dockview
  store when `onReady` fires.
- `useAutoSessionTab` observes the effective session, current task session IDs,
  and Dockview API readiness. It invokes the existing reconciliation body when
  any of these inputs changes.
- `runAutoSessionTabEffect` reads the current application state, removes stale
  Agent panels, ensures the active session panel, and adds current sibling
  session panels without activating them.
- `setupReadyDockview` restores the environment layout before it registers the
  normal session-tab event listener. It calls `setupSessionTabSync`, which
  adopts one valid restored Agent selection initially and after a delayed
  environment layout restoration completes.
- `SessionTab` owns the fine-pointer Agent-tab context menu and attaches the
  shared Dockview maximize handler to the tab surface.
- `TabRenameInput` owns the inline editor's pointer and keyboard event boundary.
  It is shared with task terminal-tab rename mode.
- `useTabMaximizeOnDoubleClick` owns maximize-or-restore behavior outside an
  active inline editor.

## Inline rename event boundary

`SessionTab` renders `TabRenameInput` inside the same `ContextMenuTrigger` that
receives tab-level double-clicks. Browsers dispatch `dblclick` as a separate
bubbling event after the individual click events. Stopping only `mousedown` and
`click` therefore does not isolate the editor from the tab-level maximize
handler.

The shared rename editor stops propagation of `mousedown`, `click`, and
`dblclick`. Its `dblclick` boundary does not prevent the default browser action,
so native input text selection remains available. The parent tab maximize
handler remains unchanged and continues to own double-clicks that originate
outside the editor. This boundary also protects the shared task terminal rename
editor without changing terminal naming behavior.

## Data and contracts

No new API, persisted value, or store field is required. The existing
`taskSessionsByTask.itemsByTaskId` collection is the session-membership source.
The existing Dockview store `api` value is the readiness source.

The environment-scoped Dockview layout already stores the selected panel for
each group in browser `sessionStorage`. A restored `session:<id>` value is a
selection hint. It is not session-membership authority. The UI accepts the hint
only when the session belongs to the active task and current environment.

## Control flow

1. Route loading and application-store hydration can finish before or after
   Dockview calls `onReady`.
2. `useAutoSessionTab` subscribes to both sources instead of reading Dockview
   readiness only inside an effect triggered by session changes.
3. The effect does nothing while no Dockview API exists.
4. When the API becomes available, the readiness subscription triggers the
   effect with the latest active task, effective session, and session list.
5. Existing reconciliation creates the active session panel first, preserves
   valid non-Agent focus, and adds all sibling session panels as inactive tabs.
6. Later membership or active-session changes use the same path.

During a full reload, the boot payload supplies the normal primary-session
fallback. The workbench then restores the environment-scoped Dockview layout.
Before normal tab-event synchronization starts, the UI resolves the selected
Agent panel from the restored group state. It validates that panel against the
active task, current session list, and current environment.

If session-to-environment metadata arrives after Dockview is ready, the
environment switch restores that saved layout later. Session-tab
synchronization repeats the same adoption when the restore flag clears. Stale
and unrelated selected panels are filtered before the UI decides whether the
remaining valid selection is ambiguous.

When one valid restored Agent selection exists, the UI applies it with
`setActiveSessionAuto`. This action aligns the application store with the
restored layout without creating a user pin. The existing explicit-intent guard
continues to reject unrelated Dockview activation events.

The readiness change is an event source, not a second copy of session state.
The effect always reads current application state when it runs.

## Failure and recovery

A null Dockview API is a temporary no-op. API readiness causes deterministic
reconciliation, so recovery does not depend on a timer, route retry, or page
reload. A session that disappears before readiness is not materialized because
the effect reads the latest session list.

The UI ignores a restored Agent selection when it is stale, cross-task,
cross-environment, or ambiguous. The boot-selected active session remains the
fallback. This rule prevents saved layout data from overriding current task
membership.

## Responsive behavior

Desktop uses Dockview Agent tabs. Phone and tablet layouts do not mount the
desktop Dockview workbench. Their existing session controls continue to read
the same application-store membership. No mobile composition or touch target
changes. Inline rename event isolation applies only where the desktop Dockview
tab and editor are mounted.

## Verification design

A React hook regression test hydrates a multi-session task while the Dockview
API is null. The test publishes the API later and verifies reconciliation.
Existing pure reconciliation tests continue to cover active-tab and sibling
behavior. A Playwright scenario opens a multi-session task from Cmd+K and
verifies that every Agent tab appears without a reload. A focused desktop
Playwright regression enters Agent-tab rename mode, double-clicks the input,
and verifies that native text selection remains active while the Dockview group
stays unmaximized. Mobile coverage is unchanged because phone and tablet do not
mount the affected Dockview tab surface.

A restoration unit test covers a valid secondary Agent selection, invalid
saved selections, mixed stale and valid selections, and delayed environment
layout restoration. A desktop Playwright regression selects the secondary
Agent tab, reloads the task, and verifies that the same session remains active.
