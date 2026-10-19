---
status: current
system: tasks
requirements:
  - REQ-TASKS-DELETE-RECOVERY-001
created: 2026-10-16
updated: 2026-10-16
owners:
  - kandev
---

# Task deletion recovery

## Boundary and evidence

The task system owns deletion admission and durable resource retention. This
design extends [normal deletion](dirty-worktree-deletion.md) and
[cleanup preparation](runtime-cleanup-preparation.md). The decision is recorded
in [retain resources during recovery deletion](../../../decisions/2026-10-16-retain-resources-on-task-delete.md).

In the reported incident, an empty `.kandev-workspace.json` made
`InspectDirtyWorktrees` fail before Git inspection. The HTTP preflight returned
503. `ValidateTaskDeleteWorktrees` also rejected MCP deletion. Skipping only the
dialog check would still fail: `persistTaskResourceCleanupWithManagedClaim`
captures HEAD identities before it stores a cleanup snapshot.

Current production boundaries are `task/service/task_delete_preflight.go`,
`task_delete_confirmation.go`, `handoff_cascade.go`, `service_tasks.go`, and
`resource_cleanup_jobs.go`. The current `TaskResourceCleanupJob` stores its
snapshot in `ResourceSnapshot` and deliberately has no task foreign key.
Use this durable cleanup owner. Do not create a second deletion service or
assume the more extensive draft archive design is already implemented.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| AC-TASKS-DELETE-RECOVERY-001.1, .4, .8 | Admission and protocol |
| AC-TASKS-DELETE-RECOVERY-001.2, .3 | Durable retention and cleanup exclusion |
| AC-TASKS-DELETE-RECOVERY-001.5 | Runtime shutdown and outcomes |
| AC-TASKS-DELETE-RECOVERY-001.6, .7 | Shared confirmation surface |

## Admission and protocol

Add a typed workspace-cleanup mode, `delete` or `retain`, to the existing
deletion options. An absent mode means `delete`. Reject unknown values and
reject `retain` combined with `discard_worktree_changes=true`.

`POST /api/v1/tasks/delete-preflight` accepts `workspace_cleanup`. In retain
mode it performs only database-backed identity, task-scope, ownership, and
confirmation checks. It never calls a worktree inspector, HEAD capture, marker
reader, filesystem probe, or provider API. A missing physical inspection
dependency therefore cannot block this mode. Database inventory errors remain
errors. A successful empty database inventory is distinct from a failed query.

The response adds `workspace_cleanup` and keeps `confirmation_id` and
`requires_discard_consent`. Retain mode returns false for discard consent
because it authorizes no file removal, not because the workspace is clean.
`Cache-Control: no-store` remains mandatory. The existing five-minute ticket
also binds the mode. `WithTaskDeleteConfirmation` rejects a ticket issued for
another mode, actor, root, descendant scope, or row version.

`DELETE /api/v1/tasks/:id` accepts `workspace_cleanup=retain` and the existing
confirmation header. `delete_task_kandev` accepts the same optional mode and
uses its existing trusted MCP authorization and admission rules. This does not
give MCP new descendant or managed-conversation deletion authority. Propagate
the mode through every wrapper to one service options value. HTTP retains its
Human confirmation requirement. Plugins and unattended callers do not gain an
automatic fallback that silently changes their requested cleanup policy.

`HandoffService` resolves and authorizes the complete deletion scope under the
existing lifecycle boundary. Retain mode bypasses physical deletion admission,
not task or database admission. It reserves cleanup before inventory capture,
as normal deletion does. Shared environment transfer and managed-delete claims
retain their existing ordering and compensation. No user-supplied path is
accepted. Selecting retain applies to all resources in that selected scope.

## Durable retention and cleanup exclusion

Add a versioned retention section to the durable cleanup snapshot. Record the
mode, trusted actor/workspace identity, task and operation IDs, recorded local
and remote paths, repository/worktree/branch identities, executor resources,
and any incomplete physical identity. These are retention references, not
proof that cleanup is authorized. Capture them from database inventory under
the lifecycle reservation. Do not invent a HEAD or repair a marker.

Preparation in retain mode skips `captureWorktreeCleanupHeadOIDs`, adoption
audits, source-manifest capture, and all other physical cleanup preparation.
Retain snapshots must survive job progress saves. Persist the snapshot before
removing any live owner row. If persistence fails, compensate preparation and
leave the task. If cascade deletion commits only some members, retain the
snapshots for committed members and report the exact partial outcome. Preserve
existing recovery for members that did not commit.

Every workspace-destructive worker step must first check the mode. This includes
worktree removal, branch/ref cleanup, registration pruning, task-root removal,
SSH directory reclamation, sandbox/container destruction, and volume removal.
Retain mode never calls those steps. A successful shutdown does not change the
mode to delete. Missing mode on old snapshots preserves current behavior.

The retained cleanup snapshot remains the sole retention authority. Add a
repository query and typed projection for retention records in **all** job
states, including prepared, succeeded, failed, and cancelled. A cancelled
preparation can keep conservative protection until rollback proves that the
original owner remains. Do not expire or prune a committed retention snapshot.
No task/workspace cascade foreign key may erase it.

