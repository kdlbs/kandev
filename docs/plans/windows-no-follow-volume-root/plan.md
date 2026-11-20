---
created: 2026-11-19
status: done
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
legacy_specs: []
---

# Implementation plan: Windows no-follow opens below the volume root

## Overview

On Windows every no-follow directory open of an absolute path fails with
`STATUS_REPARSE_POINT_ENCOUNTERED` ("The object manager encountered a reparse
point while retrieving an object"), even for an ordinary directory. The open
passes the whole NT path (`\??\C:\...`) to one `NtCreateFile` with
`OBJ_DONT_REPARSE`, and `\??\C:` is itself an object-manager link to the volume
device. Worktree admission therefore reports `worktree directory is corrupted`
for healthy checkouts, so resume, session switching, and workspace restoration
fail, and dependency cleanup and managed npm cache repair cannot open their
roots. This package resolves the volume root once and walks every component
below it without following links, as defined by
`AC-TASKS-WORKTREE-METADATA-RECOVERY-002.10` and
`AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.6`.

## Scope

In scope: a shared `internal/common/winpath` leaf package; the storage
workspace no-follow open, create, and remove paths; the managed runtime cache
root, execution-tree inventory, and repair-lock configuration opens; Windows
tests with junctions, which need no symbolic-link privilege; and a Git shim
that Windows can resolve in the main-checkout inspection tests, which this fix
lets reach Git inspection for the first time.

Out of scope: non-Windows filesystem operations, the share-mode contract for DELETE opens,
Git shims in the branch-cleanup interruption tests, and any change to what
admission accepts.

## Work orders

- [x] [Task 01: Walk Windows no-follow opens from the volume root](task-01-volume-root-walk.md)

One work order: one shared open primitive and its call sites, with one
Windows verification boundary.

## Verification strategy

The untagged Windows path parser is tested on every platform. The Windows
test allowlist includes `internal/common/winpath`; tests in
`internal/system/storage/workspaces` and `internal/agent/managedruntime`
exercise the native handle boundary, plus the existing worktree and inventory
repair packages that consume it. Junction tests prove that a link at, above,
or below the task root is still refused.

## Risks

- The volume root open follows the object-manager link. That link belongs to
  the mount manager, not to repository or user content, and every component
  below it keeps the no-reparse open and the explicit attribute check.
- Ancestors are opened with traverse access instead of generic read. That
  narrows the request, so a folder that allows traversal but not listing no
  longer blocks the walk.

## References

- [Worktree recovery requirement](../../specs/tasks/requirements/worktree-metadata-recovery.md)
- [Worktree recovery design](../../specs/tasks/system-design/worktree-metadata-recovery.md)
- [Managed npm runtime recovery requirement](../../specs/agents/requirements/managed-npm-runtime-recovery.md)
- [Managed npm runtime recovery design](../../specs/agents/system-design/managed-npm-runtime-recovery.md)
