---
id: "01-confirm-ssh-agentctl-absence"
title: "Confirm SSH controller absence before idle reclaim"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-002
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-002.3
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 01: Confirm SSH controller absence before idle reclaim

## Summary and diagnosis

The reaper's error-preserving lifecycle probe returned every agentctl HTTP failure without consulting the existing precise SSH process probe, so confirmed-dead SSH controllers retained execution ownership indefinitely. Restore the intended reclaim behavior through a narrow optional absence capability; the UI remote-status provider is unsuitable because it collapses inconclusive errors into a down state.

## Scope and files

- Lifecycle: `manager_interaction.go`, new `manager_liveness.go`, and new `executor_ssh_liveness.go` under `apps/backend/internal/agent/runtime/lifecycle/`.
- Evidence: new `manager_ssh_liveness_test.go` in lifecycle and `idle_reaper_runtime_absence_test.go` in orchestrator.
- Documentation: executor idle-runtime-parking system design and public executor reference.

No redial, new executor support, persistence policy, stop/cleanup ownership, active-turn policy, or UI changes.

## Acceptance

- A failed status request plus confirmed absence for the exact tracked SSH instance permits existing idle reclaim and preserves resume/worktree records.
- Alive, uncertain, missing identity, unsupported runtime, permission/transport/probe error, and canceled evidence preserve the original status failure.
- Replacement, removal, or transport loss during the remote probe cannot authorize reclaim of the new or uncertain owner.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'TestManagerSSH|TestProbeRemoteAgentctlLiveness|TestSSHExecutorGetRemoteStatus|TestIsAgentRunningForSession|TestAgentExecution_AcquireAgentCtlClientPinsReplacement|TestManager_CleanupStaleExecution|TestDeleteExecutorRunning|TestIdleReaper|TestReclaimIdleSession|TestClassifyIdleReclaim' -count=1)
GOFLAGS="${GOFLAGS:+$GOFLAGS }-trimpath" bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go apps/backend/internal/agent/runtime/lifecycle/manager_liveness.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_liveness.go apps/backend/internal/agent/runtime/lifecycle/manager_ssh_liveness_test.go apps/backend/internal/orchestrator/idle_reaper_runtime_absence_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- Before production edits, `go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestManagerSSHStatusFailureRequiresConfirmedAbsence$' -count=1` failed the absence subtest on HTTP 503 instead of nil. After correction, that same command passed.
- Final targeted race command passed: lifecycle 2.096s and orchestrator 2.766s. Parent independently reran it: lifecycle 2.082s and orchestrator 2.746s.
- Scoped Go lint passed with zero issues. Catalog validation, full specification lint, and whitespace checks passed.
- SSH tests cover absence, alive, permission/inconclusive errors, closed/lost/unresponsive transport, missing identity/runtime/registry, unsupported runtime, successful status precedence, cancellation, and deterministic concurrent replacement/removal/loss. Real lifecycle cleanup releases SSH instance and execution while retaining resume/worktree data.
- Periodic reaper integration uses real lifecycle and SQLite with the executor boundary faked; confirmed absence reaches cleanup, and unknown/error cases retain the execution and running row. The existing fake SSH server separately proves the wire/process boundary. No live host or complete backend/browser/release claim.
