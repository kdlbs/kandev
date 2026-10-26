# Nomad WO06: capacity admission and client disconnect, real proof

Work order 06 (`docs/plans/nomad-native-executor/task-06-admission-disconnect.md`).
All runs use a throwaway local Nomad 2.0.7 (darwin/arm64 client) with a fixture-owned
headless Kandev; no real cluster or live service was touched. Absolute fixture paths are
replaced by `<fixture-home>`.

## Admission: passed (run 3, exit 0)

Evidence: [`admission-run3.json`](admission-run3.json). Backend built from `1972111756`
(kandev sha256 `5310659560a4f3800eab9a5c2b0316d6e8c81caa08d47b05ed593d8e1ef73dab`); runner at
`1d28f1bd56` plus the fixture fixes recorded in the work order Results.

- AC-EXECUTORS-NOMAD-001.2: the node has 48 MHz CPU, A requests 48 MHz and 1024 MB, so A
  uses all schedulable CPU. B's job registered with no allocation and a blocked evaluation
  naming `cpu`. A's inventory, Nomad snapshot, process tree and fence were identical before
  and after B was queued.
- The runner marked the node ineligible, ended A (purge disabled), verified terminal
  allocation, exact process absence and compare-owner fence removal, then restored
  eligibility. B had zero allocations through that window and exactly one running
  allocation, on the same node with a new allocation id and its own fence owner, afterwards.
- `production_cleanup` is false: release of A is the runner's operator teardown.

## Disconnect: passed (run 7, exit 0, 17 steps)

Evidence: [`disconnect-run7.json`](disconnect-run7.json).

Reported by the operator's laptop run against the local throwaway fixture (server and client split, server `rpc = partition.listen`). Backend and agentctl built from `49d9be918b` (kandev sha256 `37c19ee227348453cf37c67c15c4d15ed81bc40d8aaac51bbd2ce5bdaa2976a1`, agentctl-darwin-arm64 sha256 `9d3dd33054a85aadec534667a506a8bfe0b9a706d0585d07f574f41e09786b4c`).

- All 17 steps passed, including `partition-and-assert-nothing-changes`, `heal-and-assert-original-allocation-survives` and `await-b-start-fence-rejection`.
- The runner's own teardown left the cluster clean: jobs stopped, no fence, no processes.
- Earlier runs 1 to 5 exposed runner defects (fixed) and one product finding: with the then-default `LostAfter=0`, Nomad 2.0.7 marked the original allocation lost while its node was down. The job now carries a bounded, profile-configurable `lost_after` (default 24h).
- Runner note: replacing a signed macOS binary in place gets it SIGKILLed (rc 137), so a bundle refresh must remove the old file before copying.

What the evidence shows (AC-EXECUTORS-NOMAD-002.3):

- Partition through the fixture relay: node `ready` to `disconnected`, original allocation `running` to `unknown` (not `lost`, because of the bounded `lost_after`); after the heal, node `ready` and the same allocation `running` again, with no replacement and no prompts sent.
- Queued B with the backend killed and only A's agentctl leader killed by verified pid and start time: B's allocation failed with exit code 73 on A's still-held start fence, exactly one allocation each, A's fence owner unchanged, no B agentctl process.
- `production_cleanup` is false: teardown is the runner's operator path.