Merge the retained task's earlier cleanup snapshots into its protected inventory.
Those snapshots can hold paths and provider identities whose live rows were
already removed. Keep these supporting snapshots while retention applies.
Preparation also carries earlier runtime stop handles into the retained job,
so deletion cannot lose an unfinished shutdown obligation.

Extend `backendapp/storage_inventory.go` to merge retained paths and identities
with live inventory. `LoadWorkspaceInventory` must protect the task root and
all ancestors of retained repository paths. `ContainerTaskRemovable` must
consult retention before treating an absent task as removable. Carry protection
through workspace quarantine, permanent deletion, provider-specific cleanup,
and branch maintenance. Existing generic cleanup controls do not override it.
Failed retention queries or malformed snapshots make the affected inventory
incomplete. If resource scope cannot be established, block that provider's
destructive sweep rather than treating the resources as absent.

Inventory handoff must have no unprotected interval. Keep the existing live
owner until the retention snapshot commits. Coordinate a concurrent cleanup
claim with the same resource/lifecycle guards. Recheck retention before I/O:
a scan made before the deletion is not permission to remove resources after it.
Serialize retention publication and destructive cleanup through the shared
context-aware mutation lease in `task/retention`. Acquire it before path and
repository locks. Nested cleanup calls inherit the lease through their context.
Older workers finish their physical operation before publication, or read the
retained disposition when they acquire the lease. An already
completed cleanup cannot be reversed or described as newly preserved.

No new cleanup queue is required. Audit all snapshot readers before choosing
an additive JSON change alone. Reader compatibility is a release gate: a
supported reader must never ignore a retained disposition and perform cleanup.
Mixed-version cleanup workers and downgrade to a binary without retain support
are unsupported while retained snapshots exist. Document this operational limit.

## Runtime shutdown and outcomes

Retain mode still requests shutdown of the deleted task's agents using
persisted execution and ownership handles. Stop only that task's agents in a
shared environment. Keep runtime stop separate from environment destruction.
If a provider's stop operation also destroys files, preserve the resource and
record shutdown pending instead of invoking that operation.

Failure to contact an executor after snapshot persistence does not prevent task
row deletion. The durable worker owns bounded attempts and its existing retry
or retained failure policy. Unknown process identity never permits a broad
process-name kill. Preserve stop handles even after task/session rows disappear.

Add structured outcome fields to the existing deletion result:
`workspace_cleanup: "retain"`, `retention_operation_ids`, and
`runtime_stop_pending`. The committed result must use the existing committed
outcome channel even if shutdown activation fails. Never turn a committed
deletion into a generic "unable to delete" response. Bulk callers retain
per-root outcomes. Existing lifecycle events remove committed tasks from views.

Log the operation ID, retention mode, bounded failure class, and shutdown state.
Retained snapshots provide diagnostics after response loss. Do not log tokens,
file contents, or provider credentials. Do not label file retention as successful
file cleanup. A completion message says that files were kept and reports pending
shutdown when applicable.

## Shared confirmation surface

Extend `useTaskDeletePreflight`, `kanban-api.ts`, the shared
`TaskDeleteConfirmDialog`, its cleanup summary, and every callback adapter.
The closest shipped exemplar is that centered task-delete alert, including its
fixed heading/footer and scrolling body. Keep it on phones because this is one
short confirmation with options, not a new navigation flow.

Add an unchecked "Keep workspace files" checkbox above discard consent. It is
available during ordinary inspection and errors. Selecting it clears discard
consent, cancels/ignores the old request, and obtains a retain-mode preview.
It does not submit deletion. Hide discard consent in retain mode. Replace
cleanup-removal consequences with the promise to retain workspace resources
and a statement that the task and conversation will be deleted.

The confirmation button reads "Delete task and keep files" in retain mode.
While its own preview is pending, it remains disabled. A failed normal check
shows Retry and guidance to select keep-files. Retain-mode authorization or
database failures remain actionable errors. Retry keeps the chosen mode.
Changing task IDs, cascade, or closing resets the ticket and consent. Switching
back to normal mode performs fresh inspection. Late responses cannot restore
an old ticket or overwrite the selected mode.

Use the existing AlertDialog, Checkbox, Button, semantic destructive variant,
focus return, and theme tokens. No additional modal, staggered entrance, or
custom icon animation is needed. Desktop ordinary controls retain 28px density.
Phone and coarse-pointer controls use at least 44px hit areas. The body owns
vertical scrolling with containment. The footer clears safe-area insets, and
the surface stays within the dynamic viewport. Long translations must wrap
without clipping controls or causing horizontal page scroll.

All copy uses translation keys in the seven supported languages. Continue to
announce errors and loading states. Show result feedback through the existing
deletion notification path. Do not claim files are removed in retain mode.

## Verification boundary

The [work order](../../../plans/task-delete-recovery/task-01-retained-deletion.md)
owns the corruption, permissions, restart, stale-confirmation, cleanup-race,
shared-resource, HTTP/MCP, desktop, and phone regression tests. Tests must use
disposable repositories and databases. Production incident files are evidence,
not test fixtures to modify.
