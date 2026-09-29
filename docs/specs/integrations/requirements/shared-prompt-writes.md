---
status: active
system: integrations
created: 2026-09-29
owners:
  - kandev
---

# Shared prompt writes over MCP

Configuration assistants need to apply agreed shared-prompt changes before saving
workflow steps that reference them. The integration system owns this MCP contract;
the prompt service remains the source of saved content and reference expansion.

## Requirements

### REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001: Controlled shared prompt writes

- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.1:** Configuration and external MCP clients shall expose `create_shared_prompt_kandev` and `update_shared_prompt_kandev`, each accepting name and content. Task, Office, and automation catalogs shall exclude them.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.2:** Creation shall reject an existing exact name, including a built-in name, without changing its content. Names and content shall use existing saved-prompt validation and whitespace normalization.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.3:** Updates shall resolve an exact case-sensitive name, reject missing prompts, and replace only content. Successful write results and subsequent reads shall expose saved content and agent-edit permission.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.4:** Built-in prompts and custom prompts without agent-edit permission shall reject agent updates, including through generic settings MCP. Denial shall leave content and permission unchanged and explain how an operator can enable edits for a custom prompt.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.5:** Existing prompts and custom prompts created in Settings shall default to disallowing agent edits. Prompts created through MCP shall allow subsequent agent edits. Only an operator with configuration-management authority shall change that permission in Settings, on desktop or phone.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.6:** Writes shall require the authenticated instance's configuration-management authority, preserving authentication-disabled behavior. Agents shall not grant themselves permission through MCP. A concurrent permission revocation or name change shall prevent a stale agent write.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.7:** Successful writes shall be visible in an already-open Settings > Prompts page without reload, preserve unsaved editor drafts, and affect subsequent `@name` expansion. Reconnection shall reload saved prompts.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.8:** Invalid, denied, conflicting, or failed writes shall report an error without leaking database details or publishing a successful change.

## Exclusions

MCP deletion, dedicated-tool renaming, workspace ownership, automatic rewriting of
workflow references, and changing prompts already captured by a running turn.
