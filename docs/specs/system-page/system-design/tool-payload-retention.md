---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
---

# Tool Payload Retention System Design

## Presentation allocation

The [settings storage tabs design](../../system-page/system-design/system-data-storage-pages.md)
defines the shipped page allocation. Office retention lives at Storage > Office retention.
Message compaction remains in Data & Logs > Database.
Policy, API, and persistence contracts remain unchanged.

## Purpose and boundaries

This capability belongs to system-page. It uses task persistence to
remove selected payload fields and retains the original conversation structure.
The [requirements](../requirements/tool-payload-retention.md) define the outcomes.

The Database tab of `DataLogsSettings` mounts `DatabaseStatsCard`,
`ToolPayloadRetentionCard`, `BackupsTable`, and `LogViewer`. Office run history
is owned by the separate Storage > Office retention tab through contributor
`system:retention`. The tool payload card uses a separate policy and save contributor.

The [Data & Logs route design](system-data-storage-pages.md) describes the route
split and the separate save contributors for database retention settings.
The [database footprint design](storage-database-footprint.md) remains a metadata
scan. Opening either settings route must not scan message bodies.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001 | Policy, Eligibility, Analysis, Settings surface |
| REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002 | Removal rules, Preparation, Transaction and replay safety |
| REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003 | Scheduling, Failure and observability, Security |

## Components and responsibilities

`internal/system/toolretention` owns validation, analysis jobs,
preparation, scheduling, and bounded status persistence. It is composed through
`internal/system/system.go` and backend startup/shutdown wiring.

`internal/task/repository/sqlite` owns candidate queries and guarded message
updates. A shared reducer in `internal/task/models` defines removal rules.
`internal/task/service/service_messages.go` and repository write boundaries
prevent replacement updates from restoring removed fields.

Existing `internal/system/jobs` provides job notifications. It is in memory,
so durable preparation and committed progress need their own bounded records.
Existing `internal/system/backups` owns snapshot files through
`persistence.SnapshotSQLite`. That helper copies and checks file size, but does
not run an integrity check. Preparation adds verification of the new snapshot
through a separate read-only connection. Database maintenance owns explicit `VACUUM`.

## Policy and persistence

Use a dedicated settings key `tool_payload_retention`, version 1:
`enabled=false`, `age={value:3,unit:"months"}`, and a monotonic `revision`.
The units are `weeks` and `months`. Reject invalid ranges and unknown fields.
Never derive enablement from Office or filesystem maintenance settings.

A singleton runtime record stores preparation state, approved policy revision,
backup receipt or explicit skip receipt, next due time, cursor, and last result.
Retain only the latest result and active operation, not an unbounded history.
The policy and runtime state share one versioned JSON record in the existing
settings key; no new table or index is required. Policy updates use
compare-and-swap within the writer transaction. A stale
revision returns conflict and does not overwrite another administrator's choice.

No message rewrite or full-table backfill occurs during migration. Missing
settings mean disabled. Malformed settings block deletion and surface an error.
SQLite owns the initial implementation. PostgreSQL reports unsupported before
any database-specific query or mutation.

## Eligibility

The age basis is task inactivity. Capture one UTC cutoff per pass.
Weeks subtract seven days each. Months subtract calendar months and clamp the
day to the last valid day in the destination month. Equal timestamps are retained.

Compute task activity as the latest task, session, turn, or message activity
timestamp. Task/session updates conservatively extend protection, even for a
rename. This avoids a new activity ledger and protects resumed tasks.
Validate each relevant timestamp and compare it chronologically with the cutoff.
Use the existing session created/updated indexes for covering message scans.
Do not compare timestamp text: legacy offsets can change lexical ordering.
Payload maintenance itself must not update conversation activity timestamps.

Require every session to be terminal: `COMPLETED`, `FAILED`, or `CANCELLED`.
Reject tasks with nonterminal turns, queued messages, queued workflow admission,
pending input/permission, live execution, or pending lifecycle work. An unknown
state, missing ownership, or invalid timestamp makes the task ineligible.
Use authoritative task/runtime admission state, not browser presence or process
absence. The runner never stops a task to make it eligible.

## Removal rules

`streams/tool_payload.go` defines normalized kinds. Use a JSON reducer that
retains unknown fields with `json.RawMessage` and preserves numeric precision.
Only supported tool message types with matching normalized kinds qualify.
The reducer removes the following paths beneath `metadata.normalized`:

| Kind | Removed fields | Retained examples |
| --- | --- | --- |
| `shell_exec` | `shell_exec.output.stdout`, `.stderr` | command, work directory, exit code, truncation |
| `generic` | `generic.input`, `.output` | tool name |
| `read_file` | `read_file.output.content` | file path, range, line count |
| `modify_file` | each mutation's `content`, `old_content`, `new_content`, `diff` | file paths, mutation types, line numbers |
| `code_search` | `code_search.output.files` | query, scope, file count |
| `http_request` | `http_request.response` | URL, method, error status |

Also remove `metadata.result` on these supported rows, because tool updates can
store the result there. Keep message `content` and `metadata.title`: the service
uses them as the tool's display title. Commands and titles can contain large
inputs, but removing them is outside these rules. Estimates reflect this limit.

