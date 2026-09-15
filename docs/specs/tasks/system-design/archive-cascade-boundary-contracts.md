---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001
created: 2026-09-06
updated: 2026-09-06
owners:
  - cfl
---
# Archive Cascade Boundary Contracts

## Authority

This owns revision migration, workspace deletion, actors,
authenticated evidence, Git proof, and Runtime lifecycle for
[Durable Archive Cascades](durable-archive-cascades.md).
[Task Creation Protocol](task-creation-protocol.md) owns creation;
[Workspace Configuration Fence](workspace-config-fence.md) owns config I/O;
[Archive Cascade Execution Contracts](archive-cascade-execution-contracts.md)
owns locks; [External-ID Idempotency](external-id-idempotency.md) owns release.

## Creation and retained identity
Task creation uses `BeginTaskCreation`, private `CreationHandle`, complete
`CreationPlan`, retained `task_creation_operations`/`task_creation_steps`,
`CreationCoordinator`, proof-gated Complete/Abort, and mandatory Runtime
recovery. Repository inserts, direct settlement/publication, and lifecycle-delete
rollback are removed.

Begin is revision zero/no event. Required effects have deterministic idempotency,
lease/generation, evidence, unknown reconciliation, and compensation. Complete
uses two durable commits: phase A enters non-abortable `completing`; phase B
reacquires the lock set, revalidates proof, and writes revision one plus
`task.created`. Runtime recovers between commits. Abort requires every possible
effect absent, compensated, or transferred to durable cleanup. Only exact
external-ID lookup exposes `FoundUnsettled`, without a handle.

Workspace deletion aborts preparing work with no task event and finalizes
completing work with ordered revisions one/two. An armed launch intent in the
preparing abort branch is cancelled with `workspace_deleted` and
`absent_proven` evidence in that same transaction. Exact state, storage, Store
APIs, caller cutover, and tests are authoritative in
[Task Creation Protocol](task-creation-protocol.md).

## Revision migration and contiguous delivery

Migration creates one epoch plus retained identity, delivery-watermark, and
compact historical-gap rows in one transaction:

- `task_lifecycle_revisions` is keyed by task identity and stores
  `workspace_id`, immutable `lifecycle_epoch`, terminal/tombstone state, and
  `last_revision`, initialized to the greatest retained/evidence/gap revision.
- `task_lifecycle_delivery_watermarks` stores task/epoch and
  `delivered_cursor=(revision,queue_sequence)`; historical gaps stores
  immutable inclusive ranges bound to epoch, provenance hash, and
  `pre_epoch_no_event`.
- Every task outbox row stores epoch, revision, queue sequence, event ID,
  tombstone, payload/hash, and disposition; session rows add session IDs,
  terminal payload, and tombstone suppression. Tier-6 delivery locks the
  watermark, publishes only the exact `(revision,queue_sequence)` successor,
  then advances the cursor; crash leaves the row pending.
- Authenticated gap `[a,b]` is consumable under tier 6 only when
  `a=cursor.revision+1`, no outbox/pending row lies in it, and proof/hash matches.
  The transaction consumes the gap, sets `last_revision=max(last_revision,b)`,
  and advances to `(b,0)`; initialization is `(0,0)`, no queue sequence belongs
  to a gap, replay is a no-op, and pre-commit crash changes neither row.

Migration validates retained revisions and greatest live/evidence. Each
gap requires authenticated `pre_epoch_no_event` proof bound to task, epoch, range,
and evidence hash. Evidence-only identities receive `1..max_evidence_revision`
only with proof; otherwise migration blocks with no epoch. Gaps cannot overlap
outbox/pending/retry events. Post-gap allocation is
`max(last_revision,greatest_gap_end)+1`; replay is idempotent by epoch/hash.
Tests cover missing proof, all gap positions, seeded allocation, cursor
advancement, crash/retry, and no-skip delivery.

## Complete workspace deletion transaction

