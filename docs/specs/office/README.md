---
status: draft
system: office
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Office system

## Purpose

The Office system owns autonomous agent workspaces, coordination, scheduling,
automation runs, dashboards, inboxes, and Office-specific live state.

## Ownership

This system owns Office agents and roles, autonomous task assignment,
scheduler and wakeup policy, Office routing, automation runs, inbox activity,
dashboard projections, and Office testing contracts.

## Exclusions

- Durable task and workflow primitives belong to the [task system](../tasks/README.md).
- Shared agent profiles and permissions belong to the [agent system](../agents/README.md).
- External provider connections belong to the [integration system](../integrations/README.md).

## Specification map

### Requirements



- [Office: Agent Comment Reads](requirements/agent-comment-reads.md)
- [Office: Agents](requirements/agents.md)
- [Office: Personal Assistant Agent, Channels & Agent Memory](requirements/assistant.md)
- [Automation runs — status-scoped delete all](requirements/automation-runs-delete-all-by-status.md)
- [Automation Runs](requirements/automation-runs.md)
- [Automations — "Pull request merged" trigger](requirements/automations-pr-merged-trigger.md)
- [Automations in Settings](requirements/automations-settings.md)
- [Automation Continuity](requirements/automation-continuity.md)
- [Automation Target Modes](requirements/automation-target-modes.md)
- [Automations YAML Export](requirements/automations-yaml-export.md)
- [Office: Cost Tracking & Budget Management](requirements/costs.md)
- [Office Dashboard](requirements/dashboard.md)
- [Office: Inbox, Approvals & Activity Log](requirements/inbox.md)
- [Office Live Updates](requirements/live-updates.md)
- [Office per-agent and per-role tier selection](requirements/office-agent-tier-routing.md)
- [Office: Overview](requirements/overview.md)
- [Office Provider Routing](requirements/routing.md)
- [Office Review Participant Seats](requirements/review-participant-seats.md)
- [Office Agent Runtime — Error Handling Contract](requirements/runtime.md)
- [Office Scheduler](requirements/scheduler.md)
- [Office Stall Visibility](requirements/stall-visibility.md)
- [Step Entry Sequence Execution](requirements/step-entry-sequence-execution.md)
- [Office Tasks](requirements/tasks.md)
- [Office: E2E Mock Harness for Task Sessions and Messages](requirements/testing.md)
- [Office: Slack-Style Unread Divider](requirements/unread-divider.md)

### System design



- [Office: Agent Comment Reads System Design](system-design/agent-comment-reads-01.md)
- [Office: Agents System Design Part 1](system-design/agents-01.md)
- [Office: Agents System Design Part 2](system-design/agents-02.md)
- [Office: Agents System Design Part 3](system-design/agents-03.md)
- [Office: Personal Assistant Agent, Channels & Agent Memory](system-design/assistant.md)
- [Automation Runs](system-design/automation-runs.md)
- [Automation Target Modes](system-design/automation-target-modes.md)
- [Automations — "Pull request merged" trigger System Design Part 1](system-design/automations-pr-merged-trigger-01.md)
- [Automations — "Pull request merged" trigger System Design Part 2](system-design/automations-pr-merged-trigger-02.md)
- [Automations — "Pull request merged" trigger System Design Part 3](system-design/automations-pr-merged-trigger-03.md)
- [Automations — "Pull request merged" trigger System Design Part 4](system-design/automations-pr-merged-trigger-04.md)
- [Automations — "Pull request merged" trigger System Design Part 5](system-design/automations-pr-merged-trigger-05.md)
- [Automations — "Pull request merged" trigger System Design Part 6](system-design/automations-pr-merged-trigger-06.md)
- [Automations — "Pull request merged" trigger System Design Part 7](system-design/automations-pr-merged-trigger-07.md)
- [Automations — "Pull request merged" trigger System Design Part 8](system-design/automations-pr-merged-trigger-08.md)
- [Automations in Settings System Design Part 1](system-design/automations-settings-01.md)
- [Automations in Settings System Design Part 2](system-design/automations-settings-02.md)
- [Automations YAML Export System Design Part 1](system-design/automations-yaml-export-01.md)
- [Automations YAML Export System Design Part 2](system-design/automations-yaml-export-02.md)
- [Automations YAML Export System Design Part 3](system-design/automations-yaml-export-03.md)
- [Automations YAML Export System Design Part 4](system-design/automations-yaml-export-04.md)
- [Automations YAML Export System Design Part 5](system-design/automations-yaml-export-05.md)
- [Automations YAML Export System Design Part 6](system-design/automations-yaml-export-06.md)
- [Office: Cost Tracking & Budget Management System Design Part 1](system-design/costs-01.md)
- [Office: Cost Tracking & Budget Management System Design Part 2](system-design/costs-02.md)
- [Office Live Updates System Design Part 1](system-design/live-updates-01.md)
- [Office Live Updates System Design Part 2](system-design/live-updates-02.md)
- [Office per-agent and per-role tier selection System Design Part 1](system-design/office-agent-tier-routing-01.md)
- [Office per-agent and per-role tier selection System Design Part 2](system-design/office-agent-tier-routing-02.md)
- [Office per-agent and per-role tier selection System Design Part 3](system-design/office-agent-tier-routing-03.md)
- [Office: Overview System Design Part 1](system-design/overview-01.md)
- [Office: Overview System Design Part 2](system-design/overview-02.md)
- [Office Review Participant Seats System Design Part 1](system-design/review-participant-seats-01.md)
- [Office Provider Routing System Design Part 1](system-design/routing-01.md)
- [Office Provider Routing System Design Part 2](system-design/routing-02.md)
- [Office Agent Runtime — Error Handling Contract System Design Part 1](system-design/runtime-01.md)
- [Office Agent Runtime — Error Handling Contract System Design Part 2](system-design/runtime-02.md)
- [Office Scheduler System Design Part 1](system-design/scheduler-01.md)
- [Office Stall Visibility System Design](system-design/stall-visibility.md)
- [Step Entry Sequence Execution System Design](system-design/step-entry-sequence-execution.md)
- [Office Scheduler System Design Part 2](system-design/scheduler-02.md)
- [Office Tasks System Design Part 1](system-design/tasks-01.md)
- [Office Tasks System Design Part 2](system-design/tasks-02.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Tasks](../tasks/README.md): supplies durable work and workflow primitives.
- [Agents](../agents/README.md): supplies agent profiles and permission policy.
- [Integrations](../integrations/README.md): supplies provider connections.
