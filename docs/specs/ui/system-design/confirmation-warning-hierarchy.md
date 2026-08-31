---
status: current
system: ui
requirements:
  - REQ-TASKS-CONFIRMATION-WARNING-001
  - REQ-TASKS-CONFIRMATION-SURFACE-002
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
---

# Task Confirmation Surface System Design

## Purpose and boundaries

This design owns the presentation contract for the shared still-working warning,
the fine-pointer archive confirmation surface, and the cleanup consequence
hierarchy used by task archive and delete workflows. It changes density,
surface-local composition, localized presentation, action semantics, and archive
mounting only; task state, cleanup rules, and callbacks remain owned by their
existing components and runtime contracts.

## Requirement mapping

| Requirement                            | Design section                                                                                                                                                                                                                               |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-TASKS-CONFIRMATION-WARNING-001`   | [Components and responsibilities](#components-and-responsibilities) and [Mobile and desktop containment](#mobile-and-desktop-containment)                                                                                                    |
| `REQ-TASKS-CONFIRMATION-SURFACE-002`   | [Popover width contract](#popover-width-contract), [Fine-pointer mounting](#fine-pointer-mounting), and [Mobile and desktop containment](#mobile-and-desktop-containment)                                                                    |
| `REQ-UI-TASK-CLEANUP-CONFIRMATION-001` | [Task cleanup content model](#task-cleanup-content-model), [Full-dialog composition](#full-dialog-composition), [Compact archive surfaces](#compact-archive-surfaces), and [Mobile and desktop containment](#mobile-and-desktop-containment) |

## Components and responsibilities

- `apps/web/components/task/task-still-working-warning.tsx` remains the single
  markup and style owner.
- `TaskArchiveConfirmDialog` and `TaskDeleteConfirmDialog` render it inside the
  existing full confirmation dialog.
- `TaskArchiveConfirmation` renders the same component through
  `ArchiveDescription` for the desktop popover and phone inline branch.
- Consumers continue to decide whether a warning mounts. The shared component
  does not inspect task state or alter callbacks.

### Task cleanup content model

`getCleanupSummary` and `getBulkCleanupSummary` remain the single place that
maps executor types to localized cleanup consequences. Their return value moves
from a flat `lines` array to structured effects and supporting notes. Effects
cover resources that are stopped, removed, or destroyed. Notes explain
unaffected repository scope or best-effort qualifications. The generic running
session effect remains present for every executor path.

The task subject sentence stays outside that model because archive and delete
describe different task outcomes. Single-task delete uses direct declarative
copy naming the task and its irreversibility. Archive uses corresponding
archive copy without describing archive as irreversible. Bulk strings retain
locale-aware count handling. Translations land together in the five real
catalogs: `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`. The Traditional Chinese
catalogs are generated through the repository's `i18n:zh-hant` workflow. The
QA-only `pseudo` locale remains hidden in production, must stay synchronized
with `en` under `i18n:check`, and is regenerated with `pnpm run i18n:pseudo`.

A shared task-local renderer presents effects as a semantic list and notes as
supporting prose. Full dialogs use paragraphs and a list. The existing compact
archive popover and inline confirmation use the same ordered model with their
established compact spacing. This removes copy drift without moving cleanup
policy into a visual component.

### Full-dialog composition

`TaskArchiveConfirmDialog` and `TaskDeleteConfirmDialog` keep Radix
`AlertDialog` and the existing centered inset surface. The current `size="lg"`
phone width remains because prose benefits from the available line length and
the primitive already preserves 16px viewport insets.

Each full surface uses an auto/minmax/auto layout: title in the first row, one
`minmax(0, 1fr)` body containing description, cleanup consequences, warning,
and cascade choice, then the persistent action footer. The surface is capped by
the dynamic viewport; the body owns vertical overflow. Short confirmations
retain intrinsic height, while long task names, bulk executor groups, longer
locales, warnings, and cascade copy remain reachable.

Both footer actions use `min-h-11 w-full` below `sm`, restoring compact
automatic dimensions at `sm`. Delete selects `variant="destructive"` on
`AlertDialogAction` and removes manual color utilities. This matters because
the action is slotted through `Button`: a default wrapper currently contributes
`bg-primary` while the child contributes `bg-destructive`, and stylesheet order
allows the primary color to win. Archive continues to use the default action
variant.

### Compact archive surfaces

`TaskArchiveConfirmation` consumes the same structured cleanup model for its
fine-pointer popover and coarse-pointer inline confirmation. It keeps the
existing `ActionConfirmPopover` and `InlineConfirmActions` components, widths,
touch density, callbacks, and focus-return behavior. Only the internal copy
hierarchy changes: direct archive outcome, ordered effects, then supporting
notes and the existing still-working warning.

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

No API, WebSocket, state, or persisted-data contract changes are required.
Localization catalogs change only for task confirmation presentation; executor
cleanup semantics remain unchanged.

## Control flow

The existing task-level `foregroundActivity` projection and explicit
`isInFlight` props continue to determine whether the warning is rendered. The
dialog computes the localized task outcome and cleanup model during render,
then passes the model to the shared task-local renderer. Archive/delete
callbacks and dialog state remain untouched. The context-menu adapter only
chooses the mounting branch described above based on the existing responsive
pointer classification.

## Failure and recovery

No new runtime failure path exists. If localized text is longer than the
available width, the shared surface-text contract wraps it inside the body. If
content becomes taller than the dynamic viewport, the body scrolls without
moving the title or actions. Unknown executor types retain the generic running
session effect. If no task activity is present, no warning mounts, as before.

## Persistence

None. This is a client-side presentation-only change.

## Observability

Existing component tests continue to assert warning presence and absence for
generating, background, and idle activity. Cleanup-summary tests cover every
executor, bulk grouping, ordering, effects, and supporting notes. Dialog and
compact-surface tests assert equivalent structured content, semantic action
variants, and unchanged callbacks. Rendered desktop and phone checks inspect
computed text wrapping, viewport bounds, scroll ownership, action reachability,
and document overflow.

## Mobile and desktop containment

The desktop check uses the real sidebar task row with a fine pointer. It
records the row `getBoundingClientRect().height` before opening Archive, while
the archive popover is visible, and after Cancel; all three values must remain
stable within subpixel precision. It also verifies the archive popover is
strictly wider than 256px and remains inside the viewport at compact widths.

The phone check keeps the existing coarse-pointer sidebar inline flow. It
expects the inline confirmation to remain intentionally row-owned, keeps
actions at or above 44px, and asserts zero document horizontal overflow.

The task-delete phone check enters through the real task drawer action menu and
opens Delete with a long task title and a longer bundled locale. After portal
animations settle, it verifies the surface's 16px viewport insets, zero
document horizontal overflow, title/body wrapping, body scroll ownership when
needed, and visible persistent actions. Both actions are full-width and at
least 44px high, Delete exposes `data-variant="destructive"`, and Cancel closes
the alert without deleting the task. A desktop check retains compact row
actions and the existing deletion flow.

## Related decisions

- [ADR 0049: Fine-grained foreground-idle busy signal](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md)
- [Mobile task navigation](../requirements/mobile-task-navigation.md)
- [Surface text hierarchy](../requirements/surface-text-hierarchy.md)