Task and Office HTTP/WS handlers extract
`UserActor{UserID, RequestID, AuthMode, Synthetic}` only from authenticated
request context and construct
`ConfirmedWorkspaceDelete{WorkspaceID, ConfirmName, UserActor}`. `RequestID`
is a canonical UUID bound to the request and is part of actor binding, replay
hash, and security/resolution audit. `Synthetic=true` is permitted only for
the disabled-auth installation actor and is rejected when auth is enabled.
The concrete Task/Office service signature is
`DeleteWorkspace(context.Context, ConfirmedWorkspaceDelete)`, and each service
delegates that unchanged exactly once to `Store.DeleteWorkspace`. A summary read
may reject an obvious mismatch for UX only; it supplies neither authorization
nor a replacement name/actor. Store locks the workspace, reloads workspace
name/owner/membership/auth mode, reauthorizes the original actor, and
byte-compares the original confirmation before any snapshot/write. ID-only
service/repository methods and Office loop/config callbacks are removed. Tests
rename, change membership/owner/auth mode, change request ID, or replay another
request actor after each Task/Office summary read; every stale case writes no
operation/event/step.

The checked-in workspace-deletion registry classifies every live production
table as retained, transactionally deleted, or installation/user-global. Exact
table names, owner selectors/joins, nullable-owner rules, outcomes, and child-
before-parent order are authoritative in
[Workspace Deletion Table Registry](workspace-deletion-table-registry.md).
Schema/FK/SQL inventory rejects missing, wildcard, stale, scratch, or untyped
entries.

In particular, both `task_blockers` endpoints are inventoried and locked, only
incident blockers/comments are deleted, and unrelated rows survive.
`github_app_registrations.created_for_workspace_id` is immutable provenance, not
ownership; registrations and `github_webhook_deliveries` survive, while exact
workspace registration flows/import preparations are deleted. Every registry
entry receives owned and unrelated fixtures plus writer-first/reservation-first
races on both dialects.

For settled tasks and tasks outside a live creation operation, the transaction
locks workspace, active operations, sorted tasks/revisions, groups, cleanup
jobs/resource snapshots/resume claims, integration rows, and outboxes in shared
order. It prepares retained delete snapshots, inserts one ordered `task.deleted`
tombstone per such task, inserts the immutable workspace deletion operation and
one `workspace.deleted` outbox row, deletes the transactional set, and commits
all or none. Creation states use the explicit partition below; external providers
and files are never called inside this SQL transaction.

## Workspace deletion during a cascade

Under the workspace lock, deletion inventories nonterminal archive operations,
`task_creation_operations`, `task_external_id_release_operations`, and
resume-materialization claims. It increments operation, job, step,
release-operation, and claim generations. Scope includes `preparing`, `archiving`,
`archived`, retry, and unarchive. A `preparing` archive records delete-owned
`workspace_deleted` and releases its barrier; committed membership retains its
membership/cleanup proof. Neither branch emits a task lifecycle event or permits
restore replay.
| Creation state | Live task action | Lifecycle evidence |
| --- | --- | --- |
| `preparing|preparing-release` | transfer effects; cancel armed launch with `workspace_deleted`/`absent_proven`; abort and delete partial rows | no revision/tombstone; retained rows record deletion |
| `completing` | phase-B finalization under the complete lock prefix, then delete live task rows | revision-one `task.created`, revision-two `task.deleted`; workspace event depends on both |
| no live creation | settled-task path | ordered `task.deleted` |

Phase B follows durable `completing`, never calls public Complete or relocks, and
inserts lifecycle outboxes plus workspace dependency before live-row deletion.
A `preparing` release operation is finalized under the same lock with
`workspace_deleted`, deletion generation, and stable replay result before its
task identity is cleared.

