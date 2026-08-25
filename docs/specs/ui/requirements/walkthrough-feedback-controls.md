---
status: active
system: ui
created: 2026-08-03
owners:
  - product
---
# Walkthrough Feedback Controls Requirements

## Overview

The walkthrough note form remains available while a walkthrough is open. A visible **Cancel** action is misleading when that form has no temporary mode to dismiss and the action cannot change anything.

## Requirements

### REQ-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001: Walkthrough Feedback Controls

**Intent:** The walkthrough note form remains available while a walkthrough is open. A visible **Cancel** action is misleading when that form has no temporary mode to dismiss and the action cannot change anything.

#### Acceptance criteria

- **AC-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001.1:** A walkthrough note form exposes **Add** and **Run** for saving or immediately sending non-empty feedback.
- **AC-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001.2:** A persistent walkthrough note form omits the optional `onCancel` capability, so it does not expose **Cancel**.
- **AC-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001.3:** Desktop floating and phone bottom-sheet walkthroughs use the same feedback action set.
- **AC-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001.4:** The walkthrough header's close action and the launcher's discard action remain the ways to minimize or remove a walkthrough.
- **AC-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001.5:** **GIVEN** an open desktop walkthrough, **WHEN** its note form renders, **THEN** **Add** and **Run** are available and no **Cancel** action is shown.
- **AC-UI-WALKTHROUGH-FEEDBACK-CONTROLS-001.6:** **GIVEN** an open phone walkthrough bottom sheet, **WHEN** its note form renders, **THEN** it exposes the same **Add** and **Run** actions without a **Cancel** action.

## Migrated source detail

## Why

The walkthrough note form remains available while a walkthrough is open. A
visible **Cancel** action is misleading when that form has no temporary mode to
dismiss and the action cannot change anything.

## What

- A walkthrough note form exposes **Add** and **Run** for saving or immediately
  sending non-empty feedback.
- A persistent walkthrough note form omits the optional `onCancel` capability,
  so it does not expose **Cancel**.
- Desktop floating and phone bottom-sheet walkthroughs use the same feedback
  action set.
- The walkthrough header's close action and the launcher's discard action remain
  the ways to minimize or remove a walkthrough.

## Scenarios

- **GIVEN** an open desktop walkthrough, **WHEN** its note form renders, **THEN**
  **Add** and **Run** are available and no **Cancel** action is shown.
- **GIVEN** an open phone walkthrough bottom sheet, **WHEN** its note form
  renders, **THEN** it exposes the same **Add** and **Run** actions without a
  **Cancel** action.

## Out of scope

- Changing walkthrough note storage, formatting, queueing, or agent delivery.
- Changing walkthrough navigation, close, discard, layout, or persistence.
- Changing cancellation outcomes or separately validating runtime cancellation
  in comment forms outside the walkthrough.
- Separately validating the legacy inline anchored walkthrough; it shares
  `WalkthroughStepInner` and inherits the same action set.
