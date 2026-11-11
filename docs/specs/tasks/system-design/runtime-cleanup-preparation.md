---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-09-09
updated: 2026-11-05
owners:
  - cfl
---

# Task Cleanup Preparation

## Boundary and requirement mapping

This design extends [Task Runtime Cleanup](runtime-cleanup.md) at the
preparation boundary. It implements AC-TASKS-RUNTIME-CLEANUP-001.9 and
AC-TASKS-RUNTIME-CLEANUP-001.13. The deadline amendment also maps criteria
001.1, .5, .6, .25, and .42. The task system owns preparation because its
cleanup snapshot must survive task deletion.

These identity-capture rules authorize physical cleanup in normal delete mode.
[Task deletion recovery](task-delete-recovery.md) defines separate retain-mode
preparation. It captures database inventory without physical inspection and
never authorizes workspace removal. That design also defines acknowledged
normal deletion with incomplete physical evidence. It permits a durable
per-resource blocked preparation result before deleting the task. It does not
relax the identity requirements for physical removal described here.

## Bounded coordination before deletion

This amendment is implemented in the
[settlement deadlock package](../../../plans/cancellation-completion-delete-deadlock/plan.md).
It preserves the existing delete timeout and the recorded resource ownership rules.
The rest of this design remains draft.

The cascade mutex must observe the request's captured absolute deadline.
Apply the same acquisition semantics to archive and unarchive because they share that mutex.
An expired waiter returns without later acquiring ownership or starting a mutation.

Runtime stop must also bound admission to `remoteInstanceLifecycleMu` and `promptLifecycleMu`.
Use one small mutex wrapper over the existing `golang.org/x/sync/semaphore` dependency.
The wrapper supports zero-value initialization, `Lock`, `Unlock`, `TryLock`, and context-aware acquisition.
It must not start a helper goroutine or poll for availability.
An expired acquisition leaves no queued owner that can act after the caller returns.

`preserveTurnChangesBeforeStop` returns an error when checkpoint admission or its required capture cannot complete safely.
Its existing capture budget includes mutex admission and cancellation preparation.
The earlier caller deadline still wins.
`StopAgentWithReason` preserves runtime registration, shutdown handles, and workspace resources on this error.
It must not report the runtime stopped or proceed with destructive cleanup.

Normal deletion of a verified workspace propagates synchronous stop errors before membership release or task-row deletion.
Keep archive's existing cancellation policy separate from this strict delete path.
Retain mode and explicitly accepted unknown workspace changes keep the
[recovery contract](task-delete-recovery.md): committed deletion can retain pending shutdown obligations.
Cancel only this request's uncommitted prepared jobs while its original budget permits compensation.
Use `CancelPreparedTaskResourceCleanupWithinDeadline` for that request-owned compensation.
Ordinary durable cleanup transitions retain their existing detached, bounded context.
If persistence becomes unavailable at expiry, retain the snapshot and unknown outcome for existing reconciliation.
Do not mark an unconfirmed stop successful or cancel another operation's obligation.
Committed members retain their existing cleanup obligations and partial-cascade response semantics.

A request after a failed lock admission revalidates current task, execution, and cleanup ownership.
Startup recovery still distinguishes committed deletion from an uncommitted prepared intent.
The new stop-failure compensation path receives no fresh work budget.

Verification holds each relevant mutex behind a barrier and uses a short operation context.
Assert deadline return, no delayed owner, intact uncommitted task, retained shutdown evidence, and safe retry after release.
Include a mixed cascade with a previously committed member and an uncommitted blocked member.
Every committed member retains its obligations. No blocked member authorizes resource removal.

## Identity capture

`worktree.Manager.CaptureCleanupHeadOIDs` distinguishes an absent path from
filesystem errors, symbolic links, and non-directory replacements. An existing
checkout supplies its commit identity through the bounded Git inspection path.
An absent checkout requires an exact local branch inspection in the parent
repository. A surviving branch supplies its commit identity for later audits.
A directory without checkout-local `.git` metadata counts as an absent
checkout: Git would otherwise discover an enclosing repository and report its
HEAD as this worktree's identity. The HEAD inspection pins Git to that
`.git`, so metadata removed after the presence check fails the inspection
instead of reaching an enclosing repository. The pin resolves from the
command's checkout directory, including when its recorded path is relative.

Only a successful inspection that finds no exact branch permits omission of
that worktree's commit identity. Git exit 128, diagnostic text, cancellation,
and repository access errors do not establish absence. A quiet ref probe that
also classifies broken refs as absent cannot authorize this omission.

A surviving checkout whose Git administrative directory or source repository
is unavailable remains an inspection failure. Preparation and delete preflight
follow the platform's
[cleanup failure attribution](../../platform/system-design/runtime-failure-attribution.md#cleanup-preparation-and-deletion-preview)
contract so this state is diagnosable without relaxing cleanup ownership.

## Persistence

Preparation retains the worktree in the durable snapshot. It does not mark
repository rows deleted, remove registrations, or remove branches. Other
repositories still contribute their own commit identities.

The existing snapshot represents the absent identity through an omitted map
entry. This correction requires no schema migration, new snapshot field, or
change to prepared-job activation and reconciliation.

## Execution and recovery

The cleanup worker revalidates the recorded resource through its existing
path, registration, branch, commit, and shared-reference audits. An absent
path, branch, and registration complete through the existing idempotent path.
Competing or unverifiable registrations remain retryable. Worktrees present in
the durable snapshot are sent through the identity-aware batch cleaner; task
environment teardown does not send those same IDs through its legacy ID-only
destroyer. A replacement branch or checkout without the required immutable
identity cannot authorize destructive cleanup. Archive retains its existing
branch-preservation disposition.

These checks preserve the [fail-closed cleanup decision](../../../decisions/0009-fail-closed-gc-semantics.md)
and [task-owned worktree lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md).
