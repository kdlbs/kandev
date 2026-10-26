---
id: "01-runtime-drain"
title: "Authenticated sealed runtime drain"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-003
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-003.1
  - AC-EXECUTORS-NOMAD-003.3
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 01: Authenticated sealed runtime drain

## Summary

Expose completed cooperative teardown through an authenticated capability-gated control operation. Reuse the existing instance and process managers; keep the control leader alive and permanently sealed for later Nomad stop.

## In scope

- Add runtime-seal-drain.v1 capability, request/result types, authenticated route and control client method.
- Audit all process-start surfaces: instance create; ACP/agent start and configure paths; custom process commands and ProcessRunner; StartShell/terminal reconnect; LSP start/install; workspace preview, Git/materialization/polling and comparison-target operations; provisional/abandoned instances. Route them through existing CloseAdmission, BeginOwnedOperation/admitStart and runner admission, fixing only demonstrated gaps.
- Serialize drain with main-loop Shutdown and return success only after Manager.Shutdown completes successfully. Preserve retryable failures and cache identical successful requests under the same ServerIdentity.
- Prove context/deadline bounds and admission races, including process owners started before the seal.

## Out of scope

- Nomad executor/profile wiring, rendered UI, Docker/VM execution, model-backed workloads, and production rollout.

## Acceptance

- An authenticated exact-identity request returns sealed/drained evidence only after real manager teardown; old/incorrect credentials or identity and unsupported capability fail closed.
- Creation, ACP/custom/shell/LSP/Git starts racing the seal either join the drain or are rejected; no start succeeds after the receipt, and failed drain remains sealed with retained ownership.
- Identical request retries return the same successful receipt while the server lives; changed identity/request reuse, timeout, process-stop error and concurrent main-loop shutdown cannot fabricate success.

## Verification

All paths/test names explicitly marked new in the plan are implementation deliverables.
Run this complete block from any shell with the named absolute roots configured.
No-tests-to-run or a skipped required real scenario fails the work order. Run TDD
for changed logic; do not mark done based only on existing tests.

```bash
: "${KANDEV_ROOT:?Set absolute Kandev worktree path}"
(cd "$KANDEV_ROOT/apps/backend" && go test -race ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agent/runtime/agentctl -run 'TestRuntimeSealAndDrain|TestRuntimeDrainAdmission|TestWaitGroupContextReuses|TestManager_BeginStopWaitsForInFlightAdmission|TestManager_TeardownAdmissionDrainHonorsContextAndRetries|TestProcessRunnerStopAllAndWaitRetainsLiveProcessGroupForRetry|TestSealAndDrainRuntime' -count=1 -v)
(cd "$KANDEV_ROOT/apps/backend" && go test ./cmd/agentctl -count=1)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/control_server.go`
- `apps/backend/internal/agentctl/server/api/identity.go`
- `apps/backend/internal/agentctl/server/api/runtime_drain.go and runtime_drain_test.go (new)`
- `apps/backend/internal/agentctl/server/instance/manager.go and manager_runtime_drain_test.go (new)`
- `apps/backend/internal/agent/runtime/agentctl/control_runtime_drain_test.go (new)`
- `apps/backend/internal/agentctl/server/process/manager.go and runner.go only for verified admission gaps`
- `apps/backend/internal/agent/runtime/agentctl/control.go and control_runtime_drain_test.go (new)`
- `apps/backend/cmd/agentctl/main.go`

## Dependencies

None.

## Risks

Manager.Shutdown currently waits on admitted creation and abandoned work; returning early from a detached goroutine is not proof those owners stopped. Shell/PTY and managed command paths must remain covered, not just the ACP leader.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), especially the matching implementation boundary.
- [Plan](plan.md), including strict real-worker prerequisites and scenario semantics.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Existing source/test patterns identified by the design; read scoped `AGENTS.md` before edits.

## Results

Implemented the authenticated runtime-seal-and-drain capability and client call, serializing it with the agentctl main-loop shutdown. The instance manager seals creation and existing/provisional process admission before bounded waits; deadline failures remain sealed and retryable. Successful receipts are cached only for the exact request and server identity. The manager now memoizes one completion waiter per shutdown WaitGroup, so repeated expired deadlines do not accumulate waiter goroutines.

New tests cover authenticated/current-control identity, missing credentials, exact successful retry receipt, changed-payload conflict, main-loop shutdown serialization, client request/auth/receipt validation, timed-out/retried creation cleanup, admission seal on existing and provisional process managers, repeated deadline retries reusing one waiter channel, and retained process-group teardown on retry. The process-path audit found that manager teardown already closes owned-operation, process, and shell admission before draining accepted operations; no lower-level process-manager change was needed.

Verified from `apps/backend` with `GOCACHE=/private/tmp/kandev-go-build-cache`:

- `go test -race ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agentctl/server/process ./internal/agent/runtime/agentctl -run 'TestRuntimeSealAndDrain|TestRuntimeDrainAdmission|TestWaitGroupContextReuses|TestManager_BeginStopWaitsForInFlightAdmission|TestManager_TeardownAdmissionDrainHonorsContextAndRetries|TestProcessRunnerStopAllAndWaitRetainsLiveProcessGroupForRetry|TestSealAndDrainRuntime' -count=1 -v` — passed with 14 selected tests: 6 API, 3 instance, 3 process, 2 runtime-agentctl client tests. Every package selected tests.
- `go test ./cmd/agentctl -count=1` — passed.
- `git diff --check` — passed.

The supplemental full `go test -race ./internal/agentctl/server/process -count=1` ended after its 10-minute test timeout, with unrelated process-package failures including npm cache/`ECONNRESET`, `TestGetParentPID_CurrentProcess` returning `-1`, and `TestWorkspaceGitExternalHelperBudgets` blocked in `exec.LookPath("git")`. The focused process regressions above passed; no services or workloads were started.
