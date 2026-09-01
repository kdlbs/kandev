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
membership and the active session. This UI design only projects that state into
the desktop workbench.

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

The readiness change is an event source, not a second copy of session state.
The effect always reads current application state when it runs.

## Failure and recovery

A null Dockview API is a temporary no-op. API readiness causes deterministic
reconciliation, so recovery does not depend on a timer, route retry, or page
reload. A session that disappears before readiness is not materialized because
the effect reads the latest session list.

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
