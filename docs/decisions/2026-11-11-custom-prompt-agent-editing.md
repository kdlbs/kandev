# ADR-2026-11-11-custom-prompt-agent-editing: Agent editing for every custom prompt

**Status:** accepted
**Date:** 2026-11-11
**Area:** protocol

## Context

The operator requested removal of the Allow agent edits switch for custom prompts.
Configuration assistants already receive instructions to apply agreed prompt changes.
The separate permission adds a Settings action before an otherwise authorized edit.

This decision supersedes the
[per-prompt permission decision](2026-09-29-shared-prompt-agent-permissions.md).
Implementation is recorded in the
[simplification package](../plans/custom-prompt-agent-editing/plan.md).

## Decision

Every user-created custom prompt permits agent edits through existing authorized
write paths. Built-in prompts continue to reject agent writes.
Configuration-management authority and catalog restrictions remain unchanged.

Configuration assistants apply only explicitly requested or agreed prompt changes.
This is an agent instruction. The backend does not authenticate the meaning of
a user request and gains no new confirmation protocol.

The editor has no permission switch or replacement approval control.
Existing `allow_agent_edits` response fields report `!builtin` for compatibility.
Legacy HTTP permission input has no effect. Generic settings cannot set permission.

Persistence no longer reads or writes a per-prompt permission value.
New databases do not require its column. Existing columns remain unused, with
no destructive migration or future cleanup dependency.
Conditional writes retain identity, built-in, and modification-time guards.

## Consequences

Previously protected custom prompts become editable by authorized agents.
Prompt reads continue to tell clients whether agent editing is available.
Shared edits affect future references and preserve instructions captured by a running turn.
Public documentation must describe this changed default and its authority boundary.

## Alternatives Considered

- Retain the switch. This preserves an extra operator action that the user requested to remove.
- Enable every stored flag. This retains obsolete state, migration writes, and the same future permission machinery.
- Remove response metadata immediately. This breaks clients without simplifying the operator experience further.
- Allow edits to built-in prompts. This exceeds the requested custom-prompt change.
- Require a new approval token for each write. This adds a protocol the user did not request.
