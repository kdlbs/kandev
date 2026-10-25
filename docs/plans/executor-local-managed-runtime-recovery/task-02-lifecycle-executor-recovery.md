---
id: "02-lifecycle-executor-recovery"
title: "Enable recovery across executors"
status: done
wave: 2
depends_on:
  - "01-agentctl-cache-repair"
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.5
system_design:
  - ../../specs/agents/system-design/managed-npm-runtime-recovery.md
---

# Task 02: Enable recovery across executors

## Summary

Route startup repair through the execution-scoped agentctl client. Enable the same
one-retry flow for standalone, Docker, and SSH runtimes.

## In scope

- Add the failing executor-parity regression test first.
- Replace the backend-host cache invalidator in startup recovery.
- Allow only standalone, Docker, and SSH managed npm runtimes.
- Preserve cancellation, shutdown, startup generation, and command identity.
- Remove obsolete lifecycle wiring without changing manual Settings updates.

## Out of scope

- Settings version preparation and host update jobs.
- Sprites, remote Docker, Kubernetes, native runtimes, and passthrough commands.
- New UI behavior.

## Acceptance

- Standalone, Docker, and SSH start one online replacement after strict ETARGET evidence.
- The repair call reaches the same agentctl instance that reported the failed process.
- Unsupported runtimes and repeated failures remain terminal without another retry.

## Verification

```bash
go test ./internal/agent/runtime/lifecycle ./internal/orchestrator -count=1
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_startup_recovery_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`

## Dependencies

- Task 01 supplies the executor-local agentctl action.

## Risks

- Closing the agent stream before repair can make the request unavailable.
- A stale event from the failed child can overwrite replacement state.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-001`
- Recovery flow in the system design.
- Existing managed runtime startup generation tests.

## Results

- Added the failing executor-parity regression for Docker and SSH before the
  lifecycle implementation.
- Recovery now calls the execution-scoped agentctl repair action for standalone,
  Docker, and SSH, while remote Docker and Sprites remain unsupported.
- Removed the backend host cache-invalidator lifecycle dependency without
  changing manual Settings cache updates.
- Startup generation guards defer stale process and completion events only for
  tracked startup attempts, so a failed first child cannot terminate the
  replacement while ordinary untracked lifecycle executions retain their
  terminal behavior.
- `go test ./internal/agent/runtime/lifecycle ./internal/orchestrator -count=1`
  passed with 4,050 tests across 2 packages.
