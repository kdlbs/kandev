---
status: draft
system: tasks
created: 2026-06-22
updated: 2026-09-05
owners:
  - cfl
---
# Task Runtime Cleanup Requirements

## Overview

Task lifecycle operations release runtime resources without deleting reusable task workspaces or
discarding the durable evidence needed to retry cleanup safely.

## Requirements

### REQ-TASKS-RUNTIME-CLEANUP-001: Task Runtime Cleanup

**Intent:** Make archive, delete, shutdown, and startup cleanup ownership-aware, durable, idempotent,
and safe when runtimes or task rows are already gone.

#### Acceptance criteria

- **AC-TASKS-RUNTIME-CLEANUP-001.1:** When a task is archived or deleted, the system shall stop every runtime recorded for that task before destructive workspace cleanup, using `executors_running` ownership rather than terminal session state alone.
- **AC-TASKS-RUNTIME-CLEANUP-001.2:** When a session is deleted, the system shall remove only that session and its references; the task-owned workspace, Git worktrees, branches, and reusable files shall remain until a task lifecycle operation requests cleanup.
- **AC-TASKS-RUNTIME-CLEANUP-001.3:** When a task environment is shared, cleanup shall stop the deleting task's runtimes but shall defer destructive environment or worktree teardown until no active session holds the shared environment, and shall never remove borrowed resources.
- **AC-TASKS-RUNTIME-CLEANUP-001.4:** When an agent or agentctl process does not stop within its grace period, the system shall terminate the complete process group and shall not report shutdown complete while descendants remain attached to PID 1.
- **AC-TASKS-RUNTIME-CLEANUP-001.5:** When startup reconciliation sees a stale runtime row, the system shall remove only a confirmed-dead local runtime, preserve alive, unknown, remote, or generically failed rows for retry, and emit bounded diagnostics for fail-closed outcomes.
- **AC-TASKS-RUNTIME-CLEANUP-001.6:** When lifecycle cleanup begins, the system shall persist an operation snapshot and retry state before mutating task state; repeated archive delivery shall reuse the active root operation or committed cascade, unarchiving shall cancel only cleanup owned by the mutation being reversed, and archive cleanup shall run only while its expected mutation identity matches the task.
- **AC-TASKS-RUNTIME-CLEANUP-001.7:** When an archived task is unarchived after
  cleanup removed its physical worktree, resuming the task shall recreate or
  reactivate the recoverable task-owned worktree instead of requiring
  attach-only reuse of the deleted workspace.
- **AC-TASKS-RUNTIME-CLEANUP-001.8:** When dead-row repair loses its compare-and-set to a newer execution, reconciliation shall preserve the newer row without a warning. Other repair errors shall remain warnings.
- **AC-TASKS-RUNTIME-CLEANUP-001.9:** When task deletion reclaims a Git worktree,
  the system shall verify exact recorded path, branch, and commit before mutation;
  preserve a checkout with tracked or untracked changes; preserve a clean local
  branch with commits not contained by recorded base or repository default;
  treat an absent checkout as idempotently removed only with matching
  authenticated marker and locked registration/provider proof; classify generic
  path/Git not-found as `unknown` and retry; and keep failed registration or
  branch cleanup retryable.
- **AC-TASKS-RUNTIME-CLEANUP-001.10:** When an active task archives itself
  through an agent operation, stopping that task's runtime shall not prevent
  the task from becoming archived and leaving active task views.
