---
status: current
system: ui
requirements:
  - REQ-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001
---

# Command-panel Sidebar Task Reveal System Design

## Purpose and boundaries

This design extends the existing command-panel navigation with desktop sidebar
motion and visual feedback. It does not change route ownership, task search, or
sidebar persistence.

## Requirement mapping

| Requirement                                    | Design section                                                                                                                        |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-COMMAND-PANEL-SIDEBAR-TASK-REVEAL-001` | [Control flow](#control-flow), [Motion and visual feedback](#motion-and-visual-feedback), [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- `use-command-panel-task-navigation.ts` owns the pending selected task ID. It
  starts a reveal only after the route and active-task state match that ID.
- `lib/sidebar/task-navigation.ts` finds the selected row inside a visible
  `task-sidebar-scroll` viewport. It owns bounded retries, latest-request
  cancellation, scroll behavior, and the transient cue.
- `task-item.tsx` exposes the stable task-row DOM attribute used by the helper.
- `globals.css` owns the reveal animation and its reduced-motion variant.

## Control flow

1. Cmd+K selection cancels an earlier reveal, records the selected task ID,
   and starts canonical task navigation.
2. The navigation hook waits until both the route and active-task state match.
3. The reveal helper searches visible sidebar viewports for the matching row.
4. If the row is missing, the helper retries for its bounded animation-frame
   budget. A newer request invalidates the current generation immediately.
5. When the row exists, the helper scrolls it only when it is outside the
   sidebar viewport. It then restarts the transient cue on that row.

A missing row ends as a sidebar no-op. It never blocks or reverses task
navigation.

## Motion and visual feedback

The helper uses `scrollIntoView` with nearest block and inline alignment.
Normal motion uses smooth scrolling. Reduced motion uses immediate scrolling.
The cue uses a dedicated class on the interactive task row and is removed
after a bounded interval. Repeated selection restarts the cue, and a new
selection clears any earlier cue before it marks the new row.

CSS provides an animated emphasis for normal motion. Under
`prefers-reduced-motion: reduce`, the same class provides a static visual
emphasis until cleanup, with no animation. The existing `aria-current` state
remains the durable active-task signal.

## Responsive behavior

The helper ignores viewports and rows hidden by responsive CSS. Desktop uses
the sidebar animation and cue. Phone Cmd+K navigation continues directly to
the task route and uses the existing mobile task and session controls. It does
not open the task-switcher sheet or target hidden desktop markup.

## Failure and recovery

- A filtered or collapsed row exhausts the bounded retry budget and leaves
  sidebar state unchanged.
- Guarded navigation does not consume the reveal budget because the hook does
  not start the helper until navigation settles.
- A superseded generation cannot scroll or mark a stale row.

## Verification design

Vitest covers smooth and reduced-motion scroll options, visible-row behavior,
cue restart and cleanup, latest-request cancellation, hidden viewports, and
bounded failure. The existing sidebar Playwright suite covers command-panel
navigation, nested-viewport containment, the transient cue, and unchanged
document scroll. The mobile command-panel suite protects direct navigation and
the hidden desktop sidebar boundary.
