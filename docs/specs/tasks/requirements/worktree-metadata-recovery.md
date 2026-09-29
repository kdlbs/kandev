---
status: active
system: tasks
created: 2026-09-10
updated: 2026-09-29
owners:
  - kandev
---

# Worktree metadata recovery requirements

## Overview

A linked worktree can retain its files after it loses its Git administrative
directory. Recovery preserves those files without changing an unrelated workspace.
The task system owns this contract because the task environment owns the physical
worktree inventory and its launch lifecycle.

This document defines recovery for damaged metadata and missing checkouts.
PR #3137 introduced metadata recovery. The missing-checkout extension follows
issue #4052. Implementation status is recorded in the linked plans.

## Terminology

- **Selected environment:** The physical environment that the requested session
  will use, including an inherited environment.
- **Repository slot:** One repository and branch selection in that environment.
- **Metadata loss:** The checkout exists, but its linked Git administrative
  directory is absent. The recorded branch still resolves in the correct repository.
- **Busy environment:** An environment with a runtime or another session that
  can still use its workspace. An idle agent does not prove that its runtime stopped.

## Requirements

### REQ-TASKS-WORKTREE-METADATA-RECOVERY-001: Selected environment isolation

**Intent:** Recovery must not change the meaning of an executor or repository selection.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-METADATA-RECOVERY-001.1:** For a task without repositories,
  recovery must perform no Git inspection or filesystem mutation.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-001.2:** Recovery must inspect only the
  selected environment. An unrelated damaged environment must not block the request.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-001.3:** Automatic host recovery must apply
  only to the Worktree executor. Local, Docker, SSH, Sprites, and Kubernetes retain
  their executor-owned workspace behavior.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-001.4:** An executor transition must not
  inspect or repair the previous environment through the new executor request.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-001.5:** A remote Git origin must not exclude
  an otherwise eligible host worktree. A remote execution path must not become a
  host path, even when an identical host path exists.

### REQ-TASKS-WORKTREE-METADATA-RECOVERY-002: Preserved repository content

**Intent:** Metadata repair must preserve available code and expose its limits.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.1:** A healthy worktree must retain its
  path and branch. A missing checkout must retain the normal materialization path
  unless it has a retained canonical identity covered by requirement 004.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.2:** For proven metadata loss, recovery
  must preserve tracked, untracked, and ignored files, deletions, executable modes,
  and symbolic links. It must not follow symbolic links outside the checkout.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.3:** An invalid pointer, ambiguous owner,
  unreadable path, changed snapshot, or unsupported file type must stop recovery.
  Recovery must retain the original checkout.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.4:** Recovery must use the surviving
  recorded branch. It must not substitute the task base branch after branch loss.
  Confirmed branch loss retains the separate explicit recovery action.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.5:** Recovery must retain the original
  checkout and completed snapshot. Documentation must state that recovery cannot
  reconstruct a lost index, staging choices, or unavailable commits.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.6:** For multiple repositories or
  branches, recovery must preserve each slot's identity. It must not replace a
  healthy slot or copy content between slots.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.7:** An inspection failure must stop the
  request before replacement starts. A later failure must preserve completed slot
  repairs and all original checkouts. No agent can start with an incomplete inventory.

- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.8:** A healthy main-repository
  checkout in the selected inventory shall remain usable for launch, additional
  sessions, resume, and workspace restoration. Admission shall preserve its path,
  branch, index, tracked edits, untracked files, and ignored files without repair.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-002.9:** A directory named `.git` alone
  shall not establish checkout health. Invalid, redirected, or unreadable metadata
  shall stop admission without changing the checkout or granting replacement authority.
  A symbolic `HEAD` shall name a valid local branch under `refs/heads/` or resolve
  as a detached commit. When admission includes managed-provider identity proof,
  the checkout metadata directory and origin shall match the selected destination
  and provider identity; a mismatch shall fail closed without relocating the main checkout.

### REQ-TASKS-WORKTREE-METADATA-RECOVERY-003: Exclusive recovery authority

