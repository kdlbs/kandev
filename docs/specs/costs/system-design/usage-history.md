---
status: draft
system: costs
requirements:
  - REQ-COSTS-USAGE-HISTORY-001
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-004
  - REQ-COSTS-USAGE-HISTORY-005
  - REQ-COSTS-USAGE-HISTORY-006
  - REQ-COSTS-USAGE-HISTORY-007
---

# Retained Usage History System Design

## Purpose and boundaries

The costs system owns retained observations, source selection, summaries, and accounting queries.
Tasks own execution identity and task lifecycle. Auth and organizations own identity and access policy.
Plugins supply measurements through the supported Host API, without database access.
The [retention decision](../../../decisions/2026-10-28-retained-usage-history.md) records this boundary.
The additive SDK contract and retained-source boundaries define this design.
The work package records implementation and verification status separately.

The [Token Usage design](token-usage.md) remains the collection and presentation baseline.
This design replaces its live-task storage dependency and archive-wide read-time reconciliation.
Existing task ledger APIs and session rollups keep their original arithmetic and task-scoped lifetime.
Existing explicit workspace and organization erasure remains authoritative.
This package adds no org-wide visibility right or new release flag.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-COSTS-USAGE-HISTORY-001 | Retained identity and lifecycle |
| REQ-COSTS-USAGE-HISTORY-002 | Ownership and authorization |
| REQ-COSTS-USAGE-HISTORY-003 | Measurement contract |
| REQ-COSTS-USAGE-HISTORY-004 | Source identity and indexed selection |
| REQ-COSTS-USAGE-HISTORY-005 | SQL reads and performance evidence |
| REQ-COSTS-USAGE-HISTORY-006 | Canonical facts and daily summaries |
| REQ-COSTS-USAGE-HISTORY-007 | Native admission, Migration and recovery, Presentation |

## Current evidence and integration points

| Existing boundary | Required change |
| --- | --- |
| `analytics/repository/sqlite/usage_schema.go` | Remove live-task ownership from retained observation lifetime |
| `analytics/repository/sqlite/usage.go` | Replace broad reads and global nested overlap loops |
| `task/repository/sqlite/usage_events.go` | Capture normalized native usage inside its accepted-event transaction |
| `task/usage/writer.go` | Carry trusted scope and actor evidence through the existing single writer |
| `task/repository/sqlite/task.go` and task cleanup services | Preserve legacy data before cascade and detach retained live links |
| `internal/analytics/{models,repository,service}` | Own retained identities, revisions, projection work, and queries |
| `internal/plugins/host_data.go`, root `pluginsdk/usage_history*.go`, `pluginsdk/proto/kandev/plugin/v1/plugin.proto` | Expose versioned generic measurements, segments, and retained bindings |
| `internal/org`, `backendapp/orgs.go`, and task workspace-deletion services | Include retained scopes in suspension, erasure, and no-rows-left contracts |
| `apps/web/app/stats/token-usage-*` | Render retained attribution and canonical summary state |
| Sibling Session Cost `server/{plugin,collector,import}.go` | Supply normalized observations through the generic contract |

## Retained identity and lifecycle

Add `usage_scopes` and `usage_subjects` under the accounting repository.
A scope captures immutable org and workspace identity plus historical access provenance.
A subject identifies a task/session incarnation without depending on the corresponding live row.
Retain original opaque IDs for grouping. Store nullable live references only for navigation.
Never use a nullable live reference as the deduplication key.
Task-ID reuse creates a distinct subject through the existing task/session incarnation evidence.

Extend `session_usage_measurements` and `session_usage_buckets` with subject and scope references.
These tables remain the source-observation store. Do not add another copy of all external observations.
Keep one latest accepted revision per source record and representation, not one row per poll.
Native event observations use the dated table with an event identity and exact occurrence point.
External dated observations retain an interval and calendar. Lifetime snapshots remain a separate representation.
Foreign keys reference retained subjects. Deleting a live task only clears navigation links and records deletion state.

Task archive changes no accounting amounts or query eligibility.
Task deletion preserves legacy measurements and native events before their live rows disappear.
The deletion operation uses its existing durable cleanup/admission flow for bounded preservation batches.
Final cascade is admitted only after the preservation watermark covers the task's accepted source rows.
The watermark and final deletion serialize with concurrent source admission in both databases.
A preservation error leaves deletion retryable and retains the original data.
Apply the same preservation fence before session pruning clears references or cascades turn metadata.
Retain session incarnation, turn bindings, and actor evidence before either task or session cleanup can destroy them.

