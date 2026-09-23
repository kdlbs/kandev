---
status: draft
system: tasks
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-004
created: 2026-09-14
owners:
  - kandev
---

# Workspace repository placement

## Ownership and requirement mapping

Tasks own the durable environment and attachment lifecycle. Worktree code supplies owned filesystem resources. Agent runtime owns process adoption and native-session recovery.
This design extends [Attach Workspace Sources](attach-workspace-sources.md), including its authorization and batch compensation rules.

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001 | Ownership; Preview contract; Failure behavior |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-002 | Data and migration; Launch and reuse; Presentation |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003 | Placement; Nested worktrees; Expansion and recovery; Tracking and cleanup |
| REQ-TASKS-ATTACH-WORKSPACE-SOURCES-004 | Preview contract; Presentation; Failure behavior |

The [placement ADR](../../../decisions/2026-09-14-explicit-workspace-repository-placement.md) records the changed nesting boundary.

## Data and migration

Use typed fields, not arbitrary task metadata or client-supplied absolute paths.

- `Task.InitialWorkspaceLayout`: `repository` or `task_root`. Add the optional `initial_workspace_layout` creation field. Omission means `repository` for one repository. Multiple initial repositories resolve to `task_root`.
- `TaskEnvironment.WorkspaceLayout`: `repository` or `task_root`, empty only for legacy rows awaiting validated classification. `WorkspacePath` remains the effective root authority.
- `TaskRepository.WorkspaceRelativePath`: server-owned path relative to the Kandev task folder. Empty means a legacy slot without recorded placement intent.
- `TaskEnvironmentRepo.WorktreePath` and `WorktreeID` retain physical identity. Do not introduce another physical inventory.

Add columns through the existing SQLite/PostgreSQL dialect-aware schema and upgrade paths. Include task/environment scans, transactions, DTOs, and store conformance fixtures.
Initial layout is fixed after task creation. Explicit expansion updates the environment layout, not the historical creation choice.
Reserved placement fields cannot be set through arbitrary metadata or generic task updates.

Migration must not derive layout solely from row count or move any checkout.
Validate the recorded root against task ownership and the complete active inventory. Classify its existing location as repository or task root.
A legacy promoted root with an older live agent CWD remains divergent until explicit adoption. Do not relabel the live execution as adopted.
Missing or ambiguous inventory keeps the existing typed recovery failure. Do not synthesize another worktree to complete migration.
Executor transitions retain existing admission rules. A layout unsupported by the target executor fails visibly instead of ignoring the choice.
A same-workspace subtask reuses the parent's environment. A new-workspace subtask uses its explicit creation value or the normal default.

## Launch and reuse

`WorktreePreparer.Prepare` currently branches by source count. Both its initial and `WorkspaceReuseRequired` returns use `wt.Path` for a single repository.
Resolve agent workspace independently from repository preparation. Parent-root startup uses the owned task folder even with one worktree.
Retain repository-specific setup execution at the repository path. Executor setup receives the effective workspace root. Do not run either script twice.

Propagate layout through `CreateTaskRequest`, launch requests, `EnvPrepareRequest`, and the environment projection.
`GetWorkspaceInfoForSession`, environment reuse, workspace-only promotion, and restart adoption must use the validated environment root and inventory.
The existing [reuse design](additional-session-workspace-reuse.md) remains the authority for attaching sessions without rematerialization.
Per-slot paths remain stable through branch renames, additional sessions, resume, and recovery. Cardinality cannot promote a nested workspace on resume.

## Placement

Extend `AttachWorkspaceSourcesRequest` and the existing HTTP source endpoint with optional `repository_placement`:

| Value | New destination relative to established root | Runtime operation |
| --- | --- | --- |
| `kandev_directory` | `kandev/<entry>` | Rescan only |
| `current_root` | `<entry>` | Rescan only |
| `expand_root` | `<owned-task-root>/<entry>` | Adopt task root when different |

These values are accepted only for Worktree repository-only batches.
Omitted placement keeps the existing contract for old API clients and mixed/folder batches.
The new UI makes its choice explicit. It does not pass a path string as authority.
An already parent-rooted task uses `current_root` without a redundant selector.
Keep current batch idle admission for all three placements. This package does not make batch attachment callable during an active self-tool turn.