- **AC-TASKS-RUNTIME-CLEANUP-001.11:** When a task-tree cascade reserves its membership, concurrent child creation, deletion, or reparenting shall not change that membership; traversal shall reject cycles, duplicates, and oversized trees without unbounded allocation, and unarchive shall not report full success after a non-not-found task read failure.
- **AC-TASKS-RUNTIME-CLEANUP-001.12:** When archive or unarchive commits only part of a task-tree cascade before failing, retry or startup recovery shall resume the same immutable member set and cascade identity until every non-missing member reaches the requested state.
- **AC-TASKS-RUNTIME-CLEANUP-001.13:** When a task tree contains a descendant from another workspace, cascade archive and unarchive shall fail before stopping, cleaning, archiving, or restoring any task outside the authorized root workspace.
- **AC-TASKS-RUNTIME-CLEANUP-001.14:** When any snapshotted task/group cleanup,
  ownership generation, membership, or physical restoration fails during
  unarchive, the operation shall remain recoverable and shall not report success
  or expose the task as fully restored until every required resource is ready.
- **AC-TASKS-RUNTIME-CLEANUP-001.15:** When scheduled auto-archive selects a task, it shall use the same durable root-only cascade, cleanup identity, recovery, and unarchive behavior as an explicit archive.
- **AC-TASKS-RUNTIME-CLEANUP-001.16:** When startup finds legacy cascade state
  without an operation ledger, it shall apply the explicit evidence-proof matrix
  and convert only canonically identified members with independent workspace and
  committed-membership proof. Unsafe cleanup-only, malformed, missing,
  ambiguous, or cross-workspace evidence shall block the cascade without
  mutation.
- **AC-TASKS-RUNTIME-CLEANUP-001.17:** Archive, unarchive, and delete commits persist immutable payload, revision, and queue sequence before publication; the sole outbox locks each epoch's `(revision, queue_sequence)` cursor, publishes its exact successor, and advances after success with at-least-once retry. Envelopes carry epoch/revision/queue sequence, event ID, and terminal/tombstone state; gateways/clients discard older cursors and nonterminal updates after tombstones.
- **AC-TASKS-RUNTIME-CLEANUP-001.18:** When archive membership commits, the task members, every group membership and workspace identity, the membership marker, and the operation phase shall commit in one database transaction or all roll back.
- **AC-TASKS-RUNTIME-CLEANUP-001.19:** When a task or group cleanup worker crashes, stalls, or loses its lease, a successor may claim a higher generation, and the stale worker shall be unable to complete state transitions or destructively affect the newer lifecycle.
- **AC-TASKS-RUNTIME-CLEANUP-001.20:** When a group is attached to a task or captured by archive reservation, the group workspace shall equal the task and authorized root workspace; mismatch shall fail before group, cleanup, runtime, or task side effects.
- **AC-TASKS-RUNTIME-CLEANUP-001.21:** When unarchive begins while the matching cascade is archiving or waiting to retry archive, it shall atomically invalidate the archive generation, reverse only ledger-owned committed effects, and prevent delayed archive work from mutating tasks, groups, cleanup, operation phase, or barriers.
- **AC-TASKS-RUNTIME-CLEANUP-001.22:** When legacy evidence proves multiple candidate roots but not one safe cascade, every proven candidate shall retain a durable admission barrier across restart until an audited resolution either supplies uniquely valid evidence or explicitly dismisses the diagnostic.
- **AC-TASKS-RUNTIME-CLEANUP-001.23:** When a task-session, runtime, environment,
  or physical-handle writer can create or alter a cleanup obligation, it shall
  serialize with archive preparation so writer-first state enters the immutable
  cleanup snapshot and reservation-first rejects the write. Every production
  writer to those tables shall be registered as admitted or as a column-scoped
  exemption whose invariant proves it cannot alter a cleanup obligation.
- **AC-TASKS-RUNTIME-CLEANUP-001.24:** When concurrent lifecycle mutations
  enqueue task events, each transaction shall allocate a unique monotonic task
  revision from durable state retained after deletion. Revision `n + 1` shall
  remain ineligible until revision `n` is delivered; the system shall not
  dead-letter, skip, or reset a revision.
- **AC-TASKS-RUNTIME-CLEANUP-001.25:** When tasks, groups, environments, or
  physical rows are deleted before cleanup or restore completes, operation,
  membership, group snapshot, cleanup job, resource marker, lifecycle revision,
  outbox, diagnostic, and audit evidence shall remain available to retry or
  diagnose the operation.