Preserve permission rows, plans, todos, task creation, subagent records,
attachment metadata, entity references, and provider provenance. Skip normalized
payloads with `background_work` or `monitor` provenance in the first version.
Generic results can include plan or attachment data. Exclude recognized control
tools and structured results with plan, task, attachment, or resource contracts.
The implementation must ground these exclusions in existing MCP registries and
adapter shapes. Unknown result envelopes remain intact and count as skipped.

For a changed row, add `metadata.payload_retention={version:1,removed_at,...}`
with removed field paths and net byte reduction. Store no removed text or hash.
Skip no-op transformations and rows whose marker makes the result larger.
Analysis and cleanup use this exact reducer and serialization behavior.

## Analysis and API contracts

Admin routes below `/api/v1/system/database/tool-payload-retention`:

| Method and suffix | Contract |
| --- | --- |
| GET root | Authorized read: policy, revision, capability, preparation, latest analysis/run, next due time |
| PUT root | Admin: validated draft and expected revision, plus preparation choice when required |
| POST `/analyze` | Admin: draft age, returns 202 with operation ID |
| POST `/run` | Admin: expected enabled revision, returns 202 with operation ID |
| POST `/cancel` | Admin: active operation ID, cancels at a batch boundary |

Status polling reads bounded records only. The existing job event channel sends
operation ID, state, and progress without payload bodies or task titles.
Use 400 for invalid input or a missing backup choice, 403 for denied actions,
409 for conflicts or unapproved cleanup, and 501 for unsupported engines. Duplicate active requests return
409 conflict; there is no idempotency-key contract.

Analysis records its draft fingerprint, cutoff, start/end time, scanned rows,
eligible tasks/messages, skipped reasons, and net payload bytes. The scan is
not a point-in-time snapshot: label the result an estimate as of its scan window.
Partial results include progress, never a complete-looking total. Policy edits
invalidate the displayed match. A result older than 24 hours is marked stale.
Activation can proceed without analysis, with an explicit 'Not analyzed' state.

Use keyset pagination over task/session IDs and each session’s message rowid,
with captured upper cursors. Store SQLite schema_version with the pass. A
compaction or schema change ends an unfinished pass as partial, preserving
committed counts; a later pass starts with fresh cursors. Ordinary restarts
resume the stored cursor. No `OFFSET` over growing messages. Release each reader before
the next batch. Start with at most 100 rows and 8 MiB of payload per batch,
a 200ms statement timeout, and a 100ms yield between batches. Read stored lengths
before fetching bodies, so the driver cannot materialize an oversized page.
A single larger
row counts as one batch, up to a 32 MiB row limit. Larger rows remain intact
and appear as oversize skips. Limits are internal, not settings-page controls.

A pass has a 30-second work budget and persists its cursor. Continuations wait
at least one minute, until complete or canceled. The scan must not retain a
reader transaction between pages. Synthetic scale tests must verify these bounds
and indexed eligibility probes. An eligibility query that exceeds 200ms
retains that task, records an eligibility_budget skip, and makes the pass partial.
This prevents a very large task from blocking all later candidates. The next
daily pass can retry it; no removal estimate is claimed for that task.

## Preparation and transaction safety

First enablement stores a pending revision and an explicit backup/skip choice.
It does not arm deletion yet. A pending policy change pauses cleanup, including
when the previous policy was enabled. Preparation runs after readiness. A selected
backup must finish `SnapshotSQLite`, read-only `PRAGMA integrity_check` on the
new snapshot, and final file publication. Verification does not scan the live DB.
The receipt records filename, completion time, byte size, and SHA-256.
Publish the snapshot atomically from a private staging directory. Before the
first mutation, verify the receipt under the shared maintenance lease. Read-only
probes advance empty batches without repeatedly hashing the entire backup.

On backup failure, remain blocked. Retry creates a new preparation operation.
Cancel checks operation identity; disable invalidates the policy revision.
Neither permits a late backup completion to arm cleanup. Validate requests
through the reader before interrupting matching preparation work: a snapshot
can hold the writer connection needed to persist cancellation. Repeat identity
and revision checks in the writer transaction before saving the result.
Keep a completed backup under existing manual-backup retention rules. The
snapshot helper accepts cancellation for this preparation path. Accepted manual
backup, restore, reset, vacuum, and optimize jobs detach from HTTP request
cancellation so the 202 response cannot cancel their work.
Do not promise that a backup shrinks total storage or is a per-message undo.

After preparation succeeds, commit enablement and start the initial cleanup.
Record approval before deletion so restart cannot invent a skip choice. A crash
during backup requires retry or verified receipt reconciliation. A crash after
approval can resume guarded cleanup. After a full disable, re-enabling repeats
the review. Shortening the retention period also repeats preparation.

For each cleanup batch, acquire the SQLite writer transaction before validating
the policy revision, approval, activity, and pending-work gates. Task admission,
message writes, and cleanup serialize through that writer. Update only rows
whose observed metadata still matches. Recompute or skip conflicts.
Persist cursor and committed counts in the same transaction as the changes.
Disabling commits through the same writer, so subsequent batches see it.

