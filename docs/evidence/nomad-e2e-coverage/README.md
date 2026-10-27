# Nomad e2e coverage round: scenarios and status

## Live results (default mode, laptop fixture; operator-reported, raw logs not committed)

All 14 default-mode scenarios passed on the laptop fixture: pool-placement, pool-two-node, pool-queue,
pool-refusals, pool-node-quarantine, pool-partition, pool-pre-ready-recovery, pool-backend-restart,
pool-idle-outage, pool-unplaced-cleanup, pool-register-unknown, pool-force-stop, pool-checkout-branch and
pool-binpack-refused.

- 11 passed at 17ffa37d6c. The other 3 failed there on runner-side defects and passed after fixes:
  - pool-unplaced-cleanup (dfeb528931): the runner deleted the task without the deletion-preview
    confirmation; it now requests the preview and sends the confirmation header.
  - pool-backend-restart and pool-idle-outage (8908d2ab75, then 1222b56224): the tunnel steps are now
    conditional on a recorded reverse API tunnel, which native Nomad launches do not open; the
    process-tree check now requires the same agentctl root, nothing missing, and any new process
    descending from that root (new processes are recorded as `new_processes`).
- The unplaced-cleanup run also exposed a product race: a launch rollback running after archive cleanup
  had reached absent re-drained from its stale request. Rollback now re-reads the inventory row under
  the cleanup claim (fixed in dc8afb7e70, covered by a Go test that failed first).

Not proven:

- TLS and scoped-ACL mode (`security` block) has not been run live.
- The Go-side SFTP upload cancellation (fix 7) is covered by a Go test only.
- The agent-to-Kandev path after a backend restart is not exercised for native launches: they open no
  reverse tunnel, so the restart scenarios prove instance reattachment only.

Offline tests (`python3 -m unittest discover -s scripts -p 'test_nomad*.py'`) cover each driver branch.

Each scenario is one invocation that creates its own fixture, induces the condition, asserts,
cleans up through the production path and writes evidence. On a failed assertion it stops without
retrying, keeps the fixture and prints the operator cleanup command.

Run form (`<cfg>` is the bootstrap config; add the `security` block for TLS and scoped-ACL mode):

```
scripts/nomad-native-e2e run --require-real --create-fixture --scenario <name> --config <cfg>
```

| Scenario | Fix covered | Needs |
| --- | --- | --- |
| pool-pre-ready-recovery | 2, 9 | pool block, two nodes |
| pool-backend-restart | 3 | pool block, ssh node |
| pool-idle-outage | 1 | pool block; holds a single bounded 180 s outage |
| pool-unplaced-cleanup | 4 | pool block, `pool.host_identity_token_file` |
| pool-register-unknown | 5 | pool block, loopback Nomad |
| pool-force-stop | 6 | pool block, both node types |
| pool-checkout-branch | 8 | pool block, `repository.path` (local repo) |
| pool-binpack-refused | 9 | pool block, scheduler spread, no pool-level override |

Fix 7 (SFTP upload cancellation) stays covered by Go tests only.

TLS and scoped-ACL mode: `"security": {"tls": true, "acl_policy": "<abs file>", "ca_file": "<abs file>"}`.
`nomad.address` must be loopback `https`, namespace and node pool `kandev`, placement `spread`,
`memory_max_mb == memory_mb`; `nomad.token_file` holds the management token (setup, teardown and
assertions only). Kandev receives a token scoped by `scripts/fixtures/nomad/kandev-executor.policy.hcl`.
`scripts/nomad-native-e2e security-material --out DIR` writes the throwaway CA, server cert and
server config fragment. The policy file was written from the brief; diff it against the deployment
policy before relying on it.

Known limits: the runner does not start Nomad; `pool-idle-outage` observes survival only (the Go
side forces the idle timeout to 0); `pool-register-unknown` uses its own HTTP fault proxy and may
need an https-terminating proxy in security mode.

See [remaining-fixtures.md](remaining-fixtures.md) for the disposable TLS/ACL configuration,
exact supported commands, teardown, and the additional harness needed to prove live upload
cancellation and a native agent-to-Kandev tool call after restart. Those proofs remain open.

The [2026-10-28 closeout](closeout.md) records current offline/Go checks, the full backend
result, and exact failing-test comparisons against both clean required baselines.
