---
status: shipped
created: 2026-08-03
owner: kandev
---

# Agent Todo List Panel

## Why

The coding agent's own mid-session todo list (Claude Code's `TodoWrite`-style
tool calls, and native ACP `session/update` Plan notifications) is already
tracked end-to-end by the backend and streamed live to the frontend, but today
it only ever renders inline: a small status-bar chip above the chat composer
(`TodoIndicator`) and a collapsible card in the chat transcript
(`TodoMessage`). Users who want to keep the current todo checklist visible
while scrolled away from the composer, or alongside Files/Changes, have no way
to do so. Users need a Settings option to enable or disable a persistent
**Todos** tab in the desktop right panel that shows the same checklist.

## What

- `Settings > General` exposes a single boolean preference, "Show agent todo
  list panel" (default **off**, preserving today's behavior for every existing
  user). The preference is a true, unconditional visibility gate: while it is
  off, no task's right panel ever shows a Todos tab, regardless of what any
  saved layout or profile records for that panel. While it is on, every open
  and subsequently opened desktop task's right panel shows a Todos tab.
- The **Todos** panel is registered as a new reusable, single-instance panel
  id (`todos`), alongside Agent, Files, Changes, PR Details, Terminal, Plan,
  Browser, and VS Code (`REUSABLE_PANEL_IDS`/`KNOWN_PANEL_IDS`,
  `apps/web/lib/state/layout-manager/constants.ts`), so `Settings > General >
  Layouts`' visual editor can configure *where* it goes exactly like every
  other reusable panel, independent of the preference. A saved layout's
  `todos` entry is a placement template only — analogous to how
  `docs/specs/ui/task-layout-profiles.md` already treats the canonical
  `pr-detail` panel's saved position ("a placement template, not an
  instruction to keep an empty runtime tab open"). No built-in template
  (`defaultLayout()`, `compactLayout()`, etc. in
  `apps/web/lib/state/layout-manager/presets.ts`) includes `todos`; its
  runtime presence is controlled solely by the preference, the same way
  `pr-detail`'s runtime presence is controlled solely by review linkage.
- A conditional-panel synchronization hook (mirroring the existing
  `useSyncReviewPanel`/`syncCanonicalReviewPanel` pattern in
  `apps/web/components/task/dockview-review-panel-sync.ts`, keyed on the
  preference value instead of review-linkage identity) adds or removes the
  live `todos` panel for the active task whenever the preference, active
  task, active session, or Dockview readiness changes:
  - **On:** if the active task's live layout does not already contain
    `todos`, add it as an inactive tab (so it never steals focus) in the group
    and tab index configured by the user's custom Default layout when that
    layout configures a `todos` placement, otherwise beside Files and Changes
    in the pinned right column's top group.
  - **Off:** if the active task's live layout contains `todos`, remove it.
- Once present, the Todos tab has a normal close control and normal
  drag/reorder/split behavior — no special restriction is introduced. Closing
  it removes it from the current view; there is no per-session memory of that
  closure. The preference — not the close action — is the authoritative
  on/off control: switching tasks and back, or any other event that re-runs
  the synchronization (e.g. a layout restoration completing), re-adds it while
  the preference remains on. This mirrors the existing PR Details pattern,
  minus its closed-for-session suppression, which this feature does not need
  because the preference itself is the single deliberate on/off action here
  (PR Details additionally suppresses re-creation because *review linkage*,
  not a deliberate user setting, is what re-triggers it).
- The Todos panel is also listed in the task workbench's own "+" add-panel
  menu (`apps/web/components/task/dockview-add-panel-items.tsx`), alongside
  Plan/Browser/VS Code, so a user can manually open (or refocus) it in any
  group regardless of the preference's current value — mirroring Plan's
  always-shown convention (no session-count guard) since, like Plan, it is
  off by default and single-instance rather than a near-always-open panel
  like Files/Changes. Manually adding it while the preference is off is not
  itself persisted or protected: the next event that re-runs the
  visibility-sync hook (e.g. switching tasks and back) removes it again,
  the same as it would for any other task once the preference turns off.
- The Todos panel renders the same checklist content and status semantics as
  the existing `TodoIndicator` popover (pending / in-progress / completed /
  failed rows, in emission order) for the panel's owning task's active
  session. It sources this from the live `sessionTodos.bySessionId` slice
  when the session has emitted a live update since the page loaded, falling
  back to the latest persisted `todo`-type message
  (`buildTodoItems` in `apps/web/hooks/use-processed-messages.ts`) for a
  session that already completed before the page loaded — exactly the same
  two-source fallback `TodoIndicator`'s own call site already uses, so the
  panel and the chip never disagree about a given session's todos. No new
  backend event, WS payload, or data model is introduced.
- When the active session has no todo entries yet, the panel shows an empty
  state instead of being hidden, matching how Files/Changes/Plan behave with
  no applicable content.
- The existing inline `TodoIndicator` chip and `TodoMessage` transcript cards
  are unaffected by this preference; the panel is an additional surface for
  the same data, not a replacement.
- This feature only affects the desktop Dockview task workbench. Mobile and
  tablet task views are out of scope, matching `docs/specs/ui/task-layout-profiles.md`'s existing desktop-only scope for panel placement.

## Data model

No new entity. Reuses the existing per-user settings JSON blob
(`apps/backend/internal/user/models/models.go`'s `UserSettings` struct,
persisted in SQLite) by adding one field:

| Field | Type | Constraint |
|---|---|---|
| `ShowTodoListPanel` (Go) / `showTodoListPanel` (frontend) / `show_todo_list_panel` (wire) | boolean | Defaults to `false` for new and existing users; no migration needed since the JSON blob is read with per-field defaults. |