Before superseding a cascade member, retained scope must equal `{workspace}`.
Foreign scope records canonical `foreign_scope_blocked`, retains its barrier and
foreign members, and runs no external cleanup. If a deleted workspace owns a
group with surviving foreign members, deletion removes only deleted-workspace
edges and atomically rehomes the group/shared resources to the deterministic
surviving owner with valid foreign FKs. Rehome linearizes only while holding
the same physical guard as cleanup; a contended guard records retryable
`rehome_pending`, leaves the live owner unchanged, and keeps workspace deletion
pending without running or synchronously waiting for I/O. Authenticated
root-absent reconciliation later transitions it to `foreign_scope_resolved`
without touching foreign resources. Valid scope records `confirmed_missing`,
transfers ownership to `delete_owned`, fences resume claims, transfers executed
I/O, and completes with immutable dispositions. Stale claims fail CAS.
Retry/replay returns the same deletion ID, events, dispositions, and external
state. Tests pause every creation/cascade phase and cleanup/restore/resume claim
on both dialects.


## Durable workspace aftermath

All names here are namespace-qualified by
[archive-cascade-vocabulary.yaml](archive-cascade-vocabulary.yaml). States and
dispositions are distinct; `rehome_pending`, `blocked_missing`, and
`blocked_missing_member` are classified only by that registry.

`workspace_deletion_operations` retains workspace ID/name/owner projection,
transaction hash, config-path claim/generation/marker identity, state,
attempt/error timestamps, and immutable external-step snapshots.
`workspace_deletion_steps` has one row per
`(operation_id, step_kind, resource_hash)`, enforced unique and forming
`workspace-delete/<operation-id>/<kind>/<resource-hash>`. States are
`pending|running|retry_wait|unknown|succeeded`; legal transitions are
`pending -> running|succeeded`, `running -> retry_wait|unknown|succeeded`,
`retry_wait -> running`, and `unknown -> running|retry_wait|succeeded`.
`unknown -> succeeded` is permitted only for exact untouched rehome proof with
`rehome_owned`; exact already-cleaned proof remains `unknown` with
`blocked_missing`, and ambiguity remains `retry_wait(rehome_pending)`. The
blocked disposition never means success and every result is replayable.
`group_rehome` is mandatory; it carries the group/resource snapshot, ownership
generation, due/lease, and `rehome_pending` on `retry_wait`. Runtime claims due
rows via the same CAS/key.
Phase A snapshots each resource's workspace/task/session/environment/
repository/worktree/runtime handles, generations, path/Git identity, hash, HMAC,
disposition, and idempotency key. Append-only evidence and marker CAS retain the
canonical proof; verifiers lock tiers 8/14 and accept only its exact generation.

Workspace config identity and I/O use the cycle-free `internal/workspaceconfig`
Resolver, unconstructable Fence, claim-state/access reload, and VerifiedConfigRoot.
The complete replacement method table, claim/cache/quarantine lifecycle,
path-event behavior, migration, and tests are authoritative in
[Workspace Configuration Fence](workspace-config-fence.md). Workspace deletion
atomically moves the active claim to deleting and snapshots the exact fence into
its aftermath step; stale or ambiguous access performs no filesystem/subprocess
I/O, and no name/raw-path API remains.

The mandatory steps are physical task/group cleanup, shared-group rehome,
workspace configuration-file deletion, provider/credential deletion,
credential-broker revocation/cache invalidation, and every registered provider.
Each handler accepts only its immutable snapshot and idempotency key; exact
resource not-found succeeds. Provider additions register a transactional
metadata deleter and, when needed, an external handler.

Workspace aftermath has two durable phases. Phase A locks workspace tier 1,
deletion/creation operations tier 4, sorted task/revision rows 5-6, cleanup
jobs/resource snapshots/group rows 8/13, resume claims, markers 14,
config/provider/secret metadata 15, then records all external steps as `pending`
and commits. It performs no external I/O while SQL locks are held. The worker
executes each step outside SQL under its fenced marker. Phase B reacquires the
same lower-tier rows in order, including the present workspace row or its
canonical absent workspace identity key, verifies marker/evidence and step
generation, and advances the step to `succeeded` at tier 16; a lost claim
restarts Phase B, and ambiguous I/O remains `unknown`.
Crash recovery resumes from stored phase and never assumes uncommitted physical
or provider effect. `workspace_lifecycle_event_outbox` carries the existing
`workspace.deleted` type, stable event ID, deletion operation ID, and immutable
prior workspace DTO; publication waits for dependent task tombstones. Registered
steps, not best-effort bus delivery, own destructive cleanup.

