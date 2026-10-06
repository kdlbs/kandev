---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-TITLE-OVERFLOW-001
---

# Sidebar Title Overflow System Design

## Purpose and boundaries

Apply a conditional text mask within `TaskItemTitle` in `apps/web/components/task/task-item.tsx`.
The shared `ScrollOnOverflow` primitive already clips a non-wrapping inner span and scrolls it on hover.
This design keeps that primitive unchanged and scopes the new presentation to sidebar task titles.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-SIDEBAR-TITLE-OVERFLOW-001` | [Overflow detection](#overflow-detection), [Text mask](#text-mask), [Responsive and accessible behavior](#responsive-and-accessible-behavior) |

## Overflow detection

Reuse `useIsTitleTruncated<HTMLSpanElement>(title)` from `apps/web/hooks/use-is-title-truncated.ts`.
Attach its ref to the outer `ScrollOnOverflow` span, which forwards refs.
Its geometry check and `ResizeObserver` update clipping state after title changes and width changes.
The inner non-wrapping span contributes its complete text width to the outer span's scroll width.
Browser verification must establish this behavior for the real flex row.

Use a scoped class and clipping attribute on the title span. Do not add another observer, store field, or preference.
Keep the intrinsic title width and `min-w-0` behavior so short titles retain adjacent badges.

## Text mask

In `apps/web/app/globals.css`, apply `mask-image` and `-webkit-mask-image` only when that title span is clipped.
Use a horizontal alpha gradient from opaque to transparent across at most the final 16 CSS pixels or one-third of the title width, whichever is smaller.
Use the title's full width for the mask and do not mask its containing row.
This keeps a narrow title from losing most of its visible characters.

Remove the mask while a fine pointer hovers directly over the title, including when that pointer is secondary on a hybrid device.
The existing scroll animation then reveals the ending without masking its final characters.
On pointer leave, the primitive resets its transform and the idle mask returns.
Coarse-only interaction retains the idle fade and existing tap navigation.

In forced-colors mode, remove both mask properties after the hover rules so title text remains visible even while hovered.

An alpha mask follows the actual row background in every theme and selection state.
If masks are unsupported, the existing hard clipping remains functional.
No overlay, pointer interception, ellipsis, or new motion is introduced.

## Responsive and accessible behavior

`MobileTaskList` composes the shared `TaskSwitcher`, which renders `TaskItem` rows.
The nearest mobile exemplar is `components/task/mobile/session-task-switcher-sheet.tsx`: an inset task picker with a scrolling list.
The same title mask applies in the phone picker and shared navigation task list.
Their scroll owner, safe areas, primary row tap, and visible action targets remain unchanged.
The complete string remains in the DOM. No text replacement or accessibility hiding is needed.

## Verification

Component tests cover clipped and fitting titles, resize changes, text changes, and complete DOM text.
Existing hook tests remain the evidence for observer lifecycle and content recomputation.
Desktop browser checks prove real overflow, title-only masking, forced-colors behavior, hover disclosure, and stable badge geometry.
Phone checks prove the same fade in the task picker, visible touch actions, task navigation, and viewport containment.
Capture light and dark rows in default, selected, and hovered states. Check the capped fade at a narrow available title width.

## Related designs

- [Sidebar task row presentation](sidebar-task-row-presentation.md).
- [Unified mobile navigation](unified-mobile-navigation.md).
