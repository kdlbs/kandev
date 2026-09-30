---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001
---

# Workspace Base-Branch Propagation System Design

## Purpose and boundaries

Workspaces own the recorded Git comparison base and repository tracker identity.
The task service supplies the persisted map; agent runtime transports it into
agentctl. This design repairs healthy recovery without changing Git fallback,
executor provisioning, public APIs, or persistence ownership.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| .1, .6, .7, .9 | Creation and readiness |
| .2, .3, .8, .10 | Tracker comparison |
| .4, .11 | Failure and observability |
| .5 | Data and contracts |

All criteria belong to `REQ-WORKSPACES-WORKSPACE-BASE-BRANCH-PROPAGATION-001`.

## Data and contracts

`Service.TaskBaseBranches` uses `collectTaskBaseBranches` to read task repository
base branches, with the repository default as its existing fallback. It owns
repository-name/worktree-subpath keys and the single-repository empty key.
Lifecycle consumes this through the existing `BaseBranchProvider` callback.
An incomplete inventory is an error, not a smaller authoritative replacement.

`ExecutorCreateRequest` has no `BaseBranches` field. Its existing
`MetadataKeyBaseBranches` entry transports the map. Executor adapters decode
both `map[string]string` and JSON-restored `map[string]interface{}` using
`getMetadataStringMap` into agentctl's `CreateInstanceRequest.BaseBranches`.
No new parallel field, database column, configuration, or migration is needed.

The existing `POST /api/v1/workspace/base-branches` replaces the map and stamps
trackers synchronously, then refreshes status asynchronously. An empty provider
result must not wipe valid launch metadata. Repeating the same refs preserves
comparison identity.

## Creation and readiness

Full launches already populate base-branch metadata through `buildLaunchMetadata`.
On-demand creation in `prepareExecutionCreateRequest` must inspect its copied
metadata with the existing decoder. Preserve a usable non-empty persisted map.
If none is available and a task/provider exists, synchronously hydrate the map
before `ExecutorBackend.CreateInstance`; store a copied non-empty result under
`MetadataKeyBaseBranches`. Do not mutate `WorkspaceInfo.Metadata` or a provider's
returned map. Empty, nil, or unusable metadata uses the same provider fallback.

This lets `process.NewManager` stamp initial trackers before their first scan.
Standalone/worktree, Docker, SSH, Sprites, and Kubernetes already consume this
metadata either directly or through `buildReconnectCreateInstanceRequest`.
Plugin executors depend on their existing transport capability; do not introduce
a new SDK field as part of this repair.

In `waitForAgentctlReady`, the HTTP readiness check comes first. The existing
base-branch and comparison-target pushes follow, using the client lease rules.
Only after both attempts finish may `MarkAgentctlReady`, the cached poll-mode
flush, or the `AgentctlReady` event expose readiness. A blocked push must leave
readiness false. Preserve lease release and shutdown cancellation on all paths.
The separate already-running recovery path in `Manager.Start` already pushes
before stream reconnection and must retain that ordering.

## Tracker comparison

Each tracker uses its own map key. Explicit repository-qualified comparison
targets retain precedence over branch-only bases. For branch-only comparison,
Git log and cumulative diff use the configured base/merge-base; integration
history must not appear as task commits when that base is present and valid.
No-base repositories retain independent integration fallback, even when a
sibling repository has a configured base.

## Failure and observability

Creation-time hydration derives a five-second child context from its caller
before the DB-backed provider lookup. The task repository and repository reads
honor this context; an earlier parent deadline or cancellation still wins. A
lookup timeout warns and leaves request preparation without a hydrated map.
Creation-time provider errors remain nonfatal, matching the existing best-effort
propagation contract. Readiness-time provider and HTTP push errors
also warn without blocking workspace access indefinitely. All waits retain
the lifecycle timeout and shutdown context. Missing refs keep existing fallback
and diagnostic behavior. This repair guarantees seeded initial results when
hydration succeeds; it does not guarantee accurate comparison during a failed
hydration. A mandatory seeded-state API gate is deferred because it would change
that failure contract, including repo-less and intentionally unconfigured cases.

## Delivery and related decisions

- [Recovered execution repair](../../../plans/recovered-execution-base-branches/plan.md)
- [Original repair](../../../plans/workspace-base-branch-propagation/plan.md)
- [Repository-qualified comparison targets](../../../decisions/2026-08-19-repository-qualified-comparison-targets.md)
- [Environment-owned Git status](../../../decisions/2026-08-30-environment-owned-git-status.md)
