# Pool P3 evidence

## Runs at 3f7993e0c4 (two-node laptop fixture)

All three scenarios of `scripts/nomad_native_e2e.py` passed, rc 0, against a throwaway
Nomad server with two clients in node pool `kandev`: one `ssh` client and one `local`
client.

| Scenario | Run | Result |
|---|---|---|
| `pool-two-node` | `runs/nomad-e2e-fixture-m5kto0zn` | A on the ssh node, B on the local node; local worktree vs remote checkout; both cleaned by production cleanup |
| `pool-node-quarantine` | `runs/nomad-e2e-fixture-c4yng02i` | Drain failure quarantines one node only; B placed on the other; readmission |
| `pool-placement` (fresh profile) | `runs/nomad-e2e-fixture-e6nsjljz` | Job names no node; one allocation on a contract node; production cleanup |

Fixture boundary: both clients run on one Mac, where Nomad fingerprints 48 MHz, so both
set `cpu_total_compute=4000`; the local client reserves 1000 MHz / 2048 MB. This is not a
multi-machine fleet.

## Findings from those runs, and their fixes

1. Product leak (REQ-006, REQ-004.1): every pool environment left
   `<state_root>/pool/allocations/kandev-<job>/` (bootstrap script, `.sh.env` that may hold
   bootstrap secrets, ready marker, an agentctl copy) after proven cleanup.
   Fix: `AllocationDirRemoveCommand` runs in the cleanup tail after the workspace removal
   and before the fence release; a failure keeps the environment retained. Offline tests:
   `alloc_dir_pool_test.go`, `executor_nomad_pool_cleanup_test.go`. Live assertion:
   `production_settled` in `pool-placement` and `pool-two-node` now requires the allocation
   directory to be gone.
2. Runner: `pool-node-quarantine` teardown released B through operator teardown but left
   its worktree, branch `kandev/pool/<B>`, environment directory and allocation directory.
   Fix: the teardown now removes them and asserts the branch, the worktree registration and
   the directories are gone (`remove_b_leftovers`).

## Placement wording

AC-EXECUTORS-NOMADPOOL-001.3 matches Nomad: spread places each environment on the eligible
node with the lowest reserved-resource utilisation; binpack fills a node before using
another; placement is reservation-based, not live-load-based.

## Reruns at e2fc748583 (v0.98.0-59-ge2fc748583): all passed

| Scenario | Run | Result |
|---|---|---|
| `pool-placement` | `runs/nomad-e2e-fixture-10s2t0y7` | rc 0 |
| `pool-two-node` | `runs/nomad-e2e-fixture-mpm6g4oe` | rc 0 |
| `pool-node-quarantine` | `runs/nomad-e2e-fixture-5omgj8_b` | rc 0 |

After each run: no worker processes, no extra worktree, and no allocation directory or
local-state entry left, so the leak fix holds. One remainder: after `pool-node-quarantine`
the quarantined environment A's allocation directory stayed on the ssh node. Keeping it
while quarantined is correct; the readmission procedure now deletes it once absence is
verified (`readmit-remove-allocation-directory` step, public docs updated).

## Second live round at c8d432d2f9: findings and fixes

| Scenario | Result | Finding | Fix |
|---|---|---|---|
| `pool-node-quarantine` | rc 0 | none | none |
| `pool-queue` | rc 2 | runner: stale env dict, KeyError `attach` at `assert-workspace-c` | re-read `scn.envs["C"]` |
| `pool-refusals` | rc 2 | a refused launch leaves a quarantine marker; teardown set the dynamic node meta to null | see below |
| `pool-partition` | rc 2 | relay on the server RPC partitioned both clients | SIGSTOP/SIGCONT of the ssh client agent |

Refusal quarantine: the marker is written by the rollback of a launch that never recorded a
runtime identity (reason `runtime_identity_unrecorded`), which is existing contract. The
runner now expects that reason, deletes the marker through operator teardown, and restores
the node's original host-key meta value instead of nulling it. Skipping quarantine for a
provable pre-staging refusal would need a new "never staged" state and is left as an owner
decision.

## Third live round at 605a05c32d: findings and fixes

Owner decision: keep the contract, a refused launch quarantines the node.

