---
created: 2026-09-05
status: draft
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-EXTERNAL-ID-BOUNDARIES-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup.md
  - ../../specs/tasks/system-design/durable-archive-cascades.md
  - ../../specs/tasks/system-design/archive-cascade-execution-contracts.md
  - ../../specs/tasks/system-design/archive-cascade-boundary-contracts.md
  - ../../specs/tasks/system-design/archive-cleanup-evidence.md
  - ../../specs/tasks/system-design/archive-cascade-registries.md
  - ../../specs/tasks/system-design/runtime-state-publication-order.md
  - ../../specs/tasks/system-design/external-id-idempotency.md
  - ../../specs/tasks/system-design/task-creation-protocol.md
  - ../../specs/tasks/system-design/detached-workspace-continuity.md
  - ../../specs/tasks/system-design/workspace-config-fence.md
  - ../../specs/tasks/system-design/workspace-deletion-table-registry.md
  - ../../specs/tasks/system-design/session-delete-resource-cleanup.md
  - ../../specs/tasks/system-design/environment-owned-git-status.md
  - ../../specs/tasks/system-design/runtime-startup-registry.md
legacy_specs: []
---

# Implementation Plan: Agent Self-Archive Cancellation

## Overview

Make task-tree archive survive self-cancellation, restart, and concurrent
lifecycle work. Four sequential work orders add a fenced durable cascade
ledger, make task/group cleanup identity-safe, make every archive source and
unarchive resumable, and prove both database dialects.

## Confirmed root cause

The MCP archive handler uses `HandoffService.ArchiveTaskTree` in production.
That service persists cleanup intent, calls `cancelActiveRuns`, and only then
archives the task. When the target task is also the MCP caller,
`CancelTaskExecution` stops the calling session and cancels the request context.
The archive loop reuses that cancelled context, so the repository mutation
returns `context canceled`; the tool response disappears with the stopped
session and the task remains active.

A temporary focused test reproduced the defect by making the runtime canceller
cancel the parent context synchronously and making the archive repository reject
a cancelled context. Before production changes, the test failed with
`self-archive: archive root: context canceled`.

## Design checkpoint

This package is the repository-required pre-implementation checkpoint. No
production implementation or permanent regression test is authorized in this
turn; all work orders and verification results intentionally remain pending.
Review acceptance at this checkpoint means requirements, architecture, work
orders, and executable verification are complete. The later explicit
implementation request must create/wire `internal/task/archivecascade`, make the
tests pass, record results, and update public operator documentation.

The completed `detached-workspace-continuity` and
`subtask-reparenting-drag-drop` plans are the implementation baseline, not
pending work to repeat. Their shipped Store/service/UI behavior remains in
place. The reparenting requirement is an external prerequisite and is
intentionally absent from this package's authoritative requirements,
source inventory, and traceability. This package owns only incremental
detached-cascade integration, shared-ledger replay, lock-order, dialect, and
route/provenance verification; it does not supersede or re-implement completed
plans. Command blocks record cross-wave checks without transferring acceptance
ownership.

## Detachment traceability

`REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001` and `-002` are implemented
through the completed baseline plus the incremental cascade contracts in
`detached-workspace-continuity.md`, including conditional ownership transfer,
canonical creation attachment, cleanup fencing, and publication.
- Task 01 owns the incremental cascade integration around `Store.DetachTask`,
  private dialect helpers, and route cutover.
- Task 04 owns SQLite/PostgreSQL mode, crash/replay, cleanup-race, sibling-lock,
  adjacent-inversion, and lifecycle-outbox verification.
- Task 04 acceptance and its named PostgreSQL commands are mandatory for this
  dependency; a missing isolated DSN leaves the PostgreSQL criteria pending.

## Scope

### In scope

- Add Store/Runtime over one DB. Production constructs without starts, binds
  bootstrap, runs only initial-agent/agentctl prerequisites, then gates every
  other starter/route/readiness on Runtime; all failures reverse-clean.
