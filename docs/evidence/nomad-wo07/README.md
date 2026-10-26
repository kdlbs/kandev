# Nomad WO07: cooperative cancellation and quarantine, real proof

Work order 07 (`docs/plans/nomad-native-executor/task-07-cleanup-quarantine.md`).
All runs use a throwaway local Nomad fixture with a runner-owned headless Kandev started by
`scripts/nomad-native-e2e run --require-real --create-fixture`; no real cluster or live service
was touched. The runs were executed on the operator's laptop. The raw `evidence.json` files stay on
that machine under the fixture temp root; this page records what each run reported.

## Final proof: passed (run 7, both scenarios on one build)

Build: `ffc77bb52d` (`kandev v0.98.0-37-gffc77bb52d`).

- `cleanup-positive`: exit 0. Archive reached `absent` through the production cleanup job with
  no operator action. The retained inventory row is the `absent` tombstone holding the drain
  receipt digest and generation; the job was stopped with purge disabled, one terminal
  allocation on the original node, no process tree, fence released by owner compare, and no
  quarantine marker.
- `cleanup-negative`: exit 0. With the worker leader frozen (SIGSTOP) the drain could not
  complete, archive returned within its bound, and the environment stayed `cleanup_pending`
  with the node quarantined and no `absent` or drain receipt written. A second launch on the
  node was refused, a backend restart left the state unchanged, and after the leader resumed
  the operator readmission procedure (exact process kill, job stop, fence removal, then
  `POST /api/v1/executors/nomad/readmit`) moved the row to `operator_released` while the
  archive cleanup job was still retrying.
- `cleanup-fixture` after each run: exit 0, no leftover processes.

## Run history and the defect each run found

| Run | Build | Result | Finding |
| --- | --- | --- | --- |
| 1 | before `2adb6a61c0` | cleanup-positive failed | A fence mismatch was classified as drain failure; archive deleted the retained `executors_running` row; the cleanup claim raced. |
| 2 | `2adb6a61c0` | cleanup-positive failed (`production cleanup quarantined the environment`) | The forced cascade stop tore the runtime down outside the claim, so cleanup saw an absent fence. The runner also left its own backend and agentctl running after a failed assertion. |
| 3 | `be1be03af5` | cleanup-positive failed (`expected exactly one durable inventory row`) | Cleanup itself succeeded, but the success path deleted the inventory row, leaving no durable absent or receipt evidence. Fixed by keeping the `absent` tombstone until environment deletion. |
| 4 | `31d37f8937` | positive passed; negative failed (`archive unreachable`) | The transport-only handoff stop blocked on a frozen agentctl for the client's 60s default, and shutdown ignored its 20s deadline. |
| 5 | `6f4851a023` | negative failed at `readmit-kill-exact-tree` | Runner race: a tree member exited between the identity check and `kill -KILL`. |
| 6 | `106c92f3a0` | positive passed; negative failed at `readmit-readmit` (500) | Readmission was refused by the task cleanup barrier while the archive job was in `retry_wait`. |
| 7 | `ffc77bb52d` | both passed | none |

## Contracts the runs settled

- Only archive, delete, reset and launch rollback destroy; a plain or forced stop, and the task-tree
  and cascade handoff stops, are transport-only, bounded, and leave the allocation and fence to the
  claim-holding cleanup job.
- The `executors_running` row of a Nomad environment survives archive cleanup as the `absent`
  tombstone with its drain receipt digest and generation until the environment is deleted. A missing
  row is never absence evidence. `task_environments.status` stays `ready`; it is not cleanup evidence.
- Operator readmission is explicit and admin only, is admitted past the task cleanup barrier, and a
  busy environment claim is a retryable 409.

## Test environment note

The `task/service` archive and cleanup tests that build real worktrees (for example
`TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity`) fail on macOS with the default `TMPDIR`
under `/var/folders`, because the no-follow worktree open rejects the `/var` symlink (`not a
directory`). They fail identically on the base branch and pass with `TMPDIR=/private/tmp`:

```bash
TMPDIR=/private/tmp go test ./internal/task/service -run 'Cleanup|Nomad|Archive' -count=1
```

## Evidence status

The raw run JSON for this work order stays on the operator laptop and is not committed, so the results above are operator-reported and not independently verifiable from this repository. The live runs for WO06 and WO07 used builds older than the later pool edits to the shared Nomad job, quarantine, bootstrap, state and destroy code; they were not re-run on the final head. No live run places two environments on one node and checks isolation through stop, recovery and cleanup; the offline admission test covers admission only.
