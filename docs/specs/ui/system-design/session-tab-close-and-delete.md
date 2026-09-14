---
status: current
system: ui
requirements:
  - REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001
---

# Session Tab Close and Delete System Design

## Purpose and boundaries

The desktop Agent tab separates panel visibility from session lifecycle. Closing a
tab, hiding it from the context menu, or using **Close Others** only removes
Dockview panels; the conversation and its backend session remain intact and
recoverable from **+ > Agents**. Permanent deletion stays an explicit, confirmed
Delete action on the desktop context menu and the phone Sessions picker.

The Task system remains authoritative for session membership, deletion, and
active-session promotion. The desktop workbench owns only panel visibility and
the persisted record of which panels the user closed.

## Requirement mapping

| Requirement                                     | Design section                                                                                                                                             |
| ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-SESSION-TAB-DELETE-FEEDBACK-001`       | [Components and responsibilities](#components-and-responsibilities), [Persisted hide record](#persisted-hide-record), [Control flow](#control-flow), [Verification design](#verification-design) |

## Components and responsibilities

- `apps/web/components/task/session-tab.tsx` renders the tab close action and
  wires **Hide**, **Close Others**, and scoped delete actions. The X is shown
  only while more than one agent-session panel is visible; the sole visible
  panel keeps no X regardless of backend session count or lifecycle state.
- `apps/web/components/task/dockview-session-tabs.ts` owns the in-memory hidden
  set per Dockview API and environment, gates the automatic session-tab effect,
  sibling-panel materialization, and the chat safety net on that set.
- `apps/web/lib/env-hidden-sessions.ts` persists the per-environment hidden set
  in session storage beside the env layout, with the same per-environment,
  per-browser-tab lifetime.
- `apps/web/lib/state/dockview-env-switch.ts` and
  `apps/web/lib/state/dockview-store.ts` filter every restore path (fast env
  switch, saved-layout `fromJSON`, sibling materialization, maximize restore,
  and custom-layout reuse) through the persisted hidden set.
- `apps/web/components/task/session-reopen-menu.tsx` clears the hidden record
  when a closed conversation is reopened from **+ > Agents**, restoring the
  same session.
- `apps/web/hooks/domains/session/use-session-actions.ts` keeps deletion,
  confirmation, workspace retention, and primary-session promotion unchanged.

## Persisted hide record

The saved Dockview layout cannot carry hidden-session intent: reusable layouts
normalize session panels into chat placeholders, and restore paths add every
current session back as sibling panels. The hide record therefore lives in its
own session-storage slot keyed by the task environment ID.

Hydration keeps the record authoritative only after the store's session list
is real. Pruning runs once the task sessions are loaded, so a reload into an
initially empty store cannot erase the record and resurrect closed panels
before the sessions arrive. Explicit reopen and explicit delete both clear the
record; a session that disappears from the whole store is pruned from it during
authoritative reconciliation.

## Control flow

1. The user closes a tab, hides it, or uses **Close Others**. The session ID is
   added to the env's hidden set and the panel is removed; no session request is
   sent and no lifecycle state changes.
2. Active-session synchronization, layout reconciliation, the chat safety net,
   and reload restore paths consult the hidden set before creating any
   `session:*` panel, so closed panels stay absent.
3. Selecting the hidden conversation from **+ > Agents** clears the record and
   adds the panel back with the existing conversation.
4. Deleting from the context menu or picker keeps the local confirmation
   popover/picker step and the existing deletion, feedback, and successor
   behavior; the hidden record entry is cleared with the session.

## Verification design

- Unit regressions cover: hidden siblings stay absent through the auto-session
  effect; a fresh Dockview API rehydrates from the persisted record after
  reload; pruning tolerates an initially empty session store until hydration;
  reopen clears the record; restore paths skip hidden IDs.
- Component tests cover the last-visible-panel X guard, **Close Others**
  scoping, and the unchanged delete confirmation.
- E2E regressions cover: X close keeps the session, **Hide** reopen restores
  the same conversation, **Close Others** stays closed across active-session
  synchronization, and a hidden tab stays absent across a page reload until
  reopened from the + menu.
