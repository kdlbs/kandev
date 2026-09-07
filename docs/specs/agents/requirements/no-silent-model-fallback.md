---
status: active
system: agents
created: 2026-08-23
owners:
  - kandev
---

# No Silent Model Fallback Requirements

## Overview

Kandev applies an agent profile model against the model catalog from the
selected executor. A different effective model must be deterministic and
visible to the user. The agent system owns this contract because it owns
profiles, ACP session startup, and model-selection warnings.

## Terminology

- **Requested model:** The model ID stored in the agent profile.
- **Explicit fallback:** The optional fallback model ID stored in the profile.
- **Bare model ID:** A model ID that contains no `[` or `]` character.
- **Model variation:** An advertised model ID with the exact form
  `<bare-model-id>[<non-empty-variant>]`. The variant contains no bracket
  character. Its content is otherwise opaque.
- **Unique variation:** The only distinct advertised variation for a requested
  bare model ID.

## Requirements

### REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001: Use executor-authoritative model selection

**Intent:** Keep task launch operational without hiding a difference between
the saved model and the effective executor model.

**User story:** As a user, I want Kandev to use the selected executor's model
catalog and report any fallback, so that I can understand which model runs.

#### Acceptance criteria

- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1:** When the executor advertises
  the requested model, the system shall apply that exact model.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.2:** When the requested model is
  absent and the executor advertises the configured explicit fallback, the
  system shall apply the explicit fallback.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.3:** When no permitted advertised
  model applies, the system shall make no speculative model-selection call and
  shall continue with the provider current or default model.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.4:** When the effective model differs
  from the requested model, the system shall persist one structured warning
  that identifies the requested and effective models when known.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.5:** The system shall not rewrite the
  saved profile model from an executor model-selection decision.

### REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002: Resolve one advertised model variation

**Intent:** Preserve a user's bare model choice when a CLI replaces that model
ID with one unambiguous bracketed variation.

**User story:** As a user, I want `opus` to resolve to `opus[1m]` when that is
the only advertised variation, so that a provider catalog change does not move
my session to an unrelated default model.

#### Acceptance criteria

- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.1:** When automatic fallback is
  disabled, the exact requested model is absent, and an advertised explicit
  fallback exists, the system shall apply the explicit fallback before it
  considers an inferred variation. Automatic-fallback profiles shall retain
  their legacy no-selection behavior for an absent requested model.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.2:** When no advertised explicit
  fallback exists and the catalog contains exactly one distinct variation of a
  requested bare model ID, the system shall apply that advertised variation.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.3:** When the catalog contains zero
  or more than one distinct variation, the system shall not infer a model.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.4:** When the requested ID contains a
  bracket, the system shall treat it as an exact ID and shall not infer another
  variation.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.5:** Matching shall be case-sensitive
  and shall not parse, rank, or assign meaning to variation text.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.6:** When Kandev applies a unique
  variation, it shall keep the saved model unchanged and shall persist a
  warning that identifies the inferred effective model.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.7:** The host profile probe shall
  present unique-variation resolution as an advisory. It shall not become the
  launch authority or disable the profile.
- **AC-AGENTS-NO-SILENT-MODEL-FALLBACK-002.8:** The same resolution order shall
  apply at initial launch, context reset, and workspace rebind.

## Out of scope

- Ranking multiple variations by context size, speed, price, or display order.
- Parsing provider-specific variation syntax beyond the bracketed ID shape.
- Inferring a base model from an already bracketed request.
- Rewriting the profile model to the current executor's advertised variation.
- Changing Office post-start provider routing or mid-turn model switching.

## System design

The technical source is split into [part 1](../system-design/no-silent-model-fallback-01.md)
and [part 2](../system-design/no-silent-model-fallback-02.md).
