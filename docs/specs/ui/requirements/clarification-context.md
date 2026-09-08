---
status: active
system: ui
created: 2026-08-13
owners:
  - cfl
---
# Clarification Shared Context Requirements

## Overview

Agents can provide shared background with `ask_user_question_kandev.context`, but operators currently see only the individual question fields. Questions that depend on that background can therefore appear incomplete or confusing.

## Requirements

### REQ-UI-CLARIFICATION-CONTEXT-001: Clarification Shared Context

**Intent:** Agents can provide shared background with `ask_user_question_kandev.context`, but operators currently see only the individual question fields. Questions that depend on that background can therefore appear incomplete or confusing.

#### Acceptance criteria

- **AC-UI-CLARIFICATION-CONTEXT-001.1:** A pending clarification bundle with non-empty shared context displays that context once above the active question card.
- **AC-UI-CLARIFICATION-CONTEXT-001.2:** Shared context uses normal text presentation with spacing above it. It has no bordered, padded, or filled container.
- **AC-UI-CLARIFICATION-CONTEXT-001.3:** Shared context remains visible while the operator moves between questions in a multi-question bundle.
- **AC-UI-CLARIFICATION-CONTEXT-001.4:** Task chat, Quick Chat, and other surfaces that use the shared clarification overlay display the same context behavior on desktop and mobile.
- **AC-UI-CLARIFICATION-CONTEXT-001.5:** Omitted or whitespace-only context does not create an empty context region.
- **AC-UI-CLARIFICATION-CONTEXT-001.6:** The UI preserves line breaks in the agent-authored context and allows long text to wrap within the clarification surface.
- **AC-UI-CLARIFICATION-CONTEXT-001.7:** Agents can provide multiline shared background as explicit context paragraphs. The backend joins those paragraphs with blank lines while preserving the legacy single context string verbatim.
- **AC-UI-CLARIFICATION-CONTEXT-001.8:** **GIVEN** an agent submits a multi-question clarification with non-empty shared context, **WHEN** the pending clarification appears, **THEN** the operator sees the context exactly once above the active question card.

## Migrated source detail

## Why

Agents can provide shared background with `ask_user_question_kandev.context`,
but operators currently see only the individual question fields. Questions
that depend on that background can therefore appear incomplete or confusing.

## What

- A pending clarification bundle with non-empty shared context displays that
  context once above the active question card.
- Shared context uses normal text presentation with spacing above it. It has no
  bordered, padded, or filled container.
- Shared context remains visible while the operator moves between questions in
  a multi-question bundle.
- Task chat, Quick Chat, and other surfaces that use the shared clarification
  overlay display the same context behavior on desktop and mobile.
- Omitted or whitespace-only context does not create an empty context region.
- The UI preserves line breaks in the agent-authored context and allows long
  text to wrap within the clarification surface.
- Agents can provide multiline shared background as explicit context
  paragraphs. The backend joins those paragraphs with blank lines while
  preserving the legacy single context string verbatim.

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
- **GIVEN** an agent submits multiple explicit context paragraphs, **WHEN** the
  clarification is stored and displayed, **THEN** the operator sees those
  paragraphs separated by blank lines.
- **GIVEN** an agent submits a legacy context string containing literal escape
  syntax, **WHEN** the clarification is stored and displayed, **THEN** the
  literal text remains unchanged.

## Out of scope

- Changing the `ask_user_question_kandev` schema or backend message metadata.
- Repeating shared context inside every question card.
- Adding shared context to resolved clarification messages in chat history.
- Changing clarification answer, skip, timeout, or carousel behavior.
