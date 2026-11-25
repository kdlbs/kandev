---
status: active
system: ui
created: 2026-07-30
owners:
  - kandev
---
# Mermaid Rendering Requirements

## Overview

Agent messages and task plans can contain Mermaid diagrams whose labels use ordinary prose punctuation. Users should see those diagrams instead of parser-error toasts when the intended text has an unambiguous Mermaid-safe representation.

## Requirements

### REQ-UI-MERMAID-RENDERING-001: Mermaid Rendering

**Intent:** Agent messages and task plans can contain Mermaid diagrams whose labels use ordinary prose punctuation. Users should see those diagrams instead of parser-error toasts when the intended text has an unambiguous Mermaid-safe representation.

#### Acceptance criteria

- **AC-UI-MERMAID-RENDERING-001.1:** Kandev renders Mermaid sequence-message text containing a literal semicolon as visible message text rather than treating the semicolon as a new statement.
- **AC-UI-MERMAID-RENDERING-001.2:** The normalization applies consistently to chat Markdown and task-plan Mermaid blocks.
- **AC-UI-MERMAID-RENDERING-001.3:** Existing Mermaid entity escapes such as `#59;` remain unchanged.
- **AC-UI-MERMAID-RENDERING-001.4:** Valid semicolons that separate multiple Mermaid statements on one physical line remain statement separators.
- **AC-UI-MERMAID-RENDERING-001.5:** Mermaid diagrams without affected sequence-message text retain their existing source semantics.
- **AC-UI-MERMAID-RENDERING-001.6:** When a non-cancelled Mermaid render rejects, Kandev writes one searchable browser-console error that includes the parser error and the full original diagram source.
- **AC-UI-MERMAID-RENDERING-001.7:** When Kandev normalized the source before the failed render, the console error also includes the full normalized source so users can distinguish invalid input from a normalization defect.
- **AC-UI-MERMAID-RENDERING-001.8:** Failed diagram source remains in the browser console and Kandev's in-memory frontend log buffer; Kandev does not transmit it unless the user explicitly includes browser logs in a diagnostic report.

### REQ-UI-MERMAID-RENDERING-002: Markdown diagram boundaries

**Intent:** A missing separator after a diagram fence must not hide subsequent prose when the intended boundary can be verified.

Supported recovery concerns Markdown code blocks explicitly tagged `mermaid`.
The apparent closing run starts a line, with at most three leading spaces, and directly precedes non-whitespace text.
It contains at least three backticks. The complete diagram must be invalid, and the prefix before that run must be valid.

#### Acceptance criteria

- **AC-UI-MERMAID-RENDERING-002.1:** For supported input, Kandev shall show the diagram and following prose separately. Recovery requires invalid full diagram source and valid diagram source before the apparent closing run.
- **AC-UI-MERMAID-RENDERING-002.2:** Valid Mermaid content shall retain its source semantics, including multiline labels that contain apparent fence text.
- **AC-UI-MERMAID-RENDERING-002.3:** Other languages, enclosing literal fences, and content without an eligible closing run shall retain their existing behavior. An invalid candidate diagram shall retain the existing error fallback.
- **AC-UI-MERMAID-RENDERING-002.4:** Recovery shall leave stored message content, transport chunks, and message identity unchanged.
- **AC-UI-MERMAID-RENDERING-002.5:** The recovered diagram and prose shall appear in the same order on desktop, phone, and a subsequent page reload.
- **AC-UI-MERMAID-RENDERING-002.6:** Superseded or cancelled recovery work shall not publish an earlier diagram or prose. Successful recovery shall not show a transient diagram-error notification from the failed full-source probe.
- **AC-UI-MERMAID-RENDERING-002.7:** Recovered prose shall use the same Markdown formatting, document reference definitions, and task/file-link context as adjacent chat prose. Existing HTML and URL restrictions shall remain effective.

## Migrated source detail

## Why

Agent messages and task plans can contain Mermaid diagrams whose labels use ordinary prose
punctuation. Users should see those diagrams instead of parser-error toasts when the intended text
has an unambiguous Mermaid-safe representation.

## What

- Kandev renders Mermaid sequence-message text containing a literal semicolon as visible message
  text rather than treating the semicolon as a new statement.
