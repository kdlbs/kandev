---
status: active
system: tasks
created: 2026-09-27
updated: 2026-10-02
owners:
  - kandev
---

# Managed clone relocation requirements

## Overview

A provider repository can acquire a new workspace-scoped source clone while an
older task still owns a worktree attached to its previous managed clone. The task
system owns the continuity contract because its environment owns that worktree.
The workspace system still owns source-clone placement and credentials.

## Terminology

- **Source clone change:** The repository's registered local checkout changes,
  while a task's recorded worktree remains attached to the prior clone.
- **Verified clean worktree:** Its recorded branch and commit are available, and
  it contains no tracked modifications, staged changes, untracked files, ignored
  files, or unsupported filesystem entries beyond the checkout.
- **Relocation:** A guarded replacement worktree attached to the current
  workspace-scoped clone. The original worktree remains available.

## Requirements

### REQ-TASKS-MANAGED-CLONE-RELOCATION-001: Continue safe task work

**Intent:** A source-clone change must not strand an otherwise valid task.

#### Acceptance criteria

- **AC-TASKS-MANAGED-CLONE-RELOCATION-001.1:** When an idle Worktree environment
  contains a verified clean worktree from an older managed clone after its
  registered source clone has changed, launch and
  resume shall relocate it before agent startup. The task, session, branch,
  and existing provider resume identity shall remain the same.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-001.2:** Relocation shall preserve the
  exact recorded commit, including an unpushed commit available in the source
  clone. It shall retain the original checkout and publish the replacement only
  after the selected environment has a valid repository inventory.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-001.3:** An active runtime or incomplete
  inventory shall prevent automatic relocation and agent startup. The same
  applies to ambiguous identity, changed branch or commit, missing objects,
  unexpected source path, or unsupported checkout content. Refusal shall leave
  the original checkout, repository row, and environment inventory unchanged.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-001.4:** Only selected host Worktree
  environments are eligible. Local and remote executors, other tasks, and other
  workspaces shall not be inspected or changed by this recovery.

- **AC-TASKS-MANAGED-CLONE-RELOCATION-001.5:** When a valid task checkout still
  belongs to its registered managed source clone, launch, resume, and workspace
  restoration shall reuse it without requiring a newer clone to exist. The
  checkout, branch, commit, staged changes, submodules, ignored files, and
  provider conversation shall remain unchanged. Restrictions needed only to
  transfer a checkout shall not prevent its reuse.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-001.6:** Legacy reuse shall require the
  registered source and actual checkout to identify the same recognized managed
  clone of the selected repository. A missing or invalid registered source,
  foreign workspace, wrong origin, or a mismatch between the task slot's recorded
  source identity and the actual checkout shall remain an error. A workspace
  repository moving to a new source clone remains eligible for the guarded
  relocation in .001.1. An unchanged slot shall not bypass validation of any
  other selected repository slot.

### REQ-TASKS-MANAGED-CLONE-RELOCATION-002: Recover work that cannot move silently

**Intent:** Preserve user work and offer an honest recovery path for a dirty or
otherwise ineligible checkout.

#### Acceptance criteria

- **AC-TASKS-MANAGED-CLONE-RELOCATION-002.1:** A dirty worktree shall remain
  untouched until the user explicitly chooses relocation. The failure shall say
  that files remain in the original checkout and shall not offer Resume, Start
  fresh, or Restore read-only workspace as a repair for this mismatch.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-002.2:** The explicit action shall carry
  the current failure identity and task/session authorization. It shall preserve
  the recorded commit, tracked and untracked content, ignored files, deletions,
  executable modes, and symbolic links without following links outside the
  checkout. It shall retain the original checkout and a recovery snapshot.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-002.3:** Before explicit relocation, the
  user shall be told that staging choices and unsupported Git metadata may not
  transfer. A successful move shall identify the replacement worktree and keep
  any existing provider resume identity. It shall not imply that the original
  was deleted.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-002.4:** If identity, snapshot, claim,
  object transfer, or publication fails, the system shall keep the original
  checkout and current error visible. It shall not start an agent in a partial
  replacement or retry the explicit action without a new user request.

### REQ-TASKS-MANAGED-CLONE-RELOCATION-003: Recovery presentation

**Intent:** Make the failure understandable and the safe action reachable on
desktop and phone.

#### Acceptance criteria

- **AC-TASKS-MANAGED-CLONE-RELOCATION-003.1:** A clone mismatch shall have a
  stable, path-free error category and only actions that can resolve it. The
  same error and recovery choice shall survive reload without duplicating
  session history or clearing a newer failure.
- **AC-TASKS-MANAGED-CLONE-RELOCATION-003.2:** Desktop shall present the cause
  and recovery choice in the existing session recovery surface. Phone shall
  provide the same choice in its existing focused recovery view with touch
  targets of at least 44 pixels and no horizontal page overflow.

## Out of scope

- Repair of unrelated or user-managed local repositories.
- Automatic relocation of dirty worktrees or a missing branch.
- Automatic deletion of retained originals or recovery snapshots.
- A new workspace-wide background migration or provider credential policy.

## Related contracts

- [Additional-session workspace reuse](additional-session-workspace-reuse.md)
- [Worktree metadata recovery](worktree-metadata-recovery.md)
- [Task launch failure recovery](task-launch-failure-recovery.md)
- [System design](../system-design/managed-clone-relocation.md)

## Implementation plans

- [Original relocation package](../../../plans/managed-clone-relocation/plan.md)
- [Unchanged legacy clone admission](../../../plans/legacy-clone-resume/plan.md)
- [Resume and workspace recovery convergence](../../../plans/managed-clone-recovery-convergence/plan.md)

The convergence package repairs violations of the existing acceptance criteria.
It does not authorize automatic relocation of dirty worktrees.
