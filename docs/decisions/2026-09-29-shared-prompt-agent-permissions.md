# ADR-2026-09-29-shared-prompt-agent-permissions: Operator-owned prompt permissions

**Status:** accepted
**Date:** 2026-09-29
**Area:** protocol

## Context

Shared prompt updates affect every future workflow reference. MCP needs write
parity without silently granting access to existing human-maintained instructions.

## Decision

Persist per-prompt agent-edit permission, off for existing and Settings-created
prompts, on for MCP-created prompts. Operators manage it in Settings. Built-ins
always reject MCP writes. All MCP write paths, including generic settings, share
conditional storage enforcement; MCP cannot change the permission itself.

## Consequences

Upgrades preserve human-only prompts. An operator must opt in once before an agent
can update an existing custom prompt. Newly agent-created prompts support the full
create/read/update loop. Permission revocation cannot be undone by a stale update.

## Alternatives Considered

An instance-wide switch cannot protect selected prompts. Allowing every existing
custom prompt by default changes operator ownership without consent. Confirmation
text alone cannot enforce a stored restriction or prevent another MCP write path
from bypassing it.
