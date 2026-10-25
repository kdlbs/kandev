---
status: current
system: tasks
created: 2026-09-28
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-003
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004
---

# Worktree inventory repair system design

## Ownership and integration

Task environment inventory is authoritative. The worktree store projects
`task_environment_repos` joined to `task_environments` and `repositories`;
`worktreeSelectCols` supplies the environment's `task_dir_name` for every slot.
The worktree manager independently checks the root marker and Git registration.

An incomplete or inconsistent row is not proof of corrupt source files.
`cleanupBranchIdentity` compares the saved branch's commit with the checkout
commit; `CaptureArchiveSourceManifests` requires complete repository identity;
`validateExistingWorktreePathOwner` rejects a checkout under another marked root.
These guards remain unchanged. Automatic recovery of absent Git administration
continues to follow [metadata recovery](worktree-metadata-recovery.md).

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001` | Maintenance entry point; validation; journaled application |
| `REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002` | Repository repair; cleanup evidence |
| `REQ-TASKS-WORKTREE-INVENTORY-REPAIR-003` | Stop diagnostics |
| `REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004` | Duplicate ownership reconciliation |

## Maintenance entry point

Add a bounded standalone Go maintenance command at
`apps/backend/cmd/worktree-inventory-repair`, with its implementation under
`internal/task/inventoryrepair`. This is an operator utility, not a startup
migration or an automatic retry fallback. It supports the local SQLite
installation and host Worktree executor only. Application and rollback require
Linux and visibility into host processes across all UIDs; other platforms
retain read-only preview/verification. Other stores and executors refuse
application before host path inspection.

The default mode reads the database without constructing the application's
migrating repositories and reports observations. `--apply` consumes an explicit
JSON repair description containing operation ID, database and home identity,
environment ownership generation, exact old row values, expected Git identities,
and allowed replacements. Paths and task IDs do not grant authority by themselves.
No installation-specific IDs are hardcoded in production logic.

Repair connections open the database as a SQLite `file:` URI followed by the
mode and connection options. The canonical path follows `file:` with only `%`,
`?`, and `#` percent-escaped, because SQLite decodes `%XX`, starts the query at
`?`, and ignores the rest after `#`. SQLite reads all remaining characters
literally, including a Windows drive letter and backslashes. The shared path
escape helper lives in `internal/db`. Each caller keeps its own mode and options.
Neither `mode=ro` nor `mode=rw` creates a file.

Before application, acquire the same home and database locks produced by
`backendapp/ownershiplock.Targets` and held through `ownershiplock.Acquire`.
A live backend is a refusal, not something the helper stops. Verify that no
affected agentctl, shell, or agent process can access the source or destination;
a stopped database row alone does not establish this. Unknown liveness refuses.
Inspect processes from every UID; inaccessible processes refuse repair. Confirmed
kernel threads and zombies have no live checkout consumers and can be excluded.
Startup checks repair fences against the same canonical paths used for ownership
locks, including external databases configured through file symlinks.
Use existing classified Git subprocess helpers and finite inspection deadlines.

## Validation and repository repair

Preflight the complete requested change set before any move or database write:

1. Read exact task, session, environment, repository, and cleanup rows. Verify
   workspace membership, ownership generation, slot/worktree uniqueness, and
   the expected original tuple. Reject a running cleanup or competing claim.
2. Inspect managed roots without following symlinks. Read root markers and
   verify their task, workspace, and directory identities.
3. Validate the checkout's `.git` pointer, common directory, administrative
   backlink, and exact `git worktree list` entry. Observe symbolic HEAD and OID.
   Local repair Git commands discard inherited `GIT_*` environment overrides
   so the explicit paths determine repository selection and index/object state.
   Preserve detached or ambiguous cases for separate recovery rather than
   guessing a branch.
4. Resolve a missing repository ID only from the explicitly selected authorized
   repository, with exact common-directory and registration agreement. A matching
   name, remote URL, session primary repository, or sibling slot alone is insufficient.
   Reject a collision in `(environment, repository, branch_slug)`.
5. A branch correction changes only the saved slot branch to the explicitly
   observed attached branch. Preserve both old and current refs. Do not replace
   the expected OID with the old recorded branch's present tip.
6. A cross-root relocation additionally requires an explicit source-root owner
   and destination-root identity in the repair description. Verify the child's
   saved relationship and shared environment, but never use that relationship
   to bypass the root check in ordinary launch or cleanup.
7. Every relocation requires a matching workspace repair for its environment,
   naming its canonical task root and every bound session. Reject an omitted
   workspace repair even when the Git move and inventory slot are valid.

