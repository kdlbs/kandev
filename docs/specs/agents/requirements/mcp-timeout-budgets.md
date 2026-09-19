---
status: draft
system: agents
created: 2026-09-02
owners:
  - kandev
---

# Agent MCP Timeout Budgets Requirements

## Overview

Kandev supplies default environment values to every managed agent runtime. Two
of those values describe unrelated time budgets: how long the agent may spend
reaching a usable MCP session before it dispatches the first turn, and how long
a single MCP tool call may block. Kandev needs long blocking tool calls, because
`ask_user_question_kandev` and `step_complete_kandev` hold an MCP request open
until a user answers. Kandev does not need, and cannot tolerate, a startup
budget of the same size: an agent runtime that waits out its startup budget
before the first turn holds the session at generating for that whole budget
while producing no output.

The agents system owns this contract because managed runtime environment
defaults are part of the agent-facing runtime contract, not of executor
placement or task orchestration.

## Terminology

- **Startup budget:** the maximum time an agent runtime may spend establishing
  an MCP session before it dispatches the first prompt, including any wait on a
  subscription stream that a correct MCP server holds open indefinitely.
- **Tool-call budget:** the maximum time a single MCP tool call may remain open
  before the agent runtime abandons it.
- **Managed agent default:** an environment value Kandev supplies for an agent
  runtime at the lowest precedence tier, which an executor profile or agent
  profile value replaces.

## Requirements

### REQ-AGENTS-MCP-TIMEOUT-BUDGETS-001: Independent MCP startup and tool-call budgets

**Intent:** Time an agent runtime spends waiting before its first turn must be
bounded independently of the budget Kandev's blocking MCP tools need.

**User story:** As a Kandev user, I want an agent to start within seconds when I
have MCP servers configured, so that a startup wait cannot make a task appear
to run for hours without producing output.

#### Acceptance criteria

- **AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.1:** When Kandev supplies managed agent
  defaults for an agent runtime, the startup budget and the tool-call budget
  shall be expressed as two separate environment values, and no single value
  shall define both budgets.
- **AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.2:** When Kandev supplies a managed agent
  default startup budget, that value shall be at most 60000 milliseconds.
- **AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.3:** When Kandev supplies a managed agent
  default tool-call budget, that value shall be at least 7200000 milliseconds,
  so a blocking `ask_user_question_kandev` call can remain open for two hours.
- **AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.4:** When an agent runtime declares a
  startup budget or a tool-call budget as a managed agent default, an executor
  profile or agent profile value for the same key shall replace it without
  producing an environment conflict.
- **AC-AGENTS-MCP-TIMEOUT-BUDGETS-001.5:** When an agent runtime declares any
  MCP time budget as a managed agent default, an automated check shall assert
  each declared value and its bound, so a later edit that recouples the two
  budgets fails a test rather than only changing runtime behavior.

## Out of scope

- Detecting or recovering an agent that has already stalled. That behavior
  belongs to [Agent Stall Recovery](agent-stall-recovery.md).
- Resolution and conflict rules between environment origins. Those belong to
  [Executor-Profile Environment
  Precedence](../../executors/requirements/executor-profile-env-precedence.md).
- The blocking semantics of individual Kandev MCP tools. Those belong to the
  [task and workflow system](../../tasks/README.md).
- Correcting the agent CLI's own first-turn wait. That is an upstream client
  defect, tracked as `anthropics/claude-code#91414`. Kandev bounds its cost; it
  does not fix it.
