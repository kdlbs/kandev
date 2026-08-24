---
status: active
system: tasks
specification_version: 1
migration: complete
owners:
  - kandev
---

# Task and workflow system

## Purpose

The task and workflow system owns durable work items, task relationships,
execution lifecycle, workflow steps, queued launches, and the contracts that
move a task through its configured process.

## Ownership

This system owns task identity and metadata, task documents and attachments,
parent and dependency relationships, task creation and launch behavior, task
runtime state publication, workflow definitions and transitions, completion
signals, and task-scoped scheduling contracts.

## Exclusions

- Agent identity, permissions, and runtime profiles belong to the
  [agent system](../agents/).
- Repository and worktree ownership belongs to the
  [workspace system](../workspaces/).
- Office-specific autonomous agent identities and dashboards belong to the
  [Office system](../office/).
- Presentation-only behavior belongs to the [UI specifications](../ui/).

## Specification map

### Requirements

- [Active clarification lifecycle](requirements/clarification-active-lifecycle.md)
- [Active clarification lifecycle scenarios](requirements/clarification-active-lifecycle-scenarios.md)
- [Agent-generated titles](requirements/agent-generated-titles.md)
- [Additional session workspace reuse](requirements/additional-session-workspace-reuse.md)
- [Attach workspace sources](requirements/attach-workspace-sources.md)
- [Archive confirmation](requirements/archive-confirmation.md)
- [Autopilot mode](requirements/autopilot-mode.md)
- [Blocked task escalation](requirements/blocked-task-escalation.md)
- [Cancelled turn completion](requirements/workflow-cancelled-turn-completion.md)
- [Create dialog advanced dependency settings](requirements/task-dependencies-create-dialog-advanced-settings.md)
- [Create dialog dependency selector](requirements/task-dependencies-create-dialog-dependency-selector.md)
- [Documents](requirements/documents.md)
- [Execution stages](requirements/execution-stages.md)
- [External task ID idempotency](requirements/external-id-idempotency.md)
- [External task ID idempotency boundaries](requirements/external-id-idempotency-boundaries.md)
- [External task ID idempotency scenarios](requirements/external-id-idempotency-scenarios.md)
- [Explicit workflow-step completion signal](requirements/workflow-explicit-completion-signal.md)
- [Interrupted task indicator](requirements/interrupted-task-indicator.md)
- [Labels](requirements/labels.md)
- [Link an existing task to a GitHub issue](requirements/link-existing-task-github-issue.md)
- [MCP task agent-profile default](requirements/mcp-task-agent-profile-default.md)
- [Model unification](requirements/model-unification.md)
- [Multi-branch tasks](requirements/multi-branch.md)
- [Parent-child message interrupt](requirements/parent-child-message-interrupt.md)
- [Parent-child task stop](requirements/parent-child-task-stop.md)
- [Prevent agent autostart on open](requirements/prevent-agent-autostart-on-open.md)
- [Prompt attachments](requirements/prompt-attachments.md)
- [Quick chat expiration](requirements/quick-chat-expiration.md)
- [Quick chat repository context](requirements/quick-chat-repository-context.md)
- [Remote contribution tasks](requirements/remote-contribution-tasks.md)
- [Rich task title previews](requirements/rich-task-title-previews.md)
- [Run scheduling](requirements/run-scheduling.md)
- [Runtime state publication order](requirements/runtime-state-publication-order.md)
- [Sidebar task edit](requirements/sidebar-task-edit.md)
- [Subtask checklist](requirements/subtask-checklist.md)
- [Subtask completion trigger](requirements/subtask-completion-trigger.md)
- [Subtask detachment](requirements/subtask-detachment.md)
- [Subtask reparenting](requirements/subtask-reparenting-drag-drop.md)
- [Subtree controls](requirements/subtree-controls.md)
- [Task create escape dismissal](requirements/task-create-escape-dismissal.md)
- [Task create executor default](requirements/task-create-executor-default.md)
- [Task create workflow memory](requirements/task-create-workflow-memory.md)
- [Task dependencies](requirements/task-dependencies.md)
- [Task launch failure recovery](requirements/task-launch-failure-recovery.md)
- [Task title length limit](requirements/title-length-limit.md)
- [User question turn boundary](requirements/user-question-turn-boundary.md)
- [Without repositories](requirements/without-repositories.md)
- [Runtime cleanup](requirements/runtime-cleanup.md)
- [Workflow cycle guardrails](requirements/workflow-cycle-guardrails.md)
- [Workflow duplication](requirements/workflow-duplication.md)
- [Workflow quorum decision recording](requirements/workflow-quorum-decision-recording.md)
- [Workflow quorum decision recording: agent surface](requirements/workflow-quorum-decision-recording-agent-surface.md)
- [Workflow quorum decision recording: concurrency](requirements/workflow-quorum-decision-recording-concurrency.md)
- [Workflow quorum decision recording: diagnostics](requirements/workflow-quorum-decision-recording-diagnostics.md)
- [Workflow quorum decision recording: participant slate](requirements/workflow-quorum-decision-recording-participant-slate.md)
- [Workflow quorum decision recording: re-evaluation](requirements/workflow-quorum-decision-recording-reevaluation.md)
- [Workflow quorum decision recording: regression](requirements/workflow-quorum-decision-recording-regression.md)
- [Workflow quorum decision recording: step binding](requirements/workflow-quorum-decision-recording-step-binding.md)
- [Workflow quorum decision recording: verdict vocabulary](requirements/workflow-quorum-decision-recording-verdict-vocabulary.md)
- [Workflow session settings](requirements/workflow-session-settings.md)
- [Workflow settings autosave](requirements/workflow-settings-autosave.md)
- [Workflow step agent-start ownership](requirements/workflow-step-agent-start-ownership.md)
- [Workflow sync workspace authorization](requirements/workflow-sync-workspace-authz.md)
- [Workflow task-step transition ledger](requirements/workflow-task-step-transition-ledger.md)
- [Workflow task-step transition ledger scenarios](requirements/workflow-task-step-transition-ledger-scenarios.md)
- [WIP limits and visible overflow queues](requirements/wip-limit-pull-system.md)

### System design

- [Active clarification lifecycle](system-design/clarification-active-lifecycle.md)
- [Attach workspace sources](system-design/attach-workspace-sources.md)
- [External task ID idempotency](system-design/external-id-idempotency.md)
- [External task ID idempotency operations](system-design/external-id-idempotency-operations.md)
- [Model unification](system-design/model-unification.md)
- [Remote contribution tasks](system-design/remote-contribution-tasks.md)
- [Runtime cleanup](system-design/runtime-cleanup.md)
- [Task dependencies](system-design/task-dependencies.md)
- [Workflow quorum decision recording](system-design/workflow-quorum-decision-recording.md)
- [Workflow task-step transition ledger](system-design/workflow-task-step-transition-ledger.md)
- [WIP limit and pull system](system-design/wip-limit-pull-system.md)

## Migration status

All task and workflow sources in this migration are now represented by
authoritative requirement and system-design documents under this directory.
The legacy editable sources were split by capability, lifecycle, and contract
boundary, then removed from the legacy tree.

## Related systems

- [Agents](../agents/): supplies agent identity and execution profiles.
- [Office](../office/): builds autonomous workflows on task primitives.
- [UI](../ui/): owns presentation-specific task surfaces.
- [Workspaces](../workspaces/): owns repositories and task worktrees.