## Absolute request deadline

The initiating request computes one absolute deadline: the earlier caller
boundary or entry plus `TaskArchiveTimeout`. Every synchronous post-preparation
context uses `min(original_deadline, now+step_budget)`. This applies to
preparation rollback, mutation recovery, nested finalization, vacancy
reconciliation, cleanup activation, and their compensation. A “fresh” budget
never extends the original deadline.

If no time remains, the request writes retry direction/error/wakeup in the
operation transaction and returns the durable in-progress/timeout result with
operation ID. It performs no further synchronous recovery. The independent
runtime may later claim a new persisted attempt with its own worker deadline;
that does not extend the expired request attempt. Deterministic tests expire the
request during rollback, member mutation, finalization, vacancy, activation, and
nested compensation and assert no request-owned work crosses the deadline.

## Bounded cascade and retained artifacts

Version 1 bounds are inclusive: members `10000`, depth `256`, group snapshot
bytes `4194304`, evidence envelope bytes `262144`, and retained payload bytes
`524288`. Validate counters and streaming canonical bytes before allocation or
durable insertion; excess returns typed `archive_size_exceeded` with zero task,
membership, snapshot, evidence, or resource effects.

## Complete group reconstruction snapshot

`task_archive_operation_groups.snapshot_version=1` stores every group column:
`id`, `workspace_id`, `owner_task_id`, `ownership_generation`,
`materialized_path`, `materialized_environment_id`, `materialized_kind`,
`owned_by_kandev`, `cleanup_policy`, `cleanup_status`, nullable `cleaned_at`,
`cleanup_error`, `restore_status`, `restore_error`, `restore_config_json`,
`created_at`, and `updated_at`. It stores every member column:
`workspace_group_id`, `task_id`, `role`, nullable `released_at`,
`release_reason`, `released_by_cascade_id`, and `created_at`.

Members sort by canonical task ID. JSON uses RFC 8785; timestamps are UTC
RFC3339Nano, null stays null, integers are decimal, and booleans are JSON
booleans. `snapshot_sha256` is SHA-256 of
`ASCII("kandev/archive-group-snapshot/v1") || 0x00 || canonical_json`.
Reservation validates group/workspace equality, unique members, exactly one
owner role matching `owner_task_id`, known materialized kind/cleanup policy, and
Missing-group reconstruction locks the absent insertion key and every member
task, verifies snapshot hash/authentication and current task/workspace eligibility,
and compares any existing group by operation ID, snapshot hash, ownership generation,
and sorted member set. An exact same-operation match advances restore state by CAS
and is idempotent after response loss; a foreign, mismatched, or generation-conflict
row blocks with a typed diagnostic. A valid absent row inserts byte-equivalent
group/member columns preserving null/default/timestamp values. If any snapshotted
member is `confirmed_missing`, reconstruction is forbidden: retain the complete
immutable snapshot, mark the group `blocked_missing_member`, and keep it not-ready.
The task member alone may be returned in `SkippedTaskIDs`; the unarchive operation
cannot complete while that group is blocked. Incomplete or unknown-version proof
persists `missing_group_snapshot`; nothing is guessed. Tests compare every
reconstructed column and canonical hash.

## Authenticated legacy binding

`archived_by_cascade_id` is evidence only; conversion accepts a task only with
workspace lock plus the complete authenticated marker/snapshot binding. The
version-1 envelope, identities, HMAC, issuance, verification, and mutation
matrix are authoritative in [Archive Cleanup Evidence](archive-cleanup-evidence.md).

## Trusted local and Git proof

