---
status: active
system: ui
created: 2026-08-13
updated: 2026-11-03
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
- **AC-UI-CLARIFICATION-CONTEXT-001.10:** Collapse and reopen shall preserve answers, sizing mode, and the Restore target for the current bundle. A new bundle shall start content-sized without a Restore target. Responsive changes shall preserve separate desktop and phone sizing preferences and Restore targets.
- **AC-UI-CLARIFICATION-CONTEXT-001.11:** Keyboard and touch users shall have one visible size toggle with a localized accessible name. Phone and coarse-pointer targets shall measure at least 44px in both dimensions. Context and answers shall remain reachable through one active vertical scroll owner within the bounded clarification surface. Tables shall not cause document horizontal overflow. Safe areas and the software keyboard shall not obstruct required actions.
- **AC-UI-CLARIFICATION-CONTEXT-001.12:** Agent guidance shall recommend 1-3 short sentences or a few brief bullets, aiming for about 100 words or fewer. It shall request self-contained questions and options. Detailed reports, logs, and repeated conversation shall belong in a normal assistant message before the question call. This guidance shall not reject or truncate valid context. The existing user-input barrier shall remain unchanged.
- **AC-UI-CLARIFICATION-CONTEXT-001.13:** Each open pending panel shall show one icon-only size button in a stable header position. Its icon, tooltip, and accessible name shall indicate Expand or Restore, according to its next action. The header shall contain no separate Reset button or persistent size-action text. The size button shall remain distinct from collapse and skip actions.
- **AC-UI-CLARIFICATION-CONTEXT-001.14:** Expand shall remember the current content-sizing mode or manually selected height and fill the available host space. Restore shall return to that remembered mode or height. Repeated cycles shall preserve this behavior without changing answers or sending a response.
- **AC-UI-CLARIFICATION-CONTEXT-001.15:** Host or viewport changes shall bound the rendered panel without discarding its sizing intent or Restore target. Expanded panels shall follow the available host height. Restore shall remain usable even when the host shrinks. With no usable measured headroom, Expand shall remain visible but disabled. A manual height equal to the available maximum shall not imply an earlier Expand action.
- **AC-UI-CLARIFICATION-CONTEXT-001.16:** A manual drag shall select a new normal height and clear an earlier Expand target. The next Expand/Restore cycle shall return to that height. Double-clicking the existing desktop grip shall restore content sizing and clear the current presentation's Expand target. These actions shall preserve answers and the other presentation's size.

## Formatting contract

[Clarification Markdown](clarification-markdown.md) owns supported formatting and link safety.
Pending context uses its context-specific policy. Context-only History summaries retain their existing literal presentation.
Legacy storage remains verbatim, including literal escape sequences. Rendering does not infer escaped newlines.

## Contract amendment

Criteria .2 and .6 adopt Markdown presentation. Criteria .9 through .12 add usability and authoring guidance for issue #4810.
The implementation is recorded in the [context usability plan](../../../plans/clarification-context-usability/plan.md).

The 2026-11-03 amendment replaces the two size controls in .10/.11 and adds .13 through .16.
The [single size control plan](../../../plans/clarification-size-toggle/plan.md) records the delivered implementation.
The remaining contract retains its existing behavior.

## Out of scope

- Question schema shapes, persistence, permissions, answer delivery, and wait semantics.
- Repeating context in each question or adding context to resolved chat messages.
- Hard word limits, automatic summaries, and stored-text rewriting.
- New size menus, size preferences across task navigation or reload, and touch-drag support for the desktop grip.

## Implementation plans

- [Original context rendering](../../../plans/clarification-context-rendering/plan.md).
- [Explicit paragraphs](../../../plans/clarification-context-newlines/plan.md).
- [Context usability](../../../plans/clarification-context-usability/plan.md).
- [Single size control](../../../plans/clarification-size-toggle/plan.md).
