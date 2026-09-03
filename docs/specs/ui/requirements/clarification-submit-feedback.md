---
status: active
system: ui
created: 2026-08-20
owners:
  - kandev
---

# Clarification Submit Feedback Requirements

## Overview

The shared clarification overlay must show visible progress whenever an answer
is being submitted. The UI system owns this presentation contract because the
same feedback is used by task chat and Quick Chat without changing the task
system's clarification state or response lifecycle.

## Terminology

- **Header submitting status:** The animated status indicator in the expanded
  clarification header's action cluster.
- **Skip control:** The X-shaped action that rejects the pending clarification.

## Requirements

### REQ-UI-CLARIFICATION-SUBMIT-FEEDBACK-001: Clarification submit feedback

**Intent:** Operators can tell that their answer is being submitted during a
slow response, including single-question flows that have no Submit button.

**User story:** As an operator answering an agent question, I want immediate
submission feedback, so that I know my answer was accepted for processing.

#### Acceptance criteria

- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.1:** When any clarification answer
  request is in flight, the expanded clarification header shall show an
  animated submitting status immediately before the Skip control.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.2:** The header submitting status
  shall appear for option and custom-text answers in both single-question and
  multi-question clarification bundles.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.3:** The header submitting status
  shall carry the existing translated submitting label as its accessible name
  and remain exposed to assistive technology for both single-question and
  multi-question bundles. For multi-question bundles, the Submit button shall
  keep the normal translated submit label as its accessible name while its
  visible label changes to the translated submitting label.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.4:** While a multi-question answer
  request is in flight, the Submit button shall remain disabled, keep the
  normal translated submit label as its accessible name, show its translated
  submitting label visibly, omit its idle completion check, and defer the
  animated status indicator to the header position.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.5:** When no answer request is in
  flight, the header submitting status shall not be visible and the existing
  idle clarification controls shall remain unchanged.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.6:** When submission succeeds, the
  clarification shall resolve through its existing lifecycle and the submitting
  status shall be removed with the overlay.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.7:** When submission fails and the
  clarification remains visible, the submitting status shall stop, and the
  existing answer controls shall become available for retry.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.8:** Task chat and Quick Chat shall
  render the same submitting feedback on fine-pointer desktop and coarse-pointer
  mobile surfaces.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.9:** The submitting status shall not
  overlap the Skip or Collapse controls, cause horizontal overflow, or reduce
  the existing coarse-pointer touch targets.
- **AC-UI-CLARIFICATION-SUBMIT-FEEDBACK-001.10:** Fine-pointer desktop surfaces
  shall retain the existing compact clarification header dimensions and control
  order apart from the transient status inserted before Skip.

## Out of scope

- Changing the clarification response API, submission lifecycle, or duplicate
  request guard.
- Adding new user-facing copy or changing the existing translated submitting
  label.
- Adding a Submit button to single-question clarifications.
- Changing custom-answer Send, Skip, Collapse, or collapsed-header semantics.
- Adding submission feedback to the collapsed clarification header.
