---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-09-05
updated: 2026-10-19
owners:
  - cfl
---

# Dirty Worktree Task Deletion System Design

## Purpose and boundaries

This design extends task runtime cleanup for direct and cascade deletion. It
defines how deletion detects local worktree changes, obtains explicit discard
consent, and preserves every existing ownership and Git identity safeguard.

It does not change archive, session deletion, workspace reset, automatic commit
or push behavior, or branch redundancy rules.

This document describes normal deletion with workspace cleanup. The explicit
keep-files alternative is defined in [task deletion recovery](task-delete-recovery.md).
Its retention choice does not grant permission for destructive cleanup. The
same recovery design defines explicit admission with unknown inspection. That
extension supersedes the unconditional inspection-error block below only when
its separate acknowledgement and discard consent are both present. Physical
removal audits remain mandatory.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| AC-TASKS-RUNTIME-CLEANUP-001.9 | Admission, audited removal, and failure classification below |
| AC-TASKS-RUNTIME-CLEANUP-001.6, .25 | Cleanup snapshot persistence |

## Existing system context

Task deletion currently prepares a durable cleanup job, deletes the task row,
publishes `task.deleted`, and then starts the cleanup worker. The worktree
manager checks `git status --porcelain=v1 --untracked-files=normal` during
audited removal. If it finds output, it rejects removal after the task is gone.

The cleanup worker has eight bounded attempts with backoff. A dirty-checkout
refusal is deterministic, so another scheduled attempt cannot make progress
without user consent.

## Design

### Admission before mutation

Direct and cascade deletion use one options value with independent `cascade` and
`discard_worktree_changes` fields. The service resolves the full target set,
captures each owned worktree through the existing audited snapshot path, and
checks every local checkout before it changes a task or cleanup job.

If a checkout is dirty and consent is absent, the service returns one typed
conflict with all dirty worktrees. The operation does not mutate any selected
task and does not create or activate a cleanup job.

If consent is present, the service stores it in every applicable cleanup
snapshot before it performs the existing task mutation sequence. A cascade uses
one explicit consent choice for its complete target set.

### Audited removal with consent

Consent bypasses only the clean-checkout rejection. Removal still requires:

- a pinned no-follow path handle;
- exact workspace, owner, and worktree identity;
- an expected Git worktree registration;
- the expected `HEAD` commit;
- no shared active environment reference; and
- the existing branch redundancy and ancestry result.

The worktree checkout can be removed when these checks pass. The branch remains
when it contains unique commits under the existing cleanup policy.

### Race after admission

A checkout can become dirty after admission. If its cleanup snapshot has no
discard consent, the worker preserves the checkout and records a terminal
`failed` result after its first claim. It does not schedule another attempt.

A snapshot with consent can remove the changed checkout, subject to all other
audits. Existing snapshots decode without consent and remain fail-closed.

## Components and ownership

- `task/service` owns target resolution, admission, atomic cascade behavior,
  typed errors, and the cleanup snapshot.
- `task/handlers` maps the typed service conflict to the HTTP contract.
- `worktree.Manager` owns Git status inspection and the constrained removal
  audit.
- The cleanup worker owns terminal classification for a dirty refusal found
  after task mutation.
- The UI system design owns how the user gives consent and receives conflict
  feedback.

## Interface contract

`DELETE /api/v1/tasks/:id` accepts the optional
`discard_worktree_changes=true` query parameter. The existing `cascade=true`
parameter remains independent.

Without consent, a dirty target returns HTTP 409 before task mutation:

```json
{
  "error": "task worktree contains local changes",
  "error_code": "task_delete_dirty_worktree",
  "dirty_worktrees": [
    {
      "worktree_id": "...",
      "repository_id": "...",
      "path": "...",
      "dirty_files": ["relative/path"]
    }
  ]
}
```

The response lists every dirty worktree in a direct or cascade request. The
service keeps the error typed so non-HTTP callers also fail before mutation
unless they provide explicit consent.

## Persistence

### Read-only confirmation inspection

Proposed endpoint: `POST /api/v1/tasks/delete-preflight`, with body
`{"task_ids":["..."],"cascade":false}`. This endpoint performs inspection only.
It must not prepare cleanup jobs, stop agents, change ownership, or mutate tasks.
Return `{"requires_discard_consent":false}` only after complete inspection;
return `true` when at least one owned worktree is dirty. Use `Cache-Control:
no-store`. Invalid or empty task selections fail validation. Authorization or
inspection failures use the existing HTTP error envelope, never a clean result.