Use the existing repository/branch entry naming and collision rules. The examples show repository names, but real branch slots can require suffixes.
Resolve each destination from canonical environment state under the source-mutation lock.
Record the resulting task-relative path in the new attachment row and physical path in the inventory.
Never move an existing worktree. Expansion after prior nested additions retains those nested paths and shows them in the preview.
For legacy sibling attachments outside the live CWD, disable nested choices until the existing root mismatch is resolved.

## Preview contract

Add an owner-authorized `POST /api/v1/tasks/:id/workspace-sources/preview` beside the existing mutation endpoint.
Reuse locator validation and placement computation. Preview performs no registration, clone, worktree creation, or process mutation.
Return resolved relative destinations, source rows, effective root, supported placements, disabled reasons, and session/process consequences.
Unresolvable remote identities remain unready until existing discovery provides a stable repository identity. Never show a guessed final destination as exact.

The response includes an opaque revision derived from environment generation/root, source inventory, and relevant live-session roots.
Submission sends that revision with placement. Under the mutation lock, recompute and compare it before side effects.
A revision mismatch returns a conflict and refreshed preview. This token is a concurrency check, not authorization.
At submission, also revalidate filesystem collisions and idle state. A successful earlier preview cannot authorize stale paths or restart a newly busy session.
Use the existing exact-retry path before rejecting a stale preview for an already committed identical batch. A changed placement is not an exact retry.

## Nested worktrees

Extend `worktree.CreateRequest` with an internal validated task-relative destination.
`prepareTaskWorktreePath` can use this destination only after task ownership and canonical containment checks.
Preserve safe path handles and target locks. Never bypass them with a raw `filepath.Join` or permit arbitrary caller paths.
Create real Git worktrees inside the chosen CWD. Symlinks to outside siblings do not establish the required sandbox containment.

Reject occupied final paths, tracked destinations, unsafe names, symlink/junction parents, case-folded collisions, and destinations containing unrelated content.
A pre-existing plain `kandev` directory can contain unrelated files, but final entries must be absent or an exact owned retry.
Never exclude the whole `kandev/` directory. Exclude only owned child entries.

Protect outer Git status and ordinary `git add .` before publishing nested worktrees.
Use a conditional Git config include scoped to the outer worktree's exact Git administrative directory.
Its owned config supplies a worktree-specific `core.excludesFile`. Preserve the effective pre-existing exclusions in the generated file.
Use escaped anchored directory patterns. Do not modify tracked `.gitignore`, blanket shared `info/exclude`, or global Git configuration.
The include resides in shared repository config but matches only this worktree. Do not enable repository-wide `extensions.worktreeConfig` as a shortcut.
Record ownership and previous configuration for compensation. Serialize config edits across worktrees sharing the common Git directory.
On cleanup, remove only unchanged owned entries. Preserve user edits and unrelated config. Refresh preserved exclusion input during Kandev reconciliation.