An external transcript can map to several sessions. Core admits one proven work binding or marks attribution ambiguous.
It must not copy the same lifetime amount into each subject.
Optional final transcript collection can improve coverage before resource cleanup.
It cannot guarantee usage that no source supplied, or block deletion indefinitely on an unavailable tool.

Workspace erasure removes its retained dataset through the existing authorized deletion operation.
Register every retained table and projection checkpoint with org erasure and E2E reset.
Org erasure queries captured org ownership, including historical scopes whose live workspace links changed.
Add an explicit costs-service erasure operation to the current org/workspace deletion seam.
Live-workspace deletion alone cannot erase historical captured scopes.
Fence admission, correction, backfill, and every projector generation before purging retained org scopes.
Workers must observe the erasing scope and cannot recreate observations, dirty work, or summaries after purge.
Include inactive generations and migration checkpoints in the final no-rows-left assertion.
There is no unowned fallback scope after erasure.
User removal can retain an opaque historical actor reference without retaining profile text.
Historical references confer no access to a deleted account.

Four additional tables are registered with the required-store catalog and accounting lifecycle:

| Table | Purpose and lifetime |
| --- | --- |
| `usage_collection_current_targets` | Indexed current bindings for retained subjects. Scoped accounting erasure removes these rows. |
| `usage_collection_source_guards` | Source/scope hashes and mixed/erased flags. These denial controls survive normal accounting erasure. |
| `usage_collection_guard_erasure` | Scope-bound sequence checkpoint for bounded guard capture before binding erasure. |
| `usage_collection_guard_workspaces` | Workspace/source hash membership for isolated fixture reset and shared-source protection. |

The hash guards contain no measurements, actor text, or collector payloads.
They prevent a later session or tenant from adopting previously foreign or erased source history.
They are privacy controls, not retained accounting amounts or authority to read erased history.
The gated E2E reset advances the workspace fence, clears fixture accounting state, then reopens the workspace.
It removes that workspace's guard membership and removes a source guard only when no other workspace still references it.
The reset route exists only with `KANDEV_MOCK_AGENT=true` or `only`.

## Ownership and authorization

Derive org identity from the validated workspace/execution scope, never an observation's claimed org.
Capture actor identity at execution or turn admission, before provider usage arrives.
Add durable `usage_execution_bindings` at authenticated root-turn dispatch or executor admission.
The key includes session incarnation, logical turn/prompt generation, and execution-admission generation.
An explicit authenticated turn actor takes precedence over an execution actor fallback for that admitted generation.
Child observations inherit the originating turn binding. Resume replay retains its original binding.
Automation records a supported service actor or unknown state, not an invented human user.
Publish the binding ID through the existing usage payload and resolve it in the retained writer.
Do not reread `ExecutorActorUserID` later: executor admission can replace that current-session field.
Bindings survive live deletion and are included in explicit scope erasure.
The refresher, plugin process, assignee, and workspace owner are not actor fallbacks.
An aggregate spanning several actors remains mixed unless exact per-actor coverage exists.
Unknown historical actor information stays unknown.
Token and cost attribution can differ when a collector supplies aggregate cost beside attributable native tokens.
Keep each metric's actor evidence separate. Never allocate aggregate cost proportionally across users.
Workspace cost can remain known while per-user cost remains unavailable.

Capture aggregate-source scope evidence at session creation, independently of discovery metadata.
Discovery cannot manufacture creation evidence from the current workspace owner.
Only explicit `SourceScopeStatus=eligible` permits aggregate collection. Mixed, unknown, and absent status fail closed.
Transfers, copied contexts, and erased-source guards can withhold source context or reject an aggregate even with a fresh binding.
A prior discovery actor snapshot does not authorize attribution of a later cumulative aggregate.
Resolve aggregate actors from immutable core evidence for the reported coverage. Preserve exact native event actor bindings separately.

Use existing Auth, MultiTenancy, membership, unit-reach, suspension, and operator-tier policy.
Synthetic single-user identity remains valid when Auth is off. Org identity can remain absent in that mode.
Feature-flag changes do not rewrite historical evidence.
The existing first-user/tenant migration can claim a scope only with its authoritative ownership evidence.
Do not assign an actor to old totals merely because that user owns the workspace today.

