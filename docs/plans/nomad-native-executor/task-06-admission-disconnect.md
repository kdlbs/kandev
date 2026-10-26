---
id: "06-admission-disconnect"
title: "Capacity and actual client disconnect"
status: done
wave: 6
depends_on:
  - "05-recovery"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-001
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-001.2
  - AC-EXECUTORS-NOMAD-002.3
  - AC-EXECUTORS-NOMAD-003.3
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 06: Capacity and actual client disconnect

## Summary

Prove scheduler capacity and real disconnected-client behavior. Quarantine building blocks (client, state, admission gate) must prevent placement across operations and survive GCed allocation history. The live quarantine triggers on cleanup failure and the operator readmission procedure (AC-EXECUTORS-NOMAD-003.4) belong to task 07; 003.3 here covers only the mechanism and its offline proof.

## In scope

- Two distinct environments reserve measured CPU/memory; A consumes over half capacity including overhead, B genuinely remains unallocated. Release A only with valid evidence and observe B placement.
- Partition real Nomad client/server traffic while original ACP remains alive; observe no restart/reschedule/replacement before or after reconnect. Reconcile original resource/runtime only.
- Queue B before crashing A's leader with A managed child/grandchild still alive and backend down; B may allocate but its bootstrap must reject before any controller/workload launch. No timeout/PID/history can steal A's durable fence.
- Dedicated-owner checked Nomad node eligibility plus CAS quarantine marker blocks new work before cleanup release; unavailable control API preserves a durable pending interlock. Do not re-enable over newer operator intent.

## Out of scope

- Rendered UI, Docker/VM execution, plugin implementation, provider SDK/protobuf changes, model-backed workloads, and production rollout.

## Decisions (spec review round 1)

- Topology: `admission` and `disconnect` run only with `--create-fixture`, and only against a loopback `http` Nomad address (anything else is rejected before any action). The operator supplies a throwaway Nomad 2.0.7 server and client; for `disconnect` the client joins through the fixture-owned loopback relay named in the bootstrap `partition` block (`listen`, `upstream`); the HTTP API is not relayed. The relay is the bypass proof: the node must leave `ready` when it is closed.
- Release of A is the runner's operator teardown, reported `production_cleanup=false`; production release is task 07. Because freeing A's reservation places queued B at once and B's bootstrap exits 73 on A's still-held fence, the runner marks the fixture node ineligible before stopping A, verifies terminal allocation, exact process absence and compare-owner fence removal, then restores the eligibility it changed. B must have zero allocations through that window and one running allocation after it.
- A client disconnect leaves the environment as uncertain, not quarantined. The admission gate rejects while the node is not `ready`/`eligible`; in the window after Nomad reports the node ready but before the original allocation reconciles, the worker start fence (not the gate) prevents a second controller, and the runner orders its queued-B step after the original allocation is running again. Any observed stop, loss or replacement of the original allocation fails the scenario and sends the policy back to design review.
- Queued-B crash (AC-002.3) runs inside `disconnect` after reconnect: B queued, backend killed, only A's agentctl leader killed by verified pid and start time. Evidence: B allocation terminal with exit code 73, exactly one allocation each for A and B, no B agentctl process, A's recorded child/grandchild alive, A's fence owner hash unchanged. B's post-restart envelope phase is not asserted here.
- The admission gate runs before any intent checkpoint, secret or registration and rejects on: another retained envelope for the NodeID in `QuarantineState` pending/marked or `cleanup_pending`; a Nomad Variables marker; node not `ready` and `eligible`; unreadable inventory, node or marker (fail closed). A rejection writes no inventory. Quarantine order: persist `pending`, check dedicated node, set ineligible, create marker (create-only, a conflict means already quarantined), persist `marked`.
- Dedicated owner is the profile-pinned NodeID, datacenter and platform matched against the Nomad node record. Nomad node ACLs are not scoped per node; this is recorded as a limitation, not claimed as isolation.
- Live quarantine triggers are not wired in this task: `quarantineWorker` and `reconcilePendingNomadQuarantine` are offline-tested building blocks. Triggering them on cleanup failure and reconciling `pending` on startup belong to task 07, which also owns operator readmission.
- The Kandev Nomad client exposes no eligible, drain or marker-delete call. Only the runner, as the fixture operator, restores eligibility.
- Disconnect window (owner decision after the first live disconnect run): Nomad 2.0.7 treats an absent `lost_after` as zero and marks the original allocation lost as soon as its client is down, so the job now sets `Disconnect.LostAfter` from the profile field `disconnect_lost_after` (Go duration, default 24h, range 1m to 168h), snapshotted in `disconnect_lost_after_seconds`, compared exactly by the response check, with legacy zero preserved for retained states. Replace=false, keep_original and StopOnClientAfter unset are unchanged. The system designs are amended.
- Disconnect fixture requirements: (1) the throwaway client is a SEPARATE client agent with `servers = [partition.listen]`; a combined server+client agent cannot be partitioned. (2) The throwaway server advertises `rpc = partition.listen`, otherwise the client learns the real server RPC address from heartbeat responses and bypasses the relay. (3) The client `TMPDIR` must exist, or the raw_exec logmon handshake fails and the allocation dies. (4) The runner waits for the client to register through the relay before verifying node identity. (5) The inert ACP agent is registered with `--identity-dir`, and the scenario sends one inert prompt through the normal message API to start the agentctl > acp > child > grandchild tree.
- Honest capacity statement: count=1 is per job group, not a worker limit; native reservations are scheduler bookkeeping, not hard limits on macOS or on outside processes. Recorded in the evidence `limitations`.

