---
created: 2026-11-26
status: implemented
requirements:
  - REQ-UI-TOAST-INTERACTION-001
system_design:
  - ../../specs/ui/system-design/toast-interaction.md
legacy_specs: []
---

# Fix plan for toast dismissal and modal placement

## Overview

Give each `ToastProvider` toast a dismiss button. When a modal is open, put
the stack in a row inside the active modal. Complete the controls, placement,
and browser coverage in one work order. [Issue #5533](https://github.com/kdlbs/kandev/issues/5533)

The [UI requirement](../../specs/ui/requirements/toast-interaction.md) defines
the shared behavior. The [system design](../../specs/ui/system-design/toast-interaction.md)
defines how the provider and modal primitives share the presentation.

## Evidence

In isolated Chromium at 1600 by 1440 CSS pixels, open a pending question with
images and select **Review images**. Emit a release update and a runtime
update through `system.update_available`. Wait for the toast entry animation.
Both Submit and Close intersect a toast at the higher z-index. The two
toasts have zero buttons. The runtime link cannot take focus because it is
outside the dialog's focus scope.

`ToastList` puts both viewport stacks in the `z-[60]` layer, above the review
dialog at `z-50`. The top stack starts near the header. `ToastStack` omits a
dismiss control even though the provider already has `dismissToast(id)`.

A temporary Pixel 5 layout probe put the same two rows under the review header.
The form and stage had visible space after the stage wrapper could shrink.
The bottom bar fit inside the dialog. The runtime link took focus inside it.
This probe measured placement and focus using a temporary DOM change.

## Scope

The work covers `ToastProvider` consumers and the shared modal primitives
that present them. Question image review is the full-screen reproducer.
The theme and platform notification contracts are linked from the requirement.

## Technical approach

1. Add a localized Dismiss button to `ToastStack`. Call `dismissToast(id)`.
2. Reuse the existing modal mount and unmount lifecycle to choose where to
   render the stack. The last mounted modal outlet takes priority.
3. Place the image review outlet below its header. On phones, let the stage
   give up height to keep the form and bottom bar inside the viewport.

| Toast source | Placement and controls | Verification |
| --- | --- | --- |
| Release and runtime updates via `useUpdateAvailableToast` | Top viewport stack without a modal. Shared in-modal row while a modal is open. Runtime link and Dismiss both present. | Deliver both notifications through the existing WebSocket fixture. |
| Operation feedback via `useToast` | Bottom viewport stack by default. Same in-modal row. Includes loading toasts and callback actions. | Exercise the real provider with controlled timers. |
| Shared primitive outside `ToastProvider` or with modality disabled | The primitive renders its normal content. The viewport owns the toasts if there is no active modal. | Test the outlet's provider and modality boundaries. |

## ASCII UI preview

### UI-01 · Viewport toast

Entry is a toast with an action while the workbench is active. The same row
serves top and bottom placement. The target at right measures at least
44 by 44 CSS pixels on phones and coarse pointers.

```text
+----------------------------------------+
| Runtime update available           [X] |
| Version details                         |
| [Review runtime]                        |
+----------------------------------------+
```

### UI-02 · Desktop image review

Entry is **Review images** with both update toasts active.

```text
Current
+-------------------------------------------------------+
| Review images       Questions                  [toast]|
| Image stage                       Submit and Close ^  |
| Answer form                                           |
+-------------------------------------------------------+

Proposed
+-------------------------------------------------------+
| Review images       Questions        [Submit] [Close] |
|                       Kandev update available     [X]  |
|                       Runtime update  [Review]    [X]  |
| Image stage                                           |
| Answer form                                           |
+-------------------------------------------------------+
```

### UI-03 · Phone image review

Entry is the same review with the full-height phone layout.

```text
+---------------------------------------+
| Review images        [Submit] [Close] |
| Question tabs                         |
| Kandev update available           [X] |
| Runtime update   [Review runtime] [X] |
| Image stage                           |
| Answer form (scrolls)                 |
| [Flip] [Next] [Image pair]            |
+---------------------------------------+
```

The modal region sits below the header in both layouts. It has a height limit
and its own scrolling when the toasts exceed that limit. Keep the phone bottom
bar and header outside that region. Use existing tokens. Spacing in these
views is illustrative. UI-01 maps to `AC-UI-TOAST-INTERACTION-001.1`,
`AC-UI-TOAST-INTERACTION-001.2`, and `AC-UI-TOAST-INTERACTION-001.5`.
UI-02 and UI-03 map to `AC-UI-TOAST-INTERACTION-001.3` and
`AC-UI-TOAST-INTERACTION-001.4`.

## Tests

Extend `apps/web/components/toast-provider.test.tsx` with two cases. Check
independent dismissal alongside an actionable toast. Check that the same
toast moves through nested modal outlets and expires on its original timer.
These cases cover criteria 001.1, 001.2, 001.4, and 001.5.

## E2E tests

Add **update toasts keep review actions clear** to the desktop and phone
image review specs in `apps/web/e2e/tests/chat/`. Use the existing question
image fixture and update WebSocket fixture. The desktop test fails before
the fix on the intersection between the toast and header controls.

Desktop Chromium covers keyboard access and review Close. The `mobile-chrome`
project covers dismissal by touch, Submit, the size and containment of the
touch target, and the nested drawer. These scenarios cover criteria 001.1
through 001.4.
The work order specifies the assertions and commands.

## Work orders

- [x] [Task 01 · Dismiss toasts and keep modal actions clear](task-01-toast-controls-and-placement.md)

## Verification results

The desktop defect reproduction failed on the original code because an update
toast overlapped a review action. After the fix, the full desktop image review
suite passed (10 tests) and the full mobile review suite passed (5 tests).
Provider coverage passed (9 tests). The recent task switcher suite passed (4
tests) with Enter on Dismiss leaving the switcher open. Type checking, the
production Vite build, localization checks, specification validation,
specification linting, and `git diff --check` passed.

Static review remediation added regression coverage for Enter propagation,
the persistent modal announcement region, the 667 by 390 phone layout, and
`AlertDialogContent asChild` rendering.

## Risks

- Adding a row affects the available space in fixed-height modals. Test the
  full-height review and compact shared modals with visible toasts.
- Moving a toast between modal scopes can affect focus or pointer handling.
  Test activation while the modal is active and focus return when it closes.
- Two live toasts can exceed the space on phones. Check the toast scroll
  region and use the review controls with the stack present.
