---
status: active
system: tasks
specification_version: 1
migration: in_progress
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
  [agent system](../agents).
- Repository and worktree ownership belongs to the
  [workspace system](../workspaces).
- Office-specific autonomous agent identities and dashboards belong to the
  [Office system](../office).
- Presentation-only behavior belongs to the [UI specifications](../ui).

## Specification map

### Requirements

- [Agent-Generated Task Titles](requirements/agent-generated-titles.md)
- [Additional Session Workspace Reuse](requirements/additional-session-workspace-reuse.md)
- [Task Archive Confirmation](requirements/archive-confirmation.md)
- [Attach Workspace Sources](requirements/attach-workspace-sources.md)
- [Task Autopilot Mode](requirements/autopilot-mode.md)
- [Blocked Task Escalation](requirements/blocked-task-escalation.md)
- [Active clarification lifecycle scenarios](requirements/clarification-active-lifecycle-scenarios.md)
- [Active clarification lifecycle](requirements/clarification-active-lifecycle.md)
- [Task Documents](requirements/documents.md)
- [Task Execution Stages](requirements/execution-stages.md)
- [External task ID idempotency boundaries](requirements/external-id-idempotency-boundaries.md)
- [External task ID idempotency scenarios](requirements/external-id-idempotency-scenarios.md)
- [External task ID idempotency](requirements/external-id-idempotency.md)
- [Interrupted Task Indicator](requirements/interrupted-task-indicator.md)
- [Kanban task cache preserves executor fields across merges](requirements/kanban-task-executor-cache-staleness.md)
- [Task Labels](requirements/labels.md)
- [Link Existing Task to External References](requirements/link-existing-task-github-issue.md)
- [MCP-Created Task Agent Profile Default](requirements/mcp-task-agent-profile-default.md)
- [MCP Tool Name Stability](requirements/mcp-tool-name-stability.md)
- [Missing task route recovery](requirements/missing-task-route-recovery.md)
- [Task model unification](requirements/model-unification.md)
- [Multi-branch tasks](requirements/multi-branch.md)
- [Parent-Child Message Interrupt](requirements/parent-child-message-interrupt.md)
- [Parent-Child Task Stop](requirements/parent-child-task-stop.md)
- [Passthrough Queued Prompt Dispatch](requirements/passthrough-queued-prompt-dispatch.md)
- [Task plan write consistency](requirements/plan-write-consistency.md)
- [Passthrough Initial Prompt Turn Boundary](requirements/passthrough-initial-prompt-turn-boundary.md)
- [Prevent Agent Auto-Start On Open](requirements/prevent-agent-autostart-on-open.md)
- [Prompt attachments](requirements/prompt-attachments.md)
- [Saved Prompt Delivery](requirements/saved-prompt-delivery.md)
- [Quick Chat Sessions, Persistence, and Expiration](requirements/quick-chat-expiration.md)
- [Quick Chat Agent Titles](requirements/quick-chat-agent-titles.md)
- [Quick Chat Repository Context](requirements/quick-chat-repository-context.md)
- [Remote Contribution Tasks](requirements/remote-contribution-tasks.md)
- [Rich task title previews](requirements/rich-task-title-previews.md)
- [Queued run scheduling](requirements/run-scheduling.md)
- [Task Runtime Cleanup](requirements/runtime-cleanup.md)
- [Runtime Task-State Publication Order](requirements/runtime-state-publication-order.md)
- [Session Delete Preserves Task Workspaces](requirements/session-delete-resource-cleanup.md)
- [Sidebar Task Editing](requirements/sidebar-task-edit.md)
- [Subtasks as Workflow Checklist](requirements/subtask-checklist.md)
- [Subtask Completion Trigger](requirements/subtask-completion-trigger.md)
- [Subtask detachment](requirements/subtask-detachment.md)
- [Subtask re-parenting by drag and drop](requirements/subtask-reparenting-drag-drop.md)
- [Task Subtree Controls](requirements/subtree-controls.md)
- [Create Task Escape Dismissal](requirements/task-create-escape-dismissal.md)
- [Task Create Executor Default](requirements/task-create-executor-default.md)
- [Task Create Workflow Memory](requirements/task-create-workflow-memory.md)
- [Task-create advanced settings disclosure](requirements/task-dependencies-create-dialog-advanced-settings.md)
- [Task-create dependency selector refinement](requirements/task-dependencies-create-dialog-dependency-selector.md)
- [Task Dependencies and Auto-Start Chains](requirements/task-dependencies.md)
- [Task Launch Failure Recovery](requirements/task-launch-failure-recovery.md)
- [Task Title Length Limit](requirements/title-length-limit.md)
- [User Question Turn Boundary](requirements/user-question-turn-boundary.md)
- [WIP Limits and Visible Overflow Queues](requirements/wip-limit-pull-system.md)
- [Tasks Without Repositories](requirements/without-repositories.md)
- [Cancelled Turn Completion](requirements/workflow-cancelled-turn-completion.md)
- [Workflow Cycle Guardrails](requirements/workflow-cycle-guardrails.md)
- [Workflow Duplication](requirements/workflow-duplication.md)
- [Explicit Workflow-Step Completion Signal](requirements/workflow-explicit-completion-signal.md)
- [Workflow Profile Session Lifecycle](requirements/workflow-profile-session-lifecycle.md)
- [Agent decision recording](requirements/workflow-quorum-decision-recording-agent-surface.md)
- [Quorum ordering and concurrency](requirements/workflow-quorum-decision-recording-concurrency.md)
- [Quorum diagnostics](requirements/workflow-quorum-decision-recording-diagnostics.md)
- [Quorum participant slate](requirements/workflow-quorum-decision-recording-participant-slate.md)
- [Decision-triggered quorum re-evaluation](requirements/workflow-quorum-decision-recording-reevaluation.md)
- [Quorum regression behavior](requirements/workflow-quorum-decision-recording-regression.md)
- [Quorum step binding and thresholds](requirements/workflow-quorum-decision-recording-step-binding.md)
- [Quorum verdict vocabulary](requirements/workflow-quorum-decision-recording-verdict-vocabulary.md)
- [Workflow quorum decision recording](requirements/workflow-quorum-decision-recording.md)
- [Conditional Workflow Session Settings](requirements/workflow-session-settings.md)
- [Workflow Settings Autosave](requirements/workflow-settings-autosave.md)
- [Workflow Step Agent Start Ownership](requirements/workflow-step-agent-start-ownership.md)
- [Workflow Sync — Per-User Workspace Authorization](requirements/workflow-sync-workspace-authz.md)
- [Workflow task-step transition ledger scenarios](requirements/workflow-task-step-transition-ledger-scenarios.md)
- [Workflow task-step transition ledger](requirements/workflow-task-step-transition-ledger.md)

