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
versioning are specified first in the [versioning design](remote-executor-plugin-capability-versioning.md); shared environments, admission,
and placement in the [shared design](remote-executor-plugin-shared-admission-placement.md); operations, launch policy, preparation hints,
and migration in the [operations design](remote-executor-plugin-operations-migration.md).

| # | Decision | Rationale |
| --- | --- | --- |
| D1 (confirmed) | Host-owned dialer: a bidirectional gRPC `OpenExecutorTunnel` stream bound to installation, environment, generation, and port, for control traffic. Reverse direction uses a provider-forwarded API URL. | Relaxing HTTPS or loopback checks (a) weakens validation for every provider and cannot express SSH; a plugin loopback listener (b) opens an unauthenticated port on the backend host and races the port allocator. A host-initiated stream keeps identity binding, cancellation, and generation fencing in core. |
| D2 (confirmed) | Ship both preparation modes. `core` stays the default; `provider` is added in the same release through repository intent, with the extensible contract of REQ-EXECUTORS-PLUGIN-PREPARATION-001 so a mirror, sparse checkout, or pull-request checkout needs no further core change. | The first release must keep a local-worktree checkout for a local node. Core clone alone would change that observable behavior. The PLAN suggested deferring only if the assertion could change; it cannot cheaply. |
| D3 (confirmed) | Keep built-in native executor code until the plugin passes its live scenarios. Never both on one environment. | Removing it first strands retained inventory. Confirmed by the owner; migration is the drain-based operation in the operations design. |
| D4 | One ledger in core (`executors_running` envelope plus existing recovery claims); no provider database authority. | The base design owns inventory. Dialect tests for every new SQL path. |
| D5 | This document and its requirements amend the executor spec before code. | Contract change precedes build. |
| D6 (confirmed) | Provider SDK stays a pinned-reference dependency for v1; do not publish a module yet. | No second external plugin exists to justify a versioning commitment. Revisit then. |
| D7 | Nomad-like provider state stays in `state_json` under the 32 KiB cap. The fixture proves a representative 12 KiB state. The cap is raised only if a real provider cannot fit. | Avoids a second state channel. |
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
`validatePluginExecutorWorkspace(platform, path, identity)` is a pure function in `executor_plugin_validation.go`;
it never touches the filesystem and parses by the descriptor's `platform` (`windows/*` drive-letter paths; others POSIX).
Validation runs in `validatePluginExecutorResourceForProvider`, so provision, attach, recovery, and checkpoint share it.
The envelope gains `workspace_path`, `workspace_identity`, and `workspace_preparation`. The constant
`pluginExecutorWorkspacePath` remains the value for non-declaring providers. `createPluginAgentctlInstance`,
`finishPluginExecutorLaunch`, and the recovery builder read the recorded path from the envelope.
Descriptor immutability is enforced in the same envelope-revision CAS that guards checkpoints: a candidate whose path
or identity differs from a non-empty recorded value for that generation fails before the write.

## Repository intent

**New** `ProvisionExecutorEnvironmentRequest.repositories` (message `ExecutorRepositoryIntent`: id, locator, branch,
base branch, checkout ref) and `agent_env` (map). The intent is built from the same workspace repository projection
that `ExecutorCreateRequest` carries; locators go through the existing credential scrubbing. `agent_env` starts from the
instance environment that `agentctlInstanceRequest` uses and removes entries that the managed-credential
classification (`internal/githubauth`) or the secret-classified set marks. Both are included in
`pluginExecutorInputDigest`.

For `workspace_preparation: provider` the descriptor gains `repositories` (message `ExecutorRepositoryState`: id,
relative path, branch, head commit). `verifyPluginExecutorRepositoryInventory` compares the set to the intent by
repository ID, requires every path to be inside `workspace_path` after platform-aware cleaning, and persists the
result in the envelope. In this mode `startPluginExecutorAgentctl` skips core materialization; `core` mode is unchanged.
Launch admission rejects managed Git credentials with `provider` preparation before allocation.

## Host-owned tunnel

**New** RPC `OpenExecutorTunnel(stream ExecutorTunnelFrame) returns (stream ExecutorTunnelFrame)` on the plugin service
and the optional SDK interface `ExecutorTunnelPlugin`. The first frame is a header (context, port, binding nonce);
the provider answers with an acknowledgement frame; subsequent frames carry at most 64 KiB of bytes, half-close, or an
error code. The host is always the opener.

**New** lease field `transport`. For `host_tunnel` the endpoint validator is bypassed only for the scheme and loopback
checks, `base_url` must be empty, and header rules still apply. `agentctl` builds a `net.Conn` through a
`TunnelDialer` supplied by a host-owned lease manager; the HTTP transport and a per-connection copy of the WebSocket
dialer use it with a synthetic authority that is never resolved. The dialer checks the requested port against
`{control port, recorded instance port}` before opening a stream, writes nothing before the acknowledgement, and
closes the stream when the request context ends. A tunnel registry keyed by installation, environment, generation
counts streams (limit 64) and closes all on dispatch generation change, disable, plugin exit, or environment release.
Gateway `vscode_proxy.go`, `port_proxy.go`, and `port_tunnel.go` already obtain the lease transport; they take the
tunnel dialer from the same resolver. Authentication to agentctl stays the host-applied bearer token.

## API forwarding