- Replace all task inserts/finalization/rollback with handle/CreationPlan and
  retained typed steps. Idempotency/evidence/unknown/compensation gates Complete
  or Abort; Runtime recovers without caller adoption.
- Route every deletion through trusted canonical provenance and aggregate
  commands. Recheck Task/Office confirmation/actor; classify every exact table,
  blocker endpoint, and comment. Retain GitHub installation provenance.
- Replace every name/raw-path config operation with workspaceconfig Resolver,
  unconstructable Fence, VerifiedRoot, state/access reload, and quarantine.
- Replace destructive subscribers with durable steps; remove reasonless wrapper/
  provider fallbacks and direct lifecycle publishers.
- Implement concrete sealed actor variants with exact trusted identities,
  authorization modes, scoped bindings, message/event identities, and worker
  lease/generation; reload every applicable binding transactionally.
- Add complete immutable group snapshots, retained task/group cleanup evidence,
  HMAC-bound legacy proof, resource markers, and Git/filesystem identity proof
  resistant to symlink and check/use replacement.
- Persist explicit historical revision-gap/watermark state, retry kind,
  repeated-archive/missing-group outcomes, active-workspace-delete supersession,
  no-skip outboxes, audits, and perpetual retry state.
- Apply one request deadline through every nested rollback/recovery/finalization
  context; an expired request persists a wakeup for a later worker attempt.
- Add the exact scoped loopback API/CLI with PAT/ownership checks, bound
  pagination/replay, non-vacuous empty-candidate policy, pre-auth audit fallback,
  redaction, JSON, and exit codes.
- Prove both dialects, all 17 lock tiers, runtime restarts, all public wrappers,
  and document/assert the exact `docs/public/cli.md` contract.

### Out of scope

- Changing delete-tree detached-cancellation behavior.
- Migrating or globally forbidding existing cross-workspace parent edges.
- Frontend layout/navigation/touch changes and external-ID support on
  WebSocket/plugin create remain out of scope. The existing session-delete
  resource contract and environment-owned Git-status design are an explicit
  authority dependency; Task 02 preserves the backend/store invariant and Task
  04 runs the existing environment-keyed store/hydration verification for
  desktop and mobile Changes without claiming a UI implementation.
- No new Playwright scenario is required because the frontend design records no
  layout, navigation, touch, scrolling, or viewport-dependent interaction
  change. REST/MCP lookup-first creation, aggregate release, plugin-host typed
  deletion provenance, internal operator loopback, native CLI, and CLI reference
  remain in scope.

## Technical approach

### Durable operation ledger

- Add creation operation/typed step ledger, release replay, cascades, snapshots,
  cleanup jobs, markers, revisions/gaps/watermarks, outboxes, and diagnostics.
- REST/MCP authorize/normalize external ID and lookup before non-identity decode;
  Found wins over drift while envelope errors retain precedence.
- Created alone returns a private handle bound to canonical CreationPlan,
  ManifestHash, ActorBindingHash, and token. The six closed step kinds,
  including `workspace_attachment`, define exact request/evidence schemas,
  effect-authority rows, reconciliation outcomes, compensation, and cleanup
  transfer. `task_creation_steps` solely owns state. Complete/Abort use closed
  proof predicates; request failure never directly deletes/settles; Runtime
  reconciles orphaned/unknown/compensation work.
- Complete first persists non-abortable `completing`, then revision one/event;
  ambiguous results retry the handle. Preparing release is durable
  `preparing -> preparing-release -> preparing`, clears the external-ID claim,
  emits no event, and is recoverable by the same release operation. Stable
  release operations store result and never inspect later reuse. Repository
  settle/release bypasses are removed.
- Workspace deletion carries original confirmation/actor, whose canonical
  `UserActor` includes `UserID`, `RequestID`, `AuthMode`, and `Synthetic`;
  aborts preparing or `preparing-release` without a task event and
  completes-then-deletes completing creation, removes exact owned rows,
  preserves unrelated blockers/comments and GitHub global provenance, and
  orders events.
