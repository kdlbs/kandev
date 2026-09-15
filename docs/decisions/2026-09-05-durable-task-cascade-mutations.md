# ADR-2026-09-05-durable-task-cascade-mutations: Persist task cascade mutation progress

**Status:** accepted
**Date:** 2026-09-05
**Area:** backend

## Context

Task-tree archive and unarchive mutate several independently persisted concerns: task archive stamps, runtime-cleanup jobs, workspace-group membership, and materialized workspace state. The initiating agent may stop itself before archive completes. Requests may be retried after a response disappears, the process may restart between member mutations, and concurrent task-parent mutations may change a dynamically traversed tree.

A request-scoped tree walk and a generated cascade ID cannot recover these cases. If descendants archive before the root fails, a new retry can create another cascade and make complete unarchive impossible. In-memory locks do not survive restart or coordinate PostgreSQL-backed processes. Compensating already-committed task mutations is itself fallible and can lose the cleanup ownership evidence needed for recovery.

## Decision

Task archive/unarchive uses a durable ledger with immutable workspace-scoped
task/group membership, per-member progress, and fenced attempts.

`internal/task/archivecascade` owns one Store/Runtime. Creation Begin returns a
private handle and persists the canonical CreationPlan plus one of six closed
typed step schemas at revision zero/no event:
`workspace_attachment|attachment_claim|fresh_branch_prepare|remote_contribution_associate|session_prepare|launch_intent`.
Kind-specific effect authorities, evidence, reconciliation, compensation, and
cleanup transfer gate Complete/Abort; Runtime alone recovers. External-ID
comparison is SQLite `BINARY` and PostgreSQL `"C"` with exact schema attestation.
Release has a stable operation result and never re-reads a later holder.
Workspace delete aborts preparing or completes-then-deletes completing creation.

Exact execution, boundary, creation, authenticated evidence, config, table, and
startup contracts live respectively in
[Archive Cascade Execution Contracts](../specs/tasks/system-design/archive-cascade-execution-contracts.md),
[Archive Cascade Boundary Contracts](../specs/tasks/system-design/archive-cascade-boundary-contracts.md),
[Task Creation Protocol](../specs/tasks/system-design/task-creation-protocol.md),
[Archive Cleanup Evidence](../specs/tasks/system-design/archive-cleanup-evidence.md),
[Workspace Configuration Fence](../specs/tasks/system-design/workspace-config-fence.md),
[Workspace Deletion Table Registry](../specs/tasks/system-design/workspace-deletion-table-registry.md), and
[Runtime Startup Registry](../specs/tasks/system-design/runtime-startup-registry.md).
They form one contract, not implementation options.

Reservation precedes traversal. One transaction commits all task/group snapshots,
both workspace identities, marker, and `archiving`. No cleanup exists before it.
Expired unmarked preparation may abort; committed membership always resumes.

Startup executes one typed B/Q/R/P/U symbol catalog: construct with no effects,
bind bootstrap liveness, run initial-agent plus authenticated agentctl
prerequisites, Start Runtime, then every named producer/subscription/route before
readiness. Independent discovery rejects missing/stale starters. Each partial
failure reverses all acquired resources exactly once.

Cleaner/restorer accepts byte-canonical database/resource/path keys, exact
marker generation/state/hash, and immutable snapshot binding. Local proof also
binds registered filesystem identity, canonical containment, Git directory,
branch, HEAD, and base commit; external proof requires an authenticated marker
under the protected runtime-data directory. No-follow handles and repeated
identity checks close check/use replacement. Claim commits before sorted physical
guards; no SQL transaction waits on them.
Registries cover closed creation-step schemas; every lifecycle writer; every
provider/reset provenance constructor; compensation; blocker/comment ownership;
one canonical typed table catalog shared by DDL, migration, deletion, and tests;
every workspaceconfig method; secrets; publishers; and startup effects.
AST/SSA/SQL/interface/schema checks reject omissions, aliases, and fallbacks.
PostgreSQL canonical advisory keys and SQLite immediate writes
linearize absent external/task identities; every command follows the published
17-tier mapping and restarts whole transactions after candidate/unique/deadlock
races. Sealed actors reload all bindings under lock.

