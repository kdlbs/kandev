---
status: active
system: ui
created: 2026-08-20
owners:
  - kandev
---
# Clarification submit feedback Requirements

## Overview

When a multi-question clarification is submitted, the chat button changes to `Submitting...` and becomes disabled, but its completion check icon remains. The button needs a clear in-flight signal while the response is pending. On phone viewports, progress and batch actions also need distinct rows so the pending action does not collide with the stepper or fall below standard touch-target sizing.

## Requirements

### REQ-UI-CLARIFICATION-SUBMIT-FEEDBACK-001: Clarification submit feedback

**Intent:** When a multi-question clarification is submitted, the chat button changes to `Submitting...` and becomes disabled, but its completion check icon remains. The button needs a clear in-flight signal while the response is pending.

#### Acceptance criteria

- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.1:** The shared clarification overlay keeps the translated `Submitting...` label while its answer request is in flight.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.2:** While submitting, the multi-question Submit button is disabled and shows the shared animated loading spinner in place of the completion check icon.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.3:** When the overlay is idle and answerable, the button keeps its existing translated Submit label and completion check icon.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.4:** The same behavior applies to task chat and Quick Chat on desktop and mobile, because they share the clarification overlay header.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.5:** If submission fails and the clarification remains visible, the spinner stops, the button returns to its existing retryable Submit state, and no new error behavior is introduced.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.6:** **GIVEN** every question in a multi-question clarification has an answer, **WHEN** the operator submits the answers and the response request remains pending, **THEN** the Submit button is disabled, retains the `Submitting...` label, shows an animated loading status, and does not show the completion check icon.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.7:** **GIVEN** a pending submission succeeds, **WHEN** the response is accepted, **THEN** the clarification resolves as it does today and the submitting indicator is no longer rendered.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.8:** **GIVEN** a pending submission fails, **WHEN** the response returns an error, **THEN** the clarification remains available, the spinner is removed, and the existing Submit action is available for retry.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.9:** On coarse-pointer viewports, the multi-question Submit button remains at least 44px tall in both idle and submitting states, retains its height between those states, and centers its label and status icon.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.10:** On phone viewports, the stepper and answered count occupy a progress row above a distinct full-width action row; the Submit action uses the available width, Skip and Collapse remain at least 44px in each dimension, and no action overlaps or causes horizontal overflow.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.11:** Fine-pointer desktop viewports retain the compact inline header layout and control dimensions.

## Migrated source detail

## Why

When a multi-question clarification is submitted, the chat button changes to
`Submitting...` and becomes disabled, but its completion check icon remains.
The button needs a clear in-flight signal while the response is pending.

On a touch viewport, that pending-state control can be only about 24px tall
beside the 44px clarification controls. Keeping progress and actions on the
same row also crowds the loading label, spinner, Skip, and Collapse controls.

## What

- The shared clarification overlay keeps the translated `Submitting...` label
  while its answer request is in flight.
- While submitting, the multi-question Submit button is disabled and shows the
  shared animated loading spinner in place of the completion check icon.
- When the overlay is idle and answerable, the button keeps its existing
  translated Submit label and completion check icon.
- The same behavior applies to task chat and Quick Chat on desktop and mobile,
  because they share the clarification overlay header.
- On coarse-pointer viewports, the multi-question Submit button remains at
  least 44px tall in both its idle and submitting states, with its label and
  status icon centered inside the control.
- On phone viewports, progress stays on a row above the full-width batch action
  row. Skip and Collapse retain 44px touch targets without crowding Submit.
- Fine-pointer desktop viewports keep the compact inline header dimensions.
- If submission fails and the clarification remains visible, the spinner stops,
  the button returns to its existing retryable Submit state, and no new error
  behavior is introduced.

## Scenarios

- **GIVEN** every question in a multi-question clarification has an answer,
  **WHEN** the operator submits the answers and the response request remains
  pending, **THEN** the Submit button is disabled, retains the `Submitting...`
  label, shows an animated loading status, and does not show the completion
  check icon.
- **GIVEN** a pending submission succeeds, **WHEN** the response is accepted,
  **THEN** the clarification resolves as it does today and the submitting
  indicator is no longer rendered.
- **GIVEN** a pending submission fails, **WHEN** the response returns an error,
  **THEN** the clarification remains available, the spinner is removed, and
  the existing Submit action is available for retry.
- **GIVEN** the same multi-question clarification is rendered in a phone
  viewport, **WHEN** its answer request is pending, **THEN** the loading status
  remains visible inside a Submit button that is at least 44px tall, retains
  its idle-state height, and occupies the action row below progress without
  overlap or horizontal overflow.
- **GIVEN** the clarification is rendered on a fine-pointer desktop viewport,
  **WHEN** Submit changes between idle and pending states, **THEN** its compact
  inline dimensions and behavior remain unchanged.

## Out of scope

- Changing the clarification response API, submission lifecycle, or duplicate
  request guard.
- Changing the existing `Submitting...` translation or adding new copy.
- Adding a header Submit button to single-question clarifications.
- Changing custom-answer Send behavior, Skip or Collapse semantics, or other
  loading states.
