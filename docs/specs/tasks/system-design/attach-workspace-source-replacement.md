---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002
owners:
  - kandev
---

# Complete Repository Association Replacement System Design

## Purpose and boundaries

Tasks owns the ordered `task_repositories` association set and its replacement lifecycle.
This extends [Attach Workspace Sources](attach-workspace-sources.md), whose existing design is near
its size limit, with one focused persistence contract. It does not duplicate the materialization
contract, or [multi-branch behavior](../requirements/multi-branch.md), or
[launch resolution](launch-repository-resolution.md). The latter explicitly accepts changes to the
attachment set during launch; this repair adds no attachment-to-environment inventory comparison.
The [runner-switch boundary](runner-switch-before-materialization.md) and its F19 accepted risk
remain applicable: replacement does not run the runner mutability evaluator.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002` | [Preparation and canonical finalization](#preparation-and-canonical-finalization), [Atomic store boundary](#atomic-store-boundary), [Compatibility and references](#compatibility-and-references), [Transport and consumers](#transport-and-consumers), [Failure and recovery](#failure-and-recovery) |

## Components and current paths

`Service.ReplaceTaskRepositories` and the private `replaceTaskRepositories` in
`internal/task/service/service_tasks.go` are the complete-set mutation entry points. `UpdateTask`
uses the private path after persisting ordinary task fields. HTTP `commitFreshBranch` uses the
exported path after creating/checking out the branch. There are no other live exported callers.
`createTaskRepositories` and `persistTaskRepositoryRows` resolve then write single rows; the old
replacement calls them only after `DeleteTaskRepositoriesByTask` has independently committed.
Resolving the next input or inserting its second row can therefore destroy the original set.

The actual association interface is `repository.TaskRepoRepository` in `interface.go`;
`TaskRepository` is the different task-row interface. `repository.ProvideContext` selects
`sqlite.Repository`, with writer `db` and reader `ro`, for both SQLite and PostgreSQL. There is no
existing service-accessible task transaction callback in these interfaces. `CreateWorkspaceSourceBatch`
is the closest atomic association writer, but appends folders/repositories with cross-source
positions and materialization compensation; it cannot serve full replacement unchanged.

## Preparation and canonical finalization

Use two narrowly scoped phases rather than running provider resolution inside a held writer transaction.

1. Copy request inputs so preparation and inheritance never modify caller-owned slices or snapshots.
   Preserve the registered update's omitted/null versus explicit-empty conversion. Prepare every
   repository identity, scoped local-path lookup, remote descriptor, contribution binding, base-branch
   candidate and supported metadata outside the association transaction. Reuse `resolveRepoInput`,
   typed `classifyRepositoryResolutionError`, checkout normalization, and metadata builders. Resolution
   can create workspace repository entities, so it must finish before acquiring the writer.
2. Prepare policy lookup outcomes for every explicit selector and checkout capability outcomes for
   explicit checkout changes. Retain policy errors as typed outcomes until finalization when an
   existing immutable snapshot may validly supersede that lookup. Never suppress an error if no valid
   explicit or inherited snapshot applies. Capture external capability errors for use only when the
   locked comparison actually requires that capability check; equal explicit options remain allowed.
   Prepare update capability outcomes using the pre-update task/profile context used by the current
   checkout validator; changing other task metadata in the same request must not silently change
   which executor profile validates checkout options. Precompute error labels to keep finalization
   free of repository lookups and provider/filesystem work.
3. Enter the atomic store operation below. Its locked read is the **first canonical association read**
   for replacement, including checkout inheritance. Remove `UpdateTask`'s earlier association-read
   preservation step from this path; provider selection preflight remains outside. The finalizer
   receives ordered existing rows and transactionally observed environment existence. Direct replacement
   keeps its existing policy-only inheritance behavior; only the update path applies the existing
   `matchingRepositoryCheckoutOptions` rules and checkout mutability checks.
4. Select explicit policy snapshots first, otherwise the first complete existing snapshot matching
   the original request repository ID and policy ID, otherwise the prepared selected policy. Validate
   the chosen snapshot's repository identity as today. Preserve original-input matching when a safe
   repository resolver redirects an ID. Apply policy base-branch precedence and `PreserveBaseBranch`,
   check remote-contribution agreement, apply checkout omission/matching/ambiguity, and reject an
   explicit changed checkout if an environment already exists. Missing checkout-mutability support and
   capability errors retain the existing rejection behavior when a change actually needs evaluation.
5. Produce all final rows, metadata and input positions, then enforce the existing
   `(repository_id, base_branch, checkout_branch)` duplicate key after policy application. Any error
   returns before association deletion. The store checks row ownership and serializes all metadata
   before destructive SQL; malformed metadata must not silently become `{}` in this new operation.

Extract only the reusable resolver/finalization pieces needed for this sequence. CreateTask continues
to use its current resolution and creation behavior; its multi-insert creation semantics are outside
this repair. Preserve existing supported inputs rather than inventing an alternative provider resolver.

## Atomic store boundary

Add one domain-specific complete replacement operation to `TaskRepoRepository`, implemented by the
existing dialect-aware `sqlite.Repository`. It accepts a task ID and a finalizer over an immutable
replacement snapshot (ordered associations plus environment-exists state), returning complete rows
or an error. The callback is synchronous, pure and bounded: it must not call repositories, providers,
Git, publish events, open another transaction or retain transaction-owned state. No `sqlx.Tx` escapes.
The signature is `ReplaceTaskRepositories(ctx, taskID, build func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error)) ([]*models.TaskRepository, error)`.
The domain snapshot lives in task models because repository provider wiring imports the concrete
store; defining the type in repository would introduce an import cycle. No transaction is exposed. The operation adds no generic transaction engine, schema, migration,
HTTP/WS action, or public payload. The callback is needed because inheritance must use
rows read under serialization, while repository-entity resolution can itself write through the SQLite
writer. A rows-only delete/insert transaction with earlier canonical inheritance would leave that
read-modify-write boundary unsynchronized.

The store begins `r.db.BeginTxx`, registers rollback immediately, and:

- On PostgreSQL, uses the established raw `lockTaskRowInTx`/`db.LockTaskRowInTx` task-row pattern in its
  own statement **before** any canonical association read. Use explicit READ COMMITTED for this
  operation so subsequent read statements see a preceding lock holder's commit. Map missing task to
  the established `ErrTaskNotFound` classification. Session-turn advisory locks use a different
  namespace/ownership boundary and are not acquired for association replacement.
- On SQLite, the normal single-connection writer pool serializes users of that pool. Also perform the
  established `UPDATE tasks SET updated_at = updated_at WHERE id = ?` guard before any canonical read,
  checking RowsAffected for missing task. This acquires the database writer lock before read even when
  independent writer handles target the same file. `LockTaskRowInTx` alone is a SQLite no-op and would
  not establish that independent-handle guarantee. Do not update timestamps or task fields.
- Reads associations using **tx**, never `r.ro`; reuse the full ordered scanner with
  `position ASC, created_at ASC, id ASC`. Read environment existence on **tx**, so UpdateTask's existing
  environment-created checkout invariant is not decided from a stale pre-lock read. No task-wide
  metadata snapshot guarantee is added to external capability preparation.
- Calls finalization, validates/stamps owning task, allocates replacement row IDs/timestamps with the
  current conventions, and marshals complete metadata before deletion. Preserve all branch-policy
  columns; the existing workspace-batch inserter omits them and cannot be reused unmodified.
- Deletes this task's associations and inserts **every** final row through the same transaction,
  using rebound queries. Empty rows means the delete-only clear, still with parent existence guard.
  Checks context before destructive work and before commit, commits once, returns only after commit.
  Every statement/finalization/precommit failure rolls back the delete and all attempted inserts.

The rollback baseline is the set read after locking, not a snapshot promised at request arrival.
Preparation failures perform no association write; other independently committed mutations are not
undone. No process-local mutex substitutes for database serialization. No automatic contention retry is added.
No lifecycle cleanup barrier or runner gate is introduced by using the raw task lock.

## Compatibility and references

The unchanged table has task/repository foreign keys with ON DELETE CASCADE and a four-column
uniqueness key. There is no schema FK **to** a task-repository row in the audited schemas. Worktree,
launch-error, comparison-target and workspace-inventory-recovery models/receipts can carry historical
`TaskRepositoryID` values; replacement already renews row IDs, so these are not stable-ID guarantees.
Sessions and PRs primarily bind repository/branch identities. Failure restores exact original rows;
success can detach old rows without rewriting historical receipts, environment inventory or worktrees.
Ordinary readers must resolve current rows as they do today. New folders are not cleared by a repository
replacement, and positions remain the existing repository input order rather than a new merged
repository/folder ordering scheme.

Keep policy snapshot precedence, edited/deleted policy reuse, explicit branch preservation and
checkout-option normalization intact. New metadata is built from the supported requested checkout,
PR number, remote contribution and contribution destination inputs. Existing arbitrary old metadata,
comparison targets/manual override flags omitted by replacement are not automatically carried forward.

### Live writer inventory

| Writer | Existing boundary and relation to this operation |
| --- | --- |
| `persistTaskRepositoryRows` for task creation | Independent `CreateTaskRepository` inserts; creation behavior unchanged. Complete replacement stops using this loop. |
| `CreateTaskRepository`, `DeleteTaskRepository`, `DeleteTaskRepositoriesByTask` | Standalone legacy CRUD, no explicit task-lock contract. No guarantee against all their interleavings is added. PostgreSQL FK/row effects may block particular operations but are not a general writer protocol. |
| `UpdateTaskRepository` | Transaction; identifies old/target tasks, locks sorted distinct task IDs, updates row. Used by branch recovery and backend e2e reset; existing reparent semantics unchanged. |
| `UpdateTaskRepositoryComparisonTarget`, `UpdateTaskRepositoryBaseBranchAndClearComparisonTarget` | Transaction; identifies owner then locks task, rereads exact row and verifies owner, updates metadata/base. Provider/manual branch paths keep this behavior. |
| `CreateWorkspaceSourceBatch` | Task-guarded transaction, appends sources/updates branches after guard; participates in raw task locking. |
| `CompensateWorkspaceSourceBatch` | Separate transaction deletes its newly attached rows/restores its branch updates; compensation protocol unchanged, no new general serialization claim. |
| Task/repository/workspace deletion and migrations | Existing lifecycle cascades/rebuilds remain authoritative; replacement cannot create rows for a parent deleted before its guard. No new cascade orchestration is added. |

The service add-branch and attach-source paths use workspace-source batches and compensation;
`service_branch_recovery.go` uses the in-place update; provider PR target/manual base updates use the
comparison-target operations. Schema migration `recreateTaskRepositoriesForMultiBranch` rebuilds the
relation at initialization, outside normal live replacement. Audited raw SQL writers and interface
callers require no new provider implementation or registry entry.

## Transport and consumers

`RegisterTaskRoutes` registers `PATCH /api/v1/tasks/:id` and dispatcher `task.update`. Both use
`convertUpdateRepositories` to keep absent/null distinct from `[]`, then call `UpdateTask`.
Existing typed validation/reference/provider errors and opaque storage failures keep their current
HTTP/WS mapping. Tests enter through the router and dispatcher, not private handler functions.

`UpdateTask` keeps definitive input-shape, checkout-scope and provider-selection failures before
ordinary task-field writes. Capability outcomes are deferred because equality with the canonical
checkout can make them irrelevant; inheritance, ambiguity and environment checks use the locked set.
`UpdateTask` writes unrelated fields before the association operation. Failed replacement returns
before its `TaskUpdated`/state events; earlier field edits, transition records, or repository entity
creation are not rolled back. Successful association replacement is followed by the ordinary canonical
list and event projection. Those reads can see a later committed replacement; read failure behavior
remains best-effort. The payload must never be fabricated from partially inserted candidate rows.
`taskRepositoriesForEvent` and DTO serialization retain explicit-empty handling and existing fallback
reads. No new repository success event is introduced for direct replacement.

Fresh-branch HTTP creation may already publish creation evidence and perform Git changes before
`commitFreshBranch`. Its replacement failure remains a 5xx with the existing check-repository message;
the database keeps the pre-replacement associations, and Git operations are not compensated by this fix.

Read-only consumers include service `GetTask`/lists and batched DTO hydration, orchestrator
`resolveTaskRepoInfoForSession` and primary-selection launch paths, launch PR gates and branch recovery,
backend materializers/credential adapters, GitHub PR watch and issue linking, git status/automation and
MCP repository selectors. They continue to consume canonical list/primary reads. No cross-query stable
snapshot, launch inventory synchronization, or cache revision is claimed.

Frontend `kanban-api.updateTask` sends repository inputs over PATCH. `lib/ws/handlers/kanban.ts`,
`task-merge.ts`, `lib/kanban/map-task.ts` and task/worktree domain consumers parse whole ordered arrays.
Repository chips, Changes/Review and primary repository actions retain their existing projection.
There is no new layout, navigation, touch, scrolling, viewport logic, copy or localization. The
mobile-parity assessment is pure backend data/state correction: faithful REST/WS real-database tests
cover the shared desktop/phone contract; no browser/app/build/E2E is required for this scope.

## Failure and recovery

Known failed preparation, guard/read/finalizer/serialization, delete, any insert, or precommit
cancellation leaves the old complete association set. Commit acknowledgement uncertainty is distinct:
read the canonical set before deciding what happened; database transactions guarantee no partial set,
not that every network/driver error proves rollback. Cancellation after successful commit does not undo it.
Restart exposes the durable committed set with no repair migration. Errors keep typed identity and
existing safe transport text. Existing logs suffice; no new metrics, revision or event-order protocol.

Successful UpdateTask retains the returned committed rows as a fallback before its ordinary
postcommit canonical read. A later successful read may reflect a complete successor set; no global
event revision is promised.

Public-guide audit: `docs/public/tasks-and-workflows.md` already explains multi-repository inputs,
retry and fresh-local-branch destructive effects. During implementation add a short explanation that
failed complete attachment replacement keeps the prior attachment set while earlier task edits and
Git operations may remain. Retain its existing how-to structure, provider support matrix and screenshot
catalog; no new page or media is needed. Root README and screenshots do not describe the failure contract.

## Validation boundary

Permanent service regressions use real private SQLite, two original rows with sentinel metadata/policy
and timestamps, late resolver failure and a valid second-insert trigger failure through direct and
UpdateTask paths. Store regressions cover finalizer failure, serialization failure, duplicate/FK/missing
parent errors, late insert failure, clear/order, cancellation, reopened persisted state and physical
connection serialization. Registered transport regressions observe real results and captured events.
PostgreSQL tests must use independent holder, worker and observer backend PIDs, observe the worker's
server lock wait and verify finalization has not read before release. A predecessor commits changed
snapshot values while the successor waits; the successor must inherit those committed values. SQLite
uses independent file-backed writers, a real writer-before-read blocker and a cancellation/driver wait
observation rather than sleep-only claims. All goroutines/connections are joined/closed on every path.

The implementation plan owns exact test names, acceptance traceability and command receipts.

## Related decisions

[ADR 0013](../../../decisions/0013-multi-branch-tasks.md) owns the multi-branch model. The existing local
transaction and raw-task-lock pattern is reused; no new architectural boundary warrants an ADR.
