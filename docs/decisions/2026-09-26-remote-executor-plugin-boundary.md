# ADR-2026-09-26-remote-executor-plugin-boundary: Remote executor plugin boundary

**Status:** accepted
**Date:** 2026-09-26
**Area:** backend, frontend, protocol

## Context

[Issue 3953](https://github.com/kdlbs/kandev/issues/3953) proposes Lambda MicroVM execution.
Current plugins can read executor profiles but cannot provide execution environments.
The internal executor interface includes concrete clients and callbacks that cannot cross plugin gRPC.
Provider lifetimes also differ: some environments destroy their workspace at a fixed deadline.

## Decision

Propose an additive remote executor provider contract over the existing managed plugin transport.
A plugin owns provider operations, remote bootstrap delivery, and endpoint authentication.
Core owns admission, execution identity, agentctl selection, ACP, workspace materialization, and cleanup authorization.
The executor system owns the vertical requirements and system design.

Use one generic plugin-remote backend with manifest-owned provider identities.
Keep built-in executors in place. Missing providers never fall back to local execution.
New providers must implement operation recovery and exact-resource cleanup before admission is enabled.

Core records provisional allocation and non-secret resource state in `executors_running`.
Provider state does not become a second inventory in plugin storage.
Execution identity and environment ownership generation fence all mutations and destructive operations.

Transport decorators remain host objects. Serializable connection leases cross gRPC.
All agentctl clients and gateway proxies use the same lease authority.
Provider tokens cannot replace host authentication or reach the browser.

The first contract owns one environment per session and rejects shared-workspace admission.
It supports reattachment, not replacement presented as recovery.
Retention is explicit and defaults to unknown. Provider expiry is an observed external event,
not permission for Kandev to cancel quiet work early.

Disable stops plugin dispatch while preserving credentials and resource records.
Uninstall requires confirmed absence of all retained resources and unresolved allocations.
Upgrade requires compatibility with their recorded state versions.

This decision establishes the provider boundary implemented by the host executor and plugin SDK.
The restart-required rollout flag remains disabled in shipped profiles. Production Lambda support
and migration of built-in executors remain separate work.

## Consequences

New remote vendors can ship independently without duplicating the agent protocol.
Core must extend profile discovery, recovery, proxy transport, and plugin administration together.
Cloud-bound workspaces can be supported honestly without promising preservation beyond their lifetime.

The first release requires a reachable remote-to-host API address.
A backend behind NAT needs an existing reachable route; a new relay is excluded.
The host must keep cleanup records even when the provider is disabled or missing.

## Alternatives Considered

- **Add each vendor to core:** Smaller first change, but every vendor adds SDK and release coupling.
- **Export the internal executor interface:** Leaks internal types and cannot transmit its Go callbacks over gRPC.
- **Proxy every agent operation through the plugin:** Duplicates agentctl and couples streaming availability to the plugin process.
- **Use generic event subscriptions for lifecycle:** Best-effort delivery cannot establish synchronous launch and cleanup ownership.
- **Migrate every built-in executor first:** Expands scope without being necessary to admit one provider.
- **Assume workspace retention when unspecified:** Hides unassessed data-loss behavior.
- **Uninstall while preserving only an orphan record:** Deletes the code and credentials required for ordinary cleanup.

## Related contracts

- [Requirements](../specs/executors/requirements/remote-executor-plugins.md)
- [System design](../specs/executors/system-design/remote-executor-plugins.md)
- [Runtime inventory](0003-executors-running-as-execution-id-source-of-truth.md)
- [Cleanup inventory](0025-runtime-cleanup-uses-executors-running.md)
- [Environment ownership fencing](2026-09-04-generation-fenced-task-environment-ownership.md)
