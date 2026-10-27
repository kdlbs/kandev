# Remaining disposable-fixture proof

These are operator instructions for a separately approved laptop or isolated VM window.
They were not run during closeout. Do not run them from Tank or against the actual Tank/Neo
pool. No production service, shared workflow profile, or production credential is needed.
The current default-mode results do not prove any of the three cases below.

## TLS and scoped ACL

Use a fresh two-client Nomad fixture: one local client and one SSH client, a private server
bound to loopback, fresh data directories and unused ports. Both clients must belong to
node pool `kandev`, with the normal `kandev.attach` and transport/host-identity metadata.
Reserve host headroom on the local client. The runner can change node eligibility and
scheduler settings, so neither client may serve unrelated jobs. The runner starts its own
headless Kandev on an unused port with a private home; port 8817 is excluded.

From the candidate checkout, prepare a runtime bundle and TLS material. Choose new absolute
paths outside every checkout for `BUNDLE`, `MATERIAL` and `RUNS`; `MATERIAL` must not exist.
`RUNS` must exist, be owned by the operator, and not be group/world writable.

```sh
umask 077
BUNDLE=/absolute/disposable/nomad-bundle
MATERIAL=/absolute/disposable/nomad-tls
RUNS=/absolute/disposable/nomad-runs
mkdir -p "$RUNS"
make runtime-bundle RUNTIME_BUNDLE_DIR="$BUNDLE" RUNTIME_VERSION=0.0.0-nomad-fixture
scripts/nomad-native-e2e security-material --out "$MATERIAL"
```

Use an OpenSSL supporting `req -addext`; this command's compatibility with the operator's
macOS OpenSSL remains unverified. On failure, stop and retain the diagnostic. The generated
`server.hcl` is a fragment, not a complete server/client installation. Have the operator
start only the new fixture server and its two clients with their own data directories,
ports, native execution enabled and the generated TLS/ACL fragment. RPC TLS is disabled in
that fragment; it proves HTTP CA verification only, not mutual RPC TLS. Do not restart an
existing Nomad service to apply it.

Bootstrap ACLs on that fixture only. Store the resulting management token directly in a
private owner-only file without printing or committing it. The runner uses that token for
setup/assertions/teardown, issues the scoped executor token, and revokes it on cleanup.
Provision namespace `kandev` and node pool `kandev` on the disposable server. Compare
`scripts/fixtures/nomad/kandev-executor.policy.hcl` with an operator-supplied deployment
policy export using `diff -u`; record differences before interpreting this as policy parity.
No deployment policy was read during closeout.

Create an owner-only JSON bootstrap config, using actual fixture-only values for every
placeholder (the example is deliberately not runnable as pasted):

```json
{
  "schema_version": 1,
  "kandev_binary": "/absolute/disposable/nomad-bundle/bin/kandev",
  "nomad": {
    "address": "https://127.0.0.1:14646",
    "token_file": "/absolute/disposable/management-token",
    "namespace": "kandev",
    "region": "global",
    "datacenter": "dc1",
    "node_id": "<fixture-ssh-client-node-uuid>",
    "platform": "<fixture-worker-os/arch>"
  },
  "ssh": {
    "target": "<fixture-user>@localhost",
    "port": 12222,
    "host_fingerprint": "SHA256:<fixture-host-key-base64>",
    "identity_file": "/absolute/disposable/ssh-key"
  },
  "workspace_root": "/absolute/disposable/workspaces",
  "allocation_root": "/absolute/disposable/nomad/alloc",
  "cpu_mhz": 100,
  "memory_mb": 128,
  "repository": {"path": "/absolute/disposable/source-repo", "base_branch": "main"},
  "expected_file": "README.md",
  "temp_root_parent": "/absolute/disposable/nomad-runs",
  "pool": {
    "node_pool": "kandev",
    "placement": "spread",
    "memory_max_mb": 128,
    "host_identity_token_file": "/absolute/disposable/host-identity"
  },
  "security": {
    "tls": true,
    "acl_policy": "/absolute/candidate/scripts/fixtures/nomad/kandev-executor.policy.hcl",
    "ca_file": "/absolute/disposable/nomad-tls/ca.pem"
  }
}
```

