---
id: "01-command-override"
title: "Launch managed npm agents from a preinstalled command"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PREINSTALLED-COMMAND-001
acceptance_criteria:
  - AC-EXECUTORS-PREINSTALLED-COMMAND-001.1
  - AC-EXECUTORS-PREINSTALLED-COMMAND-001.2
  - AC-EXECUTORS-PREINSTALLED-COMMAND-001.3
  - AC-EXECUTORS-PREINSTALLED-COMMAND-001.4
  - AC-EXECUTORS-PREINSTALLED-COMMAND-001.5
  - AC-EXECUTORS-PREINSTALLED-COMMAND-001.6
system_design:
  - ../../specs/executors/system-design/preinstalled-agent-command.md
---

# Task 01: Launch managed npm agents from a preinstalled command

## Summary

Add the profile `command_override` field and use it as argv[0] in place of
`npx` for managed npm agents.

## In scope

- `agents.ValidateCommandOverride`, `CommandOptions.CommandOverride`,
  `ManagedNPMRuntimeSpec.CommandFor`.
- Profile model, SQLite column, DTOs, controller validation and handlers.
- Lifecycle launch and remote preflight wiring.

## Out of scope

- OpenCode, non-npm agents, UI, and installing the executable.

## Acceptance

1. With an override set, launch argv starts with the override and never uses
   `npx`; without it the launch is unchanged.
2. Relative, empty-argv, whitespace and shell-syntax values are rejected on
   save and at launch.
3. Profile environment variables still reach the launch.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/agents ./internal/agent/settings/... -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'CommandOverride|Override' -count=1)
(cd apps/backend && go build ./... && go vet ./internal/agent/...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Results

Completed on 2026-11-01 with the tests above.
