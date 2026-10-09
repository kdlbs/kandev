---
created: 2026-09-22
status: in_progress
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
  - REQ-AGENTS-CUSTOM-ACP-002
system_design:
  - ../../specs/agents/system-design/custom-acp-agents.md
legacy_specs: []
---

# Implementation Plan: Operator-Registered ACP Agents

## Overview

Let an operator register a CLI that speaks ACP and have Kandev drive it as a structured agent, and stop
the discovery sweep from reporting an agent list captured at boot. One work order owns both, because
the protocol change is not observable without the discovery change: a newly created ACP agent would
stay absent from Installed Agents until the backend restarted.

## Work orders

| Wave | Work order | Owns |
| --- | --- | --- |
| 1 | [Task 01: Operator-registered ACP agents](task-01-operator-registered-acp-agents.md) | Live discovery, the protocol field, `CustomACPAgent`, the probe exception, and the dialog |
| 2 | [Task 02: Restore the provider session on reconnect](task-02-custom-acp-session-restore.md) | `CustomACPAgent` declares native session restore; the ACP adapter keeps negotiating it per connection |

## Sequencing

Task 02 builds on the `CustomACPAgent` type that Task 01 introduced and does not depend on Task 01's
remaining verification. Task 01 is otherwise a single sequential work order. Splitting the discovery fix into its own wave was considered and
rejected: it would leave the ACP wave unable to verify its own acceptance criteria, since every
create/delete assertion depends on the live sweep.

## Out of scope

- Built-in agent declaration, detection, and launch are untouched.
- Renaming `tui_config` or `/api/v1/agents/tui`. The stored column and the route keep their names; a
  migration and an API break to correct a name no operator sees is not a trade this plan makes.
- A login command for an operator-registered agent. A CLI whose ACP mode needs an interactive sign-in
  must be signed in from a terminal; built-ins declare `LoginCommand()` and custom definitions have no
  field for it. Recorded here because the gap is real and adjacent, not because this plan closes it.
