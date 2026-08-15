---
status: shipped
created: 2026-08-15
owner: kandev
---

# Task PR legacy workspace authorization

## Why

Legacy `github_task_prs` associations can have an empty `workspace_id`. A user
who knows one of those association IDs can currently name any workspace they
own when detaching the association or recording its disposition, even when the
owning task belongs to a different workspace.

This repair covers [GitHub issue #2615](https://github.com/kdlbs/kandev/issues/2615)
and preserves the workspace ownership rules in the authentication spec and
[ADR 0047](../../decisions/0047-github-authentication-ownership.md).

## What

- Task-PR mutation endpoints SHALL authorize the association's persisted
  workspace when `github_task_prs.workspace_id` is present.
- When an association has an empty `workspace_id`, the system SHALL derive its
  workspace from the association's owning task.
- The caller-supplied `workspace_id` SHALL match the persisted or derived
  workspace before the mutation runs.
- `DetachTaskPR` and `SetTaskPRDisposition` SHALL use the same authorization
  behavior.
- Authorization hardening SHALL NOT backfill or otherwise modify legacy
  `github_task_prs.workspace_id` values.
- Mutation events for legacy associations SHALL carry the derived workspace
  for routing while leaving the stored association workspace blank.

## Permissions

A caller can detach a task-PR association or set its disposition only when the
caller can access the association's persisted or task-derived workspace. Access
to a different workspace owned by the same caller does not grant access to the
association.

## Failure modes

- If the association is absent, belongs to another workspace, or has an empty
  `workspace_id` whose task or task workspace cannot be resolved, the endpoint
  returns the existing task-PR not-found response and performs no mutation.
- If task lookup fails for an operational reason other than task absence, the
  endpoint surfaces its existing internal-error response and performs no
  mutation.
- Authorization failure occurs before detach, disposition, metric, or event
  side effects.

## Scenarios

- **GIVEN** an association with workspace `ws-a`, **WHEN** a caller authorized
  for `ws-a` mutates it using `workspace_id=ws-a`, **THEN** the mutation succeeds
  with its existing response and event behavior.
- **GIVEN** an association with workspace `ws-a`, **WHEN** a caller mutates it
  using `workspace_id=ws-b`, **THEN** the endpoint returns not found and the row
  remains unchanged.
- **GIVEN** a legacy association with an empty workspace whose task belongs to
  `ws-a`, **WHEN** a caller authorized for `ws-a` mutates it using
  `workspace_id=ws-a`, **THEN** the mutation succeeds without backfilling the
  association workspace.
- **GIVEN** the same legacy association, **WHEN** a caller authorized for
  `ws-b` mutates it using `workspace_id=ws-b`, **THEN** the endpoint returns not
  found, no workspace authorization is granted from the supplied ID, and the
  row remains unchanged.
- **GIVEN** a legacy association whose owning task is absent or has no
  workspace, **WHEN** any caller tries to mutate it, **THEN** the endpoint
  returns not found and the row remains unchanged.

Each scenario applies to both detach and disposition mutations.

## Out of scope

- Backfilling or migrating legacy association rows.
- Changing task-PR read endpoints, discovery, synchronization, or credential
  selection.
- Changing the request or response shape of either mutation endpoint.
- Shipping the disposition endpoint itself, which remains owned by PR #2614.
