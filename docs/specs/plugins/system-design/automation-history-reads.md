---
status: current
system: plugins
created: 2026-10-29
requirements:
  - REQ-PLUGINS-AUTOMATION-READS-001
---

# Plugin Automation History Reads System Design

## Purpose and boundaries

Add three optional Host reads for automation definitions and run observations.
The plugin system owns the public DTOs, transport, authorization, and snapshots.
The existing automation service owns source queries and status projection.
Consumer policy remains outside core.

This design follows [ADR 0043](../../../decisions/0043-plugin-host-data-api.md)
and the accepted
[generic coordination boundary](../../../decisions/2026-09-25-plugin-coordination-platform.md).
It extends their existing read pattern without creating a new architecture
decision. The current-version API is implemented; the stable-version backport
remains separate scope.

## Requirement mapping

All criterion suffixes refer to `AC-PLUGINS-AUTOMATION-READS-001`.

| Requirement                        | Criteria | Design sections                                      |
| ---------------------------------- | -------- | ---------------------------------------------------- |
| `REQ-PLUGINS-AUTOMATION-READS-001` | .1-.4    | Public contract, Source observations                 |
| `REQ-PLUGINS-AUTOMATION-READS-001` | .5-.6    | Bounded snapshots                                    |
| `REQ-PLUGINS-AUTOMATION-READS-001` | .7-.9    | Authorization, Safe projections                      |
| `REQ-PLUGINS-AUTOMATION-READS-001` | .10-.12  | Persistence and recovery, Compatibility and failures |

## Current source and reusable components

- `automation.Service.ListAutomations` reads a workspace's definitions.
- `automation.Service.ListRunPage` returns per-automation pages. The store
  limits each source page to 200 rows.
- `automation.Service.ListWorkspaceRuns` returns a capped recent feed. It is
  not a complete history reader.
- `automation.Store.GetRun` returns a stored run. Existing run lists also
  derive task state through `runTaskStateColumnsSQL` and hydrate summaries
  through `hydrateRunSummaries`.
- `plugins.exactReadAdmission`, `exactPage`, and `singleReadReceipt` provide
  current authorization, immutable pages, and observation receipts.
- `plugins.exactReadSnapshotStore` bounds snapshots by row count, bytes, age,
  and count. It already binds cursors to `exactReadIdentity`.
- `backendapp.managedConversationScheduleAdapter` demonstrates service wiring
  across the automation/plugin import boundary.

The new source reader must not wrap a single capped recent-runs call and claim
that it exhausted history. It must not change raw `Store.GetRun` semantics for
existing execution and retry callers.

## Public contract

Add an optional `AutomationHistoryHost` interface in the standalone `pluginsdk`.
Its `AutomationHistory()` method returns an `AutomationHistoryReader`.
The discovery helper is `HostAutomationHistory(host)`.
The existing `Host` and `ExactQueryManager` interfaces remain unchanged.

| Reader operation  | New wire RPC                     | Request                                                   | Response                                        |
| ----------------- | -------------------------------- | --------------------------------------------------------- | ----------------------------------------------- |
| `ListAutomations` | `ListAutomationDefinitionsExact` | Request ID, workspace, definition filter, `ExactReadPage` | Definition observations and `ExactReadPageInfo` |
| `ListRuns`        | `ListAutomationRunsExact`        | Request ID, workspace, run filter, `ExactReadPage`        | Run observations and `ExactReadPageInfo`        |
| `GetRun`          | `GetAutomationRunExact`          | Request ID, workspace, run ID                             | Run observation and `HostReadReceipt`           |

Definition filters contain an optional automation ID and optional creation-time
bounds. Run filters contain an optional automation ID and optional
creation-time bounds. Bounds use UTC RFC3339 timestamps. `created_from` is
inclusive, and `created_before` is exclusive. Reversed or malformed ranges
return `InvalidArgument`. Omitted bounds select all retained matching records,
subject to the snapshot limits.

Filters do not include mutable status fields in this version. Definition order
is `(created_at ASC, id ASC)`. Run order is `(created_at DESC, id DESC)`.
The identity tie-breaker makes equal creation timestamps deterministic.
Every continuation repeats the original normalized filters.

The SDK maps optional fields without replacing absence with invented values.
Identifiers remain opaque strings. Source times use RFC3339Nano UTC strings.
Run status fields remain strings so unknown source values survive transport.

### Definition observations

An observation contains:

- Automation ID, workspace ID, name, and enabled state.
- Source creation and update times, an opaque resource version, and observation
  time.
- Trigger ID, kind, and enabled state for each trigger.
- A scheduled trigger's allowlisted `cron_expression` and optional configured
  `timezone`, plus an explicit schedule-availability flag.