### System design

- [Additional Session Workspace Reuse](system-design/additional-session-workspace-reuse.md)
- [Environment-Owned Git Status](system-design/environment-owned-git-status.md)
- [Attach Workspace Sources](system-design/attach-workspace-sources.md)
- [Quick Chat Agent Titles](system-design/quick-chat-agent-titles.md)
- [Quick Chat Session Resumption](system-design/quick-chat-session-resumption.md)
- [Active clarification lifecycle](system-design/clarification-active-lifecycle.md)
- [External task ID idempotency operations](system-design/external-id-idempotency-operations.md)
- [External task ID idempotency](system-design/external-id-idempotency.md)
- [Task model unification](system-design/model-unification.md)
- [MCP Tool Name Stability](system-design/mcp-tool-name-stability.md)
- [Remote Contribution Tasks](system-design/remote-contribution-tasks.md)
- [Passthrough Queued Prompt Dispatch](system-design/passthrough-queued-prompt-dispatch.md)
- [Saved Prompt Delivery](system-design/saved-prompt-delivery.md)
- [Passthrough Initial Prompt Turn Boundary](system-design/passthrough-initial-prompt-turn-boundary.md)
- [Prompt attachments](system-design/prompt-attachments.md)
- [Task Archive Confirmation](system-design/archive-confirmation.md)
- [Task plan write consistency](system-design/plan-write-consistency.md)
- [Task plan write lifecycle](system-design/plan-write-lifecycle.md)
- [Task Runtime Cleanup](system-design/runtime-cleanup.md)
- [Runtime Task-State Publication Order](system-design/runtime-state-publication-order.md)
- [Queued Run Scheduling](system-design/run-scheduling.md)
- [Session Delete Preserves Task Workspaces](system-design/session-delete-resource-cleanup.md)
- [Task Dependencies and Auto-Start Chains](system-design/task-dependencies.md)
- [Task Launch Failure Recovery](system-design/task-launch-failure-recovery.md)
- [WIP Limits and Visible Overflow Queues](system-design/wip-limit-pull-system.md)
- [Workflow quorum decision recording](system-design/workflow-quorum-decision-recording.md)
- [Workflow Step Agent Start Ownership](system-design/workflow-step-agent-start-ownership.md)
- [Workflow Step Fixed-Profile Routing](system-design/workflow-step-fixed-profile-routing.md)
- [Workflow Profile Session Lifecycle](system-design/workflow-profile-session-lifecycle.md)
- [Workflow task-step transition ledger](system-design/workflow-task-step-transition-ledger.md)

## Migration record

Migration remains in progress. The six requirements above now have
authoritative, wrapper-free requirement/design pairs. Other migrated files still
need the same extraction before this system can return to a complete migration
state.

## Related systems

- [Agents](../agents): supplies agent identity and execution profiles.
- [Office](../office): builds autonomous workflows on task primitives.
- [UI](../ui): owns presentation-specific task surfaces.
- [Workspaces](../workspaces): owns repositories and task worktrees.
