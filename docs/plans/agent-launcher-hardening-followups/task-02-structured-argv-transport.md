---
id: "02-structured-argv-transport"
title: "Structured argv transport"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 02: Structured argv transport

## Acceptance

- Lifecycle stores and sends structured initial/continue argv end to end;
  agentctl executes it without reparsing display strings.
- `agent_args`/`continue_args` take strict precedence when present, while legacy
  `command`/`continue_command` remain absence-only compatibility fallbacks.
- Tests cover spaces, quotes, empty arguments, Windows paths/backslashes,
  prefix/CLI flags, initial, continue, reset/recovery, serialization, and
  legacy clients.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/agentctl/server/api ./internal/agentctl/server/config ./internal/agentctl/server/process
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/command.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/agentctl/client.go`
- `apps/backend/internal/agentctl/server/config/config.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- focused tests beside those files
- this task file

## Inputs

- ADR structured-argv paragraph.
- Existing `agents.Command` token slices and lifecycle display-string builders.
- Current agentctl configure JSON and `config.ParseCommand` fallback.

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, ADR sections, risk tags, exact RED/GREEN verification
commands/results, uncertainties, and this task status set to `done`. Do not edit
`plan.md`.
