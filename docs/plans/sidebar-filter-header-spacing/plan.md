---
created: 2026-09-26
status: complete
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001
  - REQ-UI-CONTROL-SIZING-001
system_design:
  - ../../specs/ui/system-design/sidebar-automatic-task-colors.md
  - ../../specs/ui/system-design/control-sizing.md
legacy_specs: []
---

# Implementation Plan: Sidebar Filter Header Spacing

## Outcome and evidence

The desktop sidebar view editor shall keep the Filters heading and Add action visibly below the divider after the View row. The supplied screenshot shows the heading touching that line. Source inspection identifies the cause: `FilterSection` uses `pt-0` and a `-mt-1` desktop header, while its Add button also uses `-my-1`. The touch drawer uses `pt-2` without those negative margins, but its compact Add button measures only 24px. Implementation shall measure the rendered gap before and after the correction and provide the phone action with a 44px minimum hit area.

UI owns this compact editor presentation. The repair extends `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.8` and the editor composition in the paired design. The earlier sidebar-automatic-task-colors plan remains a completed delivery record.

## Scope and approach

In `sidebar-view-editor.tsx`, give the desktop Filters section the same 10px top inset as the drawer and remove the desktop-only negative vertical margins from the heading and Add button. Keep the compact Add styling while giving it a 44px minimum hit area in the phone and tablet drawer. Keep the existing bottom padding, separator, filter rows, and section order. The popover remains anchored on desktop; the drawer keeps its fixed header and one scrolling editor body. No state, API, copy, or persistence change is needed.

## ASCII UI preview

UI-01, desktop Tasks view gear, empty Filters section:

```text
Before                       After
| View: Archived           | | View: Archived           |
+--------------------------+ +--------------------------+
|FILTERS              + Add| |                          |
|                          | | FILTERS              + Add|
+--------------------------+ +--------------------------+
| SORT                     | | SORT                     |
```

UI-02, phone Tasks picker gear, existing inset drawer:

```text
| Filters drawer title     |  fixed header
| View: Archived           |
+--------------------------+
|                          |
| FILTERS              + Add|
| SORT                     |  one scrolling editor body
```

The blank line illustrates the visible top inset, not an exact pixel scale. Both views map to `AC-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-001.8`. The phone composition and touch target remain as shipped.

## Verification and risk

One work order adds a desktop Playwright geometry test that fails before the class change, then passes after it. A focused phone test checks the same boundary and that Add remains reachable inside the drawer. Run the exact commands in the work order against a fresh production web build. Risk is confined to editor height: removing the negative margins makes the desktop Filters section taller, so verify the bounded popover still scrolls and adjacent sections remain reachable.

## Work orders

1. [task-01-inset-filter-header](task-01-inset-filter-header.md) — done; no dependencies.

## Open questions

None. The requested visual correction and existing phone behavior determine the scope.