- `internal/workspaceconfig` owns resolver, unconstructable fence, state/access
  reload, VerifiedConfigRoot, cache invalidation, path-event resolution, and
  quarantine lifecycle. Replace every exact loader/writer/memory/Git/sync/
  default/settings/delete method; remove name/raw-path APIs.
- One typed catalog in `workspace-deletion-table-registry.md` drives DDL,
  replay migration, deletion, retained FK checks, and dialect tests. It owns
  every exact canonical name, selector, tier/ordinal, retention/lifecycle, and
  no-cascade invariant. Task 04 derives the production set independently.

### Membership, authorization, and lock order

- Registries cover every step/release/lifecycle writer; each HTTP/MCP/
  maintenance/provider-reset/automation/plugin/e2e/workspace provenance
  constructor; compensation; exact table/config method; secret; publisher; and
  startup effect. AST/SSA/SQL/interface/schema checks reject omissions/aliases.
- Sealed actors reload all authorization/resource/event/lease bindings.
- Enforce the complete command mapping across 17 tiers, including every
  creation-step/compensation/recovery/aftermath worker and config/operator
  command. PostgreSQL uses canonical external-ID/task advisory keys; SQLite
  begins immediate writes. Delivery locks workspace, retained revisions/
  watermarks/gaps at tier 6, then dependent task/workspace outbox rows at tier
  16; physical guards never overlap SQL locks. Tests force digest collision,
  candidate change, absent insertion, uniqueness/deadlock whole-transaction
  restart, delivery races, and every adjacent inversion.

### Caller cancellation and cleanup transitions

- Add `TaskArchiveTimeout = 2 * time.Minute` as the one request deadline.
  Preparation remains caller-cancellable; later request work drops cancellation
  but every rollback, recovery, finalization, vacancy, activation, and nested
  compensation context uses `min(original_deadline, now+budget)`.
- When no request time remains, persist retry/wakeup and return the operation ID.
  Only the mandatory runtime may later claim a separately bounded worker attempt.
- Persist the complete task/group cleanup and restore table. Unknown outcome
  requires fenced provider proof; cascade-critical failures retain diagnostics
  and retry every capped 12 hours rather than terminally wedging the operation.
- Implement `archive-cleanup-evidence.md` exactly: canonical logical-database
  namespace, resource/path/generation/state/marker/content subdocument, retained-
  key HMAC, closed kind content identities, and locked DB/physical recheck.
  Missing DB metadata is distinct from physical absence; generic physical
  not-found is unknown unless exact authenticated marker/provider proof exists.
- Claim commits before sorted guards. No SQL transaction waits for a guard;
  no-follow handles and repeated object checks reject check/use replacement.
  Restore joins identical keys only after lifecycle generation advances.

### Resumable archive

- Commit bounded membership before cleanup; abort only unmarked preparation.
- Archive deepest first. `archiving|retry_wait(archive)` may CAS directly to
  `unarchiving` with a new generation; old archive work cannot finalize.
- Retain the barrier through archive/retry/unarchive and release only on
  completed reversal or zero-commit abort.
- Scheduled auto-archive only submits roots; mandatory startup/wakeup recovery
  owns operation progress independently of scheduler enablement.

### Resumable unarchive

- Resolve unarchive from the retained ledger independent of root state.
- Cancel/join exact task/group claims; task and group jobs use explicit restore
  claims, fences, unknown reconciliation, and success criteria.
- Recreate missing groups only from a versioned canonical snapshot containing
  every group field and ordered member identity/ordinal, with authenticated
  digest; incomplete/conflicting proof remains `missing_group_snapshot`.
- Restore task metadata without physical worktree I/O. The first resumed session
  invokes admitted environment materialization to recreate/reactivate a proven
  task-owned worktree before becoming runnable.
- Commit restoration, member/group disposition, and event outbox atomically;
  record proved missing tasks and propagate other reads.

### Legacy conversion and event delivery

- Require retained-key HMAC proof over database/task/resource/path,
  generation/state, marker, and content identity; UUID/hash-only evidence stays
  blocked. Implement the exact scoped loopback authorization/audit contract.
