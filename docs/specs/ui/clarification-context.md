---
status: shipped
created: 2026-08-13
owner: cfl
---

# Clarification Shared Context

## Why

Agents can provide shared background with `ask_user_question_kandev.context`,
but operators currently see only the individual question fields. Questions
that depend on that background can therefore appear incomplete or confusing.

## What

- A pending clarification bundle with non-empty shared context displays that
  context once above the active question card.
- Shared context remains visible while the operator moves between questions in
  a multi-question bundle.
- Task chat, Quick Chat, and other surfaces that use the shared clarification
  overlay display the same context behavior on desktop and mobile.
- Omitted or whitespace-only context does not create an empty context region.
- The UI preserves line breaks in the agent-authored context and allows long
  text to wrap within the clarification surface.

## Scenarios

- **GIVEN** an agent submits a multi-question clarification with non-empty
  shared context, **WHEN** the pending clarification appears, **THEN** the
  operator sees the context exactly once above the active question card.
- **GIVEN** a visible multi-question clarification with shared context,
  **WHEN** the operator navigates to another question, **THEN** the same single
  context region remains visible.
- **GIVEN** an agent omits context or sends only whitespace, **WHEN** the
  pending clarification appears, **THEN** the UI renders no context region and
  preserves the existing question layout.
- **GIVEN** a pending clarification with shared context on a phone viewport,
  **WHEN** the operator views the question, **THEN** the context is visible,
  wraps inside the overlay, and does not introduce horizontal page overflow.

## Out of scope

- Changing the `ask_user_question_kandev` schema or backend message metadata.
- Repeating shared context inside every question card.
- Adding shared context to resolved clarification messages in chat history.
- Changing clarification answer, skip, timeout, or carousel behavior.
