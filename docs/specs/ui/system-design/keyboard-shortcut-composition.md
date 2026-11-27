---
status: current
system: ui
requirements:
  - REQ-UI-KEYBOARD-SHORTCUT-COMPOSITION-001
---

# Keyboard Shortcut Composition System Design

## Purpose and boundaries

The reusable hook boundary in `apps/web/hooks/use-keyboard-shortcut.ts` rejects
composition before shortcut matching or consumption. Task creation remains owned
by its existing setup and submit path. No task, persistence, or plugin contract
is moved into UI.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-KEYBOARD-SHORTCUT-COMPOSITION-001` | Event admission, Consumer wiring, Responsive surfaces, Evidence |

## Event admission

Add one private, stateless predicate to the hook module for
`KeyboardEvent | React.KeyboardEvent`. Normalize React events through
`nativeEvent`; otherwise inspect the native event itself. Return true for
`isComposing` or `keyCode === 229`. Do not use an `instanceof` check, clocks,
composition listeners, retained state, or delayed activation.

Both `useKeyboardShortcut` and `useKeyboardShortcutHandler` return before
`matchesShortcut`, `preventDefault`, `stopPropagation`, or callback execution
when the predicate reports composition. Retain the enabled gate. The native
listener keeps its `defaultPrevented` gate; the returned React handler currently
has no such gate and continues that behavior for ordinary events. Neither path
changes target registration, capture, callback refs, or option defaults.

The matcher remains a key/modifier utility. Its direct callers include terminal,
editor, application, plugin, and integration dispatchers. Moving admission into
the matcher would expand this repair into those independently governed paths.
The hook seam covers both qualified dispatch modes without that expansion or
scattered per-consumer guards.

`KeyboardEvent.isComposing` identifies composition-session events according to
[MDN](https://developer.mozilla.org/en-US/docs/Web/API/KeyboardEvent/isComposing).
The independent 229 fallback follows the existing first-party title editor and
`use-workflow-move-submit.ts`, rather than inferring OS timing.

## Consumer wiring

`useTaskCreateDialogSetup` supplies `handleKeyDown` through
`useDialogSubmitShortcut`, using `SHORTCUTS.SUBMIT` and `guardedHandleSubmit`.
`TaskFormInputs` receives that handler via `task-create-dialog-form-body.tsx`.
Its live `TaskPromptReferenceEditor` forwards a native keyboard event through
the React-typed handler, so admission must support both shapes at runtime.
The submit path remains `useTaskSubmitHandlers` and its injected `createTask`
boundary. Preserve compatibility and workflow override guards.

The regression mounts the real setup and real shortcut hook on an actual input.
It also keeps `useTaskSubmitHandlers` real and captures its injected `createTask`
dependency using controlled valid state and consent. Mock surrounding data and
external side effects only. A submit callback spy alone is insufficient for the
new integration test. Resolve the async submit chain before reading call counts.

## Responsive surfaces

Reuse `TaskCreateDialog` and `MobileTasksCreateDialog`. The shipped phone
full-height dialog and desktop bounded modal keep their field order, primary
actions, scroll ownership, safe-area behavior, and touch controls. The closest
phone exemplar is `mobile-task-create-escape.spec.ts`; it already enters via
the mobile FAB and checks an attached-keyboard event inside the same dialog.
No geometry or responsive primitive changes are required.

## Evidence

Hook tests cover native listener and mounted React handler with cancelable DOM
events. Independently exercise composing/keyCode13 and noncomposing/keyCode229,
Ctrl/Meta, a later ordinary chord, disabled/defaultPrevented behavior, target and
capture registration, and both consumption options. Assert propagation by an
ancestor observer as well as `defaultPrevented` on the captured DOM event.

The real consumer integration captures zero `createTask` calls for composition
and one for ordinary submission. Focused desktop and mobile Playwright coverage
uses disposable fixtures and the actual dialog input and submit wiring, checks
unconsumed event flags, keeps the dialog/text, then submits an ordinary chord and
verifies one persisted task after reload. A phone touch-action control verifies
the unchanged alternate submit path. Arm a live create-request observer before
dispatch and use the repository's negative-assertion dwell for absent requests;
arm the causal response waiter before ordinary submission. No user data is used.

DOM-event injection proves routing of synthetic composition signals. It does
not prove candidate-window behavior or physical OS IME interaction. TipTap's
ProseMirror guard is not a qualified defect and is unchanged.
Browser composition stimuli use Meta on the Linux fixture, with code0 for
`isComposing`, to isolate host admission from ProseMirror's independent
Ctrl+Enter hard-break keymap and Enter fallback. Mounted hook/consumer tests
retain composing/code13 and independent legacy229 coverage. Android also
absorbs code13 before host admission; the phone ordinary-chord control uses a
synthetic event with no legacy code, alongside real visible touch submission.

## Compatibility and operational effects

No new public API, stored state, backend changes, telemetry, settings, copy,
dependencies, or migrations are needed. This is a local correction within an
existing boundary and does not require a new architectural decision.

## Implementation Plans

- [Keyboard shortcut composition](../../../plans/keyboard-shortcut-composition/plan.md)
