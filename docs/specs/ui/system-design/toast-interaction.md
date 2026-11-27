---
status: current
system: ui
requirements:
  - REQ-UI-TOAST-INTERACTION-001
---

# Toast interaction system design

## Purpose and boundaries

This design covers the custom `ToastProvider` used by application feedback
and `useUpdateAvailableToast`. The UI system owns its rendered controls and
placement. The toast theme design describes the separate Sonner renderer.
[Toast theme](toast-theme.md)

## Toast controls and lifetime

`apps/web/components/toast-provider.tsx` owns toast IDs, state, timers,
actions, and diagnostic reports. `ToastStack` renders the shared ghost
`Button` with `size="icon"` beside the content. Use i18next for its accessible
name. Its shared size applies touch dimensions on phones and for coarse
pointers. Only the buttons and action links receive pointer events in the
viewport stacks.

The Dismiss button calls `dismissToast(id)`. The provider continues to run
the timers and action handlers. `ToastStack` renders the same controls in
either location.

Toast controls stop Enter from reaching dialog-level key handlers while
leaving the browser's native button and link activation intact.

`ToastProvider` queues announcements in creation and update order. It clears
the current live-region text before restoring each notice so repeated messages
produce a separate text change. A newer notice cancels and restarts a pending
restore; unmount cancels its timer. The provider keeps one polite `aria-live`
region in the active accessibility scope. It renders outside a modal; the
active modal outlet renders the same region inside the modal focus scope. The
region exists while the queue is empty and updates independently of toast
placement. Visual toast containers do not have live-region semantics.

## Modal placement and focus

Add one optional `ModalToastOutlet` that the shared Dialog, AlertDialog,
Sheet and Drawer use in modal mode. Use the primitive's existing mount and
unmount lifecycle to register and remove its outlet in `ToastProvider`.
The last mounted outlet shows the stack. If the last outlet closes, the
normal viewport stacks show the toasts. The provider keeps the same notice
state, so moving a toast keeps its action and remaining time.

The outlet renders inside the modal's React tree. The existing focus scope
includes the toast controls, and the modal treats a pointer press there as an
inside interaction. Dismiss is a regular `type="button"`. Enter activates
the focused toast control. Preserve polite announcements.
[Radix Dialog](https://www.radix-ui.com/primitives/docs/components/dialog)

### Modal layout

In ordinary modals, the outlet occupies a row after dialog content and actions.
It spans the available columns and collapses when empty. Limit its height and
scroll its own contents on overflow. Keep the modal's persistent actions
outside that scroll region. [Growing dialog content](dialog-content-containment.md)

Question image review supplies its outlet after `ReviewHeader` and before the
stage and form. The same content and action markup serves both layouts. On
desktop, align the region to the right under the header. On phones, place it
in an inset, single-column row below the header. The review viewport already
applies top and bottom safe-area insets. The mobile menu sheet provides the
precedent for an inset section with bounded scrolling. The toast region has
its own scroll only when the stack exceeds its available height. The answer
form retains its existing scroll region. Let the phone stage wrapper shrink
while toasts are present so the answer form and bottom controls fit. The
Pixel 5 layout probe used a height cap of `min(11rem, 22dvh)`.
[Question image review design](../../tasks/system-design/question-image-review.md)

## Requirement mapping

| Criteria                                 | Design section                                              | Evidence                                                                                                        |
| ---------------------------------------- | ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `AC-UI-TOAST-INTERACTION-001.1`, `001.2` | [Toast controls and lifetime](#toast-controls-and-lifetime) | Provider dismissal and action tests. Browser keyboard and phone target measurements.                            |
| `AC-UI-TOAST-INTERACTION-001.3`          | [Modal layout](#modal-layout)                               | Two real update notices during image review on desktop and phone. Compare rendered bounds with review controls. |
| `AC-UI-TOAST-INTERACTION-001.4`          | [Modal placement and focus](#modal-placement-and-focus)     | Nested real primitives, active focus, one instance per toast, and one persistent polite live region.            |
| `AC-UI-TOAST-INTERACTION-001.5`          | [Toast controls and lifetime](#toast-controls-and-lifetime) | Provider lifetime tests with a fake clock.                                                                      |
