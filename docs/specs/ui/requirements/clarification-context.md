---
status: active
system: ui
created: 2026-08-13
owners:
  - cfl
---
# Clarification Shared Context Requirements

## Overview

Operators need shared decision context that remains readable while they answer a clarification.

## Requirements

### REQ-UI-CLARIFICATION-CONTEXT-001: Clarification Shared Context

**Intent:** Operators need shared decision context that remains readable while they answer a clarification.

#### Acceptance criteria

- **AC-UI-CLARIFICATION-CONTEXT-001.1:** A pending clarification bundle with non-empty shared context displays that context once above the active question card.
- **AC-UI-CLARIFICATION-CONTEXT-001.2:** Shared context uses lightweight Markdown presentation with spacing above it. It has no bordered, padded, or filled container.
- **AC-UI-CLARIFICATION-CONTEXT-001.3:** Shared context remains visible while the operator moves between questions in a multi-question bundle.
- **AC-UI-CLARIFICATION-CONTEXT-001.4:** Task chat, Quick Chat, and other surfaces that use the shared clarification overlay display the same context behavior on desktop and mobile.
- **AC-UI-CLARIFICATION-CONTEXT-001.5:** Omitted or whitespace-only context does not create an empty context region.
- **AC-UI-CLARIFICATION-CONTEXT-001.6:** The UI preserves paragraph boundaries and ordinary line breaks while rendering supported Markdown. Long text wraps within the clarification surface.
- **AC-UI-CLARIFICATION-CONTEXT-001.7:** Agents can provide multiline shared background as explicit context paragraphs. The backend joins those paragraphs with blank lines while preserving the legacy single context string verbatim.
- **AC-UI-CLARIFICATION-CONTEXT-001.8:** **GIVEN** an agent submits a multi-question clarification with non-empty shared context, **WHEN** the pending clarification appears, **THEN** the operator sees the context exactly once above the active question card.

- **AC-UI-CLARIFICATION-CONTEXT-001.9:** Initial panels shall remain content-sized with the existing caps: 50% for task chat and Needs You, 35% for Quick Chat. User expansion shall exceed the initial cap when host space permits. Downward dragging shall shrink the panel. Host and viewport changes shall keep the panel and required controls reachable.
- **AC-UI-CLARIFICATION-CONTEXT-001.10:** Collapse and reopen shall preserve answers and the selected height for the current bundle. A new bundle and explicit reset shall restore content sizing. Responsive changes shall not overwrite a desktop height preference.
- **AC-UI-CLARIFICATION-CONTEXT-001.11:** Keyboard and touch users shall have visible expansion and reset controls. Phone targets shall measure at least 44px. Context and answers shall remain reachable through one active vertical scroll owner within the bounded clarification surface. Tables shall not cause document horizontal overflow. Safe areas and the software keyboard shall not obstruct required actions.
- **AC-UI-CLARIFICATION-CONTEXT-001.12:** Agent guidance shall recommend 1-3 short sentences or a few brief bullets, aiming for about 100 words or fewer. It shall request self-contained questions and options. Detailed reports, logs, and repeated conversation shall belong in a normal assistant message before the question call. This guidance shall not reject or truncate valid context. The existing user-input barrier shall remain unchanged.

## Formatting contract

[Clarification Markdown](clarification-markdown.md) owns supported formatting and link safety.
Pending context uses its context-specific policy. Context-only History summaries retain their existing literal presentation.
Legacy storage remains verbatim, including literal escape sequences. Rendering does not infer escaped newlines.

## Contract amendment

Criteria .2 and .6 adopt Markdown presentation. Criteria .9 through .12 add usability and authoring guidance for issue #4810.
The implementation is recorded in the [context usability plan](../../../plans/clarification-context-usability/plan.md).

## Out of scope

- Question schema shapes, persistence, permissions, answer delivery, and wait semantics.
- Repeating context in each question or adding context to resolved chat messages.
- Hard word limits, automatic summaries, and stored-text rewriting.

## Implementation plans

- [Original context rendering](../../../plans/clarification-context-rendering/plan.md).
- [Explicit paragraphs](../../../plans/clarification-context-newlines/plan.md).
- [Context usability](../../../plans/clarification-context-usability/plan.md).