Read scheduled fields through the existing scheduled-trigger configuration
shape. Preserve an omitted configured timezone as absent. Do not guess the
scheduler's effective timezone. Unknown trigger kinds retain their identity
and kind without exposing their configuration.

Malformed or unsupported schedule data produces an unavailable schedule
projection. It does not remove the automation from the inventory.
The definition version covers the public definition and trigger projection.

### Run observations

An observation contains:

- Run ID, workspace ID, automation ID, and available trigger ID and kind.
- Optional task, session, turn, and managed-conversation IDs.
- `source_status`, projected `status`, and independent retry and delivery
  states.
- Retry-group ID, parent-run ID, and attempt number when present.
- Source creation time and available retry-scheduling timestamps.
- Bounded summary and error excerpts.
- An opaque projection version and observation time.

The source has no general run completion timestamp or run update revision.
This API does not add either field to storage. Its projection version hashes
the canonical public observation, excluding observation time and receipt data.
It is an equality token, not an ordered revision or mutation precondition.

No success boolean is inferred from task creation, queued input, delivery
acceptance, missing identities, or an unknown status. Task, session, and turn
links are the values recorded on the run: an absent link stays absent, and a
retained link is not cleared because its task row was deleted. The UI
projection's current-session fallback is not historical source identity.
Attempt number is optional for historical standalone runs without a recorded
retry attempt.

## Source observations

Add a narrow observation reader in `internal/automation`, with service-owned
queries and internal result types. The following are planned new symbols:

- `Service.ListHistoryAutomations` with a bounded filter.
- `Service.ListHistoryRuns` with a workspace, optional automation, and time
  range.
- `Service.GetHistoryRun` with an explicit workspace and run identity.

New store queries select at most the snapshot row limit plus one. The extra
row detects an oversized query. They apply the workspace join and requested
filters in SQL before projection. Exact reads select the run through its
owning automation and requested workspace together.

Preserve raw `ar.status` as `source_status`. Derive observed status through the
same automation-owned projection used by existing run lists. Reuse the
task-state precedence, retry cancellation handling, and exact-turn summary
hydration. Both new run-list and exact-read paths use that projection.
Existing raw reads and execution writers retain their behavior.

Use an adapter in `internal/backendapp` to implement the new plugin-owned
`AutomationHistoryDataSource` interface. `internal/automation` already depends
on plugin contracts. Importing automation models into `internal/plugins` would
create a cycle. The adapter maps source observations to the explicit public
allowlist. It never exposes source structs or a repository to plugins.

Add `Service.SetAutomationHistoryDataSource` in `internal/plugins` and read the
dependency through a live getter. Wire it next to
`SetManagedConversationSchedules` in `backendapp/services.go`. Existing Host
instances must observe late wiring. The reader must work without enabling
Office or Coordinator product features.

## Bounded snapshots

Reuse `ExactReadPage`, `ExactReadPageInfo`, `exactReadAdmission`, and `exactPage`.
Do not add another cursor format or expand the shared snapshot limits.
Current bounds are:

- A normalized page size of at most 200 records.
- At most 5,000 records or 8 MiB in one snapshot.
- A five-minute snapshot lifetime and at most 32 snapshots per Host.

Source loaders request at most 5,001 records. If the filtered result exceeds
the row or byte bound, return `ResourceExhausted` before returning a partial
page. A successful empty query returns an empty item list with `has_more=false`.
Only an exhausted successful query means that the selected snapshot is complete.

Consumers can select an automation or a smaller creation-time window after an
oversized query. Automatic partitioning and arbitrarily large inventories are
outside this package. A range that still exceeds the limit remains an explicit
error. Consumers must not claim workspace-wide completeness after that error.

Snapshots freeze the public values loaded for the first page. A later status
change appears through `GetRun` or a new list query, not an existing
continuation. Each method uses its own snapshot. No cross-method or global
database transaction is promised.

The existing identity binds the installation, workspace, method, canonical
filter digest, and approval revision. Snapshot version and cursor identity
complete the binding. Expiry, eviction, or a backend restart requires a fresh
query. Cursors are temporary transport state, not durable consumer checkpoints.

## Authorization

Reuse the manifest resource `api_read: automations` and the workspace grant
`host.v2.read:automations`. Manifest declaration alone does not authorize a
read. The Host derives installation identity from the connection.

Authorize before checking automation dependencies or reading their data.
Apply admission on every page and exact read, including cache hits.
An approval revision change invalidates the prior snapshot identity.
Revocation denies the continuation even while its snapshot remains cached.