**Intent:** Recovery must not redirect a workspace that another consumer can use.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-METADATA-RECOVERY-003.1:** For a busy environment, recovery
  must refuse replacement without stopping a runtime or changing its workspace.
  This includes a runtime for the requesting session.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-003.2:** A competing launch, workspace
  restore, cleanup, or ownership transfer must not overlap replacement authority.
  The same rule applies across sessions, shared tasks, and backend connections.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-003.3:** After an ownership change, a stale
  recovery attempt must not publish a replacement under the previous owner.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-003.4:** After interruption, recovery must
  retain the same operation identity or refuse continuation. It must not bypass
  an active claim or adopt an unrelated snapshot.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-003.5:** A refusal must reach the existing
  launch or resume error response before agent startup. It must not advertise
  Start fresh as a way to bypass unsafe metadata.

### REQ-TASKS-WORKTREE-METADATA-RECOVERY-004: Missing canonical checkout recovery

**Intent:** A deleted checkout must not permanently prevent reuse of a retained
Worktree environment when its recorded branch survives.

#### Acceptance criteria

- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.1:** Before attachment or resume,
  Kandev shall automatically restore a provably absent canonical checkout from
  its surviving recorded branch. Its environment, repository slot, worktree
  identity, path, branch name, and surviving branch head shall remain unchanged.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.2:** Recovery shall require exclusive
  authority over the selected environment. Any live runtime consumer, including
  the requester or a borrowing task, shall prevent mutation. An unavailable
  liveness check shall prevent mutation. Recovery shall not stop consumers.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.3:** A present path, unsafe ancestor,
  uncertain owner, conflicting Git registration, or inspection error shall not
  qualify as an absent checkout. Existing metadata-recovery rules still apply
  to surviving checkouts. Missing local and tracking refs alone shall not count
  as confirmed branch loss.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.4:** Every selected slot shall pass
  preflight before any restoration starts. Healthy slots shall remain unchanged.
  A failed later slot shall preserve completed repairs, and no agent shall start
  with an incomplete inventory.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.5:** Concurrent or interrupted recovery
  shall not create another workspace identity or overwrite unexpected files.
  Git mutations shall use a pinned, exclusively reserved target identity, and
  cleanup shall remove only its exact proven stale administrative entry.
  Continuation shall retain exclusive authority and the original operation
  identity, or refuse without mutation.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.6:** The restored checkout shall be
  available through existing launch, resume, and workspace-restoration paths.
  Recovery shall not require a new UI action. Refusals shall use the existing
  typed recovery response before agent startup.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.7:** Recovery shall not claim to restore
  deleted uncommitted files, staging choices, or unavailable commits. It shall
  not run repository setup, reset the branch, or substitute a base branch.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.8:** Recovery shall hold an exclusive
  operating-system operation lock through mutation and the external-start
  boundary. After a crash with a completed operation record and retained database
  claim, admission shall revalidate the exact record, checkout, owner, generation,
  session, and operation under that lock before settling only the matching claim.
  It shall never clear a claim by age.
- **AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9:** If the local and tracking refs are
  absent, recovery shall probe the configured origin for the exact recorded
  branch. Probe failure shall remain an inspection error, not branch loss. A
  surviving origin branch may be fetched only at the verified advertised head.
  An interrupted remote-only operation shall re-probe and durably record the
  advertised head on each retry before fetching. If the branch advances between
  probe and fetch, that attempt shall stop safely and the next retry shall probe
  again. An available local branch shall retain its recorded head.
  Confirmed loss shall retain the existing explicit `resume_new_branch`
  authorization; automatic recovery shall never replace it with a base branch.

## Out of scope

- Recovery of lost Git objects, detached-HEAD history, or deleted uncommitted files.
- Automatic recovery inside a remote executor.
- Automatic removal of retained recovery artifacts.
- New UI controls, WebSocket actions, or provider conversation identities.

## Related contracts

- [Additional-session workspace reuse](additional-session-workspace-reuse.md)
- [Detached workspace continuity](detached-workspace-continuity.md)
- [Tasks without repositories](without-repositories.md)
- [Agent resume recovery](../../agents/requirements/agent-resume-runtime-recovery.md)
- [System design](../system-design/worktree-metadata-recovery.md)
- [Metadata implementation plan](../../../plans/worktree-metadata-recovery/plan.md)
- [Missing-checkout fix package](../../../plans/missing-worktree-checkout-recovery/plan.md)
- [Main-checkout compatibility fix](../../../plans/main-checkout-recovery-admission/plan.md) tracks criteria 002.8 and 002.9.