The complete local/worktree content identity and physical-absence protocol are
authoritative in [Archive Cleanup Evidence](archive-cleanup-evidence.md).
Cleanup holds the canonical physical guard; opens parent/target no-follow;
rechecks containment, filesystem identity, external HMAC marker, Git
registration, branch, HEAD, and base immediately before rename/removal.
Mismatch is `unknown` with no destructive I/O. An absent database row is only
metadata-idempotent; an absent physical path succeeds only with exact external
marker and locked Git-removal proof. Generic not-found remains unknown/retryable.

## Public wrapper and publisher cutover

The registry includes every task/workspace delete wrapper and callback;
`TaskEventPublisher`, `SetTaskEventPublisher`, `PublishTaskDeleted`,
archive/unarchive `PublishTaskUpdated`, `publishWorkspaceDeleteChildEvents`, and
direct `WorkspaceDeleted`; plus `finalizeCreatedTask` publication. Private
callbacks are attributed to every public caller. Handoff publisher interface,
setter, nil fallback, direct lifecycle calls, and permanent nil-publisher test
are deleted; the replacement test reads the aggregate outbox.

Completed-task deletion enters only through `DeleteTask(ctx,
DeleteTaskCommand)` or `DeleteTaskTree(ctx, DeleteTaskTreeCommand)`. Commands
contain sealed actor, target, cascade mode, and
`TaskDeletionReason{Code TaskDeletionReasonCode, Source TaskDeletionSource,
SourceID string}`. Store parses/validates every field before mutation.

`SourceID` encoding is
`<kind>:v1:<base64url-no-pad(RFC8785(canonical-object))>` using lowercase ASCII
kind. UUID fields use lowercase canonical RFC 4122 text; other IDs preserve
exact UTF-8. Enum-specific decoders reject unknown/missing/extra fields,
noncanonical JSON/base64/UUID, prefix/source mismatch, and empty IDs.

| Caller -> command | Code / Source | Canonical SourceID object |
|---|---|---|
| Task HTTP -> `DeleteTask[Tree]` | `user_requested` / `user` | `http`, `{"request_id","user_id"}` |
| MCP delete -> `DeleteTask[Tree]` | `mcp_requested` / `task_session` | `mcp`, `{"message_id","session_id"}` |
| quick-chat expiry -> `DeleteExpiredTask` | `quick_chat_expired` / `task_maintenance` | `quick-chat`, `{"sweep_id"}` |
| profile cleanup -> `DeleteEphemeralTasksByAgentProfile` | `agent_profile_deleted` / `task_maintenance` | `profile-cleanup`, `{"operation_id","profile_id"}` |
| GitHub/GitLab PR terminal -> `DeleteTask[Tree]` | `pr_merged_or_closed` / `github|gitlab` | `provider`, `{"event_id","provider","watch_id"}` |
| GitHub/GitLab approval -> `DeleteTask[Tree]` | `pr_approved_by_user` / `github|gitlab` | `provider`, `{"event_id","provider","watch_id"}` |
| GitHub/GitLab issue close -> `DeleteTask[Tree]` | `issue_closed` / `github|gitlab` | `provider`, `{"event_id","provider","watch_id"}` |
| Azure terminal cleanup -> `DeleteTask[Tree]` | `watch_terminal_cleanup` / `azure_devops` | `provider`, `{"event_id","provider":"azure_devops","watch_id"}` |
| GitHub issue/review reset -> `DeleteTask[Tree]` | `watch_reset` / `github` | `watch-reset`, `{"operation_id","provider":"github","watch_id","watch_kind"}` |
| GitLab issue/review reset -> `DeleteTask[Tree]` | `watch_reset` / `gitlab` | `watch-reset`, `{"operation_id","provider":"gitlab","watch_id","watch_kind"}` |
| Azure work-item/PR reset -> `DeleteTask[Tree]` | `watch_reset` / `azure_devops` | `watch-reset`, `{"operation_id","provider":"azure_devops","watch_id","watch_kind"}` |
| Jira issue reset -> `DeleteTask[Tree]` | `watch_reset` / `jira` | `watch-reset`, `{"operation_id","provider":"jira","watch_id","watch_kind":"issue"}` |
| Linear issue reset -> `DeleteTask[Tree]` | `watch_reset` / `linear` | `watch-reset`, `{"operation_id","provider":"linear","watch_id","watch_kind":"issue"}` |
| Sentry issue reset -> `DeleteTask[Tree]` | `watch_reset` / `sentry` | `watch-reset`, `{"operation_id","provider":"sentry","watch_id","watch_kind":"issue"}` |
| automation cleanup job -> `DeleteTask[Tree]` | `automation_run_deleted` / `automation` | `automation`, `{"automation_id","cleanup_job_id","run_id"}` |
| plugin host -> `DeleteTask[Tree]` | `plugin_requested` / `plugin` | `plugin`, `{"invocation_id","plugin_id"}` |
| e2e reset -> `DeleteTask[Tree]` | `e2e_reset` / `system_test` | `e2e`, `{"reset_id"}` |
| workspace aggregate child -> internal task delete | `workspace_deleted` / `workspace_delete` | `workspace-delete`, `{"operation_id","workspace_id"}` |

