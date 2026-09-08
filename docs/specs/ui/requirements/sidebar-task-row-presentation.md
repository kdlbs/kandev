---
status: active
system: ui
created: 2026-08-23
updated: 2026-08-28
owners:
  - kandev
---

# Sidebar Task Row Presentation Requirements

## Overview

Each saved sidebar view owns its task-row presentation so focused views can show only the metadata
needed for their workflow. The view editor controls the details row, detail order and visibility,
and the value shown on the right side of each task row across desktop and mobile layouts.

## Requirements

### REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001: Configurable task-row presentation

**Intent:** Let each saved sidebar view control task-row metadata without changing established
defaults, persistence ownership, accessibility, or responsive interaction behavior.

#### Acceptance criteria

- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.1:** Each saved sidebar view and matching draft may store whether details are enabled, the order and visibility of relative time, repository, and pull request number, and a trailing choice of Git changes, relative time, change request status, or nothing.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.2:** Missing or malformed task-row settings normalize to the established layout of enabled details in canonical order with Git changes on the right, while valid empty visible details remain empty.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.3:** The view editor shows a collapsed Task row section after Group by, summarizes its current configuration, and expands to expose a details toggle, ordered field controls, and the Right side selector.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.4:** Presentation edits preview through the existing draft workflow; Save, Save as, and Discard persist or restore them with the view, while expanding or collapsing the section does not create a draft.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.5:** Disabling details removes all under-title metadata and leaves the title row vertically centered with its task-state icon, badges, trailing content, and action menu.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.6:** Selecting relative time on the right suppresses its duplicate detail while preserving its saved detail position and visibility and marking it as shown on the right in the editor.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.7:** Grouping by repository suppresses duplicate repository details without changing the saved field setting, and absent metadata reserves no empty row space.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.8:** On an idle fine-pointer row, a present trailing value uses the far-right edge and the hidden action menu reserves no width. Row hover or keyboard focus reveals the menu beside an interactive change-request status without covering that status.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.9:** Desktop uses the anchored editor popover, while phone and tablet layouts use an inset, safe-area-aware bottom drawer with one internal vertical scroll area and separators constrained to the drawer width.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.10:** Mobile section headers, switches, selectors, and drag handles provide at least 44 by 44 CSS pixel touch targets, and field reorder supports pointer, touch, and keyboard input with an accessible announcement.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.11:** A failed draft, create, or save request cannot roll back a later confirmed sidebar write; all sidebar writes share one synchronization journal and restore only the latest confirmed state.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.12:** The editor and task rows cause no document-level horizontal overflow at supported widths, and the mobile task row remains the primary tap target.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.13:** Relative time on the right uses localized compact seconds, minutes, hours, days, weeks, or years without direction words or calendar phrases such as **ago** or **yesterday**.
- **AC-UI-SIDEBAR-TASK-ROW-PRESENTATION-001.14:** Every rendered right-side time uses the same fixed visual width, right alignment, and tabular numbers. A missing or invalid timestamp reserves no time width, and assistive technology receives the full localized relative time.

## Migrated source detail

## Why

Sidebar task rows always show the same metadata and trailing Git change count. This layout is useful
for a broad task view, but it can add noise in a focused view. Users also cannot place the metadata
in the order that matches their work.

Each saved sidebar view already owns its filter, sort, and group settings. Task-row presentation
belongs to the same view because a review view and a planning view can need different information.

## What

- Each saved sidebar view owns a task-row presentation setting.
- The view editor contains a collapsed **Task row** section after **Group by**. It starts collapsed
  each time the editor opens.
- The collapsed header summarizes the current saved or draft configuration, for example
  **3 details · Git changes**, **2 details · Relative time**, **Details hidden · Pull request status**, or
  **Details hidden · Nothing**.
- Expanding the section exposes one master **Details row** toggle, the detail fields, and a
  **Right side** choice.
- The details row supports these configurable fields:
  - Relative time
  - Repository
  - Pull request number
- Each detail field has a visibility control and a drag handle. Dragging changes the field order.
  Keyboard users can reorder the same fields without a pointer.
- Turning off **Details row** hides the complete under-title metadata area. This includes the three
  configurable fields, plugin task-row metadata, queued-prompt and WIP-queue indicators, and debug
  poll-mode data. The title row and its badges remain visible.
- **Right side** offers **Git changes**, **Relative time**, **Pull request status**, and **Nothing**.
- The current layout is the default: all three detail fields are visible in their current order,
  the details row is enabled, and Git changes appear on the right.

## Display rules

- Relative time keeps the view's current time meaning. It uses the last update time by default. A
  view sorted by last activity uses the last activity time, with the last update time as fallback.
- When relative time is selected on the right, the row does not repeat relative time in the details
  row. The editor keeps the field's saved position and visibility and marks it as shown on the
  right. It returns to the details row when the right-side choice changes.
- When the task list is grouped by repository, a visible repository detail remains suppressed
  because the group heading already provides that value.
- Pull request number and pull request status can appear together. The number is metadata. The
  status is the existing provider indicator.
- Selecting pull request status on the right moves the existing GitHub, GitLab, or registered
  provider indicator from the title area. It does not create a duplicate indicator.
- A missing value does not reserve empty space. For example, a task without a pull request shows no
  pull request number or trailing status gap.
- **Nothing** removes the trailing data. The task action menu remains available.
- An idle fine-pointer row does not reserve space for its hidden action menu. A present trailing
  value uses the far-right edge.
- Row hover or keyboard focus reveals the action menu. The menu appears beside an interactive
  pull request status and does not cover its pointer or keyboard target.
