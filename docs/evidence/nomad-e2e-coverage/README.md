# Nomad e2e coverage round: scenarios and status

Live evidence for every scenario below is PENDING the operator's run on the laptop fixture. Offline
tests (`python3 -m unittest discover -s scripts -p 'test_nomad*.py'`) cover each driver branch only.

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
