# Nomad WO05: pre-ready crash recovery, real proof

Work order 05 (`docs/plans/nomad-native-executor/task-05-recovery.md`), fenced
environment readiness publication. Evidence file:
[`pre-ready-recovery-run10.json`](pre-ready-recovery-run10.json), the runner's
`evidence.json` with absolute fixture paths replaced by `<fixture-home>`.

## Result: passed (run 10, exit 0)

- Command: `scripts/nomad-native-e2e run --require-real --scenario pre-ready-recovery --create-fixture --config <bootstrap outside the repository>`.
- Runner at `84c1e3f6fc` (loopback bind, daemon group-kill guard, SSH identity removal); backend and agentctl built from `0e73ef6a15` (the only `apps/` changes since are two `_test.go` files, so the binary is current) (kandev sha256 `87b63baed495b1ffb369c1c5128262ef451dac85dd725e0bda42c1bfa4f6d44a`, agentctl-darwin-arm64 sha256 `328d8409f9569592776eb93240e58b673ef48a0495268f6198a8b98560ed5fde`, codesign verified). All 11 steps passed.
- Cluster: throwaway local Nomad 2.0.7 on loopback, darwin/arm64 client, fixture sshd with a pinned host key. No real cluster or live service was touched.
- The fixture backend was SIGKILLed at the fixture-only pre-publish barrier, after the workspace input was durable and the inventory phase was `ready` while the `task_environments` row was still `creating`. It was restarted without the barrier.
- Before any prompt (`prompts_sent: 0`): the environment became `ready`, a normal workspace read returned the expected file, the recovered checkout HEAD `4c687ac69a70a67447ba936ed7b79ac861002683` (read over the pinned read-only SSH channel, since git snapshots exist only after an agent turn) equalled the fixture git-daemon repository HEAD, and the execution, job, allocation, node and fence were unchanged.

| Identity | Before crash | After recovery |
| --- | --- | --- |
| execution | `115810d8-3d76-4d57-8e2e-f72c4a6bf42a` | same |
| job | `kandev-f2bedea5c9b1d0ab231da7576ffe2cce`, create/modify index 186/186 | same |
| allocation | `11555eae-4303-7319-6221-34575ff94414` | same |
| node | `5dd33836-f337-39c7-8bd3-d60004c67585` | same |
| fence owner sha256 | `9b2aaa3b...2416` | same |
| environment status | `creating` | `ready` |
| remote process count | 2 | 2 |

Backend pids: 31015 before the crash, 31127 after the restart.

## Cleanup is an operator teardown, not production cleanup

An ordinary `session.stop` retains the allocation, fence, controller and
workspace (the work order 04 contract). The run asserted that retention
(allocation and job `running`, fence present and unchanged, 2 remote processes)
and recorded `production_cleanup: false`. Production whole-job stop remains work
order 07's. After the run the operator purged the retained job through the local
Nomad API and removed the fence directory; the cluster ended with 0 jobs and 0
allocations.

## Earlier attempts

Runs 1 to 6 failed and exposed runner bugs, fixed before run 7. Run 7 passed but never proved the repository HEAD. Run 8 passed every recovery step and failed only the new HEAD assertion, which wrongly read git snapshots (empty before any turn); run 9 read HEAD over SSH and passed; run 10 repeated it with the hardened runner. Bugs fixed: no agent
profile on a fresh fixture home (now an inert ACP agent is created), a missing
settings interlock header, an incomplete Nomad executor config, a local
repository without a provider origin (now a runner-owned `git daemon` plus
`insteadOf` redirect), the fence being a directory (read `<fence>/owner`), and a
cleanup that wrongly expected the allocation to end after `session.stop`. Run 6
already passed every recovery step and failed only that cleanup expectation.

## Not covered

Run 10 supersedes runs 7 and 9 and is the authoritative WO05 evidence. Production whole-job cleanup is work order 07.

The PostgreSQL race test for the publish CAS was not run (no PostgreSQL here).

Run 10 ran the hardened runner (84c1e3f6fc); the owner performed the operator teardown afterwards and the cluster showed 0 jobs and 0 allocations. Run 9 (runner 54ab535df2) is superseded; its JSON was removed.