The shared `watchreset` adapter accepts only a sealed
`WatchResetProvenance{Provider,WatchKind,WatchID,OperationID}`. Closed pairs are
`github/(issue|review_pr)`, `gitlab/(issue|review_mr)`,
`azure_devops/(work_item|pull_request)`, `jira/issue`, `linear/issue`, and
`sentry/issue`; all others fail before task lookup. Each integration constructs
it from its locked watch plus authenticated reset request, and atomically
persists one canonical reason in each `task_cleanup_jobs` row before removing
the watch. `watchreset.Run` claims/drives those rows and cannot synthesize
provider, watch, or operation identity. There is no generic `watchreset` source
or direct deleter fallback.

Trusted constructors accept only authenticated HTTP context, MCP message/session
context, claimed maintenance/provider/automation rows, plugin host invocation
context, e2e harness capability, or locked workspace-delete operation. They
return a sealed provenance variant plus the canonical reason. Public request and
plugin payloads cannot supply reason/source/SourceID. Missing trusted fields
returns `missing_deletion_provenance`; canonical mismatch returns
`invalid_deletion_provenance`; both occur before aggregate mutation.

Automation always persists/claims `automation_task_cleanup_jobs` first, so one
composite SourceID exists for direct and retried cleanup. Plugin dispatcher adds
authenticated plugin/invocation context without plugin ABI change. E2E creates
one reset UUID at the harness boundary. Provider adapters use the immutable
claimed delivery/event ID and watch ID. Delayed jobs persist the complete reason
bytes; tree children copy them byte-for-byte.

Creation compensation never calls delete: it uses handle-bound Abort. The
checked-in provenance registry is [Archive Cascade Registries](archive-cascade-registries.md);
it names every caller/constructor, closed SourceID fields, trusted context,
persistence point, child propagation, and compensation. Reasonless
service/repository methods, optional interfaces/type assertions, free strings,
empty compatibility values, and nil-deleter success are removed. The registry
rejects any unmapped caller, constructor, adapter, command, or compensation.

Aggregate outbox owns completed `task.created`, archive/unarchive/detach lifecycle
`task.updated`, every `task.deleted`, and `workspace.deleted`. Direct publisher
remains only for registry-approved non-lifecycle metadata/state and workspace
create/update. Immutable delete payload stores the complete typed reason, prior
DTO, deletion kind, revision, event, and operation IDs. AST/interface inventory
plus tests invoke every public caller and prove one committed event identity,
exact reason mapping, and zero duplicate/missing/direct publication.

## Sealed actor structures and checks

Actor is a closed Go sum type; constructors are internal and accept trusted
context or persisted claim rows, never request-body authority:

- `UserActor{UserID, RequestID, AuthMode enabled|disabled, Synthetic bool}`
  reloads current request ID, auth mode, and locked workspace owner. Synthetic
  is valid only for the disabled-auth installation actor; enabled-auth requests
  must carry a real user identity and cannot set it.
- `TaskSessionActor{TaskID, WorkspaceID, SessionID string, SessionGeneration,
  LifecycleGeneration int64, RequestID string}` derives from sealed request/session
  context and the locked rows; it reloads task ownership, session generation,
  lifecycle fence, and authorization before session mutation or resume admission.
- `AutoArchiveActor{TaskID, WorkspaceID, WorkflowStepID string, TaskUpdatedAt,
  StepUpdatedAt time.Time, AutoArchiveHours int, Cutoff time.Time}` is built from
  `ListTasksForAutoArchive`; the transaction reloads task/step and re-evaluates
  unchanged timestamps/config and eligibility.
- `OperationRecoveryActor{OperationID, RootTaskID, WorkspaceID, LeaseOwner
  string, ClaimGeneration int64}` reloads phase, due time, owner, generation,
  and unexpired lease.
- `CreationStepWorkerActor{OperationID, TaskID, WorkspaceID, StepID, StepKind,
  LeaseOwner string, ClaimGeneration, LifecycleGeneration int64}` derives from
  a committed creation-step claim and reloads operation/task/step/lease before
  dispatch, compensation, or recovery.
- `CleanupWorkerActor{JobKind, JobID, OperationID, WorkspaceID, ResourceID,
  TargetDirection, WorkDirection, LeaseOwner string, LifecycleGeneration,
  ClaimGeneration int64}` matches every named job/lease field.
- `OutboxWorkerActor{OutboxKind, EventID, TaskOrWorkspaceID, WorkspaceID,
  LeaseOwner string, Revision, ClaimGeneration int64}` matches row, minimum
  revision/dependencies, owner, generation, and lease.
- `TaskMaintenanceActor{Kind quick_chat_expiry|profile_cleanup, TaskID,
  WorkspaceID, ProfileID string, Cutoff, SelectedActivityAt time.Time,
  OriginatingUser UserActor}` is built by the trusted selector. Expiry rechecks
  activity/cutoff; profile cleanup also reloads the user binding, locks every
  workspace, and rechecks profile, ephemeral state, and session binding.
- `LegacyOperatorActor{UserID, CredentialID, AuthMode, Scope string,
  WorkspaceID string}` derives only after raw-peer/PAT checks and reloads current
  instance-admin role, auth mode, diagnostic scope, and workspace ownership.
- `InstallationRecoveryActor{InstallationID string, AuthMode disabled}` derives
  only from the dedicated raw-loopback route and reloads installation identity
  plus disabled auth mode before every page, resolution, and replay.

Takeover first commits the higher-generation claim, then constructs a new worker
actor from that row. Auth-mode changes invalidate synthetic/operator actors.
Selector timestamp/config changes invalidate scheduled/maintenance actors.
Spoofed fields, stale leases, cross-workspace resources, and actor-variant
mismatch return typed not-found/conflict with zero writes. Table tests cover all
fields, derivation boundaries, takeover, auth transitions, and stale selectors.

## Mandatory recovery runtime

`internal/task/archivecascade.Runtime` is the sole owner of creation/operation
reconciliation, task/group cleanup/restore, workspace steps, task/workspace
outboxes, and unknown reconciliation. Startup is refactored into this exact
order:

Startup exposes one seam:
`constructBootstrap -> bindBootstrapListeners -> startRuntimePrerequisites ->
Runtime.Start -> startApplicationProducersAndPublishRoutes`.