- **AC-TASKS-RUNTIME-CLEANUP-001.26:** When group cleanup outcome becomes
  unknown, archive and unarchive shall remain blocked until fenced reconciliation
  proves the cleanup succeeded or proves retry is safe; stale generations shall
  not resolve the state.
- **AC-TASKS-RUNTIME-CLEANUP-001.27:** When independent workers identify the same
  task, group, environment, worktree, or physical path, they shall derive the
  same versioned database-scoped resource key and validate the same durable
  lifecycle marker before I/O, including across path aliases and process restart.
- **AC-TASKS-RUNTIME-CLEANUP-001.28:** Blocked-evidence list/resolve surfaces
  shall provide versioned auth, redaction, idempotency, and exit contracts.
  Foreign resolution accepts auth PAT, instance-admin role, ownership proof for
  every live/retained workspace, absent-root proof, scope digest/member set, and
  preserves foreign resources; request-ID replay is stable and hash changes
  conflict.
- **AC-TASKS-RUNTIME-CLEANUP-001.29:** When HTTP, WebSocket, MCP, scheduled,
  startup, or reconciliation callers request a cascade-sensitive group command,
  the aggregate shall derive a typed actor from trusted context, recheck root
  workspace ownership in its transaction, and reject caller-supplied or
  cross-workspace authority with no side effects.
- **AC-TASKS-RUNTIME-CLEANUP-001.30:** When a lifecycle transition commits, one
  outbox row shall be its sole authoritative event identity and the event-bus
  bridge shall publish that identity exactly once per delivery attempt; no
  direct publisher path shall emit a second identity for the same transition.
- **AC-TASKS-RUNTIME-CLEANUP-001.31:** When the reconciliation operator command
  ships, `docs/public/cli.md` and its contract test shall cover both exact command
  forms, PAT/auth-disabled operation, replay and conflict codes, redaction, JSON
  output, and exit codes `0/2/3/4/5`.
- **AC-TASKS-RUNTIME-CLEANUP-001.32:** When a cascade-critical task or group
  cleanup exhausts the normal retry schedule, it shall retain the diagnostic and
  continue generation-fenced retry at the capped interval; it shall not enter an
  unrecoverable terminal state that permanently blocks archive or unarchive.
- **AC-TASKS-RUNTIME-CLEANUP-001.33:** When the local operator API rejects a
  request before identity or body parsing, it shall record a separate redacted
  security audit that permits absent actor, request, and diagnostic identities;
  an audit failure shall never permit a resolution mutation.
- **AC-TASKS-RUNTIME-CLEANUP-001.34:** When legacy evidence names a deleted task,
  a cleanup snapshot shall prove conversion only when it is cryptographically
  bound to a committed immutable operation/member marker; an unbound cleanup row
  shall remain blocked.
- **AC-TASKS-RUNTIME-CLEANUP-001.35:** Every aggregate/worker/dispatcher/operator
  command shall follow one complete order. External-ID/task absent keys shall
  have canonical dialect-specific linearization, collision/restart semantics,
  and command-by-command Begin/Complete/Abort/Release/Delete ordering.
- **AC-TASKS-RUNTIME-CLEANUP-001.36:** Revision-registry migration and workspace
  deletion seed every knowable identity and retain the historical boundary.
  Workspace deletion retains the next revision and deletion tombstone before live
  removal. `completing` commits phase A, then phase B allocates revision one and
  `task.created`; workspace deletion adds revision two and `task.deleted` in that
  finalization. `preparing|preparing-release` abort with retained evidence and no
  lifecycle revision/event. Replay preserves this matrix.
- **AC-TASKS-RUNTIME-CLEANUP-001.37:** When archive is repeated against an
  existing operation or a snapshotted group is missing during unarchive, the
  state machine shall return a deterministic stored result or recreate the group
  from complete immutable proof; incomplete/conflicting group proof shall remain
  blocked with a typed diagnostic.
