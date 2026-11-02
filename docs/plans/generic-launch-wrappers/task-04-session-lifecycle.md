---
id: "04-session-lifecycle"
title: "Session lifecycle and recovery"
status: done
wave: 4
depends_on:
  - "03-utility-execution"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WRAPPERS-001
  - REQ-AGENTS-WRAPPERS-002
  - REQ-AGENTS-WRAPPERS-003
  - REQ-AGENTS-WRAPPERS-004
acceptance_criteria:
  - AC-AGENTS-WRAPPERS-001.5
  - AC-AGENTS-WRAPPERS-002.1
  - AC-AGENTS-WRAPPERS-002.2
  - AC-AGENTS-WRAPPERS-002.3
  - AC-AGENTS-WRAPPERS-002.4
  - AC-AGENTS-WRAPPERS-002.5
  - AC-AGENTS-WRAPPERS-003.3
  - AC-AGENTS-WRAPPERS-004.1
  - AC-AGENTS-WRAPPERS-004.2
  - AC-AGENTS-WRAPPERS-004.3
  - AC-AGENTS-WRAPPERS-004.4
  - AC-AGENTS-WRAPPERS-004.5
system_design:
  - ../../specs/agents/system-design/generic-launch-wrappers.md
---

# Task 04: Session lifecycle and recovery

## Summary

Carry unresolved wrapper templates through the session configuration and recovery lifecycle. Prepare each new agent process on its execution host.

## In scope

- Separate profile wrappers from inner argv in `CommandBuilder` and lifecycle launch/reset construction. Keep display strings non-authoritative.
- Add the configure envelope to the backend client, API, process configuration, and protected serialization. Apply it to initial and one-shot continue commands.
- Integrate preparation after provider adapter arguments are complete. Preserve the outer deployment guard and existing environment stripping.
- Preserve templates during restart, context reset, managed npm retry, rollback, and process replacement. Adoption of a live process must not reapply a wrapper.
- Check old-peer capabilities before dispatch. Block ambiguous recovery state. Remove raw wrapper arguments and stderr from session diagnostics.

## Out of scope

No PTY integration, plugin implementation, new persistence table, or capability advertisement. Do not restart the retained manual instance.

## Acceptance

1. Subprocess fixtures prove exactly one wrapper on initial, continuation, reset, restart, and managed-runtime retry paths.
2. New and legacy state remain distinguishable. Missing capability or ambiguous new state refuses replacement without changing the live process.
3. Failure and cancellation retain rollback and cleanup behavior. Captured logs do not contain sentinel credentials.

## Verification

Run from the repository root. Implement new behavior with failing tests first.
New test and harness paths below are delivery targets, not files already available in this design package.

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/config ./internal/agentctl/server/process ./internal/agentctl/server/api)
(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run 'GenericWrapper|ManagedRuntime|Configure|Restart')
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths from this work order when Go files change.
Record the expanded command and result. Do not replace targeted checks with an unrecorded broad suite.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/{command,manager_launch,manager_interaction}.go` and `generic_wrapper_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/` managed-runtime retry and recovery state callers.
- `apps/backend/internal/agent/runtime/agentctl/agent.go`, configure client, recovery integration, and `generic_wrapper_test.go`.
- `apps/backend/internal/agentctl/server/api/server.go` and configure tests.
- `apps/backend/internal/agentctl/server/config/config.go` and protected configuration serialization.
- `apps/backend/internal/agentctl/server/process/manager.go`, one-shot integration, and `generic_wrapper_test.go`.
- `apps/backend/internal/agentctl/server/adapter/` one-shot argument propagation where required.

## Dependencies

[Task 03](task-03-utility-execution.md). Read its recorded results before implementation.

## Risks

Legacy argv already includes prefixes. Never recover wrapper boundaries by guessing executable names. Keep unresolved templates separate from prepared commands.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/generic-launch-wrappers.md).
- [System design](../../specs/agents/system-design/generic-launch-wrappers.md), especially the sections that match this work order.
- [Plan](plan.md), compatibility matrix and test mapping.
- [Historical reproduction](reproduction.md), which is diagnostic evidence only.
- Scoped backend/agentctl guidance and nearby existing tests. For browser work, read web guidance and the e2e and mobile-parity skills.

## Results

Implemented separate wrapper state in configure, process configuration, restart construction, and protected recovery. The capability check runs before stopping a live process for restart. Recovery reads only the retained wrapper template from authenticated agentctl state. Child commands still come from the authorized profile and registry.

Initial and repeated process preparation, one-shot initial and continuation commands, missing recovery state, and old-peer refusal have regression coverage. A one-shot startup test failed before the startup path received wrapper preparation. It now passes with two invocation receipts and control-credential stripping.

Focused tests and race checks passed. Broad package failures and their unchanged-HEAD comparison are recorded in [acceptance.md](acceptance.md). The manual instance was not restarted.

CI remediation corrected the mixed-backend recovery-summary fixture to expose a live, dynamically allocated retained-instance endpoint.
The old fixture listed an unused fixed port, so the wrapper-state read prevented its expected recovery.
The fixture now advertises the wrapper capability and an unwrapped version-1 snapshot.
The test also checks that the recovered session exists before checking unchanged per-record outcome counts.
No production behavior changed.
The exact mixed-backend test failed before correction.
`GOMAXPROCS=2 go test -trimpath -p=2 -race ./internal/agent/runtime/lifecycle -run 'TestRecoverySummary|TestDuplicateRecoveryResult|TestGenericWrapperRecovery' -count=1 -timeout=90s` passed afterward.