Version 1 does not merge slots or infer new branch slugs. An absent checkout
cannot establish an incomplete repository identity.

## Duplicate ownership reconciliation

This section describes the extension delivered by the
[legacy duplicate repair package](../../../plans/legacy-worktree-alias-repair/plan.md).
It preserves the existing explicit maintenance boundary. It is not a startup
migration, a read-time normalization, or permission to ignore a failed Git check.

### Discovery and versioned selection

Extend `cmd/worktree-inventory-repair` with a mutually exclusive `--discover`
mode. It accepts explicit `--home`, `--database`, and `--tasks-root` paths, plus
`--after` and `--limit`. Use `internal/task/inventoryrepair` and its existing
read-only SQLite opening and path-escaping contract. Do not construct migrating
repositories. Reject unsupported schemas or drivers before path inspection.

Discovery reads inventory only. Select nonempty `worktree_id` values claimed by
more than one active, non-deleted environment-repository row, across environments
and branch slots. Page by worktree ID with a default limit of 50 and maximum of
100 identities. Cap each identity at 256 claims and each response at 1,000 rows
and 1 MiB. A group that exceeds the cap is explicitly oversized, never a partial
repair candidate. Return stable cursors, claim row IDs, environment owners,
repository/branch/path fields, and a selection template. Do not traverse the
tasks directory or run Git against unselected paths. This permits safe discovery
while the backend runs, but the result is provisional until offline preview.

Retain version-1 plan decoding and journal compatibility. Introduce version 2
for duplicate repair only. It cannot combine relocation, repository reassignment,
or cleanup-job supersession with reconciliation. Its typed selection names:

- One canonical environment-repository row ID per physical worktree ID, plus
  every alias row ID to remove.
- Each redundant environment to retire, its canonical environment, its workspace
  group and ownership generations, and every session to rebind.
- Explicit canonical workspace-path corrections and every affected bound session.
- Existing row and Git/content observations, including the complete competing
  set and negative queries for new claims or dependent records.

Bound one plan to 16 physical identities, 16 environments, 16 groups, 256 alias
rows, and 256 sessions, within the existing 1 MiB input limit. Reject truncated
or omitted connected ownership sets. Discovery provides candidates, not authority
to choose the winner or apply a repair.

### Canonical proof and dependent records

Preview reloads all claims for each selected identity. The selected canonical
slot must identify a registered checkout strictly below its marked task root.
Reuse `inspectGit`, root-handle checks, content/index/ref digests, and repository
workspace authorization. Prove the branch, common directory, administrative
backlink, registration, and root owner together. A checkout cannot be the task
root itself. A task-root alias can be removed only when it names the same
repository, worktree, and branch as the proven child checkout, has no local
`.git`, and is precisely that checkout's verified enclosing task root. Do not
repair a broken `.git` pointer, an unrelated path, or two real competing
checkouts through this rule. Do not infer a branch slug or rename a branch.

For copied child environments, use `task_workspace_groups` and active
`task_workspace_group_members` to prove the current steward and canonical
`materialized_environment_id`. The child's saved workspace policy and membership
must agree. All active rows of the redundant environment must exactly match
selected canonical counterparts by physical identity, repository, branch, and
checkout path. A single unmatched or failed independent slot blocks retirement.
The task tree alone is insufficient. No group owner, membership, or ownership
generation changes during this operation.

The version-2 mutation set is closed:

| Record | Permitted change |
| --- | --- |
| Selected `task_environment_repos` aliases | Remove exact row IDs after canonical proof |
| Selected `task_sessions` | Rebind to the canonical environment and correct the explicit workspace path, preserving all other fields |
| Canonical `task_environments` | Correct only the explicit workspace path to its marked group root |
| Selected `task_session_git_snapshots` | Rebind the environment FK for observations of those same physical checkouts, preserving ID, session provenance, timestamps, metadata, and content |
| Redundant `task_environments` | Remove only after every permitted reference is rebound and every other dependent set is proven empty |

Do not merge conflicting current Git observations or fabricate capture times.
Reject snapshot scopes whose repository/path cannot be related to the selected
canonical inventory. Check both declared foreign keys and the explicit non-FK
reference set. Recovery claims, operations, artifacts, recovery receipts,
adoption receipts, Kubernetes records, terminal consumers, another group's
materialized-environment reference, and unknown referencing tables block this
version. Keep original cleanup snapshots and source manifests byte-for-byte.
Cancel neither tasks nor cleanup jobs to make the proof pass.