- **AC-TASKS-RUNTIME-CLEANUP-001.38:** When a task is created, every insertion
  entry point shall use one retained aggregate protocol: begin shall reserve the
  internal identity at revision zero without an event, completion shall verify
  all required synchronous writes and atomically allocate revision one plus
  exactly one `task.created`, and abort shall retain identity while emitting no
  lifecycle event.
- **AC-TASKS-RUNTIME-CLEANUP-001.39:** When revision/event state is migrated,
  an absent pre-epoch revision may enter a historical gap only with
  authenticated `pre_epoch_no_event` proof bound to its identity, epoch, range,
  and evidence hash; missing proof creates a blocking diagnostic/no epoch. An
  existing nondelivered event shall never be marked delivered or skipped.
- **AC-TASKS-RUNTIME-CLEANUP-001.40:** Workspace deletion shall use one
  database-owned command to delete workspace-owned rows, retain tombstones, and
  retry every external cleanup effect durably without limit.
- **AC-TASKS-RUNTIME-CLEANUP-001.41:** Workspace deletion racing a
  `{workspace}` cascade shall fence claims, transfer cleanup to delete
  ownership, terminalize workspace-deleted, release its barrier, and preserve
  task-before-workspace event order. Foreign scope enters
  `foreign_scope_blocked`; reconciliation preserves foreign members/resources.
  A deleted workspace's group with foreign members is rehomed with shared
  resources to a deterministic surviving owner, retaining foreign edges with
  valid FKs and ownership-generation evidence for later reconciliation.
- **AC-TASKS-RUNTIME-CLEANUP-001.42:** After archive preparation, every
  request-owned rollback, recovery, finalization, vacancy, activation, and
  compensation context shall use the earlier of the original absolute deadline
  and its step budget; an expired request shall persist retry/wakeup rather than
  extending work with a fresh context.
- **AC-TASKS-RUNTIME-CLEANUP-001.43:** When a snapshotted group is missing,
  immutable proof shall contain every group and ordered membership field
  required to reconstruct the exact row set, protected by canonical encoding,
  schema version, and authenticated digest; incomplete or conflicting proof
  shall remain blocked.
- **AC-TASKS-RUNTIME-CLEANUP-001.44:** Legacy cleanup conversion shall require
  the exact version-1 RFC 8785 HMAC envelope over the logical database
  namespace, operation/member/task/workspace IDs, resource kind/key/path,
  ownership generation/state, marker ID/hash, and kind-specific content identity
  under a retained verification key. Locked database/provider/physical checks
  shall recompute every field. An operation UUID, summary hash, or cleanup-row
  hash alone shall not constitute cryptographic proof.
- **AC-TASKS-RUNTIME-CLEANUP-001.45:** A cleanup transfer or legacy conversion
  involving a local worktree or Git registration shall prove the exact
  workspace/environment/repository/worktree identity, canonical path hash,
  branch/HEAD/base state, ownership marker and generation under database locks,
  plus authenticated filesystem marker and locked Git-registration evidence.
  An authenticated marker plus absent Git registration proves prior removal;
  generic path-not-found, an absent database row, credential loss, or any
  identity mismatch is unknown and remains retryable. Physical guards run only
  outside the SQL transaction and never replace the database proof.
- **AC-TASKS-RUNTIME-CLEANUP-001.46:** Every public task/workspace deletion
  wrapper and lifecycle publisher shall delegate to the aggregate exactly once;
  direct lifecycle publishers, nil fallbacks, and destructive event subscribers
  shall be removed, and deletion reason shall be retained in the event payload.
- **AC-TASKS-RUNTIME-CLEANUP-001.47:** Every external aggregate action shall use
  a sealed actor variant carrying concrete trusted identity, authorization mode,
  resource bindings, event/message identity, and worker lease/generation, and
  each mutation transaction shall reload and compare all applicable fields.
