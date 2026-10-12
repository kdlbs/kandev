# ADR-2026-09-24-native-codex-normalization: Native Codex at the adapter boundary

**Status:** accepted
**Date:** 2026-09-24
**Area:** protocol

## Context

ACP remains Kandev's default Codex integration. Its normalized surface does not expose every native lifecycle or usage detail.
The user requested a separate native agent, an inspection tool, and built-in usage displays.
Restoring the deleted adapter unchanged would restore an outdated protocol contract.

## Decision

Native Codex has a distinct agent identity and an off-by-default runtime flag.
Its transport maps native data into provider-neutral Kandev events and capabilities.
React components do not parse Codex wire messages.
Native children remain provider threads, not automatically created Kandev tasks.

The production adapter and developer inspection CLI share a typed protocol client.
Process ownership, capture policy, and presentation remain separate consumers of that client.
Unknown native data can remain in explicit local captures without becoming a public UI contract.

Usage enters the existing accounting system with explicit scope and provenance.
The UI does not require a cost plugin, and a provider estimate does not become an invoice amount.
Existing ACP records retain their meaning.

## Consequences

Kandev must maintain a tested native schema and handle missing optional capabilities.
Shared normalization gives desktop, phone, plugins, and persisted history the same interpretation.
The inspection CLI can reproduce protocol problems without starting the whole application.
Direct native support adds maintenance work alongside ACP until the user chooses a later migration.

## Alternatives considered

- ACP metadata only: less transport maintenance, but the bridge still controls access to native operations and events.
- Raw Codex messages in the frontend: faster initial integration, but duplicates provider logic across rendering and history consumers.
- A separate debug protocol implementation: simpler first tool, but its behavior can diverge from production.
- Immediate replacement of ACP: removes a transport, but loses the requested comparison and rollback path.

## Related designs

- [Native Codex](../specs/agents/system-design/codex-app-server.md)
- [Conversation usage](../specs/costs/system-design/conversation-usage.md)
- [Existing Codex ACP contract](0034-agentclientprotocol-codex-acp.md)