| Scenario | Result | Finding | Fix |
|---|---|---|---|
| `pool-queue` | rc 2 | C was placed on A's node, but its agentctl failed with "failed to allocate an available port" on the single reserved port; cleanup-fixture left C's allocation directory | product: `CreateInstance` waits (bounded, caller-context aware, lock released between tries) while every port of the range is held by a process outside the manager, and forgets in-use marks so a retry probes the range again; runner: placement is recorded before readiness so cleanup-fixture removes the node files of a launch that fails after placement |
| `pool-refusals` | rc 2 | after the expected rollback quarantine the marker was deleted, but the product's retained cleanup stayed in retry_wait and the node stayed ineligible | runner: operator teardown now follows the documented readmission: readmit API, delete marker, enable eligibility, wait ready |
| `pool-partition` | rc 2 | the probe prompt was judged while the local session was still answering, and messages the ssh session received during the partition were compared strictly | runner: wait for a promptable state before the probe, wait for an answered prompt and a settled state, and accept appended messages (prefix check) on the partitioned ssh session |

The port-wait fix rests on the operator's reading that the previous holder of the single
Nomad-reserved port was still shutting down when C started (the scheduler frees an
allocation's resources once its desired status is stop, before the client process has
exited). The live rerun confirms or refutes it.

## Final runs at 0d8ba484e9 (v0.98.0-64-g0d8ba484e9): all six passed

One build (runtime bundle rebuilt from this commit, so it includes the agentctl port fix),
one laptop two-node fixture, runs under `/private/tmp/nomad-wo05-live.rO8x/runs/`:

| Scenario | Run | Result |
|---|---|---|
| `pool-placement` (fresh profile, built runtime) | `nomad-e2e-fixture-reez1rci` | rc 0 |
| `pool-two-node` | `nomad-e2e-fixture-2x9znhup` | rc 0 |
| `pool-node-quarantine` | `nomad-e2e-fixture-ylar_aph` | rc 0 |
| `pool-queue` | `nomad-e2e-fixture-n1t36jw8` | rc 0 |
| `pool-refusals` | `nomad-e2e-fixture-8wbkzu80` | rc 0 |
| `pool-partition` | `nomad-e2e-fixture-tpik2kqi` | rc 0 |

After every run: no worker processes, no extra worktree, no allocation directory on either
node. Afterwards both nodes are ready and eligible, the ssh host-key metadata is intact,
the ssh client agent is running, and there are no live jobs and no quarantine variables.

Fixture boundary: both Nomad clients run on one Mac (`cpu_total_compute=4000`, the local
client reserves 1000 MHz / 2048 MB), with one server. Partition is therefore done by
SIGSTOP/SIGCONT of the ssh client agent, not by a network cut; this proves the server-side
behaviour (node leaves ready, allocation kept, no replacement, no new job, same allocation
after the heal) but not a real network fault between machines. A multi-machine fleet is
not covered.

## Defects found across the rounds

| Round | Defect | Kind | Fix |
|---|---|---|---|
| 1 | Allocation directory left after proven cleanup | product | `AllocationDirRemoveCommand` in the cleanup tail |
| 1 | `pool-node-quarantine` teardown left B's worktree, branch and directories | runner | `remove_b_leftovers` |
| 2 | Readmission left A's allocation directory | runner/docs | `readmit-remove-allocation-directory` step, public docs |
| 2 | `pool-queue` stale env dict (KeyError `attach`) | runner | re-read `scn.envs["C"]` |
| 2 | `pool-refusals` nulled the host-key metadata on teardown | runner | restore the original value |
| 2 | `pool-partition` relay partitioned both clients on a one-server fixture | runner | SIGSTOP/SIGCONT of the ssh client agent |
| 3 | Agentctl create failed on the single reserved port still held by the previous allocation | product | bounded, context-aware wait in `CreateInstance`; in-use marks cleared |
| 3 | C's allocation directory left after a failed launch | runner | placement recorded before readiness |
| 3 | `pool-refusals` left the node ineligible with cleanup in retry_wait | runner | documented readmission after the expected quarantine |
| 3 | `pool-partition` probe raced the local reply; strict message equality on the partitioned session | runner | wait for a settled state; prefix check on the ssh session |

## Decisions

- AC-EXECUTORS-NOMADPOOL-001.3 is worded to Nomad's semantics (spread by lowest reserved
  utilisation, reservation-based, not load-based); owner decision.
- A refused launch quarantines the node (`runtime_identity_unrecorded`) by existing
  contract; owner decision is to keep it. `pool-refusals` asserts the reason and performs
  the documented readmission.

## Not covered

- Startup timing of the built runtime was not measured in these runs.
- No multi-machine fleet, no real network partition, no Linux cgroup enforcement.

## Evidence status

The raw run JSON for this work order stays on the operator laptop and is not committed, so the results above are operator-reported and not independently verifiable from this repository. The live runs for WO06 and WO07 used builds older than the later pool edits to the shared Nomad job, quarantine, bootstrap, state and destroy code; they were not re-run on the final head. No live run places two environments on one node and checks isolation through stop, recovery and cleanup; the offline admission test covers admission only.