For `provider_forwarded` the launch path replaces `validatePluginRuntimeAPIURL` with a check that the backend's API
base URL is known. The bootstrap JSON gains `host_api_url` (the backend's own listener, loopback allowed) and
`idle_timeout_seconds`. The provider arranges worker-side reachability (for example a reverse forward) and starts
agentctl with the worker-local URL. The existing authenticated callback probe is the sole reachability proof; it rolls
back provisioning when it fails. No listener is added.

## Drain receipt

Agentctl **new** route `POST /runtime/seal-and-drain` (authenticated with the control token, body carries the expected
runtime identity and claim) stops accepting work, stops managed process groups, and returns a receipt; the
`instance.Manager` and control client carry the generic part ported from the native branch. A second call with the
same claim returns the stored receipt. Lifecycle adds `drainBeforePluginDestroy` ahead of
`destroyPluginExecutorRecord`: under the cleanup claim it calls drain with a 60 s deadline, writes the host-only
`drain_receipt` checkpoint (CAS), then calls destroy with **new** `DestroyExecutorEnvironmentRequest.drain_receipt`
and `drain_outcome`. Failed-launch rollback before a runtime exists records `not_attempted`. The destroy deadline is
`destroy_timeout_seconds`. For declaring providers the bootstrap's `idle_timeout_seconds` is 0, mapped to
`KANDEV_ACP_IDLE_TIMEOUT` disabled in the agentctl startup config.

## Destroy and recover checkpoints

`pluginExecutorCallbackRequest` today matches only an active launch. It gains a claim-scoped path: the request context
**new** field `claim_token` must match the active cleanup claim or recovery claim row (`internal/task/recoveryclaim`)
for the same environment, owner, generation, and operation, and the dispatch generation must be current. Phases
`cleanup_progress` and `recovery_progress` join the allowed set when the snapshot has `destroy_checkpoints`; a phase
rank table enforces monotonic order. The write is the existing `CheckpointPluginExecutorInventory` CAS, so two callbacks
at one revision serialize to one winner. Settlement requires `confirmed_absent` and a successful latest checkpoint.

## Durable bootstrap and ready publication

The nonce and the agentctl token are stored as secret references in `executors_running.metadata`, using the existing
`MetadataKeyAuthTokenSecret` pattern and a **new** `MetadataKeyBootstrapNonceSecret`, via the runtime secret store;
the nonce reference is persisted before `ProvisionExecutorEnvironment`, the token reference before
`createPluginAgentctlInstance`. Reads go through `revealRuntimeSecret`. Recovery replaces the `incomplete_bootstrap`
branch in `attachRecoveredPluginExecutor` for `durable_bootstrap`: reveal the nonce, handshake, adopt or create the
instance after verifying owner, workspace path, and port, then publish. A handshake rejected as consumed ends in cleanup
with reason `bootstrap_consumed`.

Publication is a single repository method `PublishRecoveredTaskEnvironment` that, under the recovery claim, verifies
operation, task, session, environment owner and generation, runtime identity, descriptor, and repository inventory and
flips the environment from `creating` to `ready`. It is idempotent on a ready row with identical evidence. It is generic:
it accepts verified evidence and carries no provider data.

## Crash barrier and fixture

`lifecycle/crash_barrier.go` defines the closed barrier names and `armBarrier(name, mode)`; the arming route sits under
`/api/v1/e2e/` and is mounted only when `KANDEV_E2E_MOCK=true`, like the child-kill route. Barrier calls in production
paths are a single cheap lookup that returns immediately when none is armed. `cmd/plugin-fixture` gains every
capability behind fixture configuration and a local in-process tunnel target, and its conformance test runs each
capability and barrier through the managed plugin process.

## Compatibility

Proto fields are additive; the manifest keys default to current behavior. The host mixed-version matrix: new host with
old plugin (no declaration, unchanged), old host with new plugin (fails the capability check before activation),
new host with upgraded plugin and retained old environments (snapshot governs). Rollback to a plugin without a
capability while retained environments hold it makes that provider unavailable for them, not absent.

## Data model

No new table and no DDL migration. The `plugin_executor` envelope in `executors_running.metadata` gains fields:
`negotiated_capabilities`, `provider_state_version`, `workspace_path`, `workspace_identity`, `workspace_preparation`, `repositories`, `drain_outcome`,
`drain_receipt_id`. The envelope stays under 32 KiB; with 32 repositories at about 200 bytes the inventory fits, and an
oversize write is rejected as `invalid_config`. Secret references add metadata keys, not columns. Publication touches
the existing task environment and repository projection rows.

- **SQLite:** the single immediate-mode writer serializes checkpoints and publication; every new method runs in one transaction.
- **PostgreSQL:** each read-modify-write takes `pg_advisory_xact_lock(hashtextextended(namespace||environment_id, 0))`
  before its first read, with namespaces `plugin_checkpoint` and `plugin_publish`; checkpoint keeps the revision CAS.
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
| Plugin restart with open tunnel (004.7) | streams close, new lease, same environment | fixture over the process boundary |
| Cancel during stream open (004.3, 004.6) | no bytes before ack, stream closed, count zero | tunnel unit with fake provider |
| Provider returns an undeclared field (001.6) | `conflict`, resource cleaned | lifecycle unit |
| Descriptor changed on attach (002.4) | `conflict`, session not started | lifecycle unit |

Remaining ACs are covered by validator tests (manifest, workspace path per platform, repository inventory set equality,
agent env filtering), the unchanged fixture conformance test (001.3), and an `-race` run over the changed packages.

## Observability

Closed-label counters join the existing executor-plugin metrics: `executor_plugin_capability_rejected_total`
(`capability`), `executor_plugin_tunnel_open` gauge and `executor_plugin_tunnel_closed_total` (`reason`: cancel, restart,
limit, ack_timeout, provider_error), `executor_plugin_drain_total` (`outcome`), and `executor_plugin_publish_total`
(`outcome`). Barrier firings log with the barrier name. Plugin, task, environment, and execution IDs are never labels.

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
