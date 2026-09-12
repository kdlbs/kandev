---
status: active
system: tasks
created: 2026-09-10
owners:
  - kandev
---

# Worktree metadata recovery requirements

## Overview

A linked worktree can retain its files after it loses its Git administrative
directory. Recovery preserves those files without changing an unrelated workspace.
The task system owns this contract because the task environment owns the physical
worktree inventory and its launch lifecycle.

This document defines the compatibility contract implemented for PR #3137.
Runtime verification status is recorded in the linked implementation plan.

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
  path and branch. A missing checkout must retain the normal materialization path.
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

## Out of scope

- Recovery of lost Git objects, detached-HEAD history, or a deleted checkout.
- Automatic recovery inside a remote executor.
- Automatic removal of retained recovery artifacts.
- New UI controls, WebSocket actions, or provider conversation identities.

## Related contracts

- [Additional-session workspace reuse](additional-session-workspace-reuse.md)
- [Detached workspace continuity](detached-workspace-continuity.md)
- [Tasks without repositories](without-repositories.md)
- [Agent resume recovery](../../agents/requirements/agent-resume-runtime-recovery.md)
- [System design](../system-design/worktree-metadata-recovery.md)
- [Implementation plan](../../../plans/worktree-metadata-recovery/plan.md)
