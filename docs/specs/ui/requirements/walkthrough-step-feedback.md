---
status: active
system: ui
created: 2026-10-22
owners:
  - product
---

# Walkthrough Step Feedback Requirements

## Overview

Users read feedback beside the walkthrough step it describes. They open the
composer only when they want to comment. Pending and submitted comments remain
visible after submission.

UI owns this existing walkthrough feedback interaction. Task and agent delivery
systems retain their existing authority. This contract replaces
[REQ-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001](walkthrough-feedback-controls.md).
The previous contract is deprecated.

## Terminology

- **Pending:** saved feedback that Kandev has not accepted for delivery.
- **Submitted:** feedback accepted for direct delivery or queueing. This does
  not mean the agent processed it.
- **Current step:** the selected step in the current walkthrough revision and
  active task session.

## Requirements

### REQ-UI-WALKTHROUGH-STEP-FEEDBACK-001: Feedback beside its step

**Intent:** Keep feedback readable in its original context without resending it.

#### Acceptance criteria

- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.1:** The current step shall display its
  pending and submitted comments below the explanation, in creation order.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.2:** Comments from another task, session,
  walkthrough revision, or step shall not appear under the current step.
  Ambiguous older comments shall remain available through existing pending
  feedback controls without guessed attachment.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.3:** Accepted feedback shall remain visible
  with Submitted status. It shall leave pending context and shall not become
  eligible for submission merely because its history remains visible.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.4:** A rejected submission shall preserve
  the comment as pending feedback. The UI shall report the error without a
  success state or a second copy of the comment.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-001.5:** Saved history shall survive step
  navigation, closing and reopening the walkthrough, and reload within the same
  browser tab. Existing session cleanup shall remove that session's history.

### REQ-UI-WALKTHROUGH-STEP-FEEDBACK-002: Composer opened by the user

**Intent:** Prioritize reading while keeping commenting easy to find.

#### Acceptance criteria

- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.1:** Opening a walkthrough or selecting
  another step shall show a Comment button and no composer. Existing comments
  shall remain visible without activating Comment.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.2:** Activating Comment shall reveal the
  composer for that step and focus its input. Cancel or Escape shall dismiss
  the composer and return focus to Comment without closing the walkthrough.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.3:** Add shall save one pending comment.
  Run shall save and submit one comment through the existing delivery flow.
  Successful saving shall clear and collapse the composer. Empty input and a
  missing active session shall not create a comment or lose entered text.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.4:** Desktop, inline, and phone surfaces
  shall provide the same comment actions. The explanation and comments shall
  share one scroll region, with navigation outside that region. Phone actions
  shall have at least 44px touch targets and remain within the viewport.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.5:** New labels, statuses, and accessible
  names shall support all shipped locales. The controls shall support keyboard
  use and reduced motion without relying on hover or animation for meaning.

- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.6:** The floating walkthrough shall fit
  its content on desktop and phone. It shall grow when comments or the composer
  require space and shrink when they require less space. Limit its height to
  the available viewport. Long content shall scroll while actions remain visible.

## Scope limits

History retains the current browser-tab and active-session scope. It is not a
shared server-side discussion or a cross-device record. Already removed sent
comments cannot be recovered. Submitted history is read-only in this change.

This package does not add replies, resolution, or server persistence for drafts.
Unsaved drafts survive navigation, composer dismissal, closing/reopening the
walkthrough, and viewport changes within this page lifetime. Reload may clear
unsaved drafts. Saved comments retain the persistence contract above.

### REQ-UI-WALKTHROUGH-STEP-FEEDBACK-003: Navigate feedback without losing drafts

**Intent:** Find commented steps and resume an unfinished comment.

#### Acceptance criteria

- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.1:** Each step shall retain its unsaved
  draft while users navigate, dismiss the composer, reopen the walkthrough, or
  change viewport size. Comment shall reopen that step's draft. Saving clears
  only the saved draft. Different tasks, sessions, and revisions remain isolated.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.2:** The current step and each picker
  option shall show the number of matching saved comments, including pending
  and submitted comments. Unsaved drafts shall not affect counts.
- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.3:** A labelled step picker shall show
  ordered step titles and counts, identify the active step, and navigate directly
  to a selected step. Untitled steps shall use a localized step-number fallback.
  Desktop keyboard users and phone touch users shall have equivalent access.

- **AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.4:** The step picker shall use the shared
  Kandev dropdown on desktop and mobile picker sheet on phones or coarse pointers.
  It shall support keyboard selection, selected-step indication,
  localized titles and counts, and touch targets of at least 44px on phones.
  The mobile picker shall stay open through viewport-height changes. One tap on
  an option shall select that step and close the picker.
  Opening, selecting, and immediately reopening shall not require repeated taps.

## Implementation plans

- [Walkthrough step feedback](../../../plans/walkthrough-step-feedback/plan.md)
