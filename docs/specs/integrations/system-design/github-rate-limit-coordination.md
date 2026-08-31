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
and persists its own scheduling state. Rate snapshots stay inside the provider
coordinator. Failed managed operations return safe rate details in their own
response.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-RATE-001` | [Response classification](#response-classification) |
| `REQ-INTEGRATIONS-GITHUB-RATE-002` | [Principal-wide coordinator](#principal-wide-coordinator) |
| `REQ-INTEGRATIONS-GITHUB-RATE-003` | [Workflow Sync recovery](#workflow-sync-recovery) |
| `REQ-INTEGRATIONS-GITHUB-RATE-004` | [Operation-local failure contract](#operation-local-failure-contract) |

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
block both classes without occupying an execution slot while waiting. Automatic
Workflow Sync admission failures remain in the scheduler's pending queue and
are requeued by the admission-change signal or retry deadline; REST-only sync
uses the Core resource and does not wait on GraphQL or Search state.

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

## Operation-local failure contract

`GitHubAPIError` carries the classified failure kind, rate resource, retry
boundary, retry source, and parsed primary snapshot. `AdmissionDeferredError`
carries the local admission reason, resource, retry boundary, and source.

An operation converts these fields to a safe response only when a rate limit
causes that operation to fail. The public rate object uses
`primary_exhaustion`, `secondary_throttle`, or `interactive_reserve`. It also
returns `core`, `graphql`, or `search`, plus the retry time, delay, and source.
Successful operations omit the object.

Manual Workflow Sync returns this object beside its existing error and config.
Automatic Workflow Sync stores its error class and next attempt. The scheduler,
telemetry, and logs can read coordinator state without an agent-facing MCP tool.
Direct `gh` commands from an agent shell stay outside this response path.

## Persistence and migration

Workflow Sync adds columns through the package's existing idempotent ALTER
pattern so both fresh and upgraded SQLite/Postgres-compatible schemas converge.
Rate coordination is process-local and reconstructs from response observations;
the persisted Workflow Sync next-attempt time prevents restart retry storms.

## Security

Quota keys and operation error details contain no bearer credentials. Responses
exclude provider bodies and coordinator snapshots. Logs use failure kinds,
retry sources, workspace IDs, and non-secret principals only.

## Observability

Classification and scheduling transitions emit structured fields. A suspended
workspace logs once when it enters suspension; skipped poll ticks are silent.
Failed operations label the retry source so callers can distinguish provider
data from Kandev estimates.

## Related decisions

- [Separate GitHub deployment, workspace automation, and personal identities](../../../decisions/0047-github-authentication-ownership.md)
- [Coordinate GitHub rate state by provider principal](../../../decisions/2026-08-29-github-provider-rate-coordination.md)