Every task whose environment or sessions participate must have no cleanup job
that could still run against the selected inventory. Only cancelled or succeeded
jobs are inert for this extension. A failed, pending, prepared, running,
retry-wait, or waiting-for-clean job blocks the selection. The existing explicit
cleanup recovery remains a separate operation. Its immutable observations must
not be rewritten by duplicate repair.

### Offline application and recovery

Retain Linux-only application, ownership locks, process census across all UIDs,
backup validation, private journal, startup fences, and expected-value guards.
All selected tasks, canonical borrowers, and redundant owners participate in
admission. Preview may report blockers while the backend runs. Application
requires known absence of every affected runtime, agentctl, shell, terminal,
and recovery consumer. `STARTING`, `RUNNING`, cancellation-in-progress, or unknown
session states block application. `CREATED`, `WAITING_FOR_INPUT`, `COMPLETED`,
`CANCELLED`, and `FAILED` are eligible only after the independent process and
claim checks succeed. Preserve their states and provider resume identities.

Capture the complete dependency graph in a single read transaction for preview.
Under the existing offline ownership lock, revalidate it in the write transaction
before publication. Journal exact original row values, including raw timestamp
text. Publish all alias removals, session/snapshot rebindings, workspace-path
corrections, and redundant-environment retirement atomically. Verify foreign
keys and the complete selected graph before commit. Before fencing, record existing
foreign-key violations in the affected inventory tables. Refuse a selected row
with an existing violation because rollback cannot safely restore that row.
Forward publication must not introduce a new violation. Unrelated legacy
violations must not prevent apply or rollback, including recovery from journals
that predate this baseline. Keep SQLite foreign-key enforcement enabled.
Preserve unrelated records.
Perform no Git or filesystem mutation in version 2. Content, index, refs, and
markers must match before and after application and rollback.

Extend journal verification for negative observations and rows intentionally
removed. Rollback restores environments before dependent rows, then restores
bindings and aliases in dependency order. Do not replay forward mutation order
blindly in reverse mode. Failpoints before commit, after commit, and before fence
release must prove restart fencing and idempotent continuation. A changed row,
new claimant, changed checkout, or conflicting journal leaves the repair fenced
and refuses both unsafe completion and unsafe rollback.

### Runtime ambiguity prevention

The current `worktree.SQLiteStore.GetWorktreeByID` uses `LIMIT 1` over a join.
`GetWorktreesByTaskID` orders newest first and `scanWorktrees` silently keeps the
first matching worktree ID. Replace those authority decisions with explicit
claim checks. Session joins may multiply one slot but cannot create a claimant.
Multiple active slots for one physical ID produce a typed ambiguity error before
attachment, identity capture, or mutation. One active slot plus deleted history
continues to resolve the active slot. Historical cleanup retains its exact
snapshot validation rather than borrowing an arbitrary active owner.

Audit ID-based mutations in `worktree/store.go`, including `CreateWorktree`,
`UpdateWorktree`, `DeleteWorktree`, and branch/recovery updates, plus physical
inventory writes in `task/repository/sqlite/task_environment.go`. An operation
must address one proven slot and cannot fan out over duplicate IDs. Serialize
physical identity claims by ID in the transaction. SQLite uses its writer lock,
and PostgreSQL uses a transaction-scoped advisory lock in the established
dialect pattern. Inventory-only rows with no physical ID remain valid. Preserve
attach-only session reuse and distinct branches of one repository. Do not add
a global unique index that would prevent an older installation from starting
before an explicit repair.

These are implementations of existing task-owned lifetime and fail-closed repair
decisions, not a new automatic ownership election. The original writer of the
August aliases is unknown. Do not attribute it to the one-time ownership cutover
or rerun that cutover on an already normalized schema.

### Operator outcome and verification

Keep source-command distribution explicit in `docs/public/git-operations.md`.
Operators build this utility from a release containing the extension, discover
their own identities, select the canonical rows, stop the backend normally,
refresh the preview, then apply and verify. Do not publish the investigated
installation's IDs or filesystem paths as a reusable repair recipe.

Use generic SQLite/Git fixtures for both observed shapes and an unrelated
multi-branch control. Test canonical parent ownership with a copied child
environment, sessions in both environments, and environment Git snapshots.
Demonstrate normal archive and confirmed delete after reopening the database,
including child-only operations that preserve an active shared workspace.
Dirty tracked, untracked, ignored, and staged files survive metadata repair.
Ordinary deletion then enforces its existing dirty-file policy. Keep completed
source evidence immutable and retain normal branch/commit safety refusals.
No rendered UI change is planned. This backend lifecycle evidence covers both
desktop and phone task actions through their existing APIs.

## Journaled application

