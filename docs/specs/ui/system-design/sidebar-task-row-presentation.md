---
status: draft
system: ui
requirements:
  - REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001
---

# Sidebar Task Row Presentation System Design

## Purpose and boundaries

The UI system owns this independent task-row presentation contract. Task and integration systems
continue to own timestamps, task state, and change-request status.

This refinement changes only the compact trailing slot. It does not change saved view data,
provider state, task navigation, or the full relative-time formatter used by other surfaces.

## Requirement mapping

| Requirement                                | Design sections                                                                                                                                                  |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-SIDEBAR-TASK-ROW-PRESENTATION-001` | [Trailing layout](#trailing-layout), [Compact elapsed time](#compact-elapsed-time), [Responsive behavior](#responsive-behavior), [Accessibility](#accessibility) |

## Components and responsibilities

- `TaskItem` resolves the saved presentation and supplies the selected timestamp and trailing value.
- `TaskItemTrailing` owns the compact Git changes, relative time, change-request status, and menu
  layout.
- `TaskMenuButton` keeps the established context-menu dispatch and focus semantics.
- `lib/i18n/formats.ts` owns a sidebar-only elapsed-time formatter and its locale-aware unit tokens.
- `app/globals.css` keeps the existing phone rule that exposes a 44 CSS pixel task action.

## Trailing layout

The change-request branch uses one flex cluster. The status is the only width-bearing item while a
fine-pointer row is idle. The menu wrapper has zero width and does not create a blank trailing gap.

Outer-row hover or focus expands the menu wrapper to the button width. The menu appears beside the
status. The status stays mounted and keeps its pointer and keyboard disclosure behavior.

The branch omits the complete cluster when no status exists. The established menu-only fallback
continues to expose task actions.

The relative-time branch keeps its overlay swap on desktop fine pointers. Replace both `w-11`
constraints in `TaskItemTrailing` with intrinsic time sizing and a 24 CSS pixel minimum matching
`TaskMenuButton`. Keep the time mounted and width-bearing at opacity zero during hover, focus,
and menu-open state. The absolute menu aligns to the right of the same column. This allocates
the maximum of the time width and button width without JavaScript measurement.

Keep right alignment, tabular numbers, and non-wrapping localized tokens. Different timestamps
may produce different widths; hover alone cannot. Do not restore `w-full` or add `flex-1` to
`TaskItemTitle`: long titles already shrink into the available content width, while short titles
must keep their badges adjacent. Preserve the normal outer `gap-2` spacing.

## Compact elapsed time

The sidebar uses a dedicated formatter. It does not change `formatRelativeTime`, because plugins and
other product surfaces depend on full localized relative phrases.

The formatter calculates elapsed time from the selected timestamp. It clamps future clock skew to
zero and floors each completed unit. It selects these buckets:

| Elapsed value        | Display unit |
| -------------------- | ------------ |
| Less than 60 seconds | seconds      |
| Less than 60 minutes | minutes      |
| Less than 24 hours   | hours        |
| Less than 7 days     | days         |
| Less than 365 days   | weeks        |
| 365 days or more     | years        |

The visual output contains one localized integer and one compact unit token. It never contains a
direction word or an idiomatic calendar phrase. Values above 99 years use a bounded `99+` magnitude
so the visual column remains stable.

Message-catalog keys define the six compact units for every shipped locale. This gives the UI a
stable width on system WebKit runtimes without a new `Intl.DurationFormat` compatibility boundary.

Invalid or empty input returns no display value. `TaskItemTrailing` then renders no time column and
does not reserve an empty gap.

## Responsive behavior

Desktop and other fine-pointer layouts use hover and focus disclosure. The idle change-request
status uses the far-right edge. The menu consumes width only while it is visible.

The phone task-switcher drawer remains the nearest shipped mobile exemplar. The row stays the
primary action. The existing mobile CSS keeps the task menu visible with a 44 CSS pixel target, so
touch users do not depend on hover. The change-request status remains passive on coarse pointers.

The sidebar task list remains the only scroll owner. The change does not add an overlay, safe-area
boundary, or horizontal scroll region.

Below the existing 640px action breakpoint, retain the in-flow menu slot and visible 44px action.
Retain the phone time width if needed to preserve its established composition; scope desktop
compaction to fine pointers at wider viewports. Test 639px, 640px, and 768px with pointer modes
explicitly set. Do not extend compact desktop button sizing into coarse-pointer layouts.

## Accessibility

The change-request status keeps its current focusable tooltip trigger. Keyboard focus can reach the
status and the task menu without overlap.

The compact visual timestamp exposes the full localized `formatRelativeTime` result as its
accessible name. The visual token is not the only time description.

## Failure behavior

A missing change-request status falls back to the menu-only branch. An invalid timestamp omits the
time column. Neither case changes the saved view or reports a user-facing error.

## Tests

Unit tests cover every time boundary, future clock skew, invalid input, the 99-year bound, and all
shipped locales. Component tests cover idle width, hover/focus classes, status reachability, menu
fallback, and accessible time semantics. Browser geometry establishes content sizing and hover stability.

Desktop Playwright coverage proves the idle status reaches the row edge and hover reveals the menu
without covering the status. Mobile Playwright coverage proves the visible touch action, primary
row navigation, touch-target containment, and absence of document horizontal overflow.

The [sidebar title width plan](../../../plans/sidebar-title-width/plan.md) owns the pending
replacement of fixed desktop columns. Earlier completed plans remain historical delivery records.

## Related designs

- [PR Task Status Summary](pr-task-status-summary.md)
- [Sidebar Task Focus](sidebar-task-focus.md)
