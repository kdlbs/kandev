---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-001
  - REQ-EXECUTORS-NATIVE-WORKER-002
  - REQ-EXECUTORS-NATIVE-WORKER-003
  - REQ-EXECUTORS-NATIVE-WORKER-004
  - REQ-EXECUTORS-NATIVE-WORKER-005
  - REQ-EXECUTORS-NATIVE-WORKER-006
  - REQ-EXECUTORS-NATIVE-WORKER-007
  - REQ-EXECUTORS-NATIVE-WORKER-008
---

# Remote executor plugins: native worker capabilities system design

## Purpose and boundaries

This design extends the [remote executor plugin design](remote-executor-plugins.md). That document remains the
authority for provider RPCs, leases, inventory, and plugin lifecycle; this one adds opt-in capabilities.
Names marked **new** do not describe existing APIs. It amends two exclusions of the base requirements
(no tunnel service, no moving built-ins): the tunnel here is a host-initiated stream to an installed provider,
not a public relay, and built-ins still do not move. See the [boundary ADR](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

Core keeps task, session, ownership generation, agentctl authorization, repository policy, drain, and
cleanup admission. The provider keeps placement, worker transport, provider fences, and its own state.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-EXECUTORS-NATIVE-WORKER-001 | Capability negotiation; Compatibility |
| REQ-EXECUTORS-NATIVE-WORKER-002 | Workspace descriptor |
| REQ-EXECUTORS-NATIVE-WORKER-003 | Repository intent |
| REQ-EXECUTORS-NATIVE-WORKER-004 | Host-owned tunnel; API forwarding |
| REQ-EXECUTORS-NATIVE-WORKER-005 | Drain receipt |
| REQ-EXECUTORS-NATIVE-WORKER-006 | Destroy and recover checkpoints |
| REQ-EXECUTORS-NATIVE-WORKER-007 | Durable bootstrap; Ready publication |
| REQ-EXECUTORS-NATIVE-WORKER-008 | Crash barrier and fixture |

## Decisions

The owner confirmed D1, D2, D3, and D6 on 2026-10-28; the others are settled here. The capability registry, negotiation, and
versioning are specified first in the [versioning design](remote-executor-plugin-capability-versioning.md); admission
and placement in the [admission design](remote-executor-plugin-admission.md); operations, launch policy, preparation hints,
in the [operations design](remote-executor-plugin-operations-migration.md); profile migration is deferred (see Out of scope there).

| # | Decision | Rationale |
| --- | --- | --- |
| D1 (confirmed) | Host-owned dialer: a bidirectional gRPC `OpenExecutorTunnel` stream bound to installation, environment, generation, and port, for control traffic. Reverse direction uses a provider-forwarded API URL. | Relaxing HTTPS or loopback checks (a) weakens validation for every provider and cannot express SSH; a plugin loopback listener (b) opens an unauthenticated port on the backend host and races the port allocator. A host-initiated stream keeps identity binding, cancellation, and generation fencing in core. |
| D2 (confirmed) | Ship both preparation modes. `core` stays the default; `provider` is added in the same release through repository intent, with the extensible contract of REQ-EXECUTORS-PLUGIN-PREPARATION-001 so a mirror, sparse checkout, or pull-request checkout needs no further core change. | The first release must keep a local-worktree checkout for a local node. Core clone alone would change that observable behavior. The PLAN suggested deferring only if the assertion could change; it cannot cheaply. |
| D3 (confirmed) | Keep built-in executor code (and the fork's native executor, which is not on main) until the plugin passes its live scenarios; those scenarios live in the vendor plugin repository and no host behavior depends on them. Never both on one environment. | Removing it first strands retained inventory. Confirmed by the owner; the drain-based migration operation is deferred to a follow-up card (see Out of scope in the operations requirements). |
| D4 | One ledger in core (`executors_running` envelope plus existing recovery claims); no provider database authority. | The base design owns inventory. Dialect tests for every new SQL path. |
| D5 | This document and its requirements amend the executor spec before code. | Contract change precedes build. |
| D6 (confirmed) | Provider SDK stays a pinned-reference dependency for v1; do not publish a module yet. | No second external plugin exists to justify a versioning commitment. Revisit then. |
| D7 | A worker-pool provider's state stays in `state_json` under the 32 KiB envelope cap, at most 12288 bytes of it. The fixture proves a representative 12 KiB state. The cap is raised only if a real provider cannot fit. | Avoids a second state channel. |
| D8 | A vendor harness lives in the vendor plugin repository; only the generic crash barrier lives here. | Keeps vendor names out of core. |

## Capability negotiation

**New** manifest keys under each `executor_providers[]` entry, validated in `internal/plugins/manifest/executor_provider.go`:
`requires_host_capabilities` (a list of `{name, version}` from the registry in the versioning design), `workspace_preparation` (`core`|`provider`), `runtime_api_url`
(`host_public`|`provider_forwarded`), and `destroy_timeout_seconds` (30 to 300). The host exports its implemented set
as a constant beside `CurrentExecutorProviderContractVersion`; install, upgrade, and enable compare the two before
activation. Cross-field rules: `workspace_preparation: provider` requires `repository_intent` and `workspace_descriptor`;
`runtime_api_url: provider_forwarded` requires `api_url_forward`; `destroy_timeout_seconds` over 30 requires `drain_receipt`.

The admitted allocation writes `negotiated_capabilities` (sorted `name@version`, see the versioning design) into the plugin envelope. Every later
decision (recovery, attach, destroy, lease validation, response validation) reads the envelope. A response field
without its capability is `conflict` (AC-EXECUTORS-NATIVE-WORKER-001.6). A host lacking a snapshot capability maps the
provider to `unavailable` through the existing availability path, never `absent`.

## Workspace descriptor

**New** `ExecutorResourceDescriptor` fields `workspace_path` and `workspace_identity` (additive proto fields).
**New** `validatePluginExecutorWorkspace(platform, path, identity)` (absent on main) is a pure function in `executor_plugin_validation.go`;
it never touches the filesystem and parses by the descriptor's `platform` (`windows/*` drive-letter paths; others POSIX).
Validation runs in `validatePluginExecutorResourceForProvider`, so provision, attach, recovery, and checkpoint share it.
The envelope gains `workspace_path`, `workspace_identity`, and `workspace_preparation`. The constant
`pluginExecutorWorkspacePath` remains the value for non-declaring providers. `createPluginAgentctlInstance`,
`finishPluginExecutorLaunch`, and the recovery builder read the recorded path from the envelope.
The path is normalized and checked for uniqueness among the installation's non-terminal environments inside the descriptor write (AC 002.7). Descriptor immutability is enforced in the same envelope-revision CAS that guards checkpoints: a candidate whose path
or identity differs from a non-empty recorded value for that generation fails before the write.
A launch-phase checkpoint (`provisioned`, `bootstrapping`) arriving while no descriptor is recorded may omit the descriptor (`resolvePluginExecutorEarlyWorkspace`); a descriptor it does carry is validated and recorded, and the provision response and every later use stay strict.

## Repository intent

**New** `ProvisionExecutorEnvironmentRequest.repositories` (message `ExecutorRepositoryIntent`: id, locator, branch,
base branch, checkout ref) and `agent_env` (map). The intent is built from the same workspace repository projection
that `ExecutorCreateRequest` carries; locators go through the existing credential scrubbing. `agent_env` starts from the
instance environment that `agentctlInstanceRequest` uses and keeps only the profile's plain `EnvVars` and the host allowlist (`KANDEV_TASK_ID`, `KANDEV_SESSION_ID`, `KANDEV_INSTANCE_ID`, `KANDEV_TASK_ENVIRONMENT_ID`, `KANDEV_AGENT_PROFILE_ID`, `KANDEV_EXECUTION_PROFILE_ID`, `LANG`, `LC_ALL`, `TZ`; a constant in code, tested against this list), dropping managed credentials (`internal/githubauth`) and any name matching the secret patterns of AC 003.4, sorted by name; over 64 entries or 16 KiB is `invalid_config`. Both are included in
`pluginExecutorInputDigest`.

For `workspace_preparation: provider` the descriptor gains `repositories` (message `ExecutorRepositoryState`: id,
relative path, branch, head commit). **New** `verifyPluginExecutorRepositoryInventory` compares the set to the intent by
repository ID, requires every path to be inside `workspace_path` after platform-aware cleaning, and persists the
result in the envelope. In this mode `startPluginExecutorAgentctl` skips core materialization; `core` mode is unchanged.
Launch admission rejects managed Git credentials with `provider` preparation before allocation.

## Host-owned tunnel

**New** RPC `OpenExecutorTunnel(stream ExecutorTunnelFrame) returns (stream ExecutorTunnelFrame)` on the plugin service
and the optional SDK interface `ExecutorTunnelPlugin`. The first frame is a header (context, port, binding nonce);
the provider answers with exactly one acknowledgement frame; subsequent frames carry at most 64 KiB of bytes, half-close, or an
error code. The host is always the opener. Frame rules: data from either side before the acknowledgement, a second acknowledgement, or an oversized frame ends the stream as
protocol error (reported unavailable); either side may half-close and the stream ends when both have, or on an error frame or context end; each call is one gRPC stream and
there is no application stream ID to reuse; an acknowledgement arriving after the 15 s timeout is ignored because the stream is already closed. The 64-stream admission is a compare-and-increment under the registry mutex.

**New** lease field `transport`. For `host_tunnel` the endpoint validator is bypassed only for the scheme and loopback
checks, `base_url` must be empty, and header rules still apply. `agentctl` builds a `net.Conn` through a
`TunnelDialer` supplied by a host-owned lease manager; the HTTP transport and a per-connection copy of the WebSocket
dialer use it with a synthetic authority that is never resolved. The dialer checks the requested port against
`{control port, recorded instance port}` before opening a stream, writes nothing before the acknowledgement, and
closes the stream when the request context ends. A tunnel registry keyed by installation, environment, generation
counts streams (limit 64 per environment across its control and instance ports) and closes all on dispatch generation change, disable, plugin exit, or environment release. A provider call that leaves the ownership generation unchanged neither closes nor re-binds streams or leases.
Gateway `vscode_proxy.go`, `port_proxy.go`, and `port_tunnel.go` already obtain the lease transport; they take the
tunnel dialer from the same resolver. Authentication to agentctl stays the host-applied bearer token.

## API forwarding

For `provider_forwarded` the launch path replaces `validatePluginRuntimeAPIURL` with a check that the backend's API
base URL is known. The bootstrap JSON gains `host_api_url` (the backend's own listener, loopback allowed) and
`idle_timeout_seconds`. The provider arranges worker-side reachability (for example a reverse forward) and starts
agentctl with the worker-local URL. No listener is added. The reachability proof is a **new** probe, since main has only the agentctl `Health` call, which proves the host-to-worker path: the host
registers a probe token (random, in memory, 60 s, bound to the environment, deleted when the probe ends) on a **new** route `GET /api/v1/runtime/ping` that answers 204 to a matching
token header and 401 otherwise, then asks agentctl through the control transport (**new** route `POST /runtime/probe-host-api` with the URL `<host_api_url>/api/v1/runtime/ping` and the token) to request it. The auth middleware blocks unauthenticated requests by default, so `isPublicPath` in `internal/auth/httpmw/middleware.go` gains `/api/v1/runtime/ping` for `GET` only (the token is the credential, checked in the handler), and the pinning test in `middleware_test.go` gains the entry plus a negative case for another method and a sibling path. A matched token marks the probe reached on the host, so a lost 204 followed by a second attempt is not a failure. Two attempts, two
seconds apart, 10 seconds in total; a failure rolls provisioning back with the cleanup reason `api_unreachable`.

## Drain receipt

Agentctl **new** route `POST /runtime/seal-and-drain` (authenticated with the control token, body carries the expected
runtime identity and claim) stops accepting work, stops managed process groups, and returns a receipt; the
`instance.Manager` and control client carry the generic part ported from the native branch. A second call with the
same claim returns the stored receipt; a call under another claim during a drain is `conflict`, and a drain that stops only some groups or hits the deadline returns no receipt. Lifecycle adds **new** `drainBeforePluginDestroy` ahead of
`destroyPluginExecutorRecord`: under the cleanup claim it calls drain with a 60 s deadline, writes the host-only
`drain_receipt` checkpoint (CAS), then calls destroy with **new** `DestroyExecutorEnvironmentRequest.drain_receipt`
and `drain_outcome`. The (cleanup reason, drain outcome) pairs a provider may rely on:

| Host state when the destroy is sent | Outcome | Meaning for the provider |
|---|---|---|
| The drain answered with a receipt | `drained` | The runtime was sealed under this claim; the receipt names the runtime identity and this acquisition's claim. |
| The agentctl credential is known, the connection was refused or the runtime identity no longer matches | `unreachable` | The runtime that was drained is gone. |
| The credential is known, the reply was lost or the 60 s deadline passed | `uncertain` | Retain unless the provider can prove the runtime is stopped. |
| An agentctl handshake was started and no usable credential remains (secret lost or unreadable) | `unreachable` | The host cannot drain; it makes no claim about a runtime. |
| No handshake was ever started, reason `launch_failed` | `not_attempted` | Positive proof that no runtime ever existed. |
| No handshake was ever started, any other reason (`task_cleanup`, `task_environment_reset`, `recovery_cleanup`, `bootstrap_consumed`, `incomplete_bootstrap`) | `unreachable` | `not_attempted` is never sent: the host did not drain and a runtime may exist. |

`not_attempted` is therefore sent only with `launch_failed`, and only when no handshake was ever started. The host records
`HandshakeStarted` in the envelope before every handshake, so a crash between the provider's ready and the host's
provision response still counts as a handshake. A runtime core that was never recorded (a hand-off, or a crash between
provider ready and the provision response) is destroyed under `launch_failed` only when no handshake happened;
otherwise it follows the rows above. A failed-launch rollback after a handshake drains with the in-memory token, or the persisted one, or sends
`uncertain`. A successor claim re-runs the drain (agentctl returns a fresh receipt for an already sealed runtime) and a replayed destroy under the same claim carries the stored outcome.

Claim identity: every claim acquisition has a unique operation id (the operation prefix, a per-process epoch and a nonce). A crashed process's claim is superseded by a successor with the same prefix and another epoch; a claim of a live epoch is never superseded. `cleanup_claim` in the destroy request and the drain `RequestID` carry that id, a provider echoes it as `claim` in `CheckpointExecutorResourceRequest`, and the host compares the echo with the active claim, so a late write from an earlier acquisition is refused. Each attempt produces a fresh drain receipt. The envelope keeps the claim's acquisition stamp across host writes. At most one provider operation is live for one (environment, generation): the core bumps the generation before it provisions the environment again, so a provider can fence on that pair. The destroy deadline is
`destroy_timeout_seconds`. For declaring providers the bootstrap's `idle_timeout_seconds` is 0, mapped to
`KANDEV_ACP_IDLE_TIMEOUT` disabled in the agentctl startup config.

Cleanup ownership: `StopInstance` for archive and delete reasons only stops agentctl; the task-environment cleanup
job calls `DestroyTaskEnvironment`, the single authority that drains and destroys from the stored record, with the
agentctl credential revealed from the runtime secret the record references. Only a failed-launch rollback cleans up
inside `StopInstance`. `DestroyTaskEnvironment` returns a retryable error while the same process still has the
record's launch in flight (an in-memory set keyed by execution ID), so it cannot settle "absent" before the provider
publishes the resource. Recover `absent` means the provider proves the operation's resource is gone and its worker cleaned; the host then deletes the row, so a provider that cannot prove this answers `found` or `unknown`, and fences the operation id so that `absent` means the resource will never exist. Recover results for `found` resources must carry `state_version`, on the launch-recovery
and the cleanup path. When a failed launch can neither take the cleanup claim nor checkpoint the resource it
provisioned, the launch records the resource durably in `unrecorded_plugin_executors` (keyed by execution ID, no foreign key,
written through `RecordUnrecordedRuntimeInventory` without the environment claim), logs the launch cause and a
`handed_off` outcome, and returns a "not recorded" error without calling the provider: a provider checkpoints
`cleanup_progress` under a persisted claim before it mutates, so a destroy under a claim that has no row would
fail closed. `DestroyTaskEnvironment` lists the environment's records, inserts the `allocating` inventory row a
record needs when none exists (so claim checkpoints are accepted), destroys the resource under its own claim, and
deletes the record only after a confirmed destroy. A record whose destroy is unconfirmed stays and fails the cleanup; the cleanup re-lists the environment's records after its loop and fails while any remains, a record whose inventory is already `absent` is settled, and a transfer or delete of the environment row is refused while a record remains. A bounded periodic sweep (every 5 minutes, at most 16 targets per pass, owned by `StartCleanupSweep` and `StopCleanupSweep`) retries the records and the retained `cleanup_pending` rows under the cleanup claim by the same path. Records survive a restart. Provider calls that need the cluster credential (attach, inspect, resolve, tunnel open)
resolve recorded secrets on each call; the host does not write them into the inventory and rejects provider state that echoes one. Under the e2e mock profile only, a `file://` or
loopback-host remote parses as the synthetic `e2e-local` provider so a local rig can use a repository, passes the
managed Git credential identity check, is cloned on the host without credentials (`repoclone.E2ELocalRemote`), is
accepted as a cloneable workspace-source locator, and takes no Git credential lease: the launch issues none and adds no
broker environment (`repoclone.IsE2ELocalProvider`). Production keeps the provider allowlist, the credentialed HTTPS
clone and the HTTPS-only broker URL.

Known limits of slice 1. The drain receipt names the runtime identity the host recorded, so it proves the host's claim and not the provider's view of the runtime; a binding that does not start from the host's own record needs a provider-signed runtime identity and is left to a later slice. A quarantine decision of a provider has no operator release path in this slice, so it is permanent until someone acts by hand; a readmit path belongs to the maintenance work (slice 3).

## Destroy and recover checkpoints

`pluginExecutorCallbackRequest` today matches only an active launch. It gains a claim-scoped path: the request context
must match the active cleanup claim or recovery claim row (`internal/task/recoveryclaim`; main identifies a claim by its environment and
`claim.OperationID`, which destroy requests already send as `cleanup_claim`, so no claim token is added)
for the same environment, owner, generation, and operation, and the dispatch generation must be current. Phases
`cleanup_progress` and `recovery_progress` join the allowed set when the snapshot has `destroy_checkpoints`; each claim type accepts only its own phase,
and a provider `sequence` per claim (new additive field, strictly increasing) enforces order. The claim kind, last sequence, and digest live in the envelope, written in the claim-taking transaction (AC 006.8); the claim table has no kind column and gains none. Check order: shape, claim identity, sequence (an equal sequence with an equal digest is an idempotent success even after the revision moved), then the revision CAS. The write is the existing `CheckpointPluginExecutorInventory` CAS, so two callbacks
at one revision serialize to one winner. Settlement requires `confirmed_absent` and a successful latest checkpoint.

## Durable bootstrap and ready publication

The nonce and the agentctl token are stored as secret references in `executors_running.metadata`, using the existing
`MetadataKeyAuthTokenSecret` and the existing `MetadataKeyBootstrapNonceSecret` (`manager_execution.go`, today written for container launches and read by `revealRuntimeSecret`); no key is added. Plugin
rows carry the nonce key only for `durable_bootstrap`, the generic resume and reuse paths shall not copy it to another session's row, and the secret is deleted when the record settles;
the nonce reference is persisted before `ProvisionExecutorEnvironment`, the token reference before
`createPluginAgentctlInstance`. Reads go through `revealRuntimeSecret`. Recovery replaces the `incomplete_bootstrap`
branch in `attachRecoveredPluginExecutor` for `durable_bootstrap`: reveal the nonce, handshake, adopt or create the
instance after verifying owner, workspace path, and port, then publish. A repeated handshake that agentctl accepts continues to readiness; one it rejects as consumed ends in cleanup
with reason `bootstrap_consumed`. The envelope records `handshake_started` before each handshake.

Publication is a **new** single repository method `PublishRecoveredTaskEnvironment` (absent on main; ported from the fork reference, then adapted) that, under the recovery claim, verifies
operation, task, session, environment owner and generation, runtime identity, descriptor, and repository inventory and
flips the environment from `creating` to `ready`. It is idempotent on a ready row with identical evidence. It is generic:
it accepts verified evidence and carries no provider data. A racing archive, delete, or reset that no-ops records `cleanup_requested`, which the cleanup sweep acts on after the claim is released (AC 007.9).

## Crash barrier and fixture

`lifecycle/crash_barrier.go` defines the closed barrier names and `armBarrier(name, mode)`; the arming route sits under
`/api/v1/e2e/` and is mounted only when `KANDEV_E2E_MOCK=true`, like the child-kill route. Barrier calls in production
paths are a single cheap lookup that returns immediately when none is armed. The closed names are `after_side_effect`, `after_drain`, `after_handshake`, `before_publish`, `after_destroy_confirm`,
`after_placement_persist`, `before_placement_commit`, and `after_cancel_mark`; the first caller to reach an armed name fires it, and arming an armed name returns `conflict`. `cmd/plugin-fixture` gains every
capability behind fixture configuration and a local in-process tunnel target, and its conformance test runs each
capability and barrier through the managed plugin process.

## Compatibility

Proto fields are additive; the manifest keys default to current behavior. The host mixed-version matrix: new host with
old plugin (no declaration, unchanged), old host with new plugin (fails the capability check before activation),
new host with upgraded plugin and retained old environments (snapshot governs). Rollback to a plugin without a
capability while retained environments hold it is allowed and makes that provider unavailable for them, not absent; a rollback that would strand a recorded
state version is rejected (versioning design, AC CAPABILITY-003.3).

## Data model

No new table and no DDL migration. The `plugin_executor` envelope in `executors_running.metadata` gains fields:
`negotiated_capabilities`, `profile_snapshot_digest`, `workspace_path`, `workspace_identity`, `workspace_preparation`, `repositories`, `drain_outcome`,
`drain_receipt_id`. The envelope stays under 32 KiB through the per-field maxima and the 20480-byte host reserve of the [versioning design](remote-executor-plugin-capability-versioning.md#state-budget); an
oversize write is rejected as `state_too_large`. The workspace path is stored once, in the envelope. `repositories` persists `id`, `relative_path`, and `head_commit` only. Secret references add metadata keys, not columns. Publication touches
the existing task environment and repository projection rows.

- **SQLite:** the single immediate-mode writer serializes checkpoints and publication; every new method runs in one transaction.
- **PostgreSQL:** each read-modify-write of the envelope takes `pg_advisory_xact_lock(hashtextextended('plugin_inventory:'||environment_id, 0))`
  before its first read. One namespace covers checkpoint, publication, drain, owner, placement, and descriptor writes, so no two mutations of one row can hold different keys; checkpoint keeps the revision CAS. The workspace uniqueness check additionally takes `'plugin_workspace:'||installation_id` first, always in that order.
- Both dialects run the new race tests; PostgreSQL ones are gated on `KANDEV_TEST_POSTGRES_DSN`.
- Replay: no schema change, so the replayable-migration tests are unaffected; envelope decode tolerates old rows (absent fields).

## Interleavings and failing-first tests

Each test is written first, fails against current main, and names its AC with `@covers`.

| Interleaving | Required outcome | Tests (level) |
| --- | --- | --- |
| Delayed checkpoint from released claim after a successor claim (006.2) | rejected, inventory unchanged | lifecycle unit; sqlite and PostgreSQL race |
| Two checkpoints at one revision (006.5) | one commits, one `conflict` | sqlite and PostgreSQL multi-connection `-race` |
| Drain reply lost, then retry under same claim (005.4, 005.2) | no receipt invented; same receipt on retry; destroy carries outcome | agentctl handler; lifecycle with fault-injecting client |
| Receipt from earlier claim presented to later claim (005.5) | rejected | lifecycle unit |
| Crash after provider side effect before checkpoint (006.7) | recovery re-destroys idempotently; no replacement | barrier e2e over the fixture |
| Crash after drain before destroy (005.1) | recovery re-drains or reuses the receipt, then destroys | barrier e2e |
| Crash after handshake before token persist (007.5) | `bootstrap_consumed` cleanup, no duplicate, no ready | barrier e2e |
| Crash before nonce persist vs before provision (007.1) | no provision without a persisted nonce | lifecycle unit ordering |
| Crash before ready publication (007.3) | recovery publishes once; repeat is a no-op | barrier e2e; sqlite and PostgreSQL |
| Archive racing recovery (007.6) | one claim, loser no-ops | sqlite and PostgreSQL race |
| Archive during provisioning (005.4) | cleanup defers; a resource the launch cannot record is destroyed by the launch | lifecycle unit with slow-provision and rotated-checkpoint fakes |
| Plugin restart with open tunnel (004.7) | streams close, new lease, same environment | fixture over the process boundary |
| Duplicate or late acknowledgement, data before acknowledgement, 65th open (004.3, 004.8) | stream closed as unavailable; 65th open `rate_limited`, others unaffected | tunnel unit with fake provider |
| Cancel during stream open (004.3, 004.6) | no bytes before ack, stream closed, count zero | tunnel unit with fake provider |
| Two environments of one installation record one normalized path (002.7) | second commit gets `conflict`, resource cleaned | sqlite and PostgreSQL race |
| Provider returns an undeclared field (001.6) | `conflict`, resource cleaned | lifecycle unit |
| Descriptor changed on attach (002.4) | `conflict`, session not started | lifecycle unit |

Remaining ACs are covered by validator tests (manifest, workspace path per platform, repository inventory set equality,
agent env filtering), the unchanged fixture conformance test (001.3), and an `-race` run over the changed packages.

## Observability

Closed-label counters join the existing executor-plugin metrics: `executor_plugin_capability_rejected_total`
(`capability`, `phase`: install, upgrade, enable, allocate, echo; one metric, defined in the [versioning design](remote-executor-plugin-capability-versioning.md#observability)), `executor_plugin_tunnel_open` gauge and `executor_plugin_tunnel_closed_total` (`reason`: cancel, restart,
limit, ack_timeout, provider_error), `executor_plugin_drain_total` (`outcome`), and `executor_plugin_publish_total`
(`outcome`). Barrier firings log with the barrier name. Every destroy call logs one `plugin executor destroy` line, on the cleanup, launch-rollback and hand-off paths, with the cleanup reason, the drain outcome sent, whether a receipt was attached, the cleanup claim, the provider's `confirmed_absent`, error code and message ID, and which unconfirmed condition fired (`call_error`, `error_response`, `not_confirmed_absent`, `claim_checkpoint_missing`); it carries no secret value. Plugin, task, environment, and execution IDs are never labels.

## Security

The tunnel is host-initiated, bound by a binding header that the provider must echo, and limited to two allowed ports;
the provider cannot reach the host or another environment. Agentctl authorization and the nonce never enter the
envelope, provider state, or logs. `agent_env` is non-secret by construction and filtered, with a test that seeds every
secret class. A drain receipt carries no authority beyond the claim that produced it. Plugins remain trusted binaries;
none of this is an isolation claim.

## Related decisions

- [Remote executor plugin boundary](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md)
- [Cleanup inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
- [Ownership generations](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
- ADR for D1, D2, D3, D6 and the medium-term decisions (work order 00).