An optional automation filter must name an automation in the selected
workspace. Unknown and foreign identities return the same `NotFound` outcome.
The same rule applies to exact run identity. No task/session/message capability
is implied by returned links. Those resources retain their own read gates.

The existing managed-conversation schedule methods remain installation-owned.
This new reader uses the explicitly approved workspace automation resource.
It adds no schedule mutations or automatic approval.

## Safe projections

Public DTOs contain named fields only. Exclude arbitrary configuration maps,
trigger bodies, headers, prompts, webhook secrets, dedup keys, and claim tokens.
Exclude retry launch snapshots and private installation metadata.

For summaries, strip system content through the existing `sysprompt` helper.
Use `securityutil.SanitizeCredentials` for summary and error excerpts. That
helper redacts credential patterns and limits UTF-8 output to 4,096 bytes.
Document excerpts as excerpts, not full transcripts or raw provider errors.
History summary hydration reads a bounded 4,096-character source prefix so a
complete system block is normally removed before the final excerpt bound; if a
prefix still contains an opening or closing system tag, the excerpt is cut at
that tag and fails closed. A credential-shaped suffix that a bounded prefix cut
before the normal redaction minimum is masked. Sanitize text before the final
bound so truncation cannot expose a credential fragment. Preserve opaque IDs
outside the text sanitizer.

Foreign keys remain identity links, not authority. Do not fetch transcripts,
secrets, provider resources, or task details while building the projection.

## Compatibility and failures

Add RPCs to `kandev.plugin.v1.Host` without changing the handshake or shipped
field numbers. Regenerate the Go protobuf and gRPC artifacts through
`make -C pluginsdk proto`. A new optional Go interface avoids adding methods
to existing Host implementations or the existing exact-query manager.

Register all three methods in current capability discovery. `supported`
reflects the presence of the backing reader. `authorized` reflects current
workspace approval. Missing dependencies never advertise a working reader.

A local SDK helper can detect an optional interface. A remote wrapper cannot
prove that an older server implements its RPCs. Consumers use capability
discovery and also handle `Unimplemented` from a call. They never substitute
private WebSocket routes or database reads.

| Condition                                | Outcome                                                     |
| ---------------------------------------- | ----------------------------------------------------------- |
| Invalid IDs, ranges, or cursor syntax    | `InvalidArgument`                                           |
| Missing, stale, or revoked authority     | Existing authorization outcome, with no source read         |
| Foreign or missing target                | `NotFound` with equivalent public detail                    |
| Expired, evicted, or mismatched snapshot | Existing `FailedPrecondition` or cursor validation outcome  |
| Snapshot row or byte limit exceeded      | `ResourceExhausted`                                         |
| Old Host or missing optional reader      | `Unimplemented`, with unsupported discovery where available |
| Backing source read error                | `Unavailable` with sanitized public detail                  |

SDK release numbers and Host release baselines follow the existing
[distribution contract](../requirements/sdk-distribution.md).
Do not invent a released version or claim registry availability during this
package. A `v0.98.0` backport needs a separate plan against that release's
backend-owned SDK layout.

## Persistence and recovery

The change adds source reads, public DTOs, and transient snapshots. No schema
migration, new durable queue, or domain write is required.
Source retention remains authoritative. Deleted records can disappear from
fresh queries and exact reads. Existing snapshots retain their frozen values
until expiry or revocation.

A consumer restarts by discovering support and current authority, then creating
new bounded queries. It persists run identities that need later refresh.
Neither creation-time filters nor events can discover every later status
change. Events remain optional wake hints.

## Observability

Use existing request logging and read receipts. Log method, result, duration,
and bounded request/receipt identifiers. Do not log observation contents,
cursor contents, credentials, or raw dependency errors. No new metric family
or identifier-valued metric labels are required.

## Validation

Source tests use real service/store fixtures. Cover scope, equal timestamps,
time bounds, source/observed status, exact-turn summaries, taskless/skipped
runs, retry links, and unchanged domain rows. Run dialect-sensitive queries
against the existing environment-gated PostgreSQL fixture.

Host tests use the real plugin broker path for authorization and wire evidence.
Cover missing grants, foreign IDs, revocation between pages, and invalid or
expired cursors.
Cover changed source values after a snapshot, row/byte limits, secret omission,
and missing dependencies.
Independent SDK consumer tests cover old Hosts and public-only imports.

No rendered UI changes require browser tests. Public consumer behavior needs
SDK-to-Host transport evidence, not an artificial Playwright scenario.

## Implementation plan

- [Plugin automation history reads](../../../plans/plugin-automation-history-reads/plan.md).
