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

This revision removes per-prompt permission for user-created prompts.
Built-in protection and configuration-management authority remain required.

## Requirements

### REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001: Controlled shared prompt writes

- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.1:** Configuration and external MCP clients shall expose `create_shared_prompt_kandev` and `update_shared_prompt_kandev`, each accepting name and content. Task, Office, and automation catalogs shall exclude them.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.2:** Creation shall reject an existing exact name, including a built-in name, without changing its content. Names and content shall use existing saved-prompt validation and whitespace normalization.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.3:** Updates shall resolve an exact case-sensitive name, reject missing prompts, and replace only content. Successful write results and subsequent reads shall expose saved content and agent-edit permission.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.7:** Successful writes shall be visible in an already-open Settings > Prompts page without reload, preserve unsaved editor drafts, and affect subsequent `@name` expansion. Reconnection shall reload saved prompts.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.8:** Invalid, denied, conflicting, or failed writes shall report an error without leaking database details or publishing a successful change.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.9:** Authorized agents shall be able to update every user-created custom prompt without a separate prompt permission. This includes existing prompts previously marked as disallowing agent edits and prompts created through Settings, HTTP, or MCP.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.10:** Built-in prompts shall reject every agent write, including generic settings updates. Rejection shall preserve saved content. Operator editing of built-ins shall remain available.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.11:** Writes shall require the authenticated instance's configuration-management authority. Existing authentication-disabled behavior and tool availability shall remain unchanged.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.12:** Desktop and phone prompt editors shall expose no agent-edit permission control. Existing name and content editing, save, discard, draft preservation, and error behavior shall remain available.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.13:** Concurrent content changes, renames, deletion, or built-in reclassification shall reject stale agent writes. Stale operator saves shall continue to report conflicts.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.14:** Configuration-agent instructions and prompt-write tool descriptions shall require explicitly requested or agreed prompt changes and readback. Permission removal shall introduce no automatic prompt rewriting.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.15:** Existing `allow_agent_edits` response fields shall report effective eligibility: true for custom prompts and false for built-ins. Legacy HTTP permission input shall not establish a per-prompt restriction. Generic settings shall continue to reject permission changes.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.16:** Upgrade and repeated initialization shall preserve prompt identity, name, content, and timestamps. Historical permission values shall have no effect on custom-prompt edit eligibility.

## Retired acceptance criteria

Criteria .4, .5, and .6 describe the former permission policy. Their IDs remain
reserved for the original implementation record. They do not govern current behavior.
Criteria .9 through .16 replace that policy.

- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.4 (retired):** Built-in prompts and custom prompts without agent-edit permission rejected agent updates, including generic settings MCP.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.5 (retired):** Existing and Settings-created prompts defaulted to human-only editing. MCP-created prompts allowed agent edits. Operators controlled permission.
- **AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.6 (retired):** Configuration authority and conditional writes protected operator-owned permission, including concurrent revocation and content-only saves.

## Exclusions

MCP deletion, dedicated-tool renaming, workspace ownership, automatic rewriting of
workflow references, and changing prompts already captured by a running turn.
New approval protocols and broader tool catalogs are also excluded.
