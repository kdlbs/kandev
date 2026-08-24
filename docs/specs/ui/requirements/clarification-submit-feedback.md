---
status: active
system: ui
created: 2026-08-20
owners:
  - kandev
---
# Clarification submit feedback Requirements

## Overview

When a multi-question clarification is submitted, the chat button changes to `Submitting...` and becomes disabled, but its completion check icon remains. The button needs a clear in-flight signal while the response is pending.

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

## Migrated source detail

## Why

When a multi-question clarification is submitted, the chat button changes to
`Submitting...` and becomes disabled, but its completion check icon remains.
The button needs a clear in-flight signal while the response is pending.

## What

- The shared clarification overlay keeps the translated `Submitting...` label
  while its answer request is in flight.
- While submitting, the multi-question Submit button is disabled and shows the
  shared animated loading spinner in place of the completion check icon.
- When the overlay is idle and answerable, the button keeps its existing
  translated Submit label and completion check icon.
- The same behavior applies to task chat and Quick Chat on desktop and mobile,
  because they share the clarification overlay header.
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
  remains visible inside the reachable Submit button without changing the
  mobile layout or introducing horizontal overflow.

## Out of scope

- Changing the clarification response API, submission lifecycle, or duplicate
  request guard.
- Changing the existing `Submitting...` translation or adding new copy.
- Adding a header Submit button to single-question clarifications.
- Changing custom-answer Send behavior, skip behavior, button placement, or
  other loading states.