## Acceptance

- Real A/B evaluations demonstrate resource reservation (A over half of node capacity minus reserve, B unallocated with exhausted-dimension evidence); B starts only after the recorded safe A release.
- Actual Nomad client partition leaves one original workload running and no duplicate execution/allocation; reconnect restores original identities without another prompt; queued B is refused by A's fence after a leader crash.
- Offline tests prove quarantine marks the owned node ineligible, rejects new operations, survives empty history and restart, and an unavailable control API leaves a durable pending interlock.

## Verification

All test names below are deliverables. Run from any shell with the absolute root and the bootstrap-schema fixture config (no production Nomad address; loopback only). A matching-test guard fails on no-tests-to-run.

```bash
: "${KANDEV_ROOT:?Set absolute Kandev worktree path}"
: "${KANDEV_NOMAD_TEST_CONFIG:?Set absolute fixture-only JSON config path}"
for spec in "./internal/agent/nomad NomadWorkerQuarantine" "./internal/agent/nomad NomadNativeJobDisconnectPolicy" "./internal/agent/runtime/lifecycle NomadAdmissionFence" "./internal/agent/runtime/lifecycle NomadWorkerQuarantine" "./internal/agent/runtime/lifecycle NomadCrashWithQueuedEnvironment"; do set -- $spec; n=$(cd "$KANDEV_ROOT/apps/backend" && go test "$1" -list "$2" | grep -c '^Test'); [ "$n" -ge 1 ] || { echo "no tests match $2 in $1" >&2; exit 1; }; done
(cd "$KANDEV_ROOT/apps/backend" && go test -race ./internal/agent/nomad ./internal/agent/runtime/lifecycle -run 'TestNomadNativeJob|TestNomadWorkerQuarantine|TestNomadAdmissionFence|TestNomadCrashWithQueuedEnvironment' -count=1)
(cd "$KANDEV_ROOT/scripts" && python3 -m unittest test_nomad_native_e2e test_nomad_native_e2e_fixture test_nomad_native_e2e_wo06)
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --create-fixture --scenario admission --config "$KANDEV_NOMAD_TEST_CONFIG")
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --create-fixture --scenario disconnect --config "$KANDEV_NOMAD_TEST_CONFIG")
```

## Files likely touched

- `apps/backend/internal/agent/nomad/quarantine.go`, `state.go` and tests
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad_admission.go and tests (new)`
- `scripts/nomad_native_e2e_wo06.py` and its tests, `scripts/nomad_native_e2e.py`, `scripts/nomad_native_e2e_bootstrap.py` (the `admission` and `disconnect` runner scenarios replace the integration Go test)

## Dependencies

[Task 05](task-05-recovery.md).

## Risks

Nomad policy must be tested on 2.0.7; defaults are not guarantees. Closing a Kandev stream is not this partition. Node eligibility is worker-wide, so shared nodes are unsupported.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using frontmatter IDs.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), matching implementation boundary and current source.
- [Plan](plan.md), including strict real-worker runner/prerequisites.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Read applicable scoped `AGENTS.md` before editing code.

## Results

Complete (2026-10-26). Real-worker proof in `docs/evidence/nomad-wo06/`: admission run 3 (AC-EXECUTORS-NOMAD-001.2) and disconnect run 7 (AC-EXECUTORS-NOMAD-002.3), both against a local throwaway Nomad 2.0.7 fixture. A bundle refresh must remove a signed macOS binary before copying; replacing it in place gets it SIGKILLed.

- Go: `nomad/quarantine.go` (node read, ineligible-only call, create-only Variables marker, dedicated-node check), `EnvironmentState.QuarantineState`, and the lifecycle admission gate (`executor_nomad_admission.go`) running before any launch intent, secret or registration. Quarantine and pending-reconcile are offline-tested building blocks with no production trigger (task 07 wires triggers and operator readmission). `go test -race` over `./internal/agent/nomad` and `./internal/agent/runtime/lifecycle` with `TestNomad` passes; each Verification prefix matches at least one test. One existing test timeout was raised from 30ms to 300ms because admission adds fake round trips; its assertion is unchanged.
- Runner: `admission` and `disconnect` run only with `--create-fixture` (`scripts/nomad_native_e2e_wo06.py`); 206 offline Python tests pass, covering each assertion failing once, first-failure-keeps-fixture, idempotent `cleanup-fixture`, and a real-socket relay.
- Independent review found and fixed: a live quarantine trigger on any bootstrap error (removed), a fail-open inventory read, a runner that could re-enable a quarantined node, a leader misidentification risk (Nomad's executor parent), and a failed disconnect run leaving the client partitioned.
- Live finding: Nomad 2.0.7 `lost_after=0` marks the original allocation lost immediately; fixed by the bounded profile `disconnect_lost_after`.
