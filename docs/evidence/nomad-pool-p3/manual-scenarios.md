# Manual P3 scenarios (superseded by runner scenarios)

**Superseded:** verification items 2, 3 and 5 are automated as `pool-queue`, `pool-partition` and
`pool-refusals` (see `fixture.md`). The procedures below remain as a reference for what each
scenario asserts.

Verification items 2, 3 and 5 of `docs/specs/executors/system-design/nomad-worker-pool.md`.
Run on the two-node fixture in `fixture.md`, after a runner scenario has left a
backend running, or start one with `scripts/nomad-native-e2e run --require-real
--create-fixture --scenario pool-placement ...` and stop it before teardown.
Variables: `NS` namespace, `LOCAL` and `SSHN` the two node ids (`nomad node
status`), `POOLCFG` the runner pool config. Observation uses `nomad job status`,
`nomad job allocs <job>`, `nomad eval list -job <job>`, `nomad node status <id>`.
Record every command's output verbatim in the evidence file.

## 2. Queue at capacity, then place after release

Setup: set the pool profile `memory_mb` (and `memory_max_mb`) above half of the
free memory of each node, so each node holds exactly one environment.

1. Launch environments A and B (two tasks, one prepared session each). Assert A
   and B are `running` on different nodes.
2. Launch C. Assert: job exists, `nomad job allocs` is empty, `nomad eval list
   -job <C>` shows a blocked evaluation naming the exhausted dimension, the
   inventory phase of C is not `ready`, and C has no worker fence on either node.
3. Archive A's task (production cleanup). Assert A reaches `absent` with a
   drain receipt and its fence, workspace and branch are gone.
4. Assert C is placed on the node A released, reaches `ready`, exactly one
   allocation, and its fence appears only after A's is gone. B is untouched.
5. Archive B and C; assert both clean.

## 3. Partition one node, the other is untouched

1. Launch A and B on different nodes (as above). Note which is on `SSHN`.
2. Partition the ssh node's Nomad client: `kill -STOP <pid of the ssh node's
   nomad agent>`. Its running agent processes keep running; heartbeats stop.
3. Wait until `nomad node status $SSHN` shows `disconnected` (or `down`). Assert:
   the environment on `LOCAL` stays `running`, the allocation on `SSHN` is not
   replaced (`nomad job allocs` still one allocation, same id), no new job is
   registered (`nomad job status` list unchanged), the `LOCAL` environment still
   answers a prompt, and Kandev reports the `SSHN` environment unavailable
   rather than recreating it.
4. Reconnect: `kill -CONT <pid>`. Assert the node returns to `ready`, the same
   allocation resumes, the environment on `SSHN` recovers on its recorded
   allocation, and no replacement allocation was created at any point.
5. Archive both; assert clean.

## 5. Refusals

### Wrong host key
1. Disable the local node: `nomad node eligibility -disable $LOCAL`.
2. Stop the ssh client, change its `kandev.ssh_host_key_sha256` to
   `SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA`, restart it, wait
   ready/eligible.
3. Launch a task. Assert it never reaches `ready`, the error names a host key
   mismatch, no agentctl runs on the node, no credential or key material appears
   in the error, and the session reports the failure. Record the resulting
   inventory phase and any quarantine marker verbatim.
4. Restore the original fingerprint, restart the client, clean up with
   `cleanup-fixture` if a job remains, re-enable the local node.

### Fake local node
1. Disable the ssh node: `nomad node eligibility -disable $SSHN`.
2. Stop the backend. Replace `$RUN_HOME/data/nomad-host-identity` with different
   content (mode 600), start the backend.
3. Launch a task. Assert it is placed on `LOCAL` and then refused with "the node
   host identity does not match this host", never falls back to SSH, and no
   agentctl is started. Record inventory phase and marker verbatim.
4. Restore the token, re-enable the ssh node, clean up.