Authorize retained scopes independently of live task joins.
For normal workspace reads, validate current access and restrict rows to the matching retained tenant scope.
Cross-org workspace transfers must not reveal the previous org's history to the new org.
Existing org policy, not a new analytics bypass, determines access to historical scopes.
An unresolved scope fails closed. No global fallback query is allowed.
No new organization-wide dashboard or cross-user role is part of this package.

## Measurement contract

Add a versioned retained-observation form through the existing capability-gated Host service.
Use additive fields or methods. Never change an existing proto field number or scalar meaning.
The contract contains:

| Group | Fields and semantics |
| --- | --- |
| Binding | Core-issued subject binding, validated workspace, schema version |
| Provenance | Authenticated collector namespace, immutable observation item ID, source record, tool version, revision, payload digest |
| Underlying work | Provider thread/event/transcript identity with a declared identity scheme |
| Segments | Optional stable decomposition and member IDs for explicitly disjoint source records, requiring report schema 2 |
| Dimensions | Exact model ID, normalized provider, optional agent client and serving platform, raw source labels |
| Tokens | Nullable input/output/cache-read/cache-write/reasoning/total plus token-semantics version |
| Cost | Nullable fixed-point amount, currency, reported/estimated/unknown basis, pricing provenance when known |
| Time | Occurrence point or half-open coverage interval, original source date/timezone, collection time |
| Quality | Token and cost completeness, actor attribution, stale state, selected-source provenance |

Initially preserve USD at 10,000 stored units per dollar, matching existing `cost_subcents`.
Other currencies remain distinct and do not pass through a USD-only formatter.
Use checked 64-bit arithmetic internally. Preserve large integers through an additive decimal-string wire representation where needed.
Existing numeric browser fields must never silently round beyond JavaScript's safe integer range.
Subset semantics must identify reasoning-inside-output and cache-inside-input explicitly.
Missing fields, inconsistent totals, invalid intervals, and overflow have explicit validation outcomes.
No adapter infers provider or pricing from an unreviewed model-name prefix.
Persist no prompt, response, source-code, secret, task-title, or personal-name copy for this capability.
Core accepts normalized source measurements without parsing collector-specific JSON or assuming any tool's grouping keys.
An absent token-semantics value remains valid unknown evidence. An explicit source total does not imply unreported subset relationships.

Missing cost remains unknown, independently of known tokens. A selected cost of zero is a measured zero, not missing cost.
Cost provenance exists only when the selected evidence supplies a cost amount.
Do not copy token-source attribution or an unselected candidate's cost into that missing metric.

Older callers keep live-task admission through the existing DTO mapper.
Core resolves a retained binding before storage and marks unsupported new dimensions unknown.
The mapper also enforces source-scope eligibility. Schema 1 remains supported without segments.
Schema 2 is required when a report carries `UsageSegmentIdentityV1`.
Unsupported schemas fail explicitly. Never downgrade a segmented report to schema 1.
Report schema versions are independent of manifest API versions and the existing v1 RPC names.

## Source identity and indexed selection

Collector identity answers who supplied evidence. Work identity answers which consumption it describes.
The Host assigns collector namespaces. Only core can create native provenance or source priority.
Normalize proven underlying identity into a scope-bound work key.
Use provider account/thread/event evidence or a core-approved transcript binding when available.
An identity must not cross orgs merely because two providers use the same thread string.
Aliases require supported evidence. Matching model names alone never establish equivalence.
One proven transcript within a captured scope remains one work family across subjects and collectors.
Distinct proven transcript contexts within one subject remain distinct work.
Unproven or copied lineage does not establish disjoint consumption.

Establish proven work/coverage equivalence before model/provider/semantic partitions can become additive.
Maintain an indexed equivalence/conflict group keyed by scope, underlying work, and coverage identity.
Known and unknown dimension variants of the same proven work enter that group together.
An unallocated unknown-model aggregate cannot be added to its known-model detail as separate consumption.
Conflicting provider or semantic claims remain alternative incomplete evidence unless a supported mapping resolves them.
Schema 2 declares a source-local decomposition through `DecompositionID` and disjoint member lineage through `SegmentID`.
Different declared members can contribute together after work and coverage checks.
For overlapping work and coverage, different collectors' decompositions remain alternatives, even with matching member ID strings.
An unsegmented whole-work aggregate also remains an alternative to its segmented detail.
Select coherent decomposition evidence rather than combining arbitrary members from alternative collectors.

