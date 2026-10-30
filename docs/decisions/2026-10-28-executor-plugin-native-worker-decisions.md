# ADR-2026-10-28-executor-plugin-native-worker-decisions: Native worker capabilities for executor plugins

**Status:** accepted
**Date:** 2026-10-28
**Area:** executors, plugins

## Context

A worker-pool provider (for example a scheduler that places agent workloads on remote nodes) cannot be expressed by the
accepted [boundary ADR](2026-09-26-remote-executor-plugin-boundary.md) alone. It needs a workspace path in the descriptor,
a host-owned transport, a drain receipt, checkpointed destroy and recover, durable bootstrap, and repository intent. The
maintainer asked for these as generic, opt-in host capabilities, not as a built-in executor. The requirement and system
design documents under `docs/specs/executors/` define them; this record settles the owner-level decisions and names the
clauses of the boundary ADR that the package amends.

## Decision

- **D1, transport.** The host owns the dialer. A bidirectional gRPC `OpenExecutorTunnel` stream is bound to installation,
  environment, generation, and port and carries control traffic. The reverse direction (agent to backend API) uses a
  provider-forwarded API URL. Owner answer 2026-10-28: confirmed.
- **D2, workspace materialization.** Both preparation modes ship. `core` stays the default; `provider` is added through
  repository intent, with the extensible preparation contract so a mirror, sparse checkout, or pull-request checkout needs
  no further core change. Owner answer 2026-10-28: confirmed.
- **D3, built-ins.** Built-in executor code stays until the plugin passes its live scenarios. Those scenarios live in the
  vendor plugin repository and no host behavior depends on them. A built-in and a plugin never share one environment.
  Owner answer 2026-10-28: confirmed.
- **D6, SDK.** The provider SDK stays a pinned-reference dependency for the first release; no module is published yet.
  Owner answer 2026-10-28: confirmed.
- **Medium-term open questions (owner, 2026-10-28).** Sharing: same principal only (binds only the deferred shared
  environments card). Maintenance audit sink: a structured log line, no new table. Admission policy scope: a hook seam with
  a default allow-all. Pending-placement UI: a status label plus the existing Stop.

### Clauses of the boundary ADR that this package amends

1. "Core owns ... workspace materialization" is amended for `workspace_preparation: provider`: the provider materializes
   and core verifies the repository inventory.
2. "Serializable connection leases cross gRPC" and "all agentctl clients use the same lease authority" are amended by the
   `host_tunnel` transport. There is still one lease authority and no public relay.
3. "Keep built-in executors in place" and "migration of built-in executors remains separate work" stand; profile migration
   is deferred to a follow-up card and removes no built-in.

The clause "one environment per session and rejects shared-workspace admission" stands unchanged. Every other clause stands.

## Consequences

- A provider that declares no new capability keeps today's behavior; each capability is a registry entry negotiated per
  installation and dispatch generation, and contract version 1 does not change.
- The 32 KiB envelope holds placement, capability snapshot, repositories, and provider state together.
- Shared environments, profile migration, resource classes, built-in removal, a per-node mirror, fleet UI, and
  hostile-workload isolation are out of scope.

## Alternatives Considered

- **Relax HTTPS or loopback checks in core:** weakens validation for every provider and cannot express SSH.
- **A plugin loopback listener:** opens an unauthenticated port on the backend host and races the port allocator.
- **Core clone only:** changes the observable local-worktree behavior of a local node.
- **Remove built-ins first:** strands retained inventory.
- **Publish the SDK module now:** no second external plugin exists to justify a versioning commitment.

## Related contracts

- [Requirements](../specs/executors/requirements/remote-executor-plugin-native-workers.md)
- [System design](../specs/executors/system-design/remote-executor-plugin-native-workers.md)
- [Boundary ADR](2026-09-26-remote-executor-plugin-boundary.md)
- [Ownership generations](2026-09-04-generation-fenced-task-environment-ownership.md)
