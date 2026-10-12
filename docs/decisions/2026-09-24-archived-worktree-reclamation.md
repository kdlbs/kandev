# ADR-2026-09-24-archived-worktree-reclamation: Reclaim retained worktrees through task lifecycle

**Status:** accepted
**Date:** 2026-09-24
**Area:** backend, frontend

## Context

Task archive now keeps a Git worktree when it contains tracked or untracked
changes. Its durable archive job can then succeed without a later retry. A
user who cleans that checkout later has no automatic reclamation path when
optional storage maintenance is disabled.

Storage maintenance currently excludes archived tasks from its active worktree
inventory. It can quarantine a still-dirty archived checkout after the orphan
grace period and permanently delete it after quarantine retention. This
contradicts the archive preservation contract. Its directory age is also not
the time at which the checkout became clean.

## Decision

The task lifecycle owns eventual reclamation of an archived task's retained,
active Git worktree. The existing task cleanup worker will run a bounded,
default-on recheck independently of the optional install-wide storage
maintenance schedule. It will preserve a checkout while it is dirty, in use,
or cannot be inspected. After it verifies that the task is still archived and
the checkout can be removed, it will use the existing audited worktree
cleanup and branch-compaction path. Rechecks need durable, fair scheduling so
a large set of dirty checkouts does not starve a newly clean one.

An active `task_environment_repos` worktree row remains a live filesystem
reference even when its task is archived. Storage maintenance must protect its
workspace from classification, quarantine, and permanent deletion, including
manual force deletion of a previously quarantined entry. A deleted worktree
row can remain as branch-recovery history without protecting an absent
checkout. Storage maintenance retains its opt-in schedule and its separate
role of reclaiming unreferenced task roots, caches, and other owned resources.

The archive confirmation explains the retained-checkout lifecycle when that
confirmation is enabled. It does not add a new archive API consent flag or
override the user's existing confirmation preference.

## Consequences

- Archived dirty checkouts can use disk space indefinitely until they become
  clean or the user explicitly deletes them with the existing discard consent.
- Cleaning a retained checkout makes its worktree eligible for a later task
  lifecycle pass even when storage maintenance is disabled.
- Storage analysis may count an archived task root as protected while an active
  worktree row owns a physical checkout. Old quarantine entries with such a
  row remain restorable instead of being purged automatically.
- The recheck needs bounded queries, persistent scheduling state, and
  mutation-time ownership and cleanliness checks. Existing task cleanup and
  unarchive races remain governed by the task lifecycle generation boundary.
- Directory contents outside managed Git worktrees and truly orphaned roots
  remain subject to the operator's separate storage policy.

## Alternatives Considered

- **Enable all scheduled storage maintenance by default.** Rejected because it
  also changes cache, container, and orphan cleanup policy and does not make
  age-based deletion of a dirty checkout safe.
- **Make storage maintenance inspect archived Git changes before quarantine.**
  Rejected as the primary reclamation path because installations with its
  schedule disabled would still retain clean checkouts, while storage would
  duplicate the task worktree manager's ownership rules.
- **Leave retained worktrees until a manual Storage run or task deletion.**
  Rejected because cleaning a checkout would not finish the archive lifecycle
  without an unrelated operator action.
- **Keep the original archive job pending while a checkout is dirty.**
  Rejected because dirtiness is a preservation decision, not a failed archive
  operation; it must not prevent the rest of archive cleanup from succeeding.
