# ADR-2026-09-16-injected-mcp-approval: Approve the injected Kandev server through ACP

**Status:** accepted
**Date:** 2026-09-16
**Area:** backend, protocol

## Context

Issue #3729 reports repeated Claude ACP permission prompts for Kandev plan and task tools.
Codex already receives server-wide approval through its launch configuration.
The issue explicitly offers server-wide Claude parity as its minimum fix.

## Decision

The proposed repair adds a server-specific decision before ACP creates a pending permission request.
It requires a provider tool identity and host-owned injection provenance.
It approves every tool on the injected Kandev server, not only bookkeeping tools.
It does not enable blanket approval or change task authorization.

Provider naming compatibility belongs in pure adapter dialect hooks.
The process manager owns policy and choice selection.
Unknown identities retain normal permission handling.
The flattened `mcp__server__tool` form is accepted only when it has one
unambiguous separator. Delimiter collisions and adjacent underscores remain
pending because the server/tool boundary cannot be trusted.

The [requirements](../specs/agents/requirements/injected-mcp-approval.md)
and [design](../specs/agents/system-design/injected-mcp-approval.md) define the implemented boundary.

## Consequences

Claude can update plans and task titles without repeated prompts.
The same policy also approves task creation, deletion, and session-management calls on the injected server.
Existing server-side authorization and user-input barriers still apply.
No approval history, new profile field, or database migration is necessary.

## Alternatives considered

- Blanket profile approval also permits shell and third-party operations. It exceeds the requested boundary.
- A selective allowlist and a restore-prompting setting need a different policy across Claude and Codex. They are outside this minimum package.
- CLI-only allow flags depend on provider behavior and do not resolve the confirmed host-side pending-request path.
- A generic title-prefix match cannot distinguish provider presentation from programmatic identity. The design restricts legacy title interpretation to Claude.
