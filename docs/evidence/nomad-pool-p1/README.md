# Nomad worker pool P1: placement and transport, real proof

Work order 01 (`docs/plans/nomad-worker-pool/task-01-pool-placement-transport.md`).
Runs used the operator's laptop fixture (a throwaway local Nomad server and one client node,
SSH-attached) with a runner-owned headless Kandev started by
`scripts/nomad-native-e2e run --require-real --create-fixture --scenario pool-placement`.
No other cluster was touched. The raw `evidence.json` files stay on the laptop under the fixture
temp root; this page records what each run reported.

## Final proof: passed (run 3)

Build: `e7515aca9f` (`kandev v0.98.0-45-ge7515aca9f`). Exit 0, 12 steps passed:
verify-node-identity, poll-node-ready, start-git-daemon, start-backend, create-fixture-objects,
await-a-running, start-managed-process-tree, assert-pool-placement, archive-task,
await-production-cleanup, assert-no-operator-cleanup, teardown.

- The pool profile named a node pool and sizing only. The registered job named no node; Nomad
  placed the single allocation on the pool node, which carried the Kandev node contract.
- Durable state was pool-placed on that node with a node contract digest, the inventory reached
  `ready` and the allocation ran at the requested CPU and memory.
- The agent attached over the SSH transport using host, port and pinned key from node metadata.
- Archive ran production cleanup alone (drain receipt, whole-job stop with purge disabled,
  fence released); no operator action. No leftover processes and no stale fence.

Evidence file on the laptop: `/private/tmp/nomad-wo05-live.rO8x/runs/nomad-e2e-fixture-9izmj3mv/evidence.json`.

## Fixture onboarding (done once, stays in place)

1. `scripts/nomad-pool-onboard ssh <state_root> <ssh_host> <ssh_port> <SHA256 fingerprint>`
   with `state_root` set to `<fixture>/worker`; paste the `meta` block into the client stanza.
2. Add `node_pool = "kandev"` to the client stanza, create the pool with
   `nomad node pool apply`, restart the client; node ready, eligible, meta present.
3. Add a `pool` block (`node_pool`, `placement`, `ssh_user`) to a separate runner config.

## Scheduler prerequisites

- Memory oversubscription enabled: `nomad operator scheduler set-config -memory-oversubscription=true`.
- Scheduler algorithm `spread` for a `spread` profile (a `binpack` profile needs `binpack`).

## Run history

| Run | Build | Result | Finding |
| --- | --- | --- | --- |
| 1 | `b14dcd28b5` | rc 2, preflight refused | Memory oversubscription was off (correct refusal); the fixture scheduler was binpack against a spread profile. Fixture set to spread with oversubscription; prerequisites added to the preflight message and docs. |
| 2 | `b14dcd28b5` | rc 2 at await-a-running | Product side worked (placed, ready, SSH attached). The runner read the fence at the legacy path derived from `allocation_root`; a pool node keeps it under its `kandev.state_root`. cleanup-fixture also left a pool fence directory. |
| 3 | `e7515aca9f` | rc 0, all steps | none |

## Known limits (P2)

P1 still uses the worker-wide start fence (exit 73 if present), so two environments on one node
cannot run yet. Per-environment fences, workspaces and per-node quarantine are task 02.

## Evidence status

The raw run JSON for this work order stays on the operator laptop and is not committed, so the results above are operator-reported and not independently verifiable from this repository. The live runs for WO06 and WO07 used builds older than the later pool edits to the shared Nomad job, quarantine, bootstrap, state and destroy code; they were not re-run on the final head. No live run places two environments on one node and checks isolation through stop, recovery and cleanup; the offline admission test covers admission only.