Reuses the existing `sessionTodos.bySessionId: Record<sessionId, PlanEntry[]>`
frontend store slice (`apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`)
and the existing `PlanEntry{Description, Status, Priority}` backend struct
(`apps/backend/internal/agentctl/types/streams/agent.go`) as the sole todo data
source — no new fields on either.

Reuses the existing `LayoutState`/`SavedLayout` reusable-panel data model
(`docs/specs/ui/task-layout-profiles.md`'s Data model section). The new
canonical panel id is `todos`, added to `REUSABLE_PANEL_IDS`/`KNOWN_PANEL_IDS`
alongside `pr-detail`; it carries no task-specific keyed variant (unlike
`pr-detail`/`mr-detail`).

## API surface

No new endpoint. Extends the existing per-user settings contract:

- `GET /api/v1/user/settings` response gains `show_todo_list_panel: boolean`.
- `PATCH /api/v1/user/settings` accepts `show_todo_list_panel?: boolean` as a
  partial update, following the same merge-patch semantics as every other
  boolean field on that endpoint (e.g. `show_transcript_auto_scroll_control`).
- The boot/hydration payload (`apps/backend/internal/backendapp/boot_state_routes.go`)
  gains `showTodoListPanel` alongside the other camelCase display-preference
  fields it already returns.
- No new WebSocket event type. The existing user-settings WS sync path
  (`apps/web/lib/ws/handlers/users.ts`) carries the new field like every other
  settings field.

## Failure modes

- If the settings PATCH fails, the Settings page keeps the unsaved toggle
  state and reports the error; the previously persisted value and the
  workbench's current tab state are unchanged (matching the existing Settings
  save-contributor failure behavior used by every other toggle on the page).
- If the active task has no open Dockview API yet (task still loading) when
  the preference is toggled on, the tab is added as soon as the workbench
  mounts, following the same deferred-sync pattern `useSyncReviewPanel`
  already uses for PR Details.
- If a session has todo data that fails to parse (malformed persisted `todo`
  message), the panel falls back to an empty state rather than throwing,
  matching `TodoMessage`'s existing parse-failure behavior.

## Persistence guarantees

- The preference is a per-user, backend-persisted setting: it survives
  browser restarts and is portable across the user's devices, exactly like
  `showTranscriptAutoScrollControl` and the other existing boolean display
  preferences.
- A `todos` entry configured in a saved layout (custom profile, Default
  override, or task-specific environment layout) persists exactly like any
  other reusable panel's placement and is never rewritten by toggling the
  preference; it supplies placement only, never runtime visibility.
- Live agent-generated todo data itself is unaffected: it continues to be
  persisted as `todo`-type chat messages and streamed live exactly as today.

## Scenarios

- **GIVEN** a user who has never touched the preference, **WHEN** they open
  any desktop task, **THEN** no Todos tab exists anywhere in the right panel
  and the existing `TodoIndicator`/`TodoMessage` surfaces behave exactly as
  before.
- **GIVEN** the preference is off, **WHEN** the user turns it on from
  `Settings > General` and saves, **THEN** the active task's right panel gains
  an inactive Todos tab beside Files and Changes without changing the
  currently selected tab, and every other currently open task gains one the
  next time it becomes active.
- **GIVEN** the preference is on and the active session has emitted todo
  entries, **WHEN** the user selects the Todos tab, **THEN** it shows the same
  pending/in-progress/completed/failed rows, in the same order, that the
  `TodoIndicator` popover shows for that session.
- **GIVEN** the preference is on and the active session has not emitted any
  todo entries yet, **WHEN** the user selects the Todos tab, **THEN** it shows
  an empty state rather than being absent.
- **GIVEN** the preference is on, **WHEN** the user turns it off and saves,
  **THEN** the Todos tab disappears from every currently open task's right
  panel immediately, without requiring a reload, even for a task whose saved
  layout still records a `todos` placement.
- **GIVEN** a custom Default layout that places `todos` in a specific group
  and tab index, **WHEN** the preference is on and a fresh task with no
  task-specific layout opens, **THEN** the Todos tab appears in that
  configured group and index instead of the Files/Changes fallback group.
- **GIVEN** the preference is off, **WHEN** the user opens `Settings > General
  > Layouts` and edits any profile, **THEN** Todos still appears in the visual
  editor's addable-panel list exactly like Plan, Terminal, and PR Details —
  configuring its placement there does not itself make it visible while the
  preference is off.
- **GIVEN** the preference is on and a Todos tab is present, **WHEN** the user
  closes it using its normal tab close control and then switches away from and
  back to the same task, **THEN** the tab reappears, because the preference —
  not the close action — is the authoritative on/off control and this feature
  introduces no closed-for-session memory.
- **GIVEN** the preference is off, **WHEN** the user opens the task
  workbench's own "+" add-panel menu on any group, **THEN** a Todos row is
  present (matching Plan/Browser/VS Code's always-shown convention) and
  selecting it opens the Todos panel in that group immediately, independent
  of the preference.

## Out of scope

- Any change to how the agent's todo data is produced, normalized, or
  streamed by the backend (ACP Plan notifications, `TodoWrite`-style tool
  parsing, `SessionTodosEventPayload`).
- Removing, hiding, or changing the existing inline `TodoIndicator` chip or
  `TodoMessage` transcript cards.
- Auto-showing the tab only when the session actually has todo entries (the
  preference is a manual on/off switch, not a content-driven auto-show like
  PR Details' review-linkage trigger).
- A closed-for-session suppression memory analogous to PR Details' — closing
  the tab while the preference is on is temporary, not sticky.
- An "unseen update" badge/dot on the Todos tab (Plan's `PlanTab` has one;
  Todos does not gain one in this iteration).
- Mobile or tablet task-detail layouts.
- A per-task or per-layout override of the preference; it is a single global
  per-user setting.
