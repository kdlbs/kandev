---
status: shipped
created: 2026-08-03
owner: product
---

# Walkthrough Feedback Controls

## Why

The walkthrough note form remains available while a walkthrough is open. A
visible **Cancel** action is misleading when that form has no temporary mode to
dismiss and the action cannot change anything.

## What

- A walkthrough note form exposes **Add** and **Run** for saving or immediately
  sending non-empty feedback.
- A persistent walkthrough note form does not expose **Cancel** when there is no
  cancellable editing or disclosure state.
- Desktop floating, phone bottom-sheet, and inline anchored walkthroughs use the
  same feedback action set.
- The walkthrough header's close action and the launcher's discard action remain
  the ways to minimize or remove a walkthrough.
- Comment forms with a real editing or disclosure state retain their working
  **Cancel** action.

## Scenarios

- **GIVEN** an open desktop walkthrough, **WHEN** its note form renders, **THEN**
  **Add** and **Run** are available and no **Cancel** action is shown.
- **GIVEN** an open phone walkthrough bottom sheet, **WHEN** its note form
  renders, **THEN** it exposes the same **Add** and **Run** actions without a
  **Cancel** action.
- **GIVEN** a comment form that is editing or creating a dismissible comment,
  **WHEN** the form renders, **THEN** its existing **Cancel** action remains
  available and invokes that form's cancellation behavior.

## Out of scope

- Changing walkthrough note storage, formatting, queueing, or agent delivery.
- Changing walkthrough navigation, close, discard, layout, or persistence.
- Removing **Cancel** from comment forms that have a real cancellation action.
