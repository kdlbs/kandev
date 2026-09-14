# ADR-2026-09-08-domain-owned-settings-catalog: Discover settings through domain-owned contracts

**Status:** accepted
**Date:** 2026-09-08
**Area:** backend, frontend, protocol

## Context

The profile editor and MCP tools maintain different field lists.
The frontend saves fields that the MCP schema and forwarding handlers cannot carry.
Existing startup and navigation catalogs describe different contracts.

The user selected searchable discovery, shared mutations, and parity tests as the direction for this work.
Future settings domains need an ownership rule that preserves their existing validation and persistence.

## Decision

Platform owns a shared metadata contract for settings discovery.
Each domain contributes descriptions and typed operation bindings while retaining ownership of values, defaults, validation, authorization, and persistence.
The first implementation slice is the agent profile domain.
The delivery scope also includes core user, workflow, workspace, repository, executor, task, prompt, utility, editor, notification, integration, automation, and system settings.
The domain adoption design defines eligible fields and explicit exceptions. Profile parity alone does not complete this initiative.

UI and MCP adapters use shared domain request contracts.
The configuration assistant uses four compact settings tools: search, describe, read, and update.
One additional bounded resource-lookup tool supplies authorized targets across domains.
Common target selectors replace profile-specific arguments on the new generic tools.
The catalog returns field schemas on demand. `update_settings_kandev` validates its `changes` object against the registered domain request before mutation.
The outer tool schema contains no field enumeration or resource-schema union.
Existing resource tools remain compatibility adapters. Resource creation, deletion, and import remain explicit lifecycle operations.
Each field has a support status. An exception requires a reason and a destination or recovery description.

The navigation catalog retains ownership of labels, translations, ordering, routes, and focus targets.
Stable setting IDs connect navigation entries to domain metadata.
Startup settings and runtime flags retain their current registries and source precedence.
Their catalog adapters must derive metadata from those owners.
Startup sources remain read-only. Runtime override writes retain environment locks and restart semantics.
Authentication enablement and account authority remain outside generic mutation authority.

Coverage checks compare independent editable contracts with catalog mappings.
Tests that iterate only the catalog do not establish completeness.
Transport tests also verify forwarding, persistence, errors, and notifications.
Required CI checks cover registry completeness, generated contract freshness, and a curated search-query set.
Coverage includes every built-in Settings control and independent editable domain contract.
Client-local state, credential enrollment, plugin-owned settings, and explicit lifecycle actions receive reasoned classifications.
An eligible field cannot pass delivery checks with a not-yet-supported classification.

## Consequences

Each adopter delivers complete coverage for a named domain.
Discovery reports the adopted domains explicitly. It does not claim complete product coverage during adoption.
The contract supports later file import/export without introducing another storage owner now.

Adapters need tests for domain-specific defaults, dynamic choices, and permission boundaries.
The UI remains hand-authored, and domain request contracts remain typed.
Discovery metadata is advisory. Invocation always enforces current authority.
Search results identify the setting, resource type, mutation field, support status, and match reason.
The caller loads detailed schemas and context-dependent choices only for relevant settings.
The backend selects schemas from trusted domain registrations. Agents cannot supply or modify a validation schema.

Each domain switches its settings operations after parity checks pass. Unadopted legacy tools remain visible during implementation.
The shared interface contains four settings tools and one resource-lookup tool. Explicit lifecycle and compatibility operations remain separate.
Schemas grow in the registry, not in the generic tool definitions.
Domain services validate resulting state and own any required transaction. An adapter-only lock cannot protect other writers.

## Alternatives Considered

- **One tool per setting:** This increases the tool surface and duplicates resource semantics across many handlers.
- **One unrestricted settings map:** This hides scope, replacement rules, dependencies, and authorization behind arbitrary keys.
- **Make a settings file authoritative first:** This introduces synchronization and precedence changes before it closes the interface gap.
- **Use the navigation catalog as the backend schema:** Navigation entries include pages and controls that do not represent writable fields.
- **Maintain manual MCP schemas indefinitely:** This preserves the source of the current field drift.
- **Advertise every generated resource schema upfront:** This prevents field drift but increases context consumption as settings grow.

## Related documents

- [Requirements](../specs/platform/requirements/agent-settings-parity.md)
- [System design](../specs/platform/system-design/agent-settings-parity.md)
- [Domain adoption](../specs/platform/system-design/agent-settings-domains.md)
- [Navigation boundaries](2026-08-04-navigation-manifest-boundaries.md)
- [MCP tool profiles](2026-08-08-mcp-tool-profiles.md)
- [Settings save coordinator](0046-settings-route-save-coordinator.md)