- The normalization applies consistently to chat Markdown and task-plan Mermaid blocks.
- Existing Mermaid entity escapes such as `#59;` remain unchanged.
- Valid semicolons that separate multiple Mermaid statements on one physical line remain statement
  separators.
- Mermaid diagrams without affected sequence-message text retain their existing source semantics.
- When a non-cancelled Mermaid render rejects, Kandev writes one searchable browser-console error
  that includes the parser error and the full original diagram source.
- When Kandev normalized the source before the failed render, the console error also includes the
  full normalized source so users can distinguish invalid input from a normalization defect.
- Failed diagram source remains in the browser console and Kandev's in-memory frontend log buffer;
  Kandev does not transmit it unless the user explicitly includes browser logs in a diagnostic
  report.
- The first Mermaid render failure for a focused task shows one error toast. Further Mermaid
  failures for that task do not show another toast during the same frontend runtime, including
  after the user focuses another task and returns.
- Toast suppression is shared across chat and task-plan diagrams. Failures still render their
  inline error state and write full console diagnostics even when their toast is suppressed.
- A Mermaid failure in a different task can show that task's first error toast. A full-page reload
  starts a new frontend runtime and resets the in-memory task-toast history.

## Scenarios

- **GIVEN** a sequence message `A->>B: Locate code; preserve message`, **WHEN** Kandev renders the
  diagram, **THEN** it renders successfully and the label contains `Locate code; preserve message`.
- **GIVEN** the reported sequence diagram containing
  `State->>State: Locate numeric code string; preserve message unchanged`, **WHEN** Kandev
  normalizes it, **THEN** the message semicolon is represented as `#59;` before Mermaid parses the
  source.
- **GIVEN** a sequence message that already contains `#59;`, **WHEN** Kandev normalizes it,
  **THEN** the escape is not changed or double-encoded.
- **GIVEN** two valid sequence statements separated by a semicolon on one physical line, **WHEN**
  Kandev normalizes them, **THEN** the separator remains a separator and both statements retain
  their original meaning.
- **GIVEN** a non-sequence Mermaid diagram containing a literal semicolon, **WHEN** Kandev
  normalizes it, **THEN** sequence-message normalization does not alter that semicolon.
- **GIVEN** a non-cancelled Mermaid render rejects with a parser error, **WHEN** Kandev handles the failure,
  **THEN** the browser console contains one `[mermaid]` error with the parser error and complete
  original diagram source.
- **GIVEN** normalization changed a diagram before a failed render, **WHEN** Kandev logs the
  failure, **THEN** the same console entry also contains the complete normalized diagram supplied
  to Mermaid.
- **GIVEN** a failed diagram is logged, **WHEN** the user does not explicitly export or attach
  browser logs, **THEN** Kandev does not send the diagram source to a backend or telemetry service.
- **GIVEN** multiple diagrams fail in one focused task, **WHEN** their render promises reject,
  **THEN** Kandev shows one Mermaid error toast for the task and logs every rejected render.
- **GIVEN** a task already showed its Mermaid error toast, **WHEN** the user focuses another task
  and later returns to the original task, **THEN** the original task does not show the toast again.
- **GIVEN** one task already showed a Mermaid error toast, **WHEN** a diagram fails in another task,
  **THEN** the second task can show its own first Mermaid error toast.
- **GIVEN** a Mermaid diagram renders outside a task context, **WHEN** it fails, **THEN** Kandev
  preserves the existing non-task toast behavior rather than assigning it to the focused task.

## Out of scope

- General repair of arbitrary invalid Mermaid syntax.
- Reconstruction of native assistant-message boundaries without provider evidence.
- Recovery inside task-plan rich-text nodes or untagged diagram blocks.
- Changes to Mermaid error-toast copy, visual presentation, or diagram controls.
- Changes to layout, responsive composition, or touch interaction.
- Redaction of diagram source from the explicitly requested failure diagnostic.

## Implementation plan

- [Mermaid sequence-message semicolon rendering](../../../plans/mermaid-sequence-semicolon-rendering/plan.md)
- [Cursor Mermaid fence recovery](../../../plans/cursor-mermaid-fence-recovery/plan.md).

The [Markdown boundary design](../system-design/mermaid-markdown-boundaries.md) defines the recovery for `REQ-UI-MERMAID-RENDERING-002`.
