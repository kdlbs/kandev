---
created: 2026-09-21
status: implemented
requirements:
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
system_design:
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
legacy_specs: []
---

# Implementation Plan: Inherit-Parent Repository Admission

## Overview

Reject an MCP subtask before creation when it requests `inherit_parent` while
its explicit repository selection cannot reuse the parent's already
materialized repository inventory. The fix adds read-only task-service
validation, invokes it before creation side effects, and returns guidance to use
`new_workspace` for a different checkout.

## Confirmed root cause

MCP subtask creation independently resolves repository inputs and workspace
policy. Explicit repository input overrides the parent's logical task
repositories and fills an omitted branch from the repository default, while an
omitted workspace mode still defaults to `inherit_parent`. The task is inserted
successfully even when the parent's materialized environment has no matching
repository/branch slot. Launch then fails after task and session creation when
canonical inventory admission detects the mismatch.

## Scope

### In scope

- Validate explicit repository inputs and top-level branch overrides for
  `inherit_parent` against an existing parent environment before task creation.
- Require exactly one active canonical inventory match per requested slot.
- Return bounded validation guidance to use `new_workspace`.
- Preserve current behavior when no parent environment is materialized.
- Prove rejection creates no task or session.

### Out of scope

- Automatically changing workspace mode.
- Rewriting repository or branch input.
- Repairing already-failed tasks.
- Changing launch-time canonical inventory validation.
- UI changes.

## Technical approach

Add a read-only task-service method that loads the parent's active environment
and validates explicit `TaskRepositoryInput` values against its repository
inventory using the same repository and sanitized branch identity contract as
launch admission. A request is compatible only when each selected slot has one
active, non-deleted inventory row.

Call the validator from MCP `create_task` after repository defaults and
`WorkspacePolicy` are resolved, only for an explicit repository list with
effective `inherit_parent` mode. Run it before remote contribution resolution
and `CreateTask`. Return a validation error that recommends
`workspace_mode=new_workspace` without exposing paths or worktree identifiers.

## Tests

- `AC-TASKS-MCP-WORKSPACE-MODE-004.5`: a materialized parent with only
  feature-branch inventory rejects explicit repository and branch-only
  default-branch child selections before creation.
- A uniquely matching explicit repository/branch slot remains allowed.
- A parent without a materialized environment preserves existing behavior.
- Rejection leaves task and session counts unchanged.

## Work orders

- [x] [Task 01: Validate inherited repository selections](task-01-validate-inherited-repository-selections.md)

## Verification results

- Service coverage passes for rejection, exact-slot acceptance, legacy
  unscoped inventory, and pre-materialization passthrough.
- MCP coverage passes for rejection and existing workspace-mode behavior.
- Specification validation and diff checks pass.

## Risks

- Repository matching must use the same sanitized branch identity semantics as
  launch admission or creation and launch can disagree.
- Validation must distinguish explicit repository input from repositories
  inherited by omission, because only the former can conflict with the caller's
  request.
