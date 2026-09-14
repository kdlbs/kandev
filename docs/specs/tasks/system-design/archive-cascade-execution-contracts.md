---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
created: 2026-09-05
updated: 2026-09-05
owners:
  - cfl
---
# Archive Cascade Execution Contracts

## Authority

This document owns persistence, locks, fences, operators, and events shared by
[Durable Archive Cascades](durable-archive-cascades.md).
[Task Creation Protocol](task-creation-protocol.md) owns creation;
[Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md) owns
migration, deletion, actors, proof, and startup; [Workspace Configuration
Fence](workspace-config-fence.md) owns config I/O; [Task Runtime
Cleanup](runtime-cleanup.md) owns provider teardown.

Retained rows cover creation/archive/workspace-delete operations, snapshots,
release replay, cleanup jobs, markers, lifecycle revisions/watermarks/gaps,
outboxes, diagnostics/candidates, and security audit. None has a cascading FK to
mutable workspace/task/session/runtime/group/environment/integration/repository
rows; retained parents use delete restrict and no automatic delete path.

Migration seeds knowable task/workspace identities, validates outboxes, and
records only absent pre-epoch revision ranges as historical gaps. A retained
non-delivered event is never treated as delivered. Watermarks, identity inserts,
conflicts, limits, and replay follow
[Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md#revision-migration-and-contiguous-delivery).

## Complete database lock order

Physical guards are not acquired while workspace/operation SQL locks are held.
After the durable claim commits, the worker acquires the physical guard, then
opens a short marker-CAS transaction at the marker tier only (no earlier-tier
locks) and holds the guard through I/O and completion CAS. Rows and insert keys
within every SQL tier are canonical-primary-key sorted. Multi-workspace commands
lock candidate identities at tier 1, revalidate closure, and repeat on scope or
digest change before later tiers.

1. workspace rows;
2. workflow, workflow-step, capacity/admission, and queue keys;
3. legacy diagnostic and candidate rows;
4. archive, creation, external-ID release, or workspace-deletion operation rows;
5. external-ID ownership keys, then task rows/absent task insertion keys;
6. retained task identity/lifecycle-revision/watermark/gap rows;
7. immutable operation-member rows and mutable creation-step rows;
8. task cleanup rows;
9. task-session rows;
10. `executors_running` rows;
11. task-environment rows, then environment-repository/worktree rows;
12. workspace-group rows, then group-member rows;
13. immutable operation-group snapshots, then group-cleanup rows;
14. resource-marker rows;
15. workspace-owned config/integration/secret metadata rows, ordered by
    registry table ordinal then canonical primary key;
16. task/workspace event-outbox and workspace-deletion step rows;
17. resolution/security audit rows.
Tier-4 archive locks are `task_archive_operation_scopes` rows keyed by `(operation_id, scope_workspace_id)`, held/reloaded with the operation row. The workspace list is never an unlocked hint.

A workspace lock is a present row lock or the canonical absent identity key
`SHA-256("workspace-identity/v1" || 0x00 || workspace-UUID)`. PostgreSQL takes
the first eight digest bytes as a signed-big-endian advisory xact lock; SQLite
uses `BEGIN IMMEDIATE` and the unique `workspace_deletion_operations.workspace_id`
row as the same linearization. Post-delete commands accept an absent workspace
only with a committed deletion operation and retain this key through every Phase
B and delivery retry.

A diagnostic command discovers candidates unlocked, then locks workspace/absent
keys, reloads, and reauthorizes. Workspace deletion inventories under workspace
and acquires every tier in canonical order. Delivery starts at workspace tier 1,
then retained watermarks tier 6 and task/workspace outboxes and deletion steps
tier 16; it never loads a live task after deletion.

External-ID key bytes are `SHA-256("task-external-id/v1" || 0x00 ||
canonical-workspace-UUID || 0x00 || normalized-ID-UTF8)`. PostgreSQL tier 5
takes `pg_advisory_xact_lock` on the first eight digest bytes interpreted as
signed big-endian `int8`, then re-reads the exact ID; collisions only serialize.
SQLite starts `BEGIN IMMEDIATE`, then follows the same logical tiers and exact
re-read. Named unique constraints remain the final backstop. Deadlock/uniqueness
restarts the whole command; no partially read state is reused.

`PostgresCascadeFixture` requires a reachable non-empty DSN, creates and drops a
unique schema, asserts a pool with at least two connections, and gives
session-level advisory locks a dedicated connection lifetime. Unset is pending;
malformed or unreachable non-empty DSNs fail. SQLite uses an isolated temporary
database.

Absent task keys use domain `task-id/v1`; candidate changes after locking restart
at tier 1. Both dialect suites force digest collisions and absent-row races.

Command mapping is normative:

| Command | Ordered acquisitions |
| --- | --- |
| Begin | candidate workspace identities 1 (UUID order); workflow/admission 2; archive barriers and scope rows 4; creation operation 4; external/task keys 5; retained identity 6; later member/session/environment/group rows; no outbox. Scope change restarts tier 1. |
| Complete | workspace 1; creation operation 4; stored external-ID key then task 5; retained identity/revision 6; required rows in tiers 7-15; outbox 16 |
| Abort | workspace 1; creation operation 4; stored external-ID key then task 5; retained identity 6; cleanup evidence 8/14; no outbox |
| Release | workspace 1; release operation and candidate creation operation 4; external-ID key then candidate task 5; retained revision 6; outbox 16 only for completed holder |
| Task delete | workspace 1; delete/archive/release operations 4; candidate external-ID keys then sorted tasks 5; revisions 6; launch-dispatch claims/resource snapshots/resume claims 8-14; outbox 16 |
The remaining aggregate commands use these exact prefixes. Writers lock affected
operations and cleanup barriers at tier 4 in sorted order; a changed barrier
restarts at workspace tier 1 and no later step acquires an earlier tier.

| Command | Ordered acquisitions and physical boundary |
| --- | --- |
| Reserve | workspace 1; workflow/step/capacity/admission 2; archive operation plus sorted membership-scope barriers 4; no physical guard |
| CommitMembership | workspace 1; archive operation 4; task keys 5; group/member rows 12; outbox 16 |
| CommitArchiveMember / CommitUnarchiveMember | workspace 1; archive operation 4; task keys 5; group/member rows 12; cleanup rows 13; resource markers 14; outbox 16 |
| CreateOrAttachGroup | workspace 1; workflow/admission 2; archive operation 4; sorted task keys 5; group/member rows 12 |
| BindSharedWorkspace | workspace 1; affected archive/creation/cleanup barriers 4; task keys 5; session rows 9; environment/repository rows 11; group/member rows 12 |
| SetTaskParent (narrow) | workspace 1; barriers 4; sorted task keys 5; revision, parent-result, and detach-result ledgers 6; selected group/member 12; outbox 16; one tx; same-parent no-op retained |
| DetachTask | workspace 1; barriers 4; tasks 5; revision, parent-result, and detach-result ledgers 6; effects/claims 7-10; environment/repository 11; group/member 12; cleanup/markers 13-14; outbox 16; registry-verified single tx |
| MutateTaskSession | workspace 1; affected archive/creation/cleanup barriers 4; task keys 5; session rows 9; no physical guard |
| MutateTaskRuntime | workspace 1; affected archive/creation/cleanup barriers 4; task keys 5; session rows 9; `executors_running` rows 10; no physical guard |
| DeleteExpiredTask / DeleteEphemeralTasksByAgentProfile | trusted selector; workspace 1; maintenance operation 4; sorted tasks 5; revisions 6; cleanup/resource rows 8-14; outbox 16 |
| MutateTaskEnvironment | workspace 1; barriers 4; tasks 5; sessions 9; environment/repository/worktree 11; group/member 12; markers 14; physical guard after commit |
| Claim/CompleteTaskResourceCleanup | workspace 1; cleanup/snapshot/launch rows 8; environment/repository 11; evidence/markers 14; physical guard after SQL |
| Claim/Complete/Fail/ResolveTaskCreationStep | workspace 1; creation operation 4; task 5; retained identity 6; step rows 7; no outbox |
| Claim/CompleteTaskCreationCompensation | workspace 1; creation operation 4; task 5; retained identity 6; step rows 7; cleanup rows 8; markers 14; physical guard after SQL |
| Creation recovery | workspace 1; operation 4; task 5; retained identity 6; steps 7; cleanup/markers 8-14; physical guard after durable claim |
| LaunchTask | workspace 1; completed creation 4; task/identity 5-6; armed step 7; dispatch 8; session 9; runtime 10; physical guard after claim; requires revision-one `task.created`, live lifecycle, matching generation, and no cancellation |
| Workspace-deletion aftermath worker | Phase A locks workspace, operations, tasks/revisions, cleanup/claims, markers, config/provider rows; Phase B performs I/O outside SQL and reacquires the same prefix before fenced completion |
| ArchiveCascadeRuntime dispatcher/worker | claims start workspace 1, then barriers 4 and task/group cleanup rows 8/13; no late earlier-tier acquisition |
| Task/workspace outbox dispatcher | starts at tier 1 for sorted workspaces; delegates to the delivery transaction above; never loads a live task or holds a physical guard |
| WorkspaceConfig resolver/fence/claim | workspace 1; config/integration/secret rows 15 in registry ordinal order; resolution audit 17; physical filesystem/Git guard only after commit |
| Diagnostic page/resolve/replay | unlocked candidate lookup; workspace 1; diagnostic/candidate rows 3; operation rows 4 when resolving; audit 17; never mutates before reauthorization |
| Security-denial audit writer | starts at tier 17 only; appends audit/emergency-journal evidence and never acquires an earlier tier |
All listed retries restart at their first tier; physical guards never overlap
SQL locks. Both dialects test every adjacent inversion.

## Creation-step lock matrix

The following matrix is normative for Claim, Complete, Fail, Resolve,
Compensate, and recovery. Each effect row is reloaded in ascending tier order
with canonical primary-key sorting; guards and provider calls occur only after
the SQL claim/verification transaction commits.

State and disposition values are namespace-qualified and closed by
[archive-cascade-vocabulary.yaml](archive-cascade-vocabulary.yaml). The
implementation verifier rejects values or transitions absent from that registry.

| Kind | Step/effect rows and tiers | Compensation/recovery boundary |
| --- | --- | --- |
| `attachment_claim` | operation 4; task 5; retained identity 6; step plus `task_message_attachments` 7 | cleanup job/marker 8/14; physical attachment proof after SQL |
| `workspace_attachment` | operation/task/identity/step 4-7; environment/repo 11; group/member 12 | exact policy/row hashes; absent -> retry; mismatch -> unknown; untouched delete or transfer |
| `fresh_branch_prepare` | operation 4; task 5; identity 6; step 7; environment/repo 11; Git registration/marker 14 | Git proof after SQL; remove/quarantine or transfer cleanup |
| `remote_contribution_associate` | operation/task/identity/step 4-7; association 15 | provider claim; delete owned row or transfer proof; PR/MR projection post-Complete |
| `session_prepare` | operation 4; task 5; identity 6; step 7; session 9; `executors_running` 10; environment/repo 11; marker 14 | process/physical proof after SQL; transfer every unknown resource |
| `launch_intent` | operation 4; task 5; identity 6; step 7; no runtime effect before Complete | Claim moves `planned|retry_wait -> running`; ArmLaunchIntent requires exact `present_exact` evidence and atomically moves `running -> armed` before Complete; Complete accepts `armed`; workspace deletion cancels armed intent with `workspace_deleted`; `LaunchTask` requires armed and claims dispatch at tier 8; observed runtime uses runtime 10 and cleanup 8/14 |

Compensation/recovery locks operation/task/identity/step, effects, cleanup/
markers, then outbox 16; claim/candidate/deadlock retry restarts at workspace 1.
SQL never holds physical guards. Task 01/04 cover SQLite/PostgreSQL six-kind
failpoints and replay.

REST/MCP lookup precedes non-identity validation; Begin persists the plan and
returns a handle only on Created. Coordinator claims effects and gates Complete
or Abort on evidence. Expired work becomes unknown; Runtime owns recovery.
Release replay is holder-independent, preparing release emits no event, and
workspace deletion transfers/aborts preparing or completes-then-deletes
completing. Direct settle/release/delete/event repository paths are removed.
Config uses Resolver, Fence, claim/access reload, and VerifiedRoot; name/raw-path
APIs are removed. Boundary, table, writer, provenance, publisher, and exemption
documents own the remaining details.

## Actor variants and binding

Requests carry no authority. Closed variants are `UserActor`, `TaskSessionActor`,
`AutoArchiveActor`, `OperationRecoveryActor`, `CreationStepWorkerActor`,
`CleanupWorkerActor`, `OutboxWorkerActor`, `TaskMaintenanceActor`,
`LegacyOperatorActor`, and `InstallationRecoveryActor`. Boundary owns their
fields, derivation/reload, authorization, takeover CAS, and stale/foreign
zero-effect rules; commands reject mismatched bindings.
## Task and group cleanup jobs

Cascade-critical task/group jobs use `cascade_retry`: after seven normal delays,
they retain diagnostics and retry every capped 12 hours, never terminal `failed`.
Non-restorable delete/shutdown jobs use `bounded_terminal` and may fail after
attempt eight. Startup sweep, due timer, lease, shutdown, and exhaustion retry
belong to the mandatory Runtime; optional loops never own retry.

Both task and group jobs store `target_direction` (`cleanup|restore`),
`work_direction`, state, lifecycle generation, claim generation/owner/lease,
immutable key set/snapshot hash, activation generation, attempts, retry time,
error, and unknown evidence. Unarchive atomically sets target to restore and
increments lifecycle generation, fencing cleanup.

| State and target | Legal transition |
| --- | --- |
| cleanup `prepared` | archive activation commit to `pending`; unarchive before I/O to `cancelled` |
| cleanup `pending` or due `retry_wait`, unclaimed, with committed activation marker | claim `running`; unarchive to `cancelled` |
| cleanup `running` | matching heartbeat; `succeeded`; `retry_wait`; bounded-policy `failed`; ambiguous I/O to `unknown` |
| cleanup `running` when unarchive wins | target restore and generation + 1; expired worker cannot complete; state `unknown` for proof |
| expired cleanup `running` | `unknown`; outcome never assumed |
| cleanup `unknown`, target cleanup | fenced proof to `succeeded` or safe `retry_wait`; otherwise unchanged |
| cleanup `unknown`, target restore | fenced no-cleanup proof to `cancelled`; cleanup proof to `restore_pending`; otherwise unchanged |
| cleanup `succeeded` | unarchive sets target restore, generation + 1, `restore_pending` |
| restore `restore_pending` or due `restore_retry_wait` | claim `restoring`, generation + 1 |
| restore `restoring` | matching heartbeat; `restored`; `restore_retry_wait`; bounded-policy `failed`; ambiguity `unknown` |
| expired restore `restoring` | `unknown` |
| restore `unknown` | fenced proof to `restored` or safe `restore_retry_wait`; otherwise unchanged |
| `cancelled`, `restored` | terminal success and idempotent |
| `failed` | terminal only for `bounded_terminal`; never for cascade archive/unarchive |

Unarchive joins an active cleanup actor only after advancing the lifecycle fence;
it never marks a possibly executed claim `cancelled` directly. Reconciliation
acquires the same physical guards and reads marker plus provider proof before
choosing `cancelled` or `restore_pending`.
Restore validates retained env/repo/branch. Materialization claims are
`planned|running|unknown|retry_wait|succeeded|blocked|cancelled|transferred`,
keyed by task/lifecycle/environment/resource identity. `planned|retry_wait` claim
`running`; matching I/O succeeds; expiry/ambiguity is `unknown`; proof resolves
`unknown` to `succeeded|retry_wait`, otherwise `blocked`. Workspace deletion
fences claims: unstarted becomes `cancelled`, ambiguous/in-flight becomes
`transferred` with `workspace_deleted` and delete ownership. Only matching
`succeeded` admits a runnable session; Runtime owns recovery under generation CAS.
`ArchiveRecoveryAction` is closed:
`reconcile_unknown_cleanup|retry_cleanup_wait|resume_pending_cleanup|reconcile_unknown_restore|retry_restore_wait|cancel_before_io|retry_archive|retry_unarchive|resume_membership|enter_restore_pending|reconcile_materialization|retry_materialization|transfer_materialization|reconcile_group_rehome_unknown|reconcile_foreign_scope|complete_archive|complete_unarchive|replay_terminal`. Exactly one is legal per locked condition; else zero writes:

| Locked condition | Legal action |
| --- | --- |
| `group_rehome` step with `cleanup_unknown` marker | `reconcile_group_rehome_unknown` |
| cleanup `unknown` other than `group_rehome` | `reconcile_unknown_cleanup` |
| cleanup due `retry_wait` | `retry_cleanup_wait` |
| cleanup `prepared|pending` unclaimed with committed archive activation and no unarchive intent | `resume_pending_cleanup` |
| restore `unknown` | `reconcile_unknown_restore` |
| restore due `restore_retry_wait` | `retry_restore_wait` |
| cleanup `prepared|pending` before I/O with unarchive intent | `cancel_before_io` |
| materialization running or unknown with workspace-deletion fence | `transfer_materialization` |
| materialization `unknown|blocked` without deletion fence | `reconcile_materialization` |
| materialization due `retry_wait` | `retry_materialization` |
| operation `retry_wait`, kind `archive` | `retry_archive` |
| operation `retry_wait`, kind `unarchive` | `retry_unarchive` |
| `preparing` with committed membership and no active claim | `resume_membership` |
| operation `archiving`, all groups succeeded, every task is succeeded or confirmed-missing with durable `delete_owned` transfer, every required nonmissing cleanup job is `succeeded`, and no cleanup job is `prepared` | `complete_archive` |
| operation `unarchiving`, metadata ready, materialization absent | `enter_restore_pending` |
| operation `restore_pending`, all tasks/groups/materialization ready | `complete_unarchive` |
| operation `foreign_scope_blocked`, absent-root tombstone and authenticated resolution proof | `reconcile_foreign_scope` |
| operation `completed|aborted|foreign_scope_resolved|workspace_deleted` | `replay_terminal` (read-only) |
Under one operation lock, table rows are evaluated top-down; the first match
exclusively claims generation/lease. Later actions wait; stale claims conflict
with zero writes. `resume_pending_cleanup` atomically activates every matching
`prepared` job to `pending` before claiming any marker, and is idempotent across
the activation failpoint. `complete_archive` therefore cannot terminalize while
a prepared job remains. Deletion-fenced materialization transfer persists
`workspace_deleted`, delete ownership, and the new generation before any stale
worker can write.
`reconcile_group_rehome_unknown` uses the guard-first marker CAS: exact
untouched proof makes the marker `cleanup_cancelled` and the rehome step
`succeeded` with `rehome_owned`; exact already-cleaned proof leaves `cleaned`
and the step `unknown`/`blocked_missing` for restore or recreation; ambiguity
leaves marker `cleanup_unknown` and step `retry_wait(rehome_pending)`.
`unknown -> succeeded` is legal only for the exact untouched proof; blocked
missing remains non-success. Every branch stores proof, generation, and replay
result.
Archive completion requires task/group fences, every required nonmissing cleanup
job `succeeded`, and durable delete ownership for confirmed-missing members,
which return in `SkippedTaskIDs`. Unarchive also requires ready materialization
and rejects affected groups as `blocked_missing_member`; any pending/running/
retry/unknown/bounded-failed state blocks exposure.
`reconcile_foreign_scope` verifies absent-root tombstone, scope digest, foreign
members, and authenticated resolution, records audit, transitions to terminal
`foreign_scope_resolved`, releases the barrier, and performs no foreign I/O.

## Resource keys, markers, and provider proof

`CleanupFence` contains claim/job IDs, operation, target/work direction,
claim/lifecycle generations, snapshot hash, and a lexically sorted unique
resource-key set. Stable resource IDs are lowercase canonical RFC 4122 UUID
text. Their key is ASCII
`archive-cleanup/v1/<task|group|environment|worktree|repo>/<uuid>`; no escaping
or alternate UUID spelling is accepted. A physical path key is
`archive-cleanup/v1/path/sha256:<64-lower-hex>` over UTF-8 canonical path bytes.
Keys are at most 128 bytes.

Canonical paths are absolute cleaned, symlink-resolved from nearest existing
ancestor, volume/case normalized, and rechecked for owned-root containment;
snapshot path bytes are collision proof. The full platform/path and TOCTOU
rules are in [Archive Cleanup Evidence](archive-cleanup-evidence.md). PostgreSQL
and SQLite lock namespaces are stable database identity strings defined there.

Lock digest is SHA-256 of `namespace || 0x00 || resource-key`. PostgreSQL passes
the digest's first eight bytes as a big-endian signed `int8` to session-level
`pg_advisory_lock` on the dedicated connection. SQLite takes an exclusive OS lock
on `<runtime-data>/archive-cleanup-locks/v1/<first-two-hex>/<64-hex>.lock`.
Digest collisions only over-serialize because every marker CAS also matches the
full namespace/key.

`task_archive_resource_markers` has these CAS states:

| Marker state | Legal transition and proof |
| --- | --- |
| absent | insert `cleanup_claimed` only from a `pending|retry_wait` current job whose committed activation generation and snapshot hash match; `prepared` cannot insert |
| `cleanup_claimed` | matching worker to `cleaned` or `cleanup_unknown`; group/resource rehome atomically supersedes it to `cleanup_unknown` with ownership-transfer evidence and the new generation; higher lifecycle generation targeting restore fences it to `cleanup_unknown` |
| `cleanup_unknown`, target cleanup | provider proof to `cleaned`, or safe higher-generation `cleanup_claimed` |
| `cleanup_unknown`, target restore | proof unchanged to `cleanup_cancelled`; proof absent/cleaned to `cleaned` then `restore_claimed`; ambiguity stays |
| `cleanup_cancelled` | terminal for job `cancelled`; later operation may claim only after admitted snapshot preparation |
| `cleaned` | matching unarchive to higher-generation `restore_claimed` |
| `restore_claimed` | matching restorer to `restored` or `restore_unknown` |
| `restore_unknown` | proof restored to `restored`, or proof still cleaned to safe higher-generation `restore_claimed` |
| `restored` | later operation may claim cleanup only after admitted snapshot preparation |

CAS matches namespace/key, operation/predecessor, direction, lifecycle/resource
generation, owner lease, and snapshot hash. Rehome takes the full resource lock,
atomically changes the marker to `cleanup_unknown` with transfer evidence and
new generation, and fences the old worker; takeover starts only from that state.
Missing/mismatched/older markers conflict with zero I/O. Reconciliation records
typed provider observation hash/time.

After claiming, a worker takes the physical guard and performs a short
marker-only SQL CAS at tier 14, with no earlier-tier locks, then holds the guard
through I/O and completion CAS. Rehome uses the same guard-first marker CAS;
contention records `group_rehome` step `retry_wait` with `rehome_pending`,
leaves ownership unchanged, and does not synchronously wait or run I/O. Once
acquired, supersession is atomic, so no old worker starts after rehome.

Provider proof is typed and exact: local/worktree canonical DB/path/generation/
state/marker/content, HMAC, filesystem/Git branch/HEAD/base; Docker/Kubernetes
immutable IDs and ownership labels; SSH/Sprites remote instance and marker.
Generic not-found without its immutable provider handle is `unknown`; full
requirements are in [Archive Cleanup Evidence](archive-cleanup-evidence.md).

## Legacy conversion proof matrix

Conversion requires a canonical archive stamp, committed operation/member and
marker, or version-1 cleanup snapshot bound by IDs, hash, workspace/root,
generation, and HMAC. Malformed, ambiguous, cleanup-only, or group-only proof
blocks the candidate; proven missing members remain `confirmed_missing`, while
groups require the authenticated full snapshot or remain
`missing_group_snapshot`.

## Local operator and security audit

The versioned routes are
`GET /api/v1/admin/archive-cascades` and
`POST /api/v1/admin/archive-cascades/{diagnostic_id}/resolve`; both require the
scope query below. Native commands are:

- `kandev archive-cascade-reconcile --api-url <loopback-url> list
  (--workspace <uuid>|--workspaces <sorted-uuids>|--installation)
  [--limit <n>] [--page-token <token>]`;
- `kandev archive-cascade-reconcile --api-url <loopback-url> resolve
  (--workspace <uuid>|--workspaces <sorted-uuids>|--installation)
  --diagnostic <uuid> --revision <n> --request-id <uuid>
  --action adopt|dismiss [--evidence <json-file>]`.

List returns canonical IDs/revisions, blocked reason/evidence hashes, candidates,
timestamps, and page token. Resolve requires request ID, expected revision,
action, and evidence; Dismiss has null evidence; Adopt requires version-1 IDs
and sorted member proofs. Unknown/duplicate/noncanonical fields are 400;
responses return IDs, revision, outcome, operation ID, and evidence hash.

Routes use dedicated loopback/raw-peer checks and ignore forwarded headers/cookies.
Auth-enabled calls require PAT, instance admin, and ownership proof; disabled-auth
calls use synthetic local admin. Nonloopback/invalid/invisible input is
404/401/403/404; malformed is 400; stale/request/evidence conflicts are 409;
persistence/audit failure is 503; error bodies omit evidence.

Request hash covers actor, scope/workspace IDs, diagnostic, expected revision,
action, and canonical evidence. Request-ID replay reauthorizes; hash changes
conflict. CLI accepts token-file or `KANDEV_API_TOKEN`, rejects unsafe URLs,
emits JSON stdout/diagnostics stderr, and exits 0/2/3/4/5.
Exactly one scope is required: `workspace&workspace_id=<uuid>`,
`workspaces&workspace_ids=<sorted-uuids>`, or `installation`. Workspaces scope
requires PAT, instance admin, ownership and absent-root proof for every member;
installation scope is disabled-auth recovery only. List defaults to 50 (1–100);
resolve rejects pagination; malformed IDs/unsorted sets/conflicting scopes are
400.

List uses exclusive `(created_at,id)` keyset continuation with a 15-minute
RFC8785/HMAC token containing version, actor, scope, IDs, limit, last tuple, and
expiry. Every page/replay reruns loopback, auth, role, ownership, visibility,
scope, and request-hash checks; real actors cannot access zero-candidate or
installation-only diagnostics.

`task_archive_security_audit` is separate from resolution audit. It retains ID,
time, normalized peer, route/method, outcome/reason/status, and nullable actor,
request, diagnostic, request/evidence hashes for at least 365 days; never token,
cookie, forwarded header, raw body, or raw evidence. Every loopback/auth/
authorization/parse/evidence denial writes through a bounded sink before the
response, with unavailable fields null. DB failure falls back to a fsynced
0600 emergency journal for startup ingestion; if both fail, generic denial and
no mutation or sensitive value is exposed.

PostgreSQL locks task, revision, and its delivery watermark with `FOR UPDATE`;
SQLite uses one write transaction. Allocation increments and inserts immutable
payload exactly once. Delivery locks the watermark, selects only the exact next
`(revision,queue_sequence)` cursor successor, and advances that pair after
successful publish. Workers serialize per task epoch; crash leaves the row
pending/retry and cannot publish a later pair. A uniqueness/deadlock retry
restarts from authorization and allocates no gap. `delivered` is terminal and
retry is unbounded.

Creation is the exception: Begin reserves identity/revision zero/no outbox;
Complete phase A/B commits `preparing -> completing` then revision-one
`task.created`; Runtime recovers unfinished phase B. Workspace deletion uses
phase B plus revision-two deletion; preparing states Abort, completing states
complete-then-delete. External-ID release from completed tasks advances revision
and writes `task.updated`; creation-time release is folded into Created/Abort.
Outbox types are `task.created|task.updated|task.deleted|workspace.deleted`;
deletion payloads carry typed reason/source. The bridge never rereads deleted
tasks; migration uses explicit gap/watermark rules from the boundary contract.

`DeleteTask` snapshots task/session/runtime/environment/repository/worktree,
provider handles, and group cleanup into delete ownership, then writes
tombstone/revision/outbox, purges queues, and deletes live rows atomically.
Non-Git jobs may be `bounded_terminal`; Git cleanup remains unbounded
generation-fenced `cascade_retry`. Active-key deletion records
`confirmed_missing`/`delete_owned` and fences the actor; prior cancelled/restored
jobs start a higher-generation claim. Group cleanup is superseded only when its
last retained owner is deleted.

`DeleteWorkspace` rechecks confirmation/actor after locking workspace, supersedes
active task/group operations, deletes the registered DB set including blocker
endpoints/comments, fences config, inserts ordered task tombstones plus
dependent `workspace.deleted`, and commits external steps without provider I/O.
Inventory, generation proof, phase fencing, barrier release, dependencies, and
aftermath are normative in [Archive Cascade Boundary
Contracts](archive-cascade-boundary-contracts.md#complete-workspace-deletion-transaction).

Archive requests reuse operation state deterministically:

| Existing state | Repeated archive result |
| --- | --- |
| `preparing`, `archiving` | same operation/member snapshot; `in_progress` |
| `retry_wait(archive)` | same operation, retry time/error; no new claim unless due |
| `archived` | stored `archived` result and member/skipped IDs |
| `unarchiving`, `restore_pending`, `retry_wait(unarchive)` | typed `unarchive_in_progress` conflict; replay returns stored pending/result state |
| `completed`, `aborted`, `foreign_scope_resolved`, `workspace_deleted` | terminal barrier is released; a new archive may reserve and old-row replay remains by operation ID |

Delete during any active phase, including `archived`, atomically applies
workspace-deletion supersession under the workspace lock: it records
delete-owned member evidence, advances the operation generation, transitions the
operation to terminal `workspace_deleted`, releases its barrier, rejects later
unarchive/restore, and retains deterministic dispositions/event IDs. It never
waits for physical I/O. Response-loss replay returns the same operation ID,
terminal phase, member dispositions and event IDs; it never creates a second
identity.

## Verification and public CLI contract

Aggregate tests cover writer/delete, registry drift, lock tiers, actors/takeover,
jobs/markers, provider proof, security, replay, migration, tombstones, repeated
archive, and missing-group recreation on SQLite and PostgreSQL.

`docs/public/cli.md` must contain heading `## Reconcile blocked archive cascades`
and subheadings `### List blocked cascades`, `### Resolve a blocked cascade`,
`### Authentication and local-only access`, and `### Output and exit codes`.
The public-doc test asserts both full command prefixes, `--workspace`,
`--installation`, `--token-file`, `KANDEV_API_TOKEN`, `adopt`, `dismiss`,
`request_id_conflict`, `stale_revision`, `invalid_evidence`, redacted 401/403/404,
JSON stdout/stderr behavior, and exit codes `0`, `2`, `3`, `4`, and `5`.
