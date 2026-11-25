---
status: current
system: tasks
requirements:
  - REQ-TASKS-DELETE-RECOVERY-001
  - REQ-TASKS-DELETE-RECOVERY-002
created: 2026-10-16
updated: 2026-10-19
owners:
  - kandev
---

# Task deletion recovery

## Boundary and evidence

The task system owns deletion admission and durable resource retention. This
design covers retain mode and normal deletion with incomplete inspection. It
extends [normal deletion](dirty-worktree-deletion.md) and
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
| AC-TASKS-DELETE-RECOVERY-002.1-.9 | Normal deletion with incomplete inspection |

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
Reserve preparation without a retention disposition. Publish retain mode and
the complete inventory together, so failed preparation cannot leave an incomplete
retention record. Retain snapshots must survive job progress saves. Persist the
snapshot before removing any live owner row. If persistence fails, compensate preparation and
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
repository locks. Runtime shutdown must finish before the cleanup worker takes
this lease. Each provider stop guards its own destructive I/O after taking its
execution lock. The worker rechecks retention under the lease before file cleanup.
Nested cleanup calls inherit the lease through their context.
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
shared environment. A saved standalone runtime with matching task, session, and
execution IDs permits local shutdown even when the task has no owned environment.
A known remote runtime overrides a local environment fallback. Keep runtime stop
separate from environment destruction.
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
uses the unknown-inspection consent flow below. Transport or database errors
show Retry and keep-files without issuing a cleanup confirmation. Retain-mode authorization or
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

## Normal deletion with incomplete inspection

This extension implements REQ-TASKS-DELETE-RECOVERY-002. The
[deletion and cleanup decision](../../../decisions/2026-10-19-separate-task-deletion-from-cleanup.md)
separates committed task deletion from verified physical cleanup. The existing
retain-mode implementation remains independent. This extension is planned in
[work order 02](../../../plans/task-delete-recovery/task-02-unavailable-cleanup.md).

### Inspection and explicit consent

Extend `TaskDeletePreflightResult` with `workspace_inspection`, a closed status
of `complete`, `unknown`, or `not_requested`. Retain mode uses `not_requested`.
Normal mode reports `unknown` only for classified physical inspection failures
against database-resolved targets. Unknown must never return a clean preview.
Keep `requires_discard_consent=true` for unknown and dirty normal previews so
older clients cannot treat unknown as permission to delete without consent.

Add `allow_unverified_workspace_changes`, default false, to `DeleteTaskOptions`
and the existing preflight, HTTP, and MCP contracts. It requires
`workspace_cleanup=delete` and `discard_worktree_changes=true`. The flag consents
to task deletion and possible loss of unknown local changes. It is not physical
ownership evidence. It is distinct from ordinary dirty-file consent, so existing
scripts that pass only discard consent do not gain broader deletion authority.

Bind both consent flags to `taskDeletePreview`, and verify them in
`WithTaskDeleteConfirmation`. The UI obtains a fresh ticket when the user checks
or unchecks the acknowledgement. No API accepts a path supplied by the caller.
Normal MCP authorization remains mandatory and its explicit arguments carry
both consent values. All option adapters and cascade members use the same value.

Introduce typed inspection failures at marker, path, Git, and provider boundaries.
Separate context cancellation, database inventory failures, and missing service
configuration from physical unavailability. Do not catch every service error or
classify from user-visible strings. A cancelled request still returns cancellation.
An internal bounded inspection timeout can produce unknown while the parent
request remains valid. Missing physical dependencies do not prove a workspace
is safe or complete. They remain configuration errors before mutation.

`TaskDeletePreflightWithOptions` resolves and authorizes all targets before
inspection. Continue collecting per-resource results when one resource is
unknown. An unknown result dominates the selection's summary, while preserving
known dirty results. Every required database read must succeed. Physical
inspection is read-only and bounded. Preserve the existing confirmed-absence
classification rather than treating all not-found errors as absence.

### Preparation and committed outcomes

`ValidateTaskDeleteWorktrees`, direct deletion, and `HandoffService` must accept
the same options. Only the explicit pair of acknowledgement and discard consent
permits an unknown physical result to pass task-deletion admission. Healthy
normal deletion continues through the existing checks. Retain preparation is
unchanged and does not call the physical inspectors.

Extend the existing versioned `taskResourceCleanupSnapshot`, not a second queue.
Before deleting task records, persist the complete database inventory, exact
runtime handles, earlier unfinished cleanup handles, consent flags, and a
per-resource preparation result. Capture available HEAD, marker, adoption, and
provider identities under the current locks. Record a classified failure for
each unavailable physical identity without dropping the resource. Do not
manufacture identities or overwrite previously captured ones. Failed database
reads, snapshot serialization, or snapshot writes abort preparation.