Repository replacement writes must preserve an existing removal marker and
reapply its field removal rules. A service-only check is insufficient: a stale
read can race cleanup. Inspect all metadata replacement and fallback-create
paths, including turn recovery. Keep tool-call identity to preserve deduplication.
New tool calls after resumption remain normal messages.

The shell output endpoint checks the marker before returning output. Return
410 with code `tool_payload_removed`, removal time, and retained summary.
Client reducers and lazy-output caches must discard removed details when the
new marker arrives. Emit changed message identities through the established
message-update path after commit. Conversation export uses the same marker.

## Scheduling, failure, and observability

One runtime worker serializes analysis, preparation, and cleanup. Manual jobs
take priority over the next scheduled pass. Reuse lifecycle patterns from
`internal/system/storage/scheduler.go`, without sharing its enable switch.
Persist the next due time, normally 24 hours after a complete cleanup pass.
An overdue job resumes after readiness with a one-minute delay, not a catchup burst.

Add a shared maintenance admission guard around backup, restore/reset, and
compaction entry points. Payload batches defer while those operations own it.
The initial snapshot owns the guard for copy and verification, then releases
it before cleanup. This guard is independent of the job tracker.
Restore/reset quiescence stops the retention worker before closing or replacing
its database. Office writes remain governed by short SQLite transactions.

Each batch rereads policy and preparation. Cancellation, policy changes,
shutdown, or contention stop at the next boundary. A statement deadline rolls
back its batch. Five consecutive batch failures end the pass as partial or failed. A later
cleanup starts a fresh pass with fresh eligibility checks. Task eligibility
deadlines instead skip that task and mark the result partial.

Report committed message counts and payload bytes, skipped reasons, pass state,
last error code, and last/next times. Interrupted work remains distinguishable
from success. Logs contain operation IDs and counts, never payload bodies.
No new unbounded telemetry table is needed.

Cleanup frees database pages for reuse but does not run `VACUUM`. The existing
database card supplies an explicit compaction action and its before/after result.
Payload bytes are not a prediction of compacted file bytes or backup duration.

## Settings surface and security

### Status error recovery

`useToolPayloadRetention` keeps status-read errors separate from action errors.
A current successful GET clears only the status-read error. An action error
takes display precedence and survives successful background reads. Explicit
Refresh status and a new user action retain their existing dismissal behavior.
Persisted operation failures remain part of the returned status.

The existing lifetime epoch and mutation generation checks apply to both error
channels. A stale GET cannot clear a newer error or overwrite a mutation result.
Polling continues at the existing cadence without automatic mutation retries.
The hook keeps its outward `error` contract for `RetentionError`.
Draft ownership stays in `useToolPayloadRetentionDraft`.

The [platform health design](../../platform/system-design/postgres-domain-store-parity.md#sqlite-maintenance-coordination)
owns maintenance admission and readiness. The card does not infer database
health from a local vacuum button state or suppress all HTTP 503 errors.
Existing desktop and phone composition, focus behavior, and translated copy
remain unchanged. The [fix package](../../../plans/vacuum-compaction-status/plan.md)
defines regression coverage and the recovered-state preview.

### Controls and permissions

Add `ToolPayloadRetentionCard` beside existing retention settings, with its own
`system:tool-payload-retention` save contributor. Reuse draft/save conflict
handling, job status, backup list, and `useIsAdmin` conventions.
Saving an enablement draft requires an explicit inline backup choice. A save
with pending backup means 'Preparing', not 'Enabled'. Disable is always available.

The card is titled Messages compaction. Control order: description, compact age
controls and Analyze savings, estimate, Automatic compaction switch, backup
review, then last/next run and Compact messages now. Place the switch beside its
label. Show the 24-hour check frequency and that Kandev must be running. Keep
scan counts, skipped reasons, and scan windows in expandable analysis/run
details; show outcomes, timestamps, partial results, and errors outside them.
Bound the card width on desktop. Backups and database compaction remain
reachable in their existing sections. Do not expose batch limits or SQL paths.
The [plan previews](../../../plans/tool-payload-retention/plan.md#ascii-ui-preview)
define desktop/phone structure and material states.

Use the existing settings route scroll owner. Phones keep the number and unit side by side, with the
analysis action below them. Use 44px touch targets and safe-area clearance for the shared Save bar.
No nested dialogs or hover-only actions. Errors use focusable inline summaries.
Use `t()` for all states in en, pt-pt, zh-cn, zh-hk, and zh-tw.

Apply existing installation read permissions and admin mutation middleware on
the server. Analysis requires admin because it triggers a costly scan. Neither
analysis nor status returns raw payloads, credentials, or cross-workspace titles.

## Related decisions

- [Tool payload removal policy](../../../decisions/2026-09-14-tool-payload-removal-policy.md)
- [Installation storage maintenance](../../../decisions/0045-install-wide-storage-maintenance.md)
- [Settings save coordinator](../../../decisions/0046-settings-route-save-coordinator.md)