Declared IDs stay stable across subject bindings, lifetime/day views, and supported corrections.
A member names an immutable producer record or a documented immutable group series.
Mutable model/provider labels do not establish new work or prove retirement of an old group.
Without record evidence for a changed group label, the producer quarantines ambiguous replacement membership.
It cannot invent lineage or retire missing groups through a partial report.

Index stable member lineage and coverage before compatible model, provider, and semantics dimensions.
Index exact event keys and coverage intervals inside the affected work group.
Use ordered intervals or a sweep within the affected partition to detect actual overlap.
Use interval predicates and bounded cursor batches even within a long-lived subject's partition.
Large lifetime reconciliation runs as durable work, without an unbounded write transaction or foreground read.
Do not compare unrelated sessions or every workspace row against every other row.
Prefer complete coverage, then trusted source policy for equivalent coverage.
Select token and cost provenance independently only for equivalent coverage.
Retain ambiguous overlap as incomplete without adding an invented remainder.
Keep disjoint proven intervals additive. Keep lifetime and dated representations alternative.
Time overlap within one member does not become additive because its labels differ.
All-time reads choose lifetime or dated coverage independently for tokens and cost, within the selected member or alternative decomposition.
A lifetime cost without tokens cannot suppress dated token evidence. An unknown lifetime cost cannot suppress a known dated cost.
A partial dated import cannot shrink a fuller lifetime total or add the same consumption again.
An incomplete native event set cannot claim complete equivalence with a fuller external snapshot merely through native priority.

A bounded batch transaction validates ownership and atomically compares revision/digest.
Concurrent inserts and updates must return the accepted current row and accurate write status.
An accepted observation enqueues durable dirty-partition work in the same transaction.
Bound batches to 200 observations, as in the current contract.
Immutable observation item identity excludes mutable model/provider labels.
Legacy label-based IDs require explicit same-source supersession or an atomic complete-set replacement with tombstones.
Supersession retires the old model/provider contribution and invalidates both old and new projection partitions.
A missing model in a partial report does not imply retirement or measured zero.
One collector cannot retire another source's evidence.
Corrections after deletion require a pre-existing retained binding and supported write capability.
Arbitrary task strings or collector-supplied tenant IDs never create orphan usage.

Expose resumable collection-target discovery through the usage binding service.
Use an immutable sequence/keyset, an upper snapshot bound, and durable bindings rather than mutable offsets.
Only authorized targets appear. Concurrent deletion does not invalidate already admitted bindings.
Core retains no transcript-access capability beyond the supported executor/source context.

Snapshot import preparation advances one bounded durable step per call.
`Preparing=true` returns no targets, snapshot, or cursor, with zero upper bound and no further page.
The collector repeats the initial query before starting the immutable snapshot scan.
Later additions, deletion, and source updates do not shift that snapshot.

