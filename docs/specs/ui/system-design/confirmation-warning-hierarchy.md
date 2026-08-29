---
status: current
system: ui
requirements:
  - REQ-TASKS-CONFIRMATION-WARNING-001
  - REQ-TASKS-CONFIRMATION-SURFACE-002
---

# Task Confirmation Warning Hierarchy System Design

## Purpose and boundaries

This design owns the presentation contract for the shared still-working warning
and the fine-pointer archive confirmation surface used by task archive and
delete workflows. It changes density, archive-only width, and mounting location
only; the task in-flight signal and destructive-action behavior remain owned by
their existing components and runtime contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-CONFIRMATION-WARNING-001` | [Components and responsibilities](#components-and-responsibilities) and [Mobile and desktop containment](#mobile-and-desktop-containment) |
| `REQ-TASKS-CONFIRMATION-SURFACE-002` | [Popover width contract](#popover-width-contract), [Fine-pointer mounting](#fine-pointer-mounting), and [Mobile and desktop containment](#mobile-and-desktop-containment) |

## Components and responsibilities

- `apps/web/components/task/task-still-working-warning.tsx` remains the single
  markup and style owner.
- `TaskArchiveConfirmDialog` and `TaskDeleteConfirmDialog` render it inside the
  existing full confirmation dialog.
- `TaskArchiveConfirmation` renders the same component through
  `ArchiveDescription` for the desktop popover and phone inline branch.
- Consumers continue to decide whether a warning mounts. The shared component
  does not inspect task state or alter callbacks.

### Popover width contract

`ActionConfirmPopover` gains a small width/size contract whose default remains
the current `w-64` surface. The archive confirmation opts into a modest wider
variant, targeting `w-72`, with a viewport-aware maximum width such as
`max-w-[calc(100vw-1rem)]`. The class contract stays local to the shared
popover primitive, so watcher and other confirmation consumers do not widen
implicitly. The archive body keeps `text-pretty` wrapping and the existing
title/action hierarchy.

### Fine-pointer mounting

At the `TaskItemWithContextMenu` adapter boundary, `useResponsiveBreakpoint`
determines where the already-created archive confirmation node mounts:

- Fine-pointer confirmation mounts as a sibling of the cloned task row inside
  the existing anchor wrapper. It does not pass `archiveConfirmation` into
  `TaskItem`, so `TaskItem` does not add its `flex-wrap` row branch or the
  `basis-full` action slot for a portaled popover.
- Coarse-pointer confirmation continues to pass through `TaskItem`'s existing
  inline action slot. Its intentional row expansion and mobile action geometry
  remain unchanged.

Both branches reuse the same `useTaskSwitcherArchiveConfirmation` node,
callbacks, anchor ref, and focus-return ref. No business logic or duplicate
confirmation markup is introduced. The change removes the layout cause of
fine-pointer row growth instead of compensating for it with row dimensions.

## Data and contracts

The component preserves `data-testid="still-working-warning"`, `role="alert"`,
the translated `task:stillWorkingWarning` and subject keys, and the existing
yellow border/background/text classes. The compact style contract is:

- warning container: `gap-1.5`, `p-2.5`, `text-xs`, `leading-5`, and
  `text-pretty`;
- warning icon: `h-3.5 w-3.5`, `mt-0.5`, and `shrink-0`;
- existing rounded border, yellow semantic colors, and dark-mode contrast stay
  unchanged.

No API, WebSocket, state, localization catalog, or persisted-data contract
changes are required.

## Control flow

The existing task-level `foregroundActivity` projection and explicit
`isInFlight` props continue to determine whether the warning is rendered. Once
mounted, the shared component formats the same localized subject and warning
text, then returns the compact alert markup. Archive/delete callbacks and
dialog state remain untouched. The context-menu adapter only chooses the
mounting branch described above based on the existing responsive pointer
classification.

## Failure and recovery

No new failure path exists. If localized text is longer than the available
width, `text-pretty` and the explicit line height allow natural wrapping inside
the existing flex container. If no task activity is present, no warning mounts,
as before.

## Persistence

None. This is a client-side presentation-only change.

## Observability

Existing component tests continue to assert warning presence and absence for
generating, background, and idle activity. A compactness regression asserts the
shared class contract. A focused archive-popover regression asserts the wider
archive-only class contract. Rendered desktop and phone checks inspect
computed type hierarchy, popover width and viewport bounds, source-row height
before/open/cancel, action reachability, and document overflow.

## Mobile and desktop containment

The desktop check uses the real sidebar task row with a fine pointer. It
records the row `getBoundingClientRect().height` before opening Archive, while
the archive popover is visible, and after Cancel; all three values must remain
stable within subpixel precision. It also verifies the archive popover is
strictly wider than 256px and remains inside the viewport at compact widths.

The phone check keeps the existing coarse-pointer sidebar inline flow. It
expects the inline confirmation to remain intentionally row-owned, keeps
actions at or above 44px, and asserts zero document horizontal overflow.

## Related decisions

- [ADR 0049: Fine-grained foreground-idle busy signal](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md)
- [Mobile task navigation](../requirements/mobile-task-navigation.md)
