---
status: active
system: tasks
created: 2026-09-19
owners:
  - kandev
---

# MCP self sibling placement

## Overview

A Kanban child agent requesting persistent follow-up work with
`create_task_kandev(parent_id="self")` reaches the one-level depth limit.
Resolve that request to a sibling in the same call and explain the placement.
Tasks owns the hierarchy and creation contract.

Source: [issue #3773](https://github.com/kdlbs/kandev/issues/3773) and the
[maintainer decision](https://github.com/kdlbs/kandev/issues/3773#issuecomment-5746147513).
The maintainer authorized implementation of this package on 2026-09-21.
Implementation acceptance is recorded in the linked plan and work order.

## Terms

- **Caller:** the task of the authenticated creating session.
- **Requested parent:** the caller when the literal `self` shorthand is used.
- **Effective parent:** the caller for ordinary creation, or its direct parent
  when the Kanban depth limit requires sibling placement.

## Requirements

### REQ-TASKS-SELF-SIBLING-001: Depth-aware self placement

**Intent:** Complete a valid child-originated request without a corrective tool call.

#### Acceptance criteria

- **AC-TASKS-SELF-SIBLING-001.1:** For a valid Kanban child C under root P,
  literal `parent_id="self"` shall create one sibling S under P in one tool call,
  subject to existing creation validation and external-ID deduplication.
- **AC-TASKS-SELF-SIBLING-001.2:** For a root Kanban caller, `self` shall still
  create a direct child. Omitted parent shall retain top-level creation. An
  explicit UUID, including C's own UUID, shall retain literal parent semantics
  and the existing depth error when it names a Kanban child.
- **AC-TASKS-SELF-SIBLING-001.3:** Redirection shall select only the caller's
  direct parent. Missing, inaccessible, cross-workspace, ephemeral, or otherwise
  invalid parent references shall fail without creating or launching work;
  the system shall not climb further or fall back to top-level placement.
- **AC-TASKS-SELF-SIBLING-001.4:** Every successful redirected request shall
  identify the requested and resolved parent and explain the Kanban depth
  limit in its tool result. Newly created work shall be described as a sibling;
  an idempotency hit shall be described as an existing task, preserving its
  actual parent and existing completion/deduplication indicators.

### REQ-TASKS-SELF-SIBLING-002: Existing ownership and creation guarantees

**Intent:** Change placement without introducing a second coordinator identity.

#### Acceptance criteria

- **AC-TASKS-SELF-SIBLING-002.1:** The effective parent shall govern inherited
  workspace, workflow, repositories and materialized-workspace policy under
  existing explicit-override rules. Existing launch-profile, executor and
  runtime-setting precedence shall still use the verified creating session
  wherever that precedence currently selects the creator.
- **AC-TASKS-SELF-SIBLING-002.2:** Creation and genesis-ledger attribution shall
  remain attached to the caller's verified session. The effective parent shall
  retain direct-parent completion, autopilot-question, interrupt and stop
  ownership. The caller shall receive no parent controls over its new sibling.
- **AC-TASKS-SELF-SIBLING-002.3:** Existing permission, admission, dependency,
  launch and idempotency rules shall still apply. A repeated external ID shall
  not create, reparent, relaunch or rewrite attribution of the existing task.
  A failed creation shall not produce a success/placement notice.
- **AC-TASKS-SELF-SIBLING-002.4:** Office-session MCP denial, permitted Office
  nesting through existing creation surfaces, external MCP's rejection of
  `self`, and automation-specific policies shall remain unchanged. Explicit
  parent creation via REST, service, CLI and other tools shall not gain fallback.
- **AC-TASKS-SELF-SIBLING-002.5:** Task-mode tool descriptions and public
  coordination/MCP guidance shall explain automatic sibling placement and
  common-parent coordination without claiming nested Kanban delegation.

## Exclusions

No deeper Kanban tree, delegator relationship, new public creation option,
permission expansion, database migration, UI control, workflow trigger change,
Office MCP enablement, historical reparenting, or automatic retry after failure.

## Related contracts

- [Creation admission](mcp-workspace-mode.md)
- [Autopilot](autopilot-mode.md)
- [Completion trigger](subtask-completion-trigger.md)
- [Design](../system-design/mcp-self-sibling-placement.md)
- [Implementation plan](../../../plans/mcp-self-sibling-placement/plan.md)
