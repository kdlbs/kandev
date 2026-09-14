---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-PROVIDER-BACKOFF-001
  - REQ-INTEGRATIONS-PROVIDER-BACKOFF-002
---

# Provider failure backoff and health System Design

## Purpose and boundaries

Integrations owns provider credentials and the background loops that call
providers. This design covers the two circuit-protected loops added by this
change: the GitHub PR-watch poll loop and the workflow-sync loop. On-demand
HTTP-triggered syncs are deliberately unprotected, matching the existing
rate-tracker precedent.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-PROVIDER-BACKOFF-001` | Shared backoff; Poller circuit; Workflow-sync circuit |
| `REQ-INTEGRATIONS-PROVIDER-BACKOFF-002` | Health; Metrics |

## Shared backoff

`apps/backend/internal/common/authcircuit` provides `FailureClass` (auth,
config, transient), `Backoff`, and `State`, shared by both loops. Missing or
not-configured sentinels classify as auth; a repo that cannot be resolved is
config; typed API errors classify by status code; everything else is
transient.

## Poller circuit

`apps/backend/internal/github/poller_circuit.go` holds an in-memory,
mutex-guarded, poller-scoped map keyed by workspace. Before each
`checkPRWatches` cycle, `filterOpenCircuitWatches` refreshes each present
workspace's connection fingerprint once (via
`Service.WorkspaceConnectionFingerprint`) and excludes every watch whose
circuit is open. All three GitHub-call sites in the poll path (batched sync,
per-watch check, branch-based PR detection) record outcomes through
`classifyPollErr`. A client without GraphQL support returns to the REST
fallback before recording, so it never opens a circuit.

## Workflow-sync circuit

`apps/backend/internal/workflowsync` persists `failure_class`,
`consecutive_failures`, `next_retry_at`, and the configuration and credential
fingerprints per config. `SyncDueConfigs` refreshes the credential fingerprint
first and forces an immediate sync when it changed; otherwise it skips
due-but-circuit-open configs before the interval gate.

## Health

`apps/backend/internal/health` exposes `WorkflowSyncChecker` mirroring the
existing `GitHubChecker`: nil-provider-safe, aggregate counts by class, no
workspace identifiers or secrets in messages. `backendapp/helpers.go` wires it
with the adapter-per-package-boundary idiom to avoid an import cycle.

## Metrics

Bounded-label expvar counters: `workflowsync_failures_total{provider,class}`,
`workflowsync_circuit_skips_total{provider}`,
`workflowsync_circuit_resets_total{provider}`,
`github_pr_watch_auth_circuit_skips_total`, and
`github_pr_watch_auth_circuit_resets_total`. No secrets, tokens, branches, or
workspace identifiers appear as labels.

## Related decisions

- [Keep PR watches task owned](../../../decisions/2026-08-31-task-owned-pr-watch-identity.md)
