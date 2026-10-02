---
status: active
system: ui
created: 2026-09-30
owners:
  - kandev
---

# Bounded Changes Rendering Requirements

## Purpose and ownership

Users must browse large Changes collections without exhausting browser resources.
UI owns this presentation contract across working-tree, commit, and provider-file rows.
Workspaces and integrations retain Git data, authorization, and mutation ownership.

Existing [row containment](changes-file-row-containment.md) and
[commit navigation](commit-file-navigation.md) contracts remain applicable.
This requirement adds missing collection-size guarantees. It does not replace
those interaction contracts or task-navigation read coordination.

## Requirements

### REQ-UI-BOUNDED-CHANGES-001: Operable large Changes collections

**Intent:** Preserve complete file and commit access while limiting rendering work.

#### Acceptance criteria

- **AC-UI-BOUNDED-CHANGES-001.1:** With 50,000 available working-tree files, Changes shall remain operable in both tree and list layouts. The user shall reach the final file and navigate to another task without a browser crash.
- **AC-UI-BOUNDED-CHANGES-001.2:** The same outcome shall apply to 50,000 commit rows, provider-file rows, or files within an expanded commit. Mixed sections shall remain operable together.
- **AC-UI-BOUNDED-CHANGES-001.3:** Every available row shall remain reachable. Counts and actions shall use the complete collection, without silent truncation, file exclusion, or changes to repository contents.
- **AC-UI-BOUNDED-CHANGES-001.4:** Scrolling shall preserve expansion, selection, pending actions, and source identity. Range selection and select-all shall include eligible offscreen entries. Equal paths in different repositories shall retain separate action targets.
- **AC-UI-BOUNDED-CHANGES-001.5:** Keyboard users shall reach offscreen entries and return from menus without losing focus. Updates that remove a focused entry shall move focus to the nearest surviving entry or the section control.
- **AC-UI-BOUNDED-CHANGES-001.6:** Hiding, reopening, resizing, or changing the layout shall produce contiguous, correctly measured rows. Live updates shall preserve the visible anchor when that entry survives.
- **AC-UI-BOUNDED-CHANGES-001.7:** Switching task or environment shall prevent old selection, pending focus, and late detail responses from affecting the new context. Unavailable-workspace state shall retain precedence.
- **AC-UI-BOUNDED-CHANGES-001.8:** Phone Changes shall provide the same collection access through its existing navigation. It shall retain one content scroller, wrapped names, visible actions, safe-area clearance, and touch targets of at least 44px.
- **AC-UI-BOUNDED-CHANGES-001.9:** Collapsed sections and commits shall not retain hidden rendered descendants. Scrolling alone shall not fetch commit details. Loading, empty, failure, and retry states shall preserve existing source restrictions.

## Compatibility and exclusions

Existing section order, tree expansion defaults, layout preferences, Git
confirmation, and provider routing remain unchanged. The guarantee covers
available metadata, not unlimited server payloads or complete diff rendering.

Backend pagination, transport limits, global store retention, diff viewers,
cache-directory cleanup, and automatic Git exclusions are outside this contract.

## System design

- [Bounded Changes rendering](../system-design/bounded-changes-rendering.md)

## Implementation plans

- [Bounded Changes rendering](../../../plans/bounded-changes-rendering/plan.md)
- [Changes timeline measurement refresh](../../../plans/changes-timeline-measurement-refresh/plan.md)
- [Changes sidebar history and spacing repair](../../../plans/changes-sidebar-history-spacing/plan.md)