| Stage | Exact allowed work |
| --- | --- |
| before bind | profile/config validation; logger/event-bus/DB pool; migrations; repository, Store, service, lifecycle/orchestrator, provider-client, hostname-resolver object, agentctl-availability object, fence, and Runtime construction; static registry and cleanup-stack registration. Constructors may read DB/config but start no goroutine, subscription, subprocess, listener, sweep, or callback |
| bootstrap bind | only existing TCP listeners plus immutable `/health` liveness and not-ready response through `handlerSwitch`; register listener close immediately |
| runtime prerequisites after bind | synchronous `runInitialAgentSetup`; start `agentctl` subprocess and wait for authenticated control health because runtime stop/proof needs it; register its stop immediately. No agent session/task is launched |
| recovery gate | synchronous `Runtime.Start`: sweep expired creation/cascade/cleanup/workspace/outbox claims, deliver due task/workspace outboxes, and return only when sweep and worker loops are live |
| after gate | hostname/event subscription; gateway; host utility, profile/migration, orchestrator recovery; automation/scheduler/maintenance; provider pollers; plugins and plugin-event delivery; router; readiness |

The exhaustive symbol/order/side-effect/cleanup/readiness inventory is
[Runtime Startup Registry](runtime-startup-registry.md). Its B/Q/R/P/U entries,
not category prose, are the checked-in executable set; startup and independent
AST/SSA verification both compare against it.

Task/workspace delivery is started only inside the R01 Runtime gate; plugin-event
delivery remains the separate P18 post-gate owner. No lifecycle outbox has a
second worker. Agentctl is the sole pre-Runtime process; its API cannot create a
task/session during bootstrap.
Initial agent setup is synchronous, bounded, and creates no task, session,
runtime, cleanup, lifecycle event, deletion, or asynchronous callback.

The single production seam is the only call site for these stages:
`startServices` constructs and binds; `startAgentInfrastructure` returns
unstarted dependencies; `startGatewayAndServe` starts post-bind work. The old
orchestrator/automation consumer helper is removed.

Liveness answers throughout prerequisite and transient Runtime delay; readiness/
real routes stay unavailable. Pre-bind migration/config/constructor failure
returns without a listener. After bind, initial-setup/agentctl failure stops any
started prerequisite and closes listeners. Runtime transient errors retry with
bounded backoff under root context. Fatal/cancel stops partial Runtime, agentctl,
and listeners. Post-gate failure closes producers, Runtime, agentctl, and
listeners in reverse registration order. No failure path publishes a real
handler or leaves a goroutine/subprocess/socket.

Successful Start registers one stop-once `Close` through `addRuntimeCleanup`.
Factory-reset `DatabaseQuiesce` composes Runtime and delivery stops;
`RestoreQuiesce` receives the same stop-once closures in `restoreCleanups`
before checkpoint/close/replace. Shutdown/reset/restore concurrency returns the
same stored Close result. Reset/restore keep the existing mandatory restart.
Runtime remains independent of optional scheduler/profile flags.

Each claimant queries the minimum due canonical key, claims generation/lease in
a short transaction, heartbeats while working, and wakes the timer when it
writes an earlier `next_attempt_at`. Attempt timeouts do not exceed the lease;
shutdown cancels local I/O, stops heartbeats, waits boundedly, and leaves rows for
lease-expiry takeover. `Close` is idempotent. The due query includes exhausted
cascade jobs and workspace steps, so attempt eight and later remain selectable
after restart at the capped interval.

Runtime health exposes counts for due/running/unknown/exhausted rows and oldest
due age; startup/query failures log and retry rather than disabling the loop.
Tests start from an attempt-eight row, restart without optional maintenance,
advance the deterministic clock to 12 hours, observe a higher-generation claim,
exercise shutdown takeover, and prove minimum-revision/dependency dispatch.

## Verification ownership

Work orders test handle/manifest recovery and Complete ambiguity; lookup-first
REST/MCP precedence; release/outbox revision and historical gaps; exact table
inventory plus GitHub retention; blockers/Office actor propagation; config
races; workspace supersession/deadlines/group/HMAC/Git proof; typed deletion
callers and compensation; and bootstrap liveness/readiness/fatal/stop-once
composition on SQLite and PostgreSQL. No component fake substitutes for
same-database aggregate or production-composition checks.