Git ignore precedence can override lower-priority excludes. Verify each destination with Git before publication and reject ineffective protection.
Do not change the user's tracked ignore rules to force success. Explicit `git add -f` is outside ordinary-staging protection.
Kandev's own staging operations also reject treating an attached repository as an outer gitlink.
A Linux disposable Git experiment confirmed worktree-scoped exclusion without hiding a same-named ordinary folder in a sibling worktree.
Windows/macOS paths, existing excludes, negated patterns, config races, and cleanup still need the assigned regression tests.
See [Git configuration](https://git-scm.com/docs/git-config) and [Git ignore precedence](https://git-scm.com/docs/gitignore).

## Tracking and cleanup

`workspaceSourceMaterializer` must select placement before creation, instead of always deriving the parent root.
Do not call legacy `branchMaterializer.finalize` for explicit nested batches: it promotes the workspace path.
Publish the existing source-adopted projection only after the full batch succeeds.
Keep the primary `worktree_path` distinct from the effective `workspace_path`.

Register nested repository roots from canonical inventory. Agentctl discovery must not depend on traversing Git-ignored directories.
Retain the outer repository tracker and separate inner trackers keyed by repository and branch slot.
Changes, file links, editor selection, and PR targets use actual paths and slot identity.
A nested repository's ignored status in its parent must not hide its own tracked changes.

Archive/delete cleanup uses existing worktree ownership, reference, and dirty/untracked preservation checks.
Process nested children before their parent when cleanup is permitted. A protected child blocks destructive parent cleanup.
Never recursively delete a parent to work around a child's refusal. Remove an owned grouping directory only when empty.

## Expansion and recovery

Refresh the status and exact head of [PR #3598](https://github.com/kdlbs/kandev/pull/3598) before implementing expansion.
At the 2026-09-15 implementation checkpoint it remains open, non-draft, at head `62851c51fa2d347bbc0d162634c25c541c441d21`.
The expansion work order depends on its workspace-aware restore coordinator and explicit context-continuation contract.
Do not copy or recreate that subsystem in this package. Other work orders can proceed independently of that external dependency.

Preflight every affected live session, not just the selected tab. No active session permits batch root adoption.
All compatible sessions adopt the prepared root through runtime. Update persistent root/layout and projections only as part of the compensated batch.
Never stop the agent that is currently invoking a tool.
When the requested root already matches every live session, rescan without restarting workspace processes.

For incompatible native resume, expose typed recovery before starting a replacement conversation.
The Add dialog offers cancellation and an explicit saved-history continuation action through the shared recovery UI.
Bind continuation to the session, target root, source batch, and preview revision. Revalidate on confirmation.
Do not silently commit a different placement after rejection. Failure or cancellation preserves prior attachments and runtime identity.
Continuation preserves canonical task history, not private harness state or an exact checkpoint.
Legacy `add_branch_to_task_kandev` remains outside this UI flow. Its active-turn rescan path retains its accepted contract.

The initial-layout, nested materialization, preview, and available placement UI work is implemented.
Expansion remains explicitly unavailable until the dependency lands; no automatic native-session replacement is permitted.

## Presentation

Reuse `AddWorkspaceSourcesDialog`, its full-height phone drawer, source-row hooks, and `WorkspaceChangeConsequences`.
Extract placement cards and preview helpers instead of growing the existing component.
Show source controls first, placement cards second, selected result third, and the action footer last.
Every card shows benefits and costs. Consequences update by operation and affected sessions, not executor type alone.
Do not retain the current unconditional host-restart warning for nested placement.

The creation control goes after existing dependencies/priority in `TaskCreateAdvancedSettings`.
Use hover/focus help on desktop and a tap-accessible help drawer on coarse pointers.
Keep the switch off by default. Multiple initial repositories show a fixed parent-layout explanation.
Use a blank placement selection on first opening a single-repository placement dialog. The selected card in the preview is illustrative.
Persist rows and the selected choice during validation errors. Do not remember a risky choice across unrelated tasks.

Phone uses the shipped Add sources full-height drawer because repository rows and consequence cards require sustained interaction.
Keep one scroll body, a fixed header/footer, dynamic viewport height, safe-area padding, and touch targets of at least 44px.
Desktop uses the existing bounded dialog with internal scrolling and ordinary 28px controls.
Share domain state and submission logic across viewports. All new copy uses localization in English, Portuguese, and the three Chinese catalogs plus pseudo.

## Failure behavior and evidence

Preserve task ownership checks, caller provenance, current source locks, and complete-batch compensation.
A mixed inventory test must contain live and failed/deleted rows. Validate all required live slots without treating a failed row as authority.
Tests must cover initial layout, persisted reuse, nested real-Git behavior, compensation, root mismatch, all-session idle checks, and unsupported executors.
UI tests must compare the rendered desktop and phone surfaces with the [saved previews](../../../plans/workspace-repository-placement/plan.md#ascii-ui-preview).
Log placement, environment identity, and operation outcome through existing structured logging. Do not log credentials, transcript content, or raw Git configuration.

## Implementation plan

[Workspace repository placement](../../../plans/workspace-repository-placement/plan.md) owns work orders, exact checks, and results.

## Follow-up scope

[Current workspace sources](current-workspace-sources.md) extends placement to Local folder/scratch tasks and clarifies remote capabilities. Its [separate plan](../../../plans/current-workspace-sources/plan.md) owns the + menu move and repositoryless attachment. This package retains its expansion recovery dependency.