- Migrate knowable identities plus explicit historical gaps/watermark; never
  synthesize delivery of retained events and reject retained-ID reuse.
- Aggregate owns completed creation, completed-task external-ID release, all
  lifecycle/deletion/workspace events. Preparing release has no event. Remove
  direct/optional publishers and reasonless/repository/type-assertion fallbacks.
- Execute the exact B/Q/R/P/U symbol catalog in `runtime-startup-registry.md`;
  Task 03 owns `apps/backend/internal/backendapp/startup_registry.go` and its
  sole runner. Task 04 independently discovers each process/goroutine/
  subscription/listener/sweep/route/readiness effect and owns the deterministic
  declared export plus verifier; reverse-clean every partial start. Creation
  compensation uses step-ledger Abort.

### Regression coverage

- Registry compares closed CreationPlan/step schemas, release/lifecycle writers,
  every provider/reset provenance constructor, config method, publisher, and
  startup effect.
- Task 04 independently derives current DDL and proves the one canonical table
  catalog, migration aliases, predicates/tiers/lifecycles, and retained FKs.
- Controlled races cover lookup preflight, per-kind creation effect/proof/
  compensation, Complete ambiguity, strict dialect collation upgrade/attestation,
  release reuse, advisory-key collision/restart, all tiers, config, blockers/
  comments, full evidence-envelope tampering, and physical absence.
- Production tests compare every discovered starter symbol with B/Q/R/P/U and
  inject each partial-start/failure boundary, readiness transition, reverse
  cleanup, and stop-once reset/restore/shutdown.
- Restart beyond retry exhaustion proves recovery with optional schedulers off.
- Update backend guidance and validate exact CLI documentation strings.

## Tests

- Runtime-cleanup criteria `.1`-`.57` and external-ID `.1`-`.8` own aggregate
  atomicity, core runtime/session/environment/process cleanup, creation/release,
  typed deletion, cleanup/proof, config, locks/actors, events, and Runtime.
- Per-kind failpoints prove canonical plan/actor/manifest/request bindings,
  effect-authority evidence, reconciliation, compensation/transfer, Complete/
  Abort predicates, and workspace delete against both unsettled states.
- Focused runtime tests cover stop ordering, session-resource retention, shared/
  borrowed deferral, process-group join, stale-row disposition, worktree
  recreation, newer-generation CAS, Git identity, and physical-absence proof.
- REST/MCP preflight preserves Found/error precedence.
- Release matrices cover every state/race and replay after later reuse.
- SQLite/PostgreSQL tests prove `BINARY`/`"C"` fresh/upgrade/replay/rollback and
  case-distinct identities, plus absent-key collision, candidate restart, and
  command-by-command tier order.
- Independent schema inventory proves each canonical workspace table, alias
  migration, owner predicate/tier/lifecycle/FK, and global retention.
- Trusted provenance tests exercise every constructor including all six closed
  reset providers and reject missing/noncanonical fields before writes.
- Resolver/Fence/VerifiedRoot tests exercise every replacement config method,
  claim state/access, cache invalidation, path event, and same-name race.
- Startup discovery and failpoint tests cover every B/Q/R/P/U effect, liveness/
  readiness publication, reverse unwind, and stop-once quiescence.
- Operator and CLI tests cover exact contracts and public documentation tokens
  on both dialects.

## Review findings

