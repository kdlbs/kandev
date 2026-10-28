---
status: current
system: ui
requirements:
  - REQ-UI-TASK-PAGINATION-PRESENTATION-001
---

# Task Pagination Presentation System Design

## Purpose and boundaries

UI owns the task-list pagination presentation defined by the
[requirements](../requirements/task-pagination-presentation.md). Existing task
query and pagination state remain authoritative; this design changes neither.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-TASK-PAGINATION-PRESENTATION-001` | Composition and behavior; Verification |

## Composition and behavior

`TasksPagination` in `apps/web/app/tasks/tasks-pagination.tsx` uses the existing
`Pagination`, `PaginationContent`, `PaginationItem`, `Button`, and Select
primitives. Keep its range, page-size selector, current page indication, page
window, first/last visibility, disabled states, and existing page-change handler.
The list's existing vertical scroll region owns the footer; no new scroll owner,
fixed element, overlay, or persisted responsive preference is needed.

Allow the local pagination content to wrap on narrow viewports, preserving item
order and all displayed direct-page choices. Restore the existing single row
when there is room using the local responsive classes. Give the row a bounded
available width so wrapping cannot be defeated by intrinsic flex sizing. The
existing 44px phone/coarse action geometry remains intact, including for a
narrow fine-pointer viewport; desktop sizing remains unchanged. Verify actual
geometry rather than inferring it from classes. At wide viewports the existing
first/last actions and page-number window remain on one row.

The existing `/tasks` list is the phone entry point. Its inline pagination is a
short, frequently used control set: wrapping is a narrow exception to the usual
one-dimensional mobile layout because it preserves immediate page actions and
avoids a second navigation surface. The list's single vertical scroll region
retains ownership. The local footer reserves phone bottom space for the existing
floating add button and its safe-area inset, so a final action can scroll clear.
The native mobile column navigator in
`components/kanban/mobile-column-tabs.tsx` is the nearest curated navigation
exemplar for visible context and previous/next actions; its hierarchy drawer is
unnecessary for this small, already visible page window. State and handlers are
shared across layouts. No new copy, settings, API, dependency, or telemetry is
required.

## Verification

A real managed backend supplies isolated synthetic tasks without starting agents.
Native mobile Chromium exercises 320px, 393px, and 430px phone viewports on page 4 of 7,
including a vertical-only scroll to the visible footer after each navigation.
It proves actual target containment, 44px targets, no unintended horizontal
scroll, Next from 4 to 5, and previous and direct-page selection. Capture the
visible footer, with truthful labels. A 1280px fine-pointer desktop run proves
its existing actions and composition. Cover a narrow fine-pointer viewport and
breakpoint-adjacent layouts as well. Component tests cover boundary disabled
states, page changes, range and page-size behavior. Neither DOM-only geometry
stubs nor an unscrolled screenshot are evidence of native containment.
