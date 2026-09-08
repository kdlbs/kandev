---
status: draft
system: ui
requirements:
  - REQ-UI-SESSION-START-COMPOSER-READINESS-001
---

# Session Start Composer Readiness System Design

## Purpose and boundaries

The UI system owns the distinction between composer editing and submission.
The task system remains the source of session state and message admission.

This design changes no backend contract, WebSocket event, persisted state, or
plugin interface. It changes how the shared composer derives its local gates
from existing session state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-SESSION-START-COMPOSER-READINESS-001` | [Derived readiness](#derived-readiness), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), and [Responsive behavior](#responsive-behavior) |

## Components and responsibilities

- `useSessionState` provides `isStarting` and `isPreparingEnvironment`. The
  existing values cover normal starts, resumes, and executor preparation.
- `useChatInputContainer` derives separate editor and submission gates.
- `ChatInputEditorArea` passes the editor gate to `TipTapInput`.
- `ChatInputEditorArea` applies the submission gate to the send action,
  keyboard submission, and the plugin composer capability.
- `useChatInputState` keeps the draft for the current session. The startup
  transition does not clear or replace that draft.

## Derived readiness

The editor gate excludes session startup. It continues to include the existing
movement, in-flight send, failure, recovery, and executor-unavailable states.

The submission gate includes these states:

- the editor gate
- session startup, unless an interactive clarification owns submission
- an attachment upload that is not complete

The environment-prepare reason remains attached to the submission gate. The
reason describes why the send action is unavailable without disabling the
editor.

The shared toolbar continues to receive the submission gate. As a result, the
send button and plugin `submittable` value remain safe during startup.

## Control flow

1. The task or Quick Chat surface shows a session in startup.
2. `useSessionState` reports `isStarting` to the shared composer.
3. The composer keeps `TipTapInput` editable and blocks submission.
4. The operator enters text. `useChatInputState` stores the draft.
5. The session becomes ready. The composer removes the startup submission
   gate and keeps the same draft.
6. The operator submits the draft through an existing submit path.

## Failure and recovery

If startup fails, the existing failure or recovery gate disables the editor.
The draft remains in composer state for the later recovery path.

If the operator uses a submit shortcut during startup, the guarded submit
handler performs no action. The plugin composer capability returns its existing
non-submittable result.

Interactive clarification submission keeps its current exception. This path
stores a clarification response and does not use regular message admission.

## Responsive behavior

Desktop and mobile surfaces use the same editor and submission gates. This
change does not alter layout, navigation, scrolling, safe-area behavior, or
touch targets.

The nearest mobile exemplar is the shared composer in
`mobile-slash-command-composer.spec.ts`. It uses the same `TipTapInput` and send
button as the desktop task surface.

This change is state normalization inside the shared composer. A focused unit
test and a session-recovery E2E test provide coverage. A separate mobile E2E
test is not necessary because no mobile-specific interaction changes.

## Test strategy

- A hook test proves that startup permits editing and blocks submission.
- The hook test keeps the clarification exception and environment-prepare
  reason under coverage.
- A session-start E2E test uses delayed workspace preparation to keep a real
  session in STARTING. It proves that the editor stays editable, the send
  action stays disabled, and the draft survives until readiness.
- A manual session-recovery E2E test verifies that the recovery card clears and
  the resumed composer remains usable after the prompt-ready response.

## Related decisions

- [ADR-0049: Fine-Grained Foreground Idle Busy Signal](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md)