Luna rounds 1-13 found and drove closure of durability, atomicity, authorization,
lock-order, cleanup, operator, migration, idempotency, and event-delivery gaps.
Round 14 identified eleven remaining boundary ambiguities: task creation bypass,
migration gaps, complete/durable workspace deletion, active-cascade deletion,
absolute deadline preservation, reconstructable group snapshots, authenticated
legacy and Git proof, public publisher/wrapper cutover, concrete actor bindings,
and mandatory recovery ownership. Acceptance criteria `.38`-`.48` and
[Archive Cascade Boundary Contracts](../../specs/tasks/system-design/archive-cascade-boundary-contracts.md)
make those behaviors implementable and assign them to the four work orders.
Round 15 found six remaining cutover/composition gaps. Criteria `.38` and
`.49`-`.53` now require manifest-verified two-phase creation, lock-time Office
confirmation, fenced name-based config identity, explicit blocker/comment
ownership, the concrete pre-poller Runtime/quiescence seam, and typed deletion
reason mappings with no compatibility publisher/fallback.
Round 16 identified eight implementability gaps plus external-ID ownership.
Criteria `.54`-`.57` and external-ID `.4`-`.8` define opaque creation handles,
completing/recovery/workspace-delete ordering, lookup-first REST/MCP behavior,
stable release-operation replay and aggregate revisions/events, exhaustive delete
callers, exact Office signatures, bootstrap-liveness Runtime ordering, GitHub
registration/delivery retention, and every config access path.
Round 17 found seven remaining authority/implementation gaps. The stale external-
ID operations design is retired. Runtime AC `.35`, `.50`, `.52`-`.54` plus
external-ID criteria now define canonical absent-key locks and command order,
typed creation-step recovery, exact config replacement APIs/lifecycle, canonical
trusted deletion provenance, the complete bootstrap starter/failure inventory,
and explicit Task 01 implementation versus Task 04 independent verification of
the exact table registry.
Round 20 resolved nine further gaps: the workspace revision matrix, AC `.45`
filesystem/Git proof, config-claim table registration, unique registry artifact
paths and ownership, complete aggregate/worker/operator lock mapping, durable
`preparing-release`, one RequestID-bearing UserActor, session-delete authority
coverage, and the independent verifier CLI contract.
Round 21 found three ownership/delivery gaps. They are resolved by making Task
03 the sole startup-registry/runner owner, making Task 04 own only the declared
export and independent verifier, defining the canonical `declared.json` export
schema/path/freshness digest, and mapping creation-step/recovery/aftermath
workers plus tier-6 watermark locking before tier-16 outbox delivery.

## Work orders

- [ ] [Task 01: Persist cascade operations and admission](task-01-persist-cascade-operations.md)
- [ ] [Task 02: Harden cleanup identity and cancellation](task-02-harden-cleanup-identity.md)
- [ ] [Task 03: Resume archive and unarchive](task-03-resume-archive-unarchive.md)
- [ ] [Task 04: Verify production surfaces and dialects](task-04-verify-surfaces-dialects.md)

## Verification results

Pending.

## Risks

- The aggregate package crosses prior repository ownership intentionally;
  bypassing it violates atomicity.
- Writer inventory must be complete and checked during implementation.
- Leases do not fence I/O; the resource guard is the linearization boundary.
- Partial unarchive must invalidate the archive generation atomically.
- Blocked candidates can halt archive until audited repair; unsafe automatic
  release is forbidden.
- Cascade jobs deliberately trade terminal dead ends for capped perpetual retry;
  diagnostics and explicit operator resolution must remain observable.
- Pre-migration tasks absent from every knowable retained source cannot be
  reserved retroactively; the recorded epoch makes that limit explicit.
- Failure to persist pre-auth denial audit must emit an fsynced emergency record
  and metric without weakening request denial.
- The local maintenance command adds public operator documentation.
- Workspace deletion completes its database boundary before unbounded external
  cleanup; retained step state and idempotency keys are therefore mandatory.
- Runtime composition must preserve launcher liveness while withholding readiness
  and all producers/real routes until recovery succeeds.
- Creation handles span physical work; `completing` is the durable no-Abort point
  for ambiguous commit and Runtime takeover.
- Lookup-first preflight intentionally ignores invalid non-identity fields on a
  Found request; authorization and external-ID validation still precede lookup.
- Plugin/automation/e2e deletion adapters must gain trusted source IDs rather
  than inventing reasons inside the generic task service.
- Legacy duplicate workspace names block every config access until ownership is
  resolved, not only deletion.
- SQLite success is not PostgreSQL proof.
