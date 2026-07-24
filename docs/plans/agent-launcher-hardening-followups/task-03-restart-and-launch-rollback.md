---
id: "03-restart-and-launch-rollback"
title: "Restart and launch rollback safety"
status: done
wave: 2
depends_on: ["02-structured-argv-transport"]
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 03: Restart and launch rollback safety

## Acceptance

- Replacement commands are fully resolved and validated before the existing
  process, streams, execution state, or agentctl configuration changes.
- Resolver or persisted-prefix failure produces no Stop/Configure/Start calls
  and leaves execution state unchanged.
- Construction failure after instance creation stops the instance exactly once,
  never registers the execution, and never configures/starts agentctl.

## Verification

```bash
cd apps/backend && go test -run 'TestManager_(RestartAgentProcess|Launch|BuildExecution)' ./internal/agent/runtime/lifecycle
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`
- lifecycle deterministic fakes/helpers
- this task file

## Inputs

- Completed task 02 structured execution state.
- `RestartAgentProcess`, `buildFreshAgentCommand`, launch cleanup and execution
  store registration ordering.
- Review thread `discussion_r3641362228`.

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact RED/GREEN verification commands/results,
uncertainties, and this task status set to `done`. Do not edit `plan.md`.
