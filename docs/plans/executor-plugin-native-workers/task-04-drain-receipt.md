---
id: "04-drain-receipt"
title: "Drain receipt into destroy (R3)"
status: pending
wave: 3
depends_on:
  - "03"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-005
acceptance_criteria:
  - AC-EXECUTORS-NATIVE-WORKER-005.1
  - AC-EXECUTORS-NATIVE-WORKER-005.2
  - AC-EXECUTORS-NATIVE-WORKER-005.3
  - AC-EXECUTORS-NATIVE-WORKER-005.4
  - AC-EXECUTORS-NATIVE-WORKER-005.5
  - AC-EXECUTORS-NATIVE-WORKER-005.6
  - AC-EXECUTORS-NATIVE-WORKER-005.7
  - AC-EXECUTORS-NATIVE-WORKER-005.8
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 04: Drain receipt into destroy (R3)

## Summary

Port the generic agentctl seal-and-drain, persist a claim-bound receipt before destroy, pass receipt and outcome to the provider, and honor the declared destroy timeout and idle setting.

## Size

M: ~400 source, ~600 test lines. Port from the fork reference named in plan.md (read-only).

## In scope

- Port the generic agentctl seal-and-drain, persist a claim-bound receipt before destroy, pass receipt and outcome to the provider, and honor the declared destroy timeout and idle setting.

## Out of scope

- Provider-side job stop; maintenance operations.

## Acceptance

- Destroy under a claim drains, persists the receipt, then destroys, in that order.
- A lost reply never produces a receipt; the destroy request carries an explicit outcome.

## Failing-first tests

- Outcome mapping: refused connection or identity mismatch gives `unreachable`, lost reply or 60 s deadline `uncertain`, rollback with no runtime `not_attempted`; a successor claim on a sealed runtime receives a fresh receipt.
- Drain reply lost: no receipt, outcome `uncertain`, record stays cleanup_pending until absence (fails: no drain exists on main).
- Same claim retried: same receipt. Earlier-claim receipt rejected.
- Stop, shutdown, plugin disable: no seal (negative tests).
- Receipt is not accepted as absence evidence.
- Bootstrap sets idle timeout disabled only for declaring providers.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/agentctl/... -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `internal/agentctl/server/api (new route)`
- `internal/agentctl/server/instance/manager.go`
- `internal/agent/runtime/agentctl (control client)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin_recovery.go`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.
