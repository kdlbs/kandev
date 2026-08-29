---
status: draft
system: agents
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Agent system

## Purpose

The agent system owns configured agent identities, profiles, roles, permissions,
provider capabilities, and agent-facing runtime contracts.

## Ownership

This system owns agent profile data, role governance, profile-backed utility
agents, provider model options, agent permissions, and the agent capability
surface shared by task and Office consumers.

## Exclusions

- Durable work items and workflow transitions belong to the [task and workflow
  system](../tasks/README.md).
- Autonomous Office identities and dashboards belong to the [Office
  system](../office/README.md).
- Presentation-only behavior belongs to the [UI system](../ui/README.md).

## Specification map

### Requirements

- [Agent Resume and Runtime Recovery](requirements/agent-resume-runtime-recovery.md)
- [Agent Rich Output](requirements/agent-rich-output.md)
- [Agent Stall Recovery](requirements/agent-stall-recovery.md)
- [Collapsible Agent Blocks on the Agents Settings Page](requirements/collapsible-agent-blocks.md)
- [Cursor Subagent Metadata](requirements/cursor-subagent-metadata.md)
- [Dynamic Agent Routing Rollout Blockers](requirements/dynamic-agent-routing-rollout-blockers.md)
- [Dynamic Agent Routing](requirements/dynamic-agent-routing.md)
- [Dynamic Provider Model Options](requirements/dynamic-provider-options.md)
- [External Agent Permission Resolution](requirements/external-permission-resolution.md)
- [Agent Git permission boundary](requirements/git-operations-permission-boundary.md)
- [Agent Creation Governance](requirements/governance.md)
- [Granular Agent Permissions](requirements/granular-permissions.md)
- [Hide Disabled Agent Profiles from Left Panel Navigation](requirements/hide-disabled-profiles-nav.md)
- [Mock-agent slow command duration syntax](requirements/mock-agent-slow-duration.md)
- [Native Code Review](requirements/native-code-review.md)
- [No Silent Model Fallback](requirements/no-silent-model-fallback.md)
- [Copy agent configuration to isolated executors](requirements/portable-agent-configuration.md)
- [Disable an Agent Profile](requirements/profile-disable.md)
- [Agent Profile Recent Use](requirements/profile-recent-use.md)
- [Duplicate an Agent Profile](requirements/profile-duplicate.md)
- [Agent Roles — Security, QA, and DevOps](requirements/roles.md)
- [Managed npm runtime recovery](requirements/managed-npm-runtime-recovery.md)
- [Managed Agent Runtime Versions and Updates](requirements/runtime-updates.md)
- [Simplify the agent settings profile layout](requirements/settings-profile-layout.md)
- [Spawn Session Effective Agent Profile](requirements/spawn-session-effective-profile.md)
- [Subagent context persistence](requirements/subagent-context-persistence.md)
- [Profile-backed Utility Agents](requirements/utility-agent-profiles.md)

### System design

- [Dynamic Agent Routing System Design Part 1](system-design/dynamic-agent-routing-01.md)
- [Dynamic Agent Routing System Design Part 2](system-design/dynamic-agent-routing-02.md)
- [Agent Profile Recent Use](system-design/profile-recent-use.md)
- [No Silent Model Fallback System Design Part 1](system-design/no-silent-model-fallback-01.md)
- [No Silent Model Fallback System Design Part 2](system-design/no-silent-model-fallback-02.md)
- [Managed Agent Runtime Versions and Updates System Design Part 1](system-design/runtime-updates-01.md)
- [Managed Agent Runtime Versions and Updates System Design Part 2](system-design/runtime-updates-02.md)
- [Managed npm runtime recovery](system-design/managed-npm-runtime-recovery.md)
- [Subagent context persistence System Design Part 1](system-design/subagent-context-persistence-01.md)
- [Subagent context persistence System Design Part 2](system-design/subagent-context-persistence-02.md)
- [Subagent context persistence System Design Part 3](system-design/subagent-context-persistence-03.md)
- [Subagent context persistence System Design Part 4](system-design/subagent-context-persistence-04.md)
- [Subagent context persistence System Design Part 5](system-design/subagent-context-persistence-05.md)
- [Subagent context persistence System Design Part 6](system-design/subagent-context-persistence-06.md)
- [Subagent context persistence System Design Part 7](system-design/subagent-context-persistence-07.md)
- [Subagent context persistence System Design Part 8](system-design/subagent-context-persistence-08.md)
- [Profile-backed Utility Agents](system-design/utility-agent-profiles.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Tasks](../tasks/README.md): consumes agent profiles for task execution.
- [Office](../office/README.md): consumes agent identities for autonomous work.
- [Platform](../platform/README.md): owns shared process and runtime services.
