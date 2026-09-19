---
status: draft
system: tasks
requirements:
  - REQ-TASKS-MANAGED-BRANCH-COMPACTION-001
created: 2026-08-30
updated: 2026-09-14
owners:
  - cfl
---

# Managed Branch Compaction System Design

## Purpose and boundaries

This design extends
[Task Runtime Cleanup](runtime-cleanup.md) for the disposition of managed local
branches at terminal task boundaries. It defines capture of ownership and
integration metadata on canonical task-environment repository rows, the
fail-closed eligibility checks under the repository lock, the exact-head
recovery protocol, bounded archived-branch maintenance, and the bounded
cleanup receipt.

It does not change task deletion semantics, remote refs, branch-policy
configuration, or archive and unarchive state ownership.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| REQ-TASKS-MANAGED-BRANCH-COMPACTION-001 | Sections below |

## Decision source

[ADR-2026-08-30-compact-integrated-managed-branches](../../../decisions/2026-08-30-compact-integrated-managed-branches.md)

## Metadata capture

Every task-environment repository row that materializes a worktree branch
persists three conservative fields, each defaulting to unknown or empty for
legacy rows without inference:

- `worktree_branch_owner`: `kandev`, `external`, or `unknown`;
- `worktree_integration_ref`: the intended base or integration branch, resolved
  from the task branch policy pull-request target first, then the contribution
  destination target, then the task repository base branch;
- `worktree_recovery_head_sha` plus `worktree_branch_compacted_at`: the exact
  compaction recovery state, empty until a deletion is prepared.

Missing or ambiguous metadata always retains the branch.

## Terminal compaction boundary

Immediately after a terminal cleanup successfully removes the worktree
registration, while still holding the existing repository lock, cleanup
considers exactly one explicit candidate local branch. The candidate is
eligible only when the persisted owner is `kandev`, exactly one durable
environment-repository row owns the repository and branch identity, no live Git
worktree uses the branch, the branch is neither protected nor the repository
default, base, or integration ref, and the branch's exact tip is an ancestor of
the exact persisted integration ref. Cleanup never fetches, globs, enumerates
unrelated refs, or infers eligibility from names, age, or convention.

Before deletion, cleanup persists the exact candidate head SHA with
compare-and-set semantics, then removes the branch with
`git update-ref -d refs/heads/<branch> <expected-head-sha>`. An expected-head
mismatch or any check failure retains the branch and leaves the state retryable.

## Exact-head recovery

Recreation after archive or unarchive restores a missing managed branch from
the recorded exact head before any refreshed-origin or contribution
materialization can select a different head. A local branch of the same name
must resolve to the recorded head or recovery fails closed. The hidden
identity-derived recovery ref is deleted before the durable recovery fields are
CAS-cleared, so every interruption point remains retryable. Branches that were
never compacted keep the existing preserved-local-ref behavior and restore
without rewriting their tip.

## Bounded archived maintenance

Storage maintenance selects at most 100 archived, inactive, deleted-worktree
rows per run without scanning names, revalidates archived state and session
inactivity immediately before mutation, and reuses the same manager-owned
policy. Interrupted rows retry; a durable completion marker prevents
reselection. A branch that becomes live during deletion is restored at the
exact recorded head and retained.

## Observability

Each cleanup operation returns a bounded receipt with attempted, deleted, and
retained counts plus fixed retained-reason counts. Logs and metrics expose the
same bounded labels without branch lists, secrets, commit contents, or
unbounded cardinality.
