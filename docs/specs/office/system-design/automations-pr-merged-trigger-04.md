---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001
created: 2026-08-09
updated: 2026-08-09
owners:
  - nova28
---
# Automations — "Pull request merged" trigger System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Permissions

- Creating, editing, enabling and firing an automation carrying this trigger is
  workspace-scoped exactly as every other automation is. No new permission surface.
- The run task's agent reaches `archive_task_kandev` over in-session MCP, which is scoped
  to the owner of the *run task's* workspace. The run task and the target task are in the
  same workspace by **gate 8**, and that gate compares the automation's workspace against
  the **task lookup's** workspace rather than the payload's — which is what makes this
  argument hold. Were the gate to trust `TaskPR.WorkspaceID`, a stale row could satisfy it
  while the task actually lived elsewhere, and the claim below would be false. See
  [Task lookup](#task-lookup).
- In an unowned workspace (pre-auth rows), in-session MCP scopes to the unowned sentinel,
  which reaches unowned rows only. The target task is in the same unowned workspace, so
  it remains reachable.
- The existing owner authorization remains necessary but is no longer the only bound.
  `handleArchiveTask` receives the current MCP run-task id as server-injected context. For a
  `github_pr_merged` automation run, it loads that caller and requires the requested target
  to equal the persisted event target before calling `ArchiveTask`. The agent cannot choose
  or override the caller id through the tool schema.
- What gate 8 does and does not buy: it guarantees the **intended** target is reachable **at
  the moment the trigger fires**, so the archive the feature is for cannot fail on
  authorization for any reason present at that moment. It is a point-in-time check, not a
  standing guarantee — the archive itself happens minutes later inside the agent's turn, and
  `tasks.workspace_id` is writable, so a target moved to another workspace (or a workspace
  whose owner changes) in that window can still be denied. [Failure modes](#failure-modes)
  carries the row. The generic tool remains owner-scoped for other callers, but this run's
  target-binding check narrows the mutation to the event-selected task.
- A missing binding, a malformed target value, or a requested id mismatch fails closed and
  archives nothing. Other task sessions and other trigger types keep the existing generic
  owner-scoped archive behavior.
- The trigger never archives anything itself. Every archive in this flow is an agent tool
  call, audited as such.
