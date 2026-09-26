---
status: active
system: ui
created: 2026-08-03
updated: 2026-08-11
owners:
  - kandev
---
# Agent Todo List Panel Requirements

## Overview

The coding agent's own mid-session todo list (Claude Code's `TodoWrite`-style
tool calls, and native ACP `session/update` Plan notifications) is already
tracked end-to-end by the backend and streamed live to the frontend, but today
it only ever renders inline: a small status-bar chip above the chat composer
(`TodoIndicator`) and a collapsible card in the chat transcript
(`TodoMessage`). Users who want to keep the current todo checklist visible
while scrolled away from the composer, or alongside Files/Changes, have no way
to do so. Users need a Settings option to enable or disable a persistent
**Todos** tab in the desktop right panel that shows the same checklist.

## Requirements

### REQ-UI-AGENT-TODO-LIST-PANEL-001: Agent Todo List Panel

**Intent:** The coding agent's own mid-session todo list (Claude Code's `TodoWrite`-style tool
calls, and native ACP `session/update` Plan notifications) is already tracked end-to-end by the
backend and streamed live to the frontend, but today it only ever renders inline: a small status-bar
chip above the chat composer (`TodoIndicator`) and a collapsible card in the chat transcript
(`TodoMessage`). Users who want to keep the current todo checklist visible while scrolled away from
the composer, or alongside Files/Changes, have no way to do so. Users need a Settings option to
enable or disable a persistent **Todos** tab in the desktop right panel that shows the same
checklist.

#### Acceptance criteria

- **AC-UI-AGENT-TODO-LIST-PANEL-001.1:** `Settings > General > Task Actions` exposes a boolean preference, "Show agent todo list panel" (default **off**, preserving today's behavior for every existing user). Its description states explicitly that it automatically pins the agent's live todo checklist as a Todos tab in the right panel, and that the Todos tab can always be added manually even while the preference is off. The preference remains the master visibility gate: while it is off, no task's right panel ever shows a Todos tab automatically, regardless of what any saved layout or profile records for that panel. While it is on, every open and subsequently opened desktop task's right panel shows a Todos tab, subject to the sub-option below.
- **AC-UI-AGENT-TODO-LIST-PANEL-001.2:** A sub-option, "Only pin when todo list is not empty" (default **off**), is rendered beneath the main toggle and appears only while the main preference is on. It is never rendered disabled: turning the main preference off inhibits it (hides it entirely) while preserving its saved value, so re-enabling the main preference restores the sub-option in its previous state. When the sub-option is on, the automatic pin adds the Todos tab only when the active session's todo list is not empty — "not empty" meaning the same two-source fallback the panel content uses: live `sessionTodos.bySessionId` entries, or the latest persisted `todo`-type message (`buildTodoItems`) when the session completed before the page loaded. The sub-option gates only the automatic pin: it never removes an already-open Todos tab, and it never affects the workbench "+" menu's always-available manual Todos row.
- **AC-UI-AGENT-TODO-LIST-PANEL-001.3:** The **Todos** panel is registered as a new reusable, single-instance panel id (`todos`), alongside Agent, Files, Changes, PR Details, Terminal, Plan, Browser, and VS Code (`REUSABLE_PANEL_IDS`/`KNOWN_PANEL_IDS`, `apps/web/lib/state/layout-manager/constants.ts`), so `Settings > General > Layouts`' visual editor can configure *where* it goes exactly like every other reusable panel, independent of the preference. A saved layout's `todos` entry is a placement template only — analogous to how `docs/specs/ui/requirements/task-layout-profiles.md` already treats the canonical `pr-detail` panel's saved position ("a placement template, not an instruction to keep an empty runtime tab open"). No built-in template (`defaultLayout()`, `compactLayout()`, etc. in `apps/web/lib/state/layout-manager/presets.ts`) includes `todos`; its runtime presence is controlled solely by the preference, the same way `pr-detail`'s runtime presence is controlled solely by review linkage.
- **AC-UI-AGENT-TODO-LIST-PANEL-001.4:** A conditional-panel synchronization hook (mirroring the existing `useSyncReviewPanel`/`syncCanonicalReviewPanel` pattern in `apps/web/components/task/dockview-review-panel-sync.ts`, keyed on the preference value instead of review-linkage identity) adds or removes the live `todos` panel for the active task whenever the preference, active task, active session, or Dockview readiness changes:
- **AC-UI-AGENT-TODO-LIST-PANEL-001.5:** **On:** if the active task's live layout does not already contain `todos`, add it as an inactive tab (so it never steals focus) in the group and tab index configured by the user's custom Default layout when that layout configures a `todos` placement, otherwise beside Files and Changes in the pinned right column's top group. When the "Only pin when todo list is not empty" sub-option is on, the add is suppressed while the active session's todo list is empty; the sync re-runs (and then adds) as soon as todo entries arrive, either live via WS or via persisted message history.
- **AC-UI-AGENT-TODO-LIST-PANEL-001.6:** **Off:** if the active task's live layout contains `todos`, remove it.
- **AC-UI-AGENT-TODO-LIST-PANEL-001.7:** Once present, the Todos tab has a normal close control and normal drag/reorder/split behavior — no special restriction is introduced. Closing it removes it from the current view; there is no per-session memory of that closure. The preference — not the close action — is the authoritative on/off control: switching tasks and back, or any other event that re-runs the synchronization (e.g. a layout restoration completing), re-adds it while the preference remains on. This mirrors the existing PR Details pattern, minus its closed-for-session suppression, which this feature does not need because the preference itself is the single deliberate on/off action here (PR Details additionally suppresses re-creation because *review linkage*, not a deliberate user setting, is what re-triggers it).
- **AC-UI-AGENT-TODO-LIST-PANEL-001.8:** The Todos panel is also listed in the task workbench's own "+" add-panel menu (`apps/web/components/task/dockview-add-panel-items.tsx`), alongside Plan/Browser/VS Code, so a user can manually open (or refocus) it in any group regardless of the preference's current value — mirroring Plan's always-shown convention (no session-count guard) since, like Plan, it is off by default and single-instance rather than a near-always-open panel like Files/Changes. Manually adding it while the preference is off is not itself persisted or protected: the next event that re-runs the visibility-sync hook (e.g. switching tasks and back) removes it again, the same as it would for any other task once the preference turns off.

## System design

The migrated technical source is split into [part 1](../system-design/agent-todo-list-panel.md).
