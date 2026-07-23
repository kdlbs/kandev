# ADR-2026-07-21-work-item-reference-search: Backend-Normalized Work-Item References

**Status:** accepted
**Date:** 2026-07-21
**Area:** backend, frontend, protocol

## Context

Kandev's chat composer currently builds `@` suggestions in the browser from unrelated local sources. Work-item search spans seven provider families with different credentials, workspace scopes, query languages, pagination, identities, and failure modes. A selected item must also remain unambiguous after its title or repository changes and while a message waits in the durable queue.

## Decision

Kandev owns work-item typeahead behind a backend provider registry in `internal/mentions`. Every provider implements a plain-text, workspace-scoped search boundary and translates that query into its private API dialect. Registry and aggregate service depend only on hand-defined, versioned normalized DTOs plus a provider descriptor; they do not import native Jira/GitHub/etc. models, plugin wire models, `structpb`, or switch exhaustively on native provider IDs. The aggregate endpoint runs providers concurrently with bounded work and timeouts, returns deterministic provider/kind groups, and treats provider failures as partial results with safe status codes.

Providers return candidates with provider-local immutable identity plus non-secret connection scope. The registry injects its registered provider identity, validates candidate fields, and constructs the canonical versioned `ref`; an adapter cannot spoof another provider. Each `(provider, kind)` registration also owns a `ReferenceAuthorizer`, used both to filter search destinations and to authorize message submission against the trusted conversation workspace. This keeps configured-origin and scope checks provider-owned without a central provider switch. Jira and GitHub adapters retain upstream immutable IDs that their current projections drop; providers without a single global ID use a documented provider-native composite. URLs and titles are presentation snapshots, not identity.

The frontend uses a separate `#` `entityReference` atom. `@` remains the context-attachment channel for files, prompts, and plans. New task discovery moves to `#`, while legacy `@task` nodes remain compatible.

Selected references serialize into both portable Markdown links in visible message content and typed `entity_references` in existing message/queue metadata JSON. A neutral `internal/entityrefs` leaf owns structural normalization, canonical identity, and deduplication; provider authorization remains in `internal/mentions`. Metadata drives chip rendering and sanitized agent context; Markdown is the durable fallback. No new reference table or synchronized cross-provider index is introduced.

Built-in integrations register adapters through this host seam. `provider` and `kind` are validated, additive strings with generic presentation fallbacks, not closed enums. A future `PluginProviderBridge` can therefore enumerate active plugins with an explicit mention-provider contribution and adapt a typed Kandev-to-plugin `Plugin.SearchMentions` RPC to the same `MentionProvider` interface. This is not part of the plugin-to-Kandev `Host` data API and does not reuse `api_read`. Defining the manifest contribution, permission/grant, additive RPC, lifecycle/error mapping, and workspace authorization is a separate public-contract decision following [ADR 0043](0043-plugin-host-data-api.md); this feature does not add them. Older plugins that return gRPC `Unimplemented` will map to provider-unavailable without failing other sources.

## Consequences

- Workspace isolation, provider query escaping, timeouts, error classification, and identity normalization live in one backend boundary instead of being duplicated in React.
- Adding a built-in provider requires an adapter and conformance tests but does not change the composer contract.
- Migrating a built-in integration to a plugin replaces its adapter registration with a plugin bridge while preserving registry-issued provider/source IDs (or explicit aliases), normalized reference identity, message metadata, and frontend behavior. New plugin-only providers use reserved namespaced IDs.
- Jira and GitHub search models must preserve upstream immutable IDs. Azure DevOps and Sentry adapters need bounded project/repository or instance/organization discovery because their current browse APIs require extra scope.
- Typeahead consumes external API quota. Client debounce/cancellation, server caps, and provider timeouts are correctness requirements. Any provider/client cache must be short-lived and keyed by workspace plus provider connection scope so it cannot cross authorization boundaries.
- Partial results are expected. Cross-provider scores are not compared; results stay grouped and deterministic.
- Message and queue metadata already provide restart durability, avoiding a migration and reference lifecycle table. Stale targets keep their display snapshot but are not guaranteed to open successfully.
- Plugins cannot contribute `#` results until a future contribution, permission, and Kandev-to-plugin RPC contract is designed, but current core code must not add coupling that would require a second composer/search contract.

## Alternatives Considered

### Fan out from the frontend

Rejected. It would expose provider query dialects and failure semantics to React, duplicate workspace/config checks, complicate cancellation, and make every new provider a composer change.

### Reuse the existing `@` mention pipeline

Rejected. `@` items attach local context and have different serialization/selection semantics. Mixing external durable entities into that union would preserve the current overloaded menu and make migration harder.

### Store only plain text or Markdown links

Rejected. Mutable labels and URLs cannot reliably identify a task or work item for agent tools, queue edits, history recall, or sent-message chip rendering.

### Create a normalized reference table or local search index

Rejected for this release. References do not own target lifecycle, and synchronizing every provider would add migrations, webhook/polling consistency, retention, and stale-index behavior before typeahead needs them.