The binary path must match the bundle's actual layout, with the bundle's runtime metadata
and agentctl artifacts preserved. SSH key, management token and host identity files must be regular,
non-symlink files owned by the operator with mode 0600. The source repository must be outside
the Kandev checkout and contain the expected file on the named branch. Allocation and
workspace roots must match the fixture clients, and be separate from all Kandev homes/data.
Select reservations that fit the fixture. Add `ssh_client: {"pid": <fixture-client-pid>,
"config": "/absolute/disposable/client.hcl"}` for `pool-partition`; that PID must identify
only the disposable SSH Nomad client.

After approval of the exact fixture targets, run one scenario at a time, stopping on the
first failure. Each invocation creates a fresh Kandev fixture and ends through production
cleanup; it writes `evidence.json` under the printed fixture root.

```sh
CFG=/absolute/disposable/tls-bootstrap.json
scripts/nomad-native-e2e preflight --config "$CFG"
scripts/nomad-native-e2e run --require-real --create-fixture --scenario pool-placement --config "$CFG"
scripts/nomad-native-e2e run --require-real --create-fixture --scenario pool-backend-restart --config "$CFG"
scripts/nomad-native-e2e run --require-real --create-fixture --scenario pool-binpack-refused --config "$CFG"
```

These three commands sample transport/CA, recovery and scoped scheduler-policy behavior;
they do not certify all 14 scenarios. Repeat other default scenarios individually with
the security config for full mode parity. `pool-register-unknown` retains a plaintext
loopback fault-proxy listener even in security mode; its upstream uses the configured CA.
Record that distinction and do not describe it as encrypted end-to-end.
On failure, use only the cleanup command printed for that invocation:

```sh
scripts/nomad-native-e2e cleanup-fixture --fixture-root /absolute/printed/fixture-root
```

Retain sanitized evidence, exact candidate SHA, commands, exit codes, scenario, TLS/ACL
assertions and successful production teardown. Stop only the fixture-owned Nomad/SSH
processes after teardown. Never include token files, runtime homes or environment dumps.

## SFTP upload cancellation (fix 7)

The current evidence is the Go test below. There is no live upload-cancellation scenario
in the runner; do not substitute `pool-partition`, which interrupts an already-running
session rather than an in-flight upload.

```sh
cd apps/backend
go test -trimpath -count=1 ./internal/agent/runtime/lifecycle/ -run '^TestNomadSSHTransportUploadReturnsOnContextDeadlineWhenLinkIsWedged$'
```

A live proof needs an additional test-first runner scenario: use a fixture-only SSH/SFTP
relay with a bounded stall after the upload starts, a staging file large enough to keep
the upload in flight, and a caller deadline. Assert timely return, released SSH/SFTP
resources, no registration/publication after cancellation, and production rollback. Record
the injected stall, upload-start acknowledgment, deadline and return times, resource
inventory and cleanup. This harness extension must exist before a runnable E2E command
can be supplied. No relay or live upload was manipulated during closeout.

## Agent-to-Kandev tool call after restart

`pool-backend-restart` proves native instance reattachment. The default inert ACP peer
returns text and does not call Kandev tools; a reverse-tunnel absence is not a successful
tool-call assertion. Repeating that scenario, including with `agent_profile_name`, cannot
by itself close this gap.

A disposable proof needs a test-first extension of that scenario before its archive
cleanup: use an explicit fixture-only tool-capable agent/profile, capture execution,
allocation, agentctl and conversation identities, restart only its owned backend, then
have that same agent make an authenticated read-only Kandev tool call, for example
`list_task_sessions_kandev` with the fixture task's ID. Correlate it to a unique nonce in
the post-restart probe prompt. Assert the successful tool result in the resumed conversation and a matching
backend tool invocation bound to the original task/session/execution. An operator-side
HTTP request is insufficient. Keep callback credentials private; never use
`step_complete_kandev` as the probe because it can move a task. Finish through production
cleanup and record both identities and the sanitized tool-result evidence. The present
runner has no command that asserts this, so no exact live invocation is claimed yet.