- Relative time on the right uses a compact elapsed-time ladder: seconds, minutes, hours, days,
  weeks, and years. It does not show direction words or calendar phrases.
- Every right-side time uses one fixed visual column with right-aligned tabular numbers. The full
  localized relative phrase remains available to assistive technology.

## Editor behavior

- A presentation edit previews immediately in the active task list and creates the existing saved-
  view draft.
- **Save**, **Save as...**, and **Discard** use the existing view workflow. Saving overwrites the
  active view, saving as creates an independent view, and discarding restores the saved layout.
- Opening or closing the **Task row** section is temporary interface state. It does not create a
  draft, mark the view as changed, or persist.
- The section always starts collapsed when the desktop popover or mobile drawer opens.
- The field order stays stable when a field is hidden. Re-enabling the field restores it at its
  previous position.

## Data model and persistence

The user-settings representation adds one optional `task_row` object to each saved sidebar view
and its draft:

```json
{
  "details_enabled": true,
  "detail_order": ["relative_time", "repository", "pull_request_number"],
  "visible_details": ["relative_time", "repository", "pull_request_number"],
  "trailing": "git_changes"
}
```

The other trailing values are `relative_time`, `change_request_status`, and `none`.

- The backend-owned user settings remain the durable source. No local-storage preference is added.
- The saved view and its matching draft carry the complete presentation object.
- A client normalizes omitted, null, or malformed presentation data before rendering.
- Normalization ignores unknown fields, removes duplicates, and appends missing known detail fields
  to the canonical order. It limits visible details to known fields.
- An absent object resolves to the current layout. This rule preserves old saved views and lets an
  older client ignore an unknown future field when it reads the view.
- Existing optimistic save, rollback, active-view validation, and 50-view limit behavior do not
  change.

See [ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md) for ownership of
portable user settings.

## Responsive and accessible behavior

- Desktop keeps the view editor in its anchored popover with one internal vertical scroll area.
- Phone and tablet layouts present the same view editor in an inset bottom drawer. The drawer owns
  one vertical scroll area, respects the bottom safe area, and returns focus to its trigger when it
  closes.
- The mobile **Task row** header, switches, selection controls, and drag handles have at least a
  44 by 44 CSS pixel touch target.
- Field reordering supports pointer, touch, and keyboard input. The order is exposed to assistive
  technology, and a completed reorder receives an accessible announcement.
- The mobile task list keeps the row as its primary tap target. A passive pull request indicator
  does not become a competing touch action.
- The phone layout keeps its visible, 44 CSS pixel task action. It does not depend on hover to open
  the task menu.
- The editor and task rows do not cause document-level horizontal overflow at supported widths.

## Failure behavior

- A failed draft or save write restores the last confirmed saved views, active view, and draft. It
  uses the existing sidebar-view error message.
- A failed or unavailable metadata source only omits that value. It does not change the saved
  presentation setting.
- An unknown trailing value resolves to Git changes. A missing or malformed detail order resolves
  to the canonical order. A valid empty `visible_details` list remains empty.

## Scenarios

- **GIVEN** an existing saved view without task-row settings, **WHEN** Kandev loads it after an
  upgrade, **THEN** its rows keep the current details and Git changes layout.
- **GIVEN** the view editor opens, **WHEN** the user has not expanded **Task row**, **THEN** the
  section uses one compact header and its open state does not make the view dirty.
- **GIVEN** the user hides repository and moves pull request number before relative time, **WHEN**
  the active rows preview the draft, **THEN** the details row shows pull request number before
  relative time and no repository value.
- **GIVEN** details are disabled, **WHEN** a task has plugin metadata, queued prompts, or WIP status,
  **THEN** no under-title metadata row renders and the title row remains complete.
- **GIVEN** relative time is visible in details, **WHEN** the user selects relative time for the
  right side, **THEN** each task shows one relative time value on the right and none in details.
- **GIVEN** pull request status is selected for the right side, **WHEN** a task has a linked change
  request, **THEN** the provider status uses the idle far-right edge without a blank menu gap. The
  menu appears beside it on row hover or keyboard focus.
- **GIVEN** relative time is selected for the right side, **WHEN** rows contain timestamps from
  seconds through years ago, **THEN** each row uses the same short time column and shows only a
  localized number and unit.
- **GIVEN** a view is grouped by repository, **WHEN** repository is enabled in its details, **THEN**
  the task rows omit the repeated repository while the saved field remains enabled.
- **GIVEN** the user saves, saves as, discards, reloads, or opens the view on another client,
  **THEN** task-row settings follow the same result as the view's filters, sort, and grouping.
- **GIVEN** the mobile view editor is open, **WHEN** the user expands and reorders task-row fields,
  **THEN** the drawer remains within the viewport, scrolls internally, respects the safe area, and
  provides touch-sized controls.

## Out of scope

- Configuring the title, task-state icon, task color, issue badge, remote-executor badge, archived
  badge, subtask toggle, or task action menu.
- Adding new metadata sources or changing provider status derivation.
- Applying this preference to Kanban cards or the `/tasks` rich task list.
- A global task-row preference outside saved sidebar views.
- A new cross-tab conflict model for full-array user-settings writes.
- Changing relative-time copy on task details, activity feeds, plugins, or other surfaces.

## Implementation plan

[Sidebar task row presentation plan](../../../plans/sidebar-task-row-presentation/plan.md)

[Compact sidebar trailing content plan](../../../plans/sidebar-task-row-compact-trailing/plan.md)
