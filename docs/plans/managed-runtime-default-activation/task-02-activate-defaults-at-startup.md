---
id: "02-activate-defaults-at-startup"
title: "Activate defaults during startup"
status: done
wave: 2
depends_on:
  - "01-reconcile-default-generations"
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-002
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-002.1
  - AC-AGENTS-RUNTIME-UPDATES-002.2
  - AC-AGENTS-RUNTIME-UPDATES-002.4
  - AC-AGENTS-RUNTIME-UPDATES-002.5
  - AC-AGENTS-RUNTIME-UPDATES-002.6
system_design:
  - ../../specs/agents/system-design/runtime-default-activation.md
---

# Task 02: Activate Defaults During Startup

## Summary

Wire default-generation reconciliation into service startup. The backend must finish this operation before discovery, probes, launches, or readiness.

## In scope

- Build trusted generations from registered managed agents.
- Run reconciliation before runtime consumers start.
- Stop startup when reconciliation returns an error.
- Add structured logs for changed generations and errors.
- Prove that later resume commands use the reconciled effective version.

## Out of scope

- npm metadata requests or candidate ACP probes during startup.
- Replacement of active agent processes.
- Frontend changes.

## Acceptance

- Startup removes stale selections before any effective-version consumer starts.
- An unchanged generation preserves a selection through another startup.
- A reconciliation error prevents readiness and leaves the operation retryable.

## Verification

```bash
go test ./internal/backendapp ./internal/agent/registry ./internal/agent/hostutility ./internal/agent/runtime/lifecycle ./internal/agent/settings/controller -run 'ManagedRuntime|RuntimeVersion|AgentUpdate|BuildAgentCommand' -count=1
```

Run the command from `apps/backend`.

## Files likely touched

- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/managed_runtime_defaults_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_command_test.go`
- Focused registry, host-utility, or Settings tests affected by startup order.

## Dependencies

Task 01.

## Risks

- Reconciliation must run before a boot probe reads the old selection.
- Mock and custom agents must remain outside the built-in managed generation list.

## Parallelism

`sequential`

## Inputs

- Task 01 store contract.
- `docs/specs/agents/system-design/runtime-default-activation.md`
- Existing `backendapp.provideServices` and effective-version command builders.

## Results

Startup now reconciles managed runtime generations after trusted agent
registration and before discovery. Reconciliation errors are logged and
returned from `provideServices`, preventing readiness; future launches fall
back to the reviewed default when a selection was cleared.

Verification: the work-order command passed, including backendapp, registry,
hostutility, lifecycle, and Settings controller tests.