Take and validate a SQLite backup in a private `0700` directory with a `0600`
database file before changing state; include WAL content through the existing SQLite `VACUUM INTO` snapshot
facility and validate integrity. Retain the selected original rows and Git identity/content
verification in a private journal. File content remains in the preserved
checkout and backup artifacts, not diagnostic logs.

For relocation, use `git worktree move` into a verified, absent destination under
the canonical environment root. Never use recursive deletion or overwrite a
destination. Preserve the child root's marker. Verify the moved worktree's HEAD,
index, content, and backlink before publication. Reject unsupported move layouts,
including submodule cases the Git operation cannot safely handle.

Publish branch, repository, and path changes with exact old-value comparisons
inside a SQLite write transaction. Update only the selected canonical slot and
explicit dependent session/environment workspace paths. For multi-repository
host environments, the workspace is the canonical task root. Preserve a
stopped executor's unrelated fields and provider resume identity.

Filesystem moves and SQL commits are not one atomic transaction. The journal
records prepared, moved, and published phases. Under the same ownership locks,
backend startup must refuse an unresolved repair journal before migrations or
runtime recovery, naming the operation needed to finish or reverse it. Repair
re-entry validates both locations and database tuples before rolling forward
or back. Failure to roll back leaves the journal unresolved and startup blocked;
the helper never reports success for partial application. This startup gate is
limited to journals created by this explicit utility. Its typed refusal reports
repair recovery guidance independently from a running-instance ownership conflict.

## Cleanup evidence

Archive-reclaim jobs contain an exact path/repository/archived-generation intent
and reload the current slot. A branch-only inventory correction leaves that
intent valid; the next normal worker attempt must perform its full audit.

A legacy cascade job can embed incomplete worktrees and a head-identity map that
omits the incomplete row. Correcting only the canonical slot will not fix that
snapshot, and merely adding a repository ID still leaves
`CleanupHeadOIDUnavailable` asserted on replay.

For such a job, keep the old `resource_snapshot` byte-for-byte. In the guarded
repair transaction, cancel only the exact non-running predecessor and create a
new pending job with a unique operation ID linked to that predecessor in its
snapshot. Reuse the prior operation's resource scope and completed progress;
do not add newly discovered resources, change discard consent, or rerun
already-completed unrelated cleanup. Verify current task archive identity.
Use the SQLite driver's timestamp text format for new cleanup timestamps so
ordinary queue ordering remains valid. Preserve original cleanup timestamp text
in the journal and restore it byte-for-byte on rollback.

Build complete identities and current cleanup HEAD evidence for the successor
after all consumers are absent. Existing source manifests stay with their
original job. The successor has its own capture and observation time; normal
`executeTaskResourceCleanupJob` still stops targets and persists its own source
manifest before removal. An already-missing historical checkout remains an
explicit absent observation, never a reconstructed archive-time source state.
The predecessor link and exact job identities make repeated repair application
idempotent. No broad retry-counter reset or succeeded-job reopening is allowed.

## Stop diagnostics

`Executor.StopExecution` currently logs WARN before recognizing
`lifecycle.ErrExecutionNotFound`. Move exact typed absence classification before
that warning, preserving its existing public `runtime.ErrNotFound` and executor
sentinels; also recognize the public runtime sentinel directly where returned
by the runtime facade. Optional debug logging can identify an already-absent
execution. Other failures keep WARN and their original error chain.

`runtimeStopAlreadyComplete` already handles typed absence in task cleanup.
Do not broaden it to error-string matching, session lookup failures, or unknown
remote liveness. This change does not itself repair inventory failures.

## Validation and operator outcome

Use real temporary Git repositories and SQLite databases for preview, guarded
application, partial-move recovery, and cleanup replay tests. Prove that the old
refs and original evidence survive, and that ownership and immutable-commit
refusals still work. Repair must preserve dirty files before cleanup; subsequent
archive cleanup follows [the current archive policy](dirty-worktree-archive.md)
and removes eligible dirty checkouts. Integrate the repaired inventory through
the real cleanup worker and existing attach-only worktree admission boundary.
Run the affected executor tests separately; do not launch a real agent in a repair
fixture.

No rendered UI changes are needed. Existing resume and workspace restoration
should pass admission after the explicit repair; unrelated ownership errors
remain visible. Installation repair and any backend restart are separate
operational steps after the utility and targeted checks are ready.

## Related decisions

- [Archived-worktree reclamation](../../../decisions/2026-09-24-archived-worktree-reclamation.md)
- [Source manifest](archive-source-manifest.md)
- [Implementation plan](../../../plans/worktree-inventory-repair/plan.md)