Current eligibility uses `ListTargets.CurrentTarget` with a workspace and at least one exact task/session/source/binding constraint.
Indexed point lookup returns at most one target and requires `CurrentTarget=true` acknowledgement even for an empty result.
Its snapshot and cursor are empty. Its upper bound is zero, and `HasMore` and `Preparing` are false.
An older Host without acknowledgement is unsupported. The consumer must not substitute archive enumeration or cached snapshot eligibility.
Revalidate before a raw source read and before its report. Current authorization and source guards still apply after deletion.
The [authoring contract](../../../public/plugins-authoring.md#retained-usage-collection) records the exact DTO constraints and compatibility behavior.

## Canonical facts and daily summaries

Add `usage_canonical_facts`, `usage_daily_summaries`, and durable projection state.
The current read-time selection becomes a projector for dirty partitions.
Canonical facts identify selected token and cost sources separately, their coverage, and projection generation.
Keep dated and lifetime fact kinds distinct.
Per-subject canonical detail supports task/session reads without scanning native source history.
Shared work with ambiguous ownership remains visible once in workspace totals without an invented task/session allocation.
Reporting cannot discard that amount through a mandatory live-subject join.

Daily summary keys include scope, actor key/status, date/calendar, model, provider, currency, and semantic version.
Retain optional client/platform distinctions when the query exposes them.
Store known sums and missing/partial counters, not a zero substitute for absent values.
Separate token and cost contribution kinds when their actor evidence differs, without duplicating token totals in aggregate actor groups.
Compute weighted effective rates only when the matching numerator and denominator are complete.
Keep a separate lifetime projection for unallocated historical totals.
Never populate a consumption day from a first collection timestamp.

For each changed partition, replace selected canonical facts and rebuild only affected day keys.
An old-to-new contribution change handles downward correction, source replacement, and model corrections.
Do not add an entire cumulative snapshot or subtract an unknown amount as zero.
Index day membership so a day rebuild reads only its affected facts.
Apply fact changes, summary changes, watermark, and dirty-work completion in one transaction.
Serialize competing writers through database revision predicates or row locks, not only a process mutex.

Reads use the last complete generation and disclose projection freshness.
Write responses return accepted observations independently of projector lag.
Saved canonical reads return the last selected generation with a pending indicator when work remains.
Never display an unaccepted submitted amount as canonical current usage.
A first build with no complete generation returns explicit preparing status.
Assign accepted changes an internal monotonic commit sequence, independently of collector revision numbers.
Allocate it under the scope's transactional admission lock with the observation commit.
An allocated database sequence alone does not prove that every lower-numbered transaction committed.
Store replayable upsert/tombstone payloads in the same transaction, including invalidated old partition keys.
Rebuild records a start sequence and stable upper source-ID bound, then scans source rows in bounded keyset batches.
Each copied item carries its accepted sequence. Apply later catch-up only when its item sequence is newer.
Replay every committed change after the start sequence, including items changed or removed before the scan reached them.
This permits bounded scans without a long-lived database snapshot or an inconsistent partial-state switch.
Keep change-log/progress state per projection generation. Active projection cannot consume the shadow generation's catch-up work.
Before cutover, replay changes through a captured sequence and acquire the same projection/admission fence as live publication.
Advance the shadow through that fence, atomically switch generation, then release live publication.
A write racing cutover is included in the switched generation or remains durable work for that generation.
Test a correction, supersession, deletion, and erasure between snapshot copy and generation switch.
Retain change-log entries until every active/rebuild consumer passes them. Then prune only the internal replay log, not usage history.
Crash recovery replays durable work idempotently and reconciles missed work by indexed revision watermarks.

## SQL reads and performance evidence

Push `coverage_start < end AND coverage_end > start` into dated observation/fact queries.
Use occurrence-time predicates for point events and a separate explicit path for undated lifetime totals.
Timezone/calendar matching must precede summary selection.
Native point facts can form exact requested calendar days.
Coarse source-day facts retain their own calendar and cannot be proportionally split across another timezone.
Boundary overlap returns partial coverage under the existing time-attribution contract.

Materialize UTC and supported source calendars in durable daily summaries.
Chart reads aggregate those summaries when the requested calendar matches.
For another IANA calendar, build a bounded list of UTC day intervals using Go timezone rules.
Aggregate timed canonical facts in SQL against those intervals, with indexed start/end predicates.
This fallback does not load observations or reconcile sources again.
Keep long range interval lists in bounded query batches and return every requested period.
Do not create unbounded persistent summary copies for arbitrary browser timezone requests.
Coarse source days remain tagged with their original calendar and boundary completeness.
Include cold-calendar reads, DST, and fractional UTC offsets in the five-year benchmark.
Detail reads use indexed canonical facts, not native-ledger joins or source selection.
Apply provider filters before grouping and database sorting/pagination after grouping.
Compute full filtered totals and available providers from scoped summaries or dimension/existence indexes.
Maintain scoped history/freshness metadata with projection commits, so empty ranges require no archive scan.
Use the existing analytics admission gate and queue-plus-query deadline for all retained reporting paths.
Timeouts return the established `analytics_busy` response, without replacing saved values or starting late uncancelled queries.
Paginate breakdowns and export long trends completely or aggregate explicitly to the requested granularity.
Never silently stop at the default 200 rows.

Reference fixtures contain five years, at least one million native events, and 100,000 external observations.
Include multiple actors/orgs, daily and lifetime records, overlap, corrections, and deleted tasks.
Compare a 30-day read before and after four unrelated years are added.
Record SQLite query plans, PostgreSQL `EXPLAIN (ANALYZE, BUFFERS)`, visited rows, latency percentiles, and peak memory.
Verify that recent charts use date-bounded summaries and no raw-history pairwise pass.
Record hardware and fixture shape. Latency remains measured evidence, not an unsupported capacity promise.

## Native admission

Keep one native writer in `internal/task/usage`.
Extend `CreateTaskUsageEvent` through a narrow transactional costs capability with no task/analytics import cycle.
The accepted event transaction stores the ledger row, existing rollup, retained normalized observation, and durable projection work.
It uses trusted execution/turn binding for actor and scope evidence.
Do not rely on `RecordedObserver`, the event bus, or a later polling tick as the only retained copy.
Existing duplicate-event, session-FK retry, deadlock retry, and shutdown bounds remain intact.
Late events with a valid retained execution binding can persist retained usage after live deletion.
They must not recreate a task, mutate a removed session rollup, or bypass tenant checks.
An event without such evidence remains a reported gap.

## Migration and recovery

Use compatible expansion, bounded backfill, then read cutover.
Avoid full-table updates or archive scans on every startup.
Store migration version, last stable source key, high-water marks, and projection generation durably.
Copy native events and old external observations in bounded, replay-safe batches.
Use the same normalization and idempotency service as live admission.
New writes dual-write while older source ranges backfill.
Deletion preservation and ingestion serialize on a subject admission boundary.
Projection cutover requires verified source watermarks, not merely an empty batch.

Legacy native adoption captures authoritative scope and a source high-water bound before replay continues.
Later migration pages do not derive historical ownership from the workspace's current org.
Missing ownership evidence remains a disclosed gap, including after transfer or erasure.
Gap records alone do not satisfy deletion preservation.
Ordinary task or session cleanup must stop before it removes a native ledger row without an accepted retained copy.
Retain its raw source values and the last committed preservation cursor so retry remains safe.
Authorized workspace or organization erasure can still purge this source data.

Initial work/segment upgrades can traverse retained history once in bounded durable repair pages.
Ordinary source-context changes enqueue targeted repair for that subject's unproven external rows.
They do not restart an archive-wide repair or rewrite already proven/native work under the latest transcript.
Freeze an observation's proven transcript context at admission and preserve it across replay and index repair.
Repair records old/new invalidations through the shared transactional mutation log.
Publication remains fenced until the affected context repair and projection group are coherent.

Retain source rows and old read compatibility until validation permits cutover.
After cutover, retained observations become authoritative for the statistics path.
Do not union the same native events from both retained and original stores.
A rollback to an older binary that cannot preserve deletion must disable destructive task cleanup or restore a compatible backup.
Switching reads back is not authorization to discard accepted retained records.
Backups include tables and progress records through the existing database boundary.
No migration can reconstruct records already removed from every surviving source.

## Presentation

Reuse `/stats/token-usage`, its workspace/range/provider state, and current desktop and phone compositions.
Deleted task/session rows display localized labels and omit navigation links.
Archived live tasks retain valid links and remain included by default.
Expose mixed/unknown actor and provider states, cost basis, calendar limits, and projection freshness in existing details.
Use a preparing state during first projection, and keep saved values visible during later rebuilds.
Copy Stats uses the same selected scope and complete filtered totals as the page.
The plan owns labelled previews UI-H01 through UI-H03.
Phone uses the existing focused breakdown list and touch-accessible details with one body scroll owner.
Preserve keyboard focus, safe areas, localized copy, and 44px touch controls without enlarging ordinary desktop controls.

## Observability and verification

Record bounded counters for accepted/replayed/rejected observations, dirty partitions, summary lag, rebuild progress, and migration lag.
Record partition sizes and rows visited, not unbounded per-task metric labels.
Log no transcript content, credentials, or personal display text.
Test native deletion races, external revisions, actor ambiguity, cross-org access, projection crash points, and calendar boundaries.
SQLite and real PostgreSQL parity are release evidence. A skipped PostgreSQL suite is not a passing parity result.
The [work package](../../../plans/usage-history/plan.md) owns commands and pending results.

## Related decisions

- [Independent retained usage](../../../decisions/2026-10-28-retained-usage-history.md)
- [Core usage projections](../../../decisions/2026-09-13-core-session-usage-projections.md)
- [Capability-gated Host data](../../../decisions/0043-plugin-host-data-api.md)