Audit every physical preparation hook, including `captureCleanupHeadOIDsForMode`,
adoption capture and source-manifest paths. Archive-only preparation must not
become a hidden physical prerequisite for acknowledged task deletion. Skip or
record only classified physical failures under the new consent, never unrelated
persistence or admission errors. Preserve managed-delete claims, shared-owner
transfer, preparation compensation, and partial cascade semantics.

Task deletion remains committed when worker activation or later shutdown fails.
Generalize the result projection in `task_delete_result.go` for delete mode with
`workspace_cleanup`, `cleanup_operation_ids`, `workspace_cleanup_pending`, and
`runtime_stop_pending`. Retain responses keep `retention_operation_ids` and
never imply pending removal. Pending flags come from durable operation state,
including prepared jobs that have not started. Preserve computed flags across
HTTP and MCP post-commit errors. Return per-root committed results for bulk
operations, and use the existing task-deleted event to remove tasks from views.

### Cleanup execution and protection

Preserve the shutdown-before-destruction order. The worker processes independent
resources separately, persists completed steps, and continues eligible sibling
resources after one fails. A shared runtime or environment dependency can block
all resources that depend on it. No cleanup step may erase another step's
pending inventory. Retry resumes only unfinished effects.

The new consent does not bypass `auditWorktreeCleanup`. Preserve no-follow path
handles, marker ownership, exact registration, branch identity, expected commit,
shared-reference checks, and retention guards. Do not automatically rewrite a
corrupt marker, move a task directory, or run broad Git pruning. A task record
can disappear while its damaged directory still needs repair.

A worker may retry missing evidence only when retained immutable ownership and
physical identity prove the same target. It must persist verified evidence
before destructive I/O. If no trustworthy pre-deletion identity was captured,
a later directory or branch at the same name cannot supply that missing proof.
Leave that target blocked with a durable reason. Never adopt a replacement as
the original task's resource merely because the old owner row is absent.

Use the existing bounded retry schedule. A terminal failed operation keeps
inventory and diagnostics and is not a cleanup success. Transient unavailable
resources can complete after their original identities become verifiable.
No new automatic repair engine or operator screen is part of this change.

Pending and failed delete snapshots must protect their unfinished resources
from storage GC, quarantine, container sweeps, and stale cleanup jobs. Extend
the existing cleanup inventory projection with a distinct outstanding-cleanup
classification. Do not mark these resources retained forever: the exact owning
cleanup job may remove its verified targets. Other destructive callers cannot.
Enforce that distinction using an internal job identity and current claim,
never a caller-supplied bypass flag. Explicit retain protection always wins.

Publish outstanding inventory under the existing mutation lease before live
owner removal. Recheck the current claim, shared ownership, and retention under
that lease immediately before I/O. Keep runtime shutdown outside the lease.
Release outstanding protection only when each effect is durably complete, with
resource-specific progress retained after task/workspace deletion and restart.
Unreadable protection inventory fails closed at the affected scope. Read only
needed task/job rows and use indexed cleanup disposition when required by the
retention review fixes. Do not add a whole-table JSON decode to each resource
check. Older snapshots retain their existing behavior unless the new consent
and complete preparation envelope are both present.

### Confirmation surface and compatibility

Reuse `TaskDeleteConfirmDialog`, its existing centered AlertDialog, and
`useTaskDeletePreflight`. This is a short confirmation rather than navigation.
An unknown result shows an unchecked checkbox: "Delete even if workspace
changes cannot be checked". Supporting text states: "Local changes may be
lost. Files that cannot be safely removed will remain pending cleanup."
Keep the separate "Keep workspace files" checkbox. The unknown acknowledgement
sets both consent values through a fresh preview. It replaces the ordinary
discard checkbox in that state, so users do not need two acknowledgements.

Loading keeps the action disabled. A transport, authorization, or database
failure does not show the unknown acknowledgement as a bypass. Retry remains
available. Mode, scope, cascade, close, and reopen invalidate consent and the
confirmation ticket. Late responses cannot restore an old choice. Selecting
retain clears both removal consent flags and preserves its existing action.
Clean and known-dirty previews keep their current behavior.

Desktop keeps the compact action row. Phone keeps the inset centered alert,
fixed heading/footer, contained scrolling body, stacked 44px touch actions,
safe-area clearance, visible focus, and status announcements. Use existing
semantic colors and primitives without new motion. Localize all copy in the
seven languages and generated pseudo/Traditional Chinese catalogs.

After a committed delete with pending effects, report "Task deleted. Workspace
cleanup is pending." Report shutdown pending separately when true. Do not say
files were removed. Public task, Git, and MCP guidance must distinguish this
retryable cleanup authorization from permanent keep-files retention.

## Verification boundary

The [retain work order](../../../plans/task-delete-recovery/task-01-retained-deletion.md)
and [normal-delete work order](../../../plans/task-delete-recovery/task-02-unavailable-cleanup.md) own the corruption, permissions, restart, stale-confirmation, cleanup-race,
shared-resource, HTTP/MCP, desktop, and phone regression tests. Tests must use
disposable repositories and databases. Production incident files are evidence,
not test fixtures to modify.
