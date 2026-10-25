---
status: draft
system: tasks
created: 2026-08-30
updated: 2026-09-14
owners:
  - cfl
---

# Managed Branch Compaction Requirements

## Overview

Task lifecycle cleanup removes Git worktree registrations but must bound the
growth of local branches that Kandev itself created. Compaction may delete a
managed local branch only when ownership, liveness, and integration are proven
from persisted metadata; every unproven case retains the branch. Deleted
branches remain exactly recoverable from their recorded head.

The decision record is
[ADR-2026-08-30-compact-integrated-managed-branches](../../../decisions/2026-08-30-compact-integrated-managed-branches.md).

## Requirements

### REQ-TASKS-MANAGED-BRANCH-COMPACTION-001: Managed Branch Compaction

**Intent:** Bound generated local branch growth at terminal task boundaries
without inferring ownership from branch names, force-deleting work, touching
remote refs, or breaking archive and unarchive.

#### Acceptance criteria

- **AC-TASKS-MANAGED-BRANCH-COMPACTION-001.1:** When terminal lifecycle
  cleanup removes a worktree registration, it shall delete a local branch only
  when the persisted owner is Kandev, no live Git worktree uses the branch,
  exactly one durable environment-repository row owns it, and its exact head
  is contained in the persisted intended integration ref. Every other case
  shall retain the branch.
- **AC-TASKS-MANAGED-BRANCH-COMPACTION-001.2:** Before deleting an eligible
  integrated branch, cleanup shall persist its exact head SHA and create an
  opaque, worktree-identity-derived local recovery ref at that SHA before
  removing the branch. Unarchive and worktree recreation shall restore a
  missing managed branch from that SHA before remote recovery, then clear the
  compacted marker and remove the recovery ref only after the exact local
  branch protects the commit. An existing local branch must equal the recorded
  head or recovery fails closed. Branches with unpublished commits retain their
  original local ref and restore exactly.
- **AC-TASKS-MANAGED-BRANCH-COMPACTION-001.3:** Managed branch compaction
  shall delete only one explicit local ref with an atomic expected-head
  compare-and-delete. It shall never delete remote refs, protected or base
  refs, inferred branch globs, externally owned refs, or refs with legacy,
  missing, or ambiguous ownership metadata.
- **AC-TASKS-MANAGED-BRANCH-COMPACTION-001.4:** Terminal cleanup shall emit
  bounded attempted, deleted, and retained totals plus fixed retained-reason
  counts. Receipts and metrics shall not contain branch lists, repository
  contents, or credentials.
- **AC-TASKS-MANAGED-BRANCH-COMPACTION-001.5:** When archive cleanup retains a
  managed branch because it is not yet integrated, storage maintenance shall
  revisit at most a fixed number of archived, inactive worktree rows per run.
  It shall revalidate that the task remains archived immediately before
  invoking the worktree manager's existing safety policy. A later unarchive
  shall restore a safely compacted branch from its exact recorded head.
  Persisting that head without completing deletion shall remain retryable
  after interruption, while a durable completion marker shall prevent completed
  rows from being selected again. A branch that becomes live during deletion
  shall be restored at the exact recorded head and retained.
