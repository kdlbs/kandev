---
status: draft
system: tasks
created: 2026-08-19
updated: 2026-08-30
owners:
  - kandev
---

# Additional Session Workspace Reuse Requirements

## Overview

Additional sessions must use the task's existing workspace. They must not
materialize a second worktree or change the files that the first session owns.

## Requirements

### REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001: Additional Session Workspace Reuse

**Intent:** Let additional sessions attach to a validated task workspace while
preserving independent session runtime state.

#### Acceptance criteria

- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1:** An additional session
  shall attach only to a ready canonical environment with a complete,
  validated repository inventory.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2:** Attach-only preparation
  shall not create, recreate, clone, fetch, pull, checkout, reset, or otherwise
  modify the shared workspace.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3:** Each attached session
  shall receive independent execution identity and runtime state.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4:** Unsafe or unsupported
  reuse shall fail with a typed, recoverable API error without creating a
  session or replacement workspace.

### REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002: Canonical Workspace Identity Continuity

**Intent:** Keep the task's effective workspace identity stable when Kandev
reconstructs or resumes its runtime.

#### Acceptance criteria

- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1:** When Kandev recovers
  or resumes a task with a ready canonical environment, the persisted and
  projected workspace path shall remain the materialized workspace used by the
  recovered runtime.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.2:** When the task reloads
  after recovery, Files and later attached sessions shall resolve the same
  canonical workspace; the repository's source checkout shall not replace it.
- **AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.3:** When no materialized
  or recovered runtime workspace exists, a legacy repository-backed session
  can continue to use its source checkout as a compatibility fallback.

## Migrated source detail

## Why

An additional session belongs to an existing task workspace. Treating it as a
new worktree materialization can collide with the branch already checked out by
the task and can hide or modify uncommitted work from the new agent.

## What

- `task_environments` owns the workspace. `task_environment_repos` is the sole
  physical worktree inventory; sessions only reference `task_environment_id`.
- The first eligible launch may materialize a workspace. Every later session
  attaches to the ready canonical environment and its exact repository/branch
  inventory.
- Attach-only preparation does not create, recreate, clone, fetch, pull,
  checkout, reset, run repository setup, configure contribution state, or copy
  files. It preserves both tracked and untracked working-tree changes.
- Multiple agents may write the shared workspace concurrently. This does not
  imply a task-wide writer lock or a filesystem read-only capability.
- A session name, review state, or reviewer profile does not grant a special
  filesystem mode. Workspace-only restore/view may attach without launching an
  additional mutating agent.

## Workspace state matrix

| Mode | Canonical environment | Result |
| --- | --- | --- |
| `new_workspace` | none | Elect one initial materializer. |
| `new_workspace` | `creating` | Return recoverable `workspace_preparing`; do not create a session or workspace. |
| `new_workspace` | ready and complete | Attach with `reuse_required`. |
| `inherit_parent` | parent/group ready and complete | Attach to that environment. |
| `inherit_parent` | missing or unsafe | Return a reuse error; never create a child environment. |
| `shared_group` | none | Atomically elect one group materializer. |
| `shared_group` | creating | Return recoverable `workspace_preparing`. |
| `shared_group` | ready and complete | Attach to the group environment. |

An incomplete, failed, deleted, path-mismatched, branch-mismatched, or
duplicate repository slot is `workspace_reuse_unsafe`. An executor that cannot
create an independent session runtime against the validated environment returns
`workspace_reuse_unsupported`.

## Executor boundary

The environment contributes only stable workspace handles. A new session always
gets independent execution identity: agentctl/process and ACP state for host
execution; agentctl instance for Docker; session directory, PID, port and
forward for SSH; and corresponding per-session runtime state for Sprites.
Sibling `PreviousExecutionID` data is never a workspace identity.

## API failure contract

`spawn_session_kandev` maps typed workspace state errors to `CONFLICT` with a
stable `reason`, `recoverable` boolean, and bounded retry guidance. It never
includes a path, branch, secret, token, or executor credential in those
details. Its successful response shape and profile precedence do not change;
an optional session name remains best effort after a successful launch.

## Scenarios

- A named or unnamed additional session sees the same uncommitted tracked and
  untracked files as the task's first session.
- Git worktree inventory, HEAD, index, branch and status remain unchanged by
  an additional launch.
- A terminal primary or zero-session task can reuse its retained ready
  environment.
- A preparing or unsafe environment fails before lifecycle preparation and does
  not silently repair or replace the workspace.
- Every current repository/branch slot must have exactly one active canonical
  row before an attach-only launch begins.

## Out of scope

- Preventing concurrent agents from editing the same file.
- A trusted filesystem read-only agent mode.
- Automatic workspace repair, reset, branch switching, or replacement during
  session spawn.
- Reconstructing a missing physical worktree from filesystem guesses.
