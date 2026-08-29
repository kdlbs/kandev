---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
  - REQ-INTEGRATIONS-GITHUB-RATE-004
---

# GitHub Rate-Limit Coordination System Design

## Purpose and boundaries

The GitHub integration owns provider response classification, rate observation,
and provider admission. Workflow Sync consumes the typed failure/retry contract
and persists its own scheduling state. MCP exposes a task-authorized snapshot
of the same admission state without refreshing it.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-RATE-001` | [Response classification](#response-classification) |
| `REQ-INTEGRATIONS-GITHUB-RATE-002` | [Principal-wide coordinator](#principal-wide-coordinator) |
| `REQ-INTEGRATIONS-GITHUB-RATE-003` | [Workflow Sync recovery](#workflow-sync-recovery) |
| `REQ-INTEGRATIONS-GITHUB-RATE-004` | [Agent snapshot](#agent-snapshot) |

## Response classification

`internal/github` returns a typed `GitHubAPIError` carrying a non-secret
failure kind, rate resource, provider retry time/source, and parsed primary
snapshot. REST and GraphQL clients classify from status, body, and headers.
The CLI client classifies rate prose as primary only when its tracker already
has an exhausted primary snapshot; otherwise it records secondary throttling.

A reset header with positive remaining quota is primary metadata, not evidence
of primary exhaustion. `GET /rate_limit` updates primary buckets but does not
clear the independent secondary state. A later accepted provider response can
clear the secondary state before its conservative retry estimate.

## Principal-wide coordinator

`RateCoordinator` is a singleton owned by `github.Service`. It maps a stable,
non-secret quota key to one tracker and admission state. Human keys use GitHub
host plus normalized login; App keys use host, App registration, and
installation. Workspace ID and credential generation are excluded from the
quota key even though they remain part of credential and cache ownership.

Kandev-routed requests default to interactive work. Periodic GitHub and
Workflow Sync pollers mark their context as background. Background requests
are serialized and paced per principal/resource, stop at a ten-percent primary
reserve, and yield to interactive waiters. Primary and secondary retry windows
block both classes without occupying an execution slot while waiting.

## Workflow Sync recovery

`workflow_sync_configs` persists consecutive failure count, next attempt,
failure class, and automatic-poll suspension. Transient failures use
equal-jitter exponential delay based on the larger of the configured interval
and one minute, doubling to a one-hour cap. A later provider retry/reset is the
lower bound.

Invalid credentials/access and missing targets suspend automatic polling and
write one actionable error. A configuration save clears scheduling state.
Explicit Sync now bypasses automatic scheduling state, updates the single error
on failure, and clears the state on success. GitLab receives generic transient
backoff but does not use GitHub-specific response classification.

## Agent snapshot

`get_github_rate_limit_kandev` is registered for Kanban and Office task
profiles. Its backend action derives and authorizes the current task's
workspace, then calls a snapshot-only GitHub service method. The method does
not call `FetchRateLimit`, `GetWorkspaceAuthStatus`, or any provider endpoint.

The DTO returns known primary buckets with observation times, observed
secondary retry state and source, non-secret principal scope, and separate
interactive/background admission results. Unknown or stale provider state is
labelled rather than refreshed.

## Persistence and migration

Workflow Sync adds columns through the package's existing idempotent ALTER
pattern so both fresh and upgraded SQLite/Postgres-compatible schemas converge.
Rate coordination is process-local and reconstructs from response observations;
the persisted Workflow Sync next-attempt time prevents restart retry storms.

## Security

Quota keys and snapshots contain no bearer credentials. MCP access follows the
current task-to-workspace authorization boundary and cannot inspect an
arbitrary workspace. Logs use failure kinds, retry sources, workspace IDs, and
non-secret principals only.

## Observability

Classification and scheduling transitions emit structured fields. A suspended
workspace logs once when it enters suspension; skipped poll ticks are silent.
The rate snapshot exposes observation time and retry source so operators can
distinguish provider data from Kandev estimates.

## Related decisions

- [Separate GitHub deployment, workspace automation, and personal identities](../../../decisions/0047-github-authentication-ownership.md)
- [Coordinate GitHub rate state by provider principal](../../../decisions/2026-08-29-github-provider-rate-coordination.md)