- **AC-TASKS-RUNTIME-CLEANUP-001.48:** A mandatory archive-cascade runtime shall
  start after migrations and bootstrap-listener bind but before any producer,
  production route, or readiness publication; it shall sweep/wake due work,
  deliver both outboxes, and retry indefinitely without optional scheduling.
- **AC-TASKS-RUNTIME-CLEANUP-001.49:** Every confirmed workspace-delete surface
  shall carry confirmation name and trusted actor into the aggregate, which
  shall reload both after acquiring the workspace lock; a pre-lock summary match
  shall never authorize deletion.
- **AC-TASKS-RUNTIME-CLEANUP-001.50:** Every configuration read/write/reload/Git/
  sync/default/settings/memory/delete operation shall use the closed resolver,
  unconstructable workspace-ID fence, claim-state/access-mode reload, and
  verified-root lifecycle; no name/raw-path API or stale/ambiguous I/O remains.
- **AC-TASKS-RUNTIME-CLEANUP-001.51:** The workspace table registry shall
  explicitly classify `task_blockers` in both endpoint directions and
  `task_comments`; deletion shall lock affected internal and external tasks,
  remove only edges incident to deleted tasks, and preserve unrelated rows.
- **AC-TASKS-RUNTIME-CLEANUP-001.52:** Production shall execute one checked-in
  symbol-level starter registry containing stage, side effects, exact cleanup,
  reverse-unwind order, failure policy, and readiness impact. It shall construct
  without starts, bind only bootstrap liveness, then run only synchronous
  initial-agent setup and non-task-producing authenticated agentctl health before
  Runtime sweep. Every other process, goroutine, subscription, producer, route,
  and readiness publication shall be an enumerated post-gate entry. Independent
  discovery shall reject omissions/stale entries, and every partial-start or
  post-bind failure shall restore not-ready state and close all acquired
  resources exactly once in reverse order.
- **AC-TASKS-RUNTIME-CLEANUP-001.53:** Every deletion caller shall use its exact
  closed code/source and canonical versioned SourceID through a trusted typed
  constructor. HTTP, MCP, expiry, profile, automation, plugin, e2e, workspace,
  terminal provider events, and GitHub/GitLab/Azure/Jira/Linear/Sentry reset
  pairs shall be deterministic. Reset reasons shall be persisted before watch
  removal; missing/unmapped provenance fails before mutation; compensation uses
  Abort; no generic provider/reset/deleter fallback remains.
- **AC-TASKS-RUNTIME-CLEANUP-001.54:** Created shall return a private opaque
  handle bound to actor/token/complete CreationPlan. Every required effect shall
  use a durable typed step with deterministic idempotency key, lease/generation,
  evidence, unknown-result reconciliation, and compensation state. Only all-
  succeeded proof may Complete; only proved/compensated/transferred permanent
  failure may Abort; ambiguous Complete retries the same handle and never Abort.
- **AC-TASKS-RUNTIME-CLEANUP-001.55:** One typed table catalog shall drive fresh
  DDL, replay migration, workspace deletion, retained-FK validation, and dialect
  tests. It shall enumerate every canonical live/retained/global table name,
  owner selector/join, lock tier/ordinal, retention/lifecycle, and FK invariant,
  including `workspace_config_path_claims` and nullable GitHub provenance.
  Missing, stale, wildcard, scratch, or simultaneously live alias entries fail.
- **AC-TASKS-RUNTIME-CLEANUP-001.56:** Office HTTP deletion shall extract a
  trusted user actor and pass it with the caller's original confirmation through
  the Office service directly to the aggregate; no summary-derived name or
  authorization value may replace either field.
- **AC-TASKS-RUNTIME-CLEANUP-001.57:** Workspace deletion shall abort
  `preparing` creation operations without task lifecycle events and shall finish
  `completing` operations before deleting them, producing ordered created then
  deleted revisions; replay shall not strand an unsettled task.