The task service owns the operation. Authorize all roots and descendants using
the same task-write scope as deletion before inspecting any paths. Reuse the
descendant resolution used by `HandoffService` for cascade deletion, including
archived descendants, then deduplicate the target set. Reuse `WorktreeProvider`
inventory and `WorktreeDirtyInspector.InspectDirtyWorktrees` from delete admission.
Keep caller-supplied paths out of the API. Missing inspection dependencies must
return an error; existing optional dependency fallbacks are not evidence of a
clean checkout. Confirmed absent paths retain the inspector's existing behavior.

The shared delete dialog requests this result when opened and whenever task IDs
or cascade selection change. A task-local hook owns loading, resolved, and error
states, ignores stale responses, and resets consent when its request key changes.
Executor type, descendant count alone, and cached session Git summaries do not
establish whether consent is needed. The explicit `requireDiscardConsent` prop
must no longer override a fresh clean result; remove its heuristic callers.

Loading and transport/admission errors hide discard consent and disable Delete.
A typed unknown-inspection result uses the recovery acknowledgement flow.
The independent keep-files choice remains available under the recovery design.
Errors offer localized retry feedback. Clean results hide discard consent and send
`discardWorktreeChanges:false`. Dirty results show the existing unchecked option
and enable Delete only after selection. Closing resets all consent. The existing
typed mutation conflict remains the race guard; reopening performs fresh
inspection before retry. No preflight response authorizes cleanup or bypasses
the mutation-time audits.

The existing centered phone alert remains the exemplar. Its body owns scrolling
and its footer keeps stacked touch actions; desktop keeps compact row actions.
Archive's full dialog, popover, inline confirmation, and preference bypass remain
unchanged. Its cascade checkbox concerns descendants, not dirty files.

The normal inspection boundary adds no alternate cleanup policy. The separate
retain mode has its own linked design and retention decision.

## Cleanup snapshot persistence

The cleanup snapshot JSON gains an additive discard-consent field. No database
schema change is required. A missing field is `false`.

## Failure and recovery

- A status inspection error fails ordinary admission closed. The recovery
  design permits explicit consent to delete the task with unknown changes and
  durable pending cleanup. This does not authorize removal of an uncertain path.
  The separate keep-files action does not inspect or remove the workspace.
- A dirty conflict preserves the task, worktree, branch, and local changes. The
  user can commit the changes or retry with explicit discard consent.
- A non-cleanliness audit failure preserves the checkout even when consent is
  present. Its existing retry or terminal policy remains active.
- A raced unconsented dirty refusal is terminal because retry cannot obtain
  consent for an already deleted task.

## Security and privacy

The response contains only task-owned worktree identifiers, paths, and relative
dirty file names available to the authorized task caller. Consent never accepts
a caller-supplied filesystem target and never weakens symlink or ownership
checks.

## Observability

The service logs the typed admission conflict without treating it as an internal
cleanup failure. A post-admission dirty refusal records one terminal cleanup job
error. It does not emit repeated retry attempts.

## Test scenarios

- **GIVEN** one or more dirty owned worktrees without consent, **WHEN** direct or
  cascade admission runs, **THEN** it reports every dirty target and changes no
  task or cleanup job.
- **GIVEN** a dirty owned worktree with consent, **WHEN** every other audit
  passes, **THEN** cleanup removes the checkout and applies the existing branch
  redundancy result.
- **GIVEN** one identity audit fails, **WHEN** consent is present, **THEN**
  cleanup preserves the checkout.
- **GIVEN** an unconsented checkout becomes dirty after admission, **WHEN** the
  worker sees the refusal, **THEN** it records terminal failure without retry.
- **GIVEN** an older cleanup snapshot, **WHEN** it targets a dirty checkout,
  **THEN** missing consent is false and cleanup fails closed.

## Related records

- [Conditional discard confirmation plan](../../../plans/conditional-discard-confirmation/plan.md)

- [Task Runtime Cleanup](runtime-cleanup.md)
- [Fail-closed GC semantics](../../../decisions/0009-fail-closed-gc-semantics.md)
- [Task-owned worktree lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
- [Confirmation Warning Hierarchy](../../ui/system-design/confirmation-warning-hierarchy.md)
