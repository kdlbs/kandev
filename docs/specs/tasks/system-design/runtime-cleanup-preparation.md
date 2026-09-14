---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-09-09
updated: 2026-09-09
owners:
  - cfl
---

# Task Cleanup Preparation

## Boundary and requirement mapping

This design extends [Task Runtime Cleanup](runtime-cleanup.md) at the
preparation boundary. It implements AC-TASKS-RUNTIME-CLEANUP-001.9 and
AC-TASKS-RUNTIME-CLEANUP-001.13. The task system owns preparation because its
cleanup snapshot must survive task deletion.

## Identity capture

`worktree.Manager.CaptureCleanupHeadOIDs` distinguishes an absent path from
filesystem errors, symbolic links, and non-directory replacements. An existing
checkout supplies its commit identity through the bounded Git inspection path.
An absent checkout requires an exact local branch inspection in the parent
repository. A surviving branch supplies its commit identity for later audits.

Only a successful inspection that finds no exact branch permits omission of
that worktree's commit identity. Git exit 128, diagnostic text, cancellation,
and repository access errors do not establish absence. A quiet ref probe that
also classifies broken refs as absent cannot authorize this omission.

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
