---
id: "02-builtin-inventory"
title: "Built-in identity and durable inventory"
status: done
wave: 2
depends_on:
  - "01-runtime-drain"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-001
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
  - REQ-EXECUTORS-NOMAD-005
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-001.1
  - AC-EXECUTORS-NOMAD-001.3
  - AC-EXECUTORS-NOMAD-002.1
  - AC-EXECUTORS-NOMAD-002.2
  - AC-EXECUTORS-NOMAD-003.3
  - AC-EXECUTORS-NOMAD-005.1
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 02: Built-in identity and durable inventory

## Summary

Add the built-in Nomad runtime/profile identity and durable operation state. Establish the headless real-fixture runner contract without launching an unmanaged agentctl.

## In scope

- Add RuntimeNomad/NameNomad/ExecutorTypeNomad and backendapp registration, typed profile validation, host-mode/remote/resume predicates and bootstrap projections; no rendered UI work or local fallback.
- Add cohesive Nomad API adapter and versioned canonical environment/executors_running state with generation, operation/input digest, immutable job, allocation/NodeID, admitted SSH target, ports and receipt fields. Keep secret references in the existing vault.
- Wire checkpoint, recovery and cleanup hooks for this runtime explicitly. Use existing executors_running metadata and narrow execution/environment-generation/envelope-revision CAS methods; no SQL NodeID claim, worker table or schema migration. The worker-local non-expiring start fence owns cross-session launch exclusion, with Nomad Variables CAS quarantine/operation interlocks. Do not describe row CAS or a row scan as atomic NodeID ownership.
- Bridge CheckpointRuntimeInventory/ReleaseRuntimeInventory at both manager_launch.go and manager_execution.go entry points. Retain ordinary-stopped/dead cleanup-pending envelopes in startup/retry/cleanup listing; the current live-remote lister alone filters them out. Follow the paired design's Persistence and worker authority clarification for exact repository/claim checks.
- Add scripts/nomad-native-e2e preflight/run contract, fixture JSON schema and inert ACP payload source. Missing/unimplemented requested scenarios fail, and preflight never starts services.

## Out of scope

- Rendered UI, Docker/VM execution, plugin implementation, provider SDK/protobuf changes, model-backed workloads, and production rollout.

## Acceptance

- Backend registration and API profile validation route nomad to its own host-mode backend and reject invalid/shared-workspace inputs without local fallback; existing runtime predicates remain correct.
- Durable ownership/operation state survives repository reopen, fences successor generations and checkpoints intent before external allocation; secrets cannot serialize into the state.
- Runner preflight validates fixture ownership, native tooling, pinned SSH and partition capabilities; strict no-tests/skips/unimplemented scenarios fail explicitly.

## Verification

All new paths/test names are future deliverables. Run the complete block from
any shell with the named absolute root and fixture-only config set. Use TDD for
changed logic; no-tests-to-run or a skipped required real scenario fails the
work order. Do not mark done based only on existing tests.

```bash
: "${KANDEV_ROOT:?Set absolute Kandev worktree path}"
: "${KANDEV_NOMAD_TEST_CONFIG:?Set absolute fixture-only JSON config path}"
cd "$KANDEV_ROOT/apps/backend"
go test -race ./internal/agentruntime -run '^TestRuntime' -count=1
go test -race ./internal/agent/executor -run '^TestExecutorTypeToBackendMapsNomad$' -count=1
go test -race ./internal/task/models -run '^TestNomadExecutor' -count=1
go test -race ./internal/agent/nomad -run '^(TestNomad|TestUnmarshalEnvironmentState|TestEnvironmentState)' -count=1
go test -race ./internal/task/service -run '^TestNomadExecutorValidatesDedicatedNativeProfile$' -count=1
go test -race ./internal/agent/runtime/lifecycle -run '^(TestNomadExecutorIsRegisteredHostModeButFailsClosedUntilLaunchImplemented|TestWireNomadInventory)' -count=1
go test -race ./internal/task/repository/sqlite -run '^(TestNomadExecutorInventory|TestListExecutorsRunningNomadRetainsTerminalInventory)' -count=1
go test -race ./internal/backendapp -run '^TestRegisterNomadBackendPreventsStandaloneFallback$' -count=1
cd "$KANDEV_ROOT"
python3 scripts/test_nomad_native_e2e.py
scripts/nomad-native-e2e preflight --config "$KANDEV_NOMAD_TEST_CONFIG"
```

## Files likely touched

- `apps/backend/internal/agentruntime/runtime.go and tests`
- `apps/backend/internal/agent/executor/executor.go and tests`
- `apps/backend/internal/task/models/executor*.go and typed profile validation/projections`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/agent/nomad/ (new)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad*.go and manager checkpoint/recovery/cleanup branches (new)`
- `scripts/nomad-native-e2e (new); apps/backend/internal/agent/nomad/integration/fixture (new)`

## Dependencies

[Task 01](task-01-runtime-drain.md).

## Risks

Existing K8s/plugin checkpoint hooks are runtime-specific. A taxonomy constant alone does not supply durable ownership. TaskEnvironment has no generic metadata column; its owner/generation remains canonical while the versioned Nomad envelope lives in ExecutorRunning.Metadata. A generic Upsert or timestamp-only delete is not the full Nomad ownership/cleanup guard. Do not add public SDK/protobuf or plugin manifest work.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using frontmatter IDs.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), matching implementation boundary and current source.
- [Plan](plan.md), including strict real-worker runner/prerequisites.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Read applicable scoped `AGENTS.md` before editing code.

## Results

Implemented the Nomad runtime/profile identity, host-mode backend registration,
typed profile checks, metadata-only `executors_running` revision CAS, and
retained terminal inventory listing for restart reconciliation. Checkpoint and
receipt-guarded tombstone callbacks are wired at both lifecycle launch and
execution entry points. No SQL schema migration or global NodeID claim was
added; the runtime still fails closed until the launch path is implemented.

Focused `-race` checks passed with `GOFLAGS=-p=1` and
`GOCACHE=/private/tmp/kandev-go-build-cache`:

- Lifecycle registration and inventory callback tests: 2 tests passed.
- SQLite Nomad inventory CAS and terminal-row retention: 3 tests passed.
- Backend Nomad registration without standalone fallback: 1 test passed.
- `python3 scripts/test_nomad_native_e2e.py`: 4 tests passed.
- Real local-tool preflight against a private temporary fixture config passed
  using Nomad 2.0.7 and OpenSSH; it did not start services or contact Nomad.
- `scripts/nomad-native-e2e run --scenario admission --require-real` exited 2
  with the expected “not implemented; no services were started” result.

These are WO02 persistence/registration and preflight results. They do not
prove allocation launch, worker admission, recovery against a running Nomad
cluster, or process cleanup; those remain later work-order acceptance gates.