Completed creation, external-ID release, archive/unarchive, and delete commit
state, retained revision, and immutable outbox together. A preparing creation
release uses `preparing -> preparing-release -> preparing`, clears its
external-ID claim, and emits no event; workspace deletion aborts either
pre-complete state without a task revision. Creation Complete moves only from
preparing to `completing`; `completing` cannot Abort and workspace deletion
settles it before emitting the ordered created/deleted revisions.
Repeated archive returns stored outcome. Closed typed delete reasons own cleanup
transitions. Unarchive reconstructs groups only from complete authenticated data.

Migration records absent pre-epoch revisions as gaps/watermark without marking a
retained event delivered. Task/Office handlers pass original confirmation and
trusted user actor through command-bearing service boundaries; Store reauthorizes both
under the workspace lock. Deletion uses exact table predicates, preserves
installation-global GitHub registrations/deliveries, removes incident blocker
edges/comments, supersedes cascades, and orders task tombstones before
`workspace.deleted`. `internal/workspaceconfig` resolves ID to unconstructable
fence and VerifiedConfigRoot with state/access reload; every loader/writer/Git/
sync/default/settings/memory/path-event API uses it. Name/raw-path APIs are gone.
Provider/config steps retry outside SQL. Both outboxes are sole lifecycle
publishers with no skip/dead-letter/attempt limit.

Legacy conversion accepts only live-task, committed-member, or retained
version-1 cleanup evidence whose canonical logical-database namespace,
task/resource/path/generation/state/marker/content identity, and RFC 8785
envelope are authenticated by retained-key HMAC. The complete contract and
per-kind identity matrix live in [Archive Cleanup Evidence](../specs/tasks/system-design/archive-cleanup-evidence.md).
Operation UUIDs and unbound cleanup/group rows only corroborate and remain
blocked. Physical absence is never inferred from generic not-found.
The dedicated-loopback API/CLI requires explicit workspace scope, or
auth-disabled local installation-recovery scope, even for zero candidates.
Pagination and request replay bind actor, scope/filter, ownership, cursor, limit,
expiry, action, evidence, and diagnostic revision and reauthorize each use.
Pre-auth/body/authz denials enter a separate redacted security audit with nullable
identities and fsynced emergency fallback; resolution-audit failure rolls back
the mutation and returns unavailable.

## Consequences

- Self-archive can outlive cancellation and restart without splitting one
  logical cascade.
- Response-loss retries converge without transport idempotency metadata.
- Cross-workspace parent edges remain supported outside cascade, while cascade
  fails closed before foreign side effects.
- Every sensitive writer shares aggregate admission and is guarded by AST
  inventory.
- The aggregate package intentionally crosses prior repository ownership.
- Physical cleanup/restore serialize on one resource fence.
- Partial unarchive invalidates archive phase and generation atomically.
- Real-repository same-database failpoint tests are required on both dialects.
- Events are ordered at-least-once with immutable payloads.
- Legacy candidates remain blocked until authenticated audited resolution.
- Cascade cleanup can retry indefinitely at a capped rate; exhausted diagnostics
  require operator visibility rather than an unrecoverable terminal state.
- The revision epoch cannot reserve a pre-migration ID absent from every retained
  source; this historical boundary remains explicit.

## Alternatives Considered

- **Client-provided idempotency keys:** rejected because current HTTP and MCP paths do not share a stable retry identity, and a key does not freeze tree membership or preserve partial mutation progress.
- **In-memory per-root locks:** rejected because they do not survive process restart or coordinate multiple backend processes.
- **Best-effort compensation of committed members:** rejected because rollback can fail after runtime cleanup starts and can erase the mutation identity required for safe recovery.
- **Dynamic re-traversal on every retry:** rejected because membership can change and produce mixed cascade identities or cross an authorization boundary.
- **Service-coordinated task and Office transactions:** rejected because no commit protocol makes independently constructed repository transactions atomic across a crash.
- **Re-read task state when publishing:** rejected because a later unarchive or delete cannot reconstruct the immutable payload and revision of the earlier committed transition.
