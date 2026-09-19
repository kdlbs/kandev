---
status: current
system: ui
requirements:
  - REQ-UI-CLARIFICATION-SUBMIT-FEEDBACK-001
---

# Clarification Submit Feedback System Design

## Purpose and boundaries

The UI system owns the visible in-flight feedback for the shared clarification
overlay. The task and clarification services continue to own the pending
question, response endpoint, winner selection, and resume lifecycle. This
design changes no backend contract, persisted state, or WebSocket event.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-CLARIFICATION-SUBMIT-FEEDBACK-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [Responsive and accessible presentation](#responsive-and-accessible-presentation), and [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `useClarificationGroup` remains the owner of the local `submitState` and the
  single in-flight request guard. It synchronously enters `submitting` before
  awaiting the response.
- `ClarificationInputOverlay` derives `isSubmitting` from that state and passes
  it to the existing shared header and question controls.
- `ClarificationHeaderActions` renders one design-system `Spinner` immediately
  before `ClarificationSkipButton` while `isSubmitting` is true. This position
  exists for both single-question and multi-question bundles, and the spinner
  remains exposed to assistive technology for both bundle types.
- The multi-question Submit button retains its translated pending label and
  disabled behavior, but keeps the normal translated submit label as its
  accessible name while the pending label changes visually. It does not render
  a second spinner, and its idle completion check is absent while the request
  is pending.
- Task chat and Quick Chat continue to compose the same
  `ClarificationPanelSection`, so they receive identical behavior without
  surface-specific state or handlers.

## Data and contracts

No wire or persistence contract changes. The presentation consumes the existing
`SubmitState` values `idle`, `submitting`, `ok`, and `error` through the current
`isSubmitting` boolean.

The header spinner receives the existing translated `task:submitting` value as
its accessible name and remains announced for every bundle. The multi-question
Submit button receives the normal translated `task:submit` value as a stable
accessible name while its visible label changes to the pending value. A stable
`clarification-submitting-status` test identifier anchors component and
Playwright assertions without using translated text as a selector.

## Control flow

1. A single-question option or custom-text action calls
   `submitCollected` immediately, or a fully answered multi-question bundle
   calls it from the explicit Submit action.
2. `runClarificationRequest` sets `submitState` to `submitting` before awaiting
   the response.
3. React re-renders the expanded header with the submitting status before the
   Skip control. Answer controls remain protected by the current disabled and
   in-flight guards.
4. A successful response applies the existing resolved status and closes the
   clarification. An error returns `submitState` to `error`, which removes the
   spinner and restores the existing retry path.

## Responsive and accessible presentation

The existing header composition remains shared across viewport classes. On
fine-pointer desktop, the small non-interactive status joins the compact action
cluster without changing the dimensions of Skip, Collapse, or the
multi-question Submit button. On coarse pointers, the existing 44px Skip and
Collapse targets and the full-width multi-question action row remain intact.
The status occupies its own flex item immediately before Skip and must not
become a touch target.

The design-system spinner keeps `role="status"`, and its accessible name uses
the translated submitting label for every bundle. The multi-question Submit
button keeps a stable accessible name with the normal translated submit label,
so the visible pending-label change does not compete with the header status
announcement. Rendered desktop and Pixel 5 tests verify its order,
containment, and absence of document-level horizontal overflow.

## Failure and recovery

Network and non-success responses continue to produce the existing `error`
state. The header status is conditional only on `submitting`, so it stops
without a separate cleanup path. Existing answer state and retry behavior
remain unchanged.

Collapsing the clarification while a request is pending retains today's compact
header behavior; this design does not project submit state into that collapsed
surface.

## Verification strategy

- A focused component test holds a single-question response and verifies that
  the status appears before Skip, uses the translated accessible label, and is
  removed outside the in-flight state.
- A focused component test verifies that a multi-question status remains
  announced and that its Submit button keeps a stable accessible name while a
  failed request removes the status and re-enables Skip.
- Desktop Playwright coverage holds a single-question response to prove the
  status is visible in the expanded header before the Skip control, then
  releases the response and verifies normal resolution.
- Existing desktop multi-question coverage moves its spinner assertion from
  the Submit button to the shared header while retaining disabled-label and
  idle-check assertions.
- Pixel 5 coverage proves the same single-question feedback, control order,
  touch-target preservation, and lack of horizontal overflow. Existing
  multi-question geometry coverage is updated for the new spinner location.

## Security

The change introduces no new input, authorization decision, data exposure, or
external side effect.
