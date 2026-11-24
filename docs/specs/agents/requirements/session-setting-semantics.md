---
status: draft
system: agents
created: 2026-11-17
owners:
  - Kandev
---

# Session setting semantics

## Overview

Agents owns the meaning of provider settings, their saved intent, and their application before prompts.
This contract extends [dynamic provider options](dynamic-provider-options.md) across startup and recovery.
It distinguishes thinking levels from the [permission control contract](permission-control-integrity.md).
The existing dynamic-provider requirement is near its document size limit; this sibling owns setting meaning and compatibility across session lifecycles.

## Terminology

- **Thinking level:** A provider option controlling reasoning effort. It does not grant tool or workspace permissions.
- **Provider default:** No explicit profile override for the setting. It is distinct from an explicit `off` value.
- **Legacy alias:** An older saved session-mode value that denotes the same thinking setting.

## Requirements

### REQ-AGENTS-SESSION-SETTING-SEMANTICS-001: Typed session settings and saved intent

**Intent:** Users can select supported thinking settings and recover from stale choices without changing permission policy or model identity.

#### Acceptance criteria

- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.1:** Pi thinking controls and startup failures shall identify the setting as a thinking level. Changing thinking shall not change automatic approval, tool permissions, or workspace access policy.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.2:** After a model change, settings editors and live sessions shall use the complete authoritative option snapshot for that selected model. Old-model or old-session results shall not replace it. Missing capability evidence shall remain unknown rather than appear as a supported choice.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.3:** Startup shall select the required model before validating and applying an explicit thinking level. An unsupported, refused, mismatched, or unconfirmed explicit choice shall hold the prompt and expose a thinking-specific reason. It shall not silently select another model or thinking level.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.4:** A profile shall offer Use provider default, including when the model advertises only off. Saving that choice shall remove both its explicit thinking value and any equivalent legacy alias. A subsequent fresh session without a workflow or session override shall send no thinking-setting mutation. It shall retain the exact selected model.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.5:** Existing Pi profiles, workflow overrides, and saved session overrides shall retain their thinking intent. A canonical thinking choice shall win over an equivalent legacy alias within the same source. Existing precedence between sources shall remain unchanged. Reading or opening a record shall not rewrite it.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.6:** Unsupported saved choices shall remain visible with their source and an explanation. Recovery shall expose an explicit route to edit that source. Clearing a profile choice shall not claim to clear a stronger session or workflow override, resend a prompt, or repair a running session.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.7:** Profile Save, reload, duplication, workflow override changes, ordinary resume, and context reset shall preserve the distinction between an absent override and an explicit value. Explicit provider-restored recovery shall retain its existing exception and shall not reapply saved thinking intent.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.8:** Real permission settings shall retain their existing refusal, mismatch, confirmation, and prompt-admission guarantees. Unknown or ambiguous legacy mode semantics shall not grant permission or be relabelled as thinking from their display text.
- **AC-AGENTS-SESSION-SETTING-SEMANTICS-001.9:** Desktop and phone shall expose equivalent localized selection and recovery actions. Phone choices shall use one touch-accessible sheet with visible selected/default/unavailable states, focus return, and no horizontal document overflow. Saving remains an explicit action.

## Out of scope

- Changing pi-cursor-agent metadata, Cursor model mappings, or upstream packages.
- Diagnosing or masking empty provider responses.
- Replacing current live-session reset or workflow inheritance policies with a new default policy.
- Automatically rewriting existing databases, global Pi settings, or historical failure records.
