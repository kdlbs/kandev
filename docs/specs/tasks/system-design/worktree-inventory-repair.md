---
status: current
system: tasks
created: 2026-09-28
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-003
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

## Maintenance entry point

Add a bounded standalone Go maintenance command at
`apps/backend/cmd/worktree-inventory-repair`, with its implementation under
`internal/task/inventoryrepair`. This is an operator utility, not a startup
migration or an automatic retry fallback. It supports the local SQLite
installation and host Worktree executor only. Application and rollback require
Linux and visibility into the installation owner's host processes; other platforms
retain read-only preview/verification. Other stores and executors refuse
application before host path inspection.

The default mode reads the database without constructing the application's
migrating repositories and reports observations. `--apply` consumes an explicit
JSON repair description containing operation ID, database and home identity,
environment ownership generation, exact old row values, expected Git identities,
and allowed replacements. Paths and task IDs do not grant authority by themselves.
No installation-specific IDs are hardcoded in production logic.

Before application, acquire the same home and database locks produced by
`backendapp/ownershiplock.Targets` and held through `ownershiplock.Acquire`.
A live backend is a refusal, not something the helper stops. Verify that no
affected agentctl, shell, or agent process can access the source or destination;
a stopped database row alone does not establish this. Unknown liveness refuses.
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

The helper does not merge slots or infer new branch slugs. An absent checkout
cannot establish an incomplete repository identity.

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
limited to journals created by this explicit utility.

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
refs and original evidence survive, and that ownership, dirty-worktree, and
immutable-commit refusals still work. Integrate the repaired inventory through
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
