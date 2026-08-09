# ADR-2026-08-08-mcp-tool-profiles: Compose MCP Tool Profiles From Typed Context

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, agentctl, protocol, workflow
**Related ADR:** [ADR-2026-08-08-autopilot-mcp-question-capabilities](2026-08-08-autopilot-mcp-question-capabilities.md)
**Related spec:** [Task Autopilot Mode](../specs/tasks/autopilot-mode.md)

## Context

Kandev already has MCP modes, but the implementation is a large switch in
`internal/mcp/server/server.go`. It registers tool groups for `task`,
`task-title-pending`, `config`, `office`, and `external`. A separate
`disableAskQuestion` boolean removes the user-question tool, and a provider list
adds GitHub or GitLab automation tools.

This works for the current cases, but each new context needs another branch,
another tool-count adjustment, and another transport field. It also makes it easy
to expose a tool that does not belong to the current task. Autopilot needs a parent
question tool, title ownership needs a title tool, and Office tasks need a smaller
surface without Kanban task creation.

## Decision

1. Model the MCP surface as a typed profile context. The context has:
   - a base surface;
   - additive, named capability groups; and
   - provider capabilities.
2. The initial base surfaces are:

   | Surface | Core tool groups |
   |---|---|
   | `kanban-task` | Kanban task operations, planning, walkthrough, review, related tasks, workspace/branch actions, step completion, and diagnostics |
   | `office-task` | Interaction, planning, related tasks, and task documents; no Kanban task creation tools |
   | `configuration` | Workflow, agent, MCP, executor, and configuration-task tools |
   | `external` | Configuration tools plus `create_task_kandev`; no live-session tools |

3. The initial additive capability groups are:
   - `user-question`;
   - `parent-question`;
   - `task-title`;
   - provider groups such as GitHub PR and GitLab MR automation.
4. The backend resolves the profile context from session purpose and task state:
   base surface, autopilot value, direct-parent presence, title ownership, and
   attached providers. Agentctl receives the resolved typed profile context. The
   agent cannot request arbitrary tool names or capabilities.
5. The MCP server owns one declarative registry of tool groups. Each group has one
   registration function and one enablement rule. Profile assembly loops over the
   registry and atomically replaces the MCP tool set. New context-specific tools
   are added by adding a capability group and a resolver rule, not by copying a
   complete mode branch or editing a manual count.
6. Question capabilities use the existing mutually exclusive rule:
   - normal task → `user-question`;
   - autopilot child → `parent-question`;
   - autopilot root → neither question group.
7. Existing `McpMode` values remain accepted as compatibility aliases and map to
   base surfaces. `task-title-pending` maps to `kanban-task` plus `task-title`.
   `SetMcpMode` and `SetMcpProviders` continue to work during migration; a typed
   profile update becomes the backend-owned path for additive capability changes.
8. Profiles are runtime state. Only the task's `autopilot` value is durable. The
   profile is rebuilt on launch, resume, and an authorized live profile update.
   Every replacement sends one `tools/list_changed` notification.

## Consequences

- Tool discovery is smaller and matches the session context.
- Office agents cannot discover Kanban task creation tools and continue to use the
  Office skill/CLI path.
- Autopilot can remove the user-question group and add the parent-question group
  without changing the base Kanban profile.
- Title tools, provider automation, and future context features become additive
  capabilities instead of new mode branches.
- The backend and agentctl gain a typed profile transport and registry tests.
- Profile changes require atomic rebuild and client notification. A stale client
  may still call an old tool, so handlers must keep their existing authorization and
  context checks.

## Alternatives Considered

### Keep adding branches to `registerTools`

Rejected. The branch and count logic grows with every context and can expose tools
that are not valid for the session.

### Create one enum for every final tool combination

Rejected. Autopilot, title ownership, providers, and future features would create a
large cross-product of enum values.

### Let the agent send an arbitrary tool allowlist

Rejected. Tool discovery is a backend capability boundary. An agent must not grant
itself configuration, task mutation, or provider tools.

### Keep `McpMode` as the only extension point

Rejected. Base surfaces and optional capabilities have different lifecycles. A
single mode string cannot represent provider additions or title ownership without
repeating full profiles.
