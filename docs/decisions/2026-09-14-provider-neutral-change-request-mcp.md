# ADR-2026-09-14-provider-neutral-change-request-mcp: Share the change request MCP contract

**Status:** accepted
**Date:** 2026-09-14
**Area:** backend, protocol, integrations
**Introducing release:** 0.95.0 (planned)

## Context

PR #3506 introduced shared association services under PR-named MCP tools.
Automation tools still split by provider, while their operations largely overlap.
GitLab supports automation but lacks GitHub's server-bound outcome protocol.
Both providers store custom prompts per task and switches per association.

## Decision

Use four tools: read change requests, manage an association, update automation,
and report a dispatched auto-fix outcome. Keep outcome reporting separately bound
to server execution identity. Report actual capabilities for each provider.

Use `provider`, canonical `repository_id`, and `number` for contribution identity.
Require an explicit automation target. Task targets require a provider list.
Prompts remain task/provider settings, and association targets reject prompt changes.
Cross-provider automation reports partial application without a shared transaction.
Keep provider services and stores behind the shared MCP contract.

Remove legacy MCP names at cutover. Retain backend transport compatibility for
one release transition, then require old runtimes to resume before its removal.
The [system design](../specs/integrations/system-design/task-change-link-mcp.md)
defines this transition and its validation.

This decision amends the generic-tool rejection in the
[provider-scoped runtime ADR](2026-08-03-provider-scoped-task-mcp-tools.md).
Backend provider derivation and transport remain unchanged.
It also changes the tool name in the
[explicit-outcome ADR](2026-09-06-explicit-pr-auto-fix-outcomes.md), without changing attempt semantics.
The amendments take effect in the planned 0.95.0 release. The retained backend
transport handlers are removed in the first stable release after 0.95.0.

## Consequences

New providers add capabilities and adapters instead of more tool families.
Agents can distinguish an association update from a task/provider update.
A mixed-provider write can partially succeed. Its response must identify remaining work.
Cached callers must rediscover tools or resume with the current runtime.
The implementation does not require a store migration or GitLab outcome tracker.

## Alternatives Considered

- Keep eight names: provider growth expands discovery and duplicates tool selection.
- Use one universal tool: reporting would share a schema with ordinary management despite different execution authority.
- Keep link, unlink, and replace separate: slightly shorter schemas cost two names without removing identity validation.
- Merge provider prompts: this changes durable settings and can overwrite different user instructions.
- Omit target identity for task fan-out: a missing field could affect every linked contribution.
- Make mixed writes atomic: this requires cross-store coordination outside the requested MCP boundary.
- Hide callable aliases with SDK filters: the installed SDK filters calls too, so this cannot preserve old invocation.

These choices retain the preferred four-tool surface. No fifth operation has a
separate authority or lifecycle that requires another tool.
