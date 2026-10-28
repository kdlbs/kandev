---
status: draft
system: executors
created: 2026-10-28
owners:
  - kandev
---

# Remote executor plugins: native worker capabilities requirements

## Overview

The [remote executor plugin contract](remote-executor-plugins.md) assumes a fixed `/workspace` path, a public
HTTPS endpoint, in-memory bootstrap credentials, and launch-scoped checkpoints. A provider that runs agents on
operator-owned machines (reached over SSH or a local socket) needs more. This document adds generic,
opt-in host capabilities so such a provider can ship as a plugin. It names no vendor and moves no built-in.

A provider declaring none of these keeps today's behavior, wire format, and inventory.

## Terminology

- **Host capability:** A named, closed-set behavior the host offers a provider; a provider can require it in its manifest.
- **Workspace descriptor:** The validated runtime path and stable identity of an environment's workspace.
- **Control transport:** The path from the host to the agentctl control and instance ports of an environment.
- **Drain receipt:** Host-persisted evidence that the exact recorded runtime sealed and drained its managed processes.
- **Cleanup claim:** The existing exclusive claim on an environment owner and generation that fences destroy.
- **Crash barrier:** A test-only named pause or exit point in host launch, recovery, and cleanup.

## Requirements

### REQ-EXECUTORS-NATIVE-WORKER-001: Capability negotiation and compatibility

**Intent:** Providers adopt new host behavior explicitly, and incompatible hosts fail before any allocation.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-001.1:** The manifest validator shall accept `requires_host_capabilities` on an executor provider as defined by REQ-EXECUTORS-PLUGIN-CAPABILITY-001, and shall reject unknown or duplicate names. The seven names of this document are registry entries at version 1.
- **AC-EXECUTORS-NATIVE-WORKER-001.2:** When a provider requires a capability the host does not implement, install, upgrade, and enable shall fail before activation and name the missing capability. An upgrade failure shall leave the working installation active.
- **AC-EXECUTORS-NATIVE-WORKER-001.3:** When a provider declares no capability, its requests, responses, and persisted inventory shall be unchanged from the current contract, and the existing fixture conformance test shall pass without modification.
- **AC-EXECUTORS-NATIVE-WORKER-001.4:** The host shall snapshot the declared capability set into inventory when an allocation is admitted. Recovery, attach, and destroy shall use that snapshot, not the installed manifest, so an upgrade never changes the rules for an existing environment.
- **AC-EXECUTORS-NATIVE-WORKER-001.5:** When the running host lacks a capability in a recorded snapshot, the provider shall be unavailable for that environment, the record shall be retained, and no cleanup shall be treated as confirmed.
- **AC-EXECUTORS-NATIVE-WORKER-001.6:** When a provider returns a field belonging to a capability it did not declare, the host shall reject the response with `conflict` before persisting it and shall run the existing cleanup for any resource the response created.
- **AC-EXECUTORS-NATIVE-WORKER-001.7:** The provider contract version shall remain 1. New protocol fields shall be additive, and a manifest requiring a higher contract version shall still be rejected.

### REQ-EXECUTORS-NATIVE-WORKER-002: Workspace descriptor

**Intent:** The workspace location is a recorded, validated fact of the environment, not a host constant.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-002.1:** When a provider requires `workspace_descriptor`, every provision and attach response shall carry a non-empty `workspace_path` and `workspace_identity`. A response missing either shall be rejected and the created resource cleaned up.
- **AC-EXECUTORS-NATIVE-WORKER-002.2:** The host shall validate `workspace_path` against the declared environment platform, not the backend's filesystem. It shall reject an empty, relative, root-only, UNC, NUL-bearing, control-character-bearing, or over-1024-byte path, any `.` or `..` segment, and a path whose form does not match the platform (POSIX absolute for Linux and macOS; drive-letter absolute for Windows).
- **AC-EXECUTORS-NATIVE-WORKER-002.3:** `workspace_identity` shall be 1 to 128 characters from `[A-Za-z0-9._:-]` and shall be treated as opaque, non-secret, and compared only for equality.
- **AC-EXECUTORS-NATIVE-WORKER-002.4:** The host shall persist the descriptor in the plugin inventory envelope before the first use of the path, and the descriptor shall be immutable for one environment generation. A later provision, attach, recovery, or checkpoint response with a different path or identity shall be rejected with `conflict`, shall not change inventory, shall retain the environment, and shall not start a session.
- **AC-EXECUTORS-NATIVE-WORKER-002.5:** The host shall use the recorded path for instance creation, agent working directory, repository materialization, and the instance workspace path. It shall never read, stat, create, or write that path on the backend host.
- **AC-EXECUTORS-NATIVE-WORKER-002.6:** When a provider does not require `workspace_descriptor`, the workspace path shall remain `/workspace`, and a provider response carrying either field shall be rejected under AC-EXECUTORS-NATIVE-WORKER-001.6.
- **AC-EXECUTORS-NATIVE-WORKER-002.7:** Two environments of one provider that share a worker shall record distinct paths and identities. Destroying one shall not alter the other's recorded descriptor.

### REQ-EXECUTORS-NATIVE-WORKER-003: Repository intent and provider-prepared workspaces

**Intent:** A provider that prepares the workspace itself receives only approved repository intent and returns verifiable evidence.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-003.1:** A provider can declare `workspace_preparation` as `core` (default) or `provider` in its manifest; `provider` requires `repository_intent` and `workspace_descriptor`. The mode shall be fixed at allocation, recorded in inventory, and used for recovery.
- **AC-EXECUTORS-NATIVE-WORKER-003.2:** When a provider requires `repository_intent`, the provision request shall carry one entry per repository with the repository ID, a credential-free locator, branch, base branch, and checkout ref, ordered by the workspace's repository position then repository ID. It shall carry at most 32 entries, each locator at most 2048 bytes. A task with no repository shall send an empty list.
- **AC-EXECUTORS-NATIVE-WORKER-003.3:** The host shall include the repository intent in the operation input digest. Retrying one operation with the same intent shall not allocate a second environment; the same operation ID with changed intent shall return `conflict`.
- **AC-EXECUTORS-NATIVE-WORKER-003.4:** When a provider requires `repository_intent`, the provision request shall also carry non-secret `agent_env` entries, at most 64 entries and 16 KiB, selected from the host's instance environment after removing every managed credential, broker lease, and secret-classified entry. No credential or token shall appear in the intent or `agent_env`.
- **AC-EXECUTORS-NATIVE-WORKER-003.5:** When `workspace_preparation` is `core`, the host shall materialize repositories through agentctl exactly as today.
- **AC-EXECUTORS-NATIVE-WORKER-003.6:** When `workspace_preparation` is `provider`, the provider response shall return, per repository, its ID, a path inside `workspace_path`, the checked-out branch, and the head commit. The host shall verify the returned set equals the intent set (no missing, extra, or duplicate repository) and shall not clone or fetch into that workspace. A mismatch shall run cleanup with reason `repository_inventory_mismatch`.
- **AC-EXECUTORS-NATIVE-WORKER-003.7:** When a workspace selects managed Git credentials and the provider prepares the workspace, launch shall be rejected before allocation and shall name the credential mode. Provider preparation uses executor-owned credentials only.
- **AC-EXECUTORS-NATIVE-WORKER-003.8:** The repository inventory shall be persisted with the descriptor, and recovery shall re-verify it under AC-EXECUTORS-NATIVE-WORKER-007.3.

### REQ-EXECUTORS-NATIVE-WORKER-004: Negotiated control transport and API forwarding

**Intent:** A provider can reach workers that expose no public HTTPS endpoint, without weakening endpoint validation for providers that do.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-004.1:** A connection lease shall carry a `transport` of `https` (the default and today's behavior) or `host_tunnel`. A provider shall return `host_tunnel` only when it requires `tunnel_transport`; otherwise the host shall reject the lease.
- **AC-EXECUTORS-NATIVE-WORKER-004.2:** For `host_tunnel`, the host shall open a bidirectional stream to the provider bound to installation, environment, ownership generation, and one port. The port shall be the control port or the recorded instance port; any other port shall be refused before a stream opens. The provider shall not be able to open a stream to the host or to another environment.
- **AC-EXECUTORS-NATIVE-WORKER-004.3:** The host shall send no request byte, header, or credential on a stream until the provider acknowledges the binding. A stream that is not acknowledged within 15 seconds shall be closed and reported as unavailable, never as absent.
- **AC-EXECUTORS-NATIVE-WORKER-004.4:** A `host_tunnel` lease shall carry no `base_url`. Header and subprotocol rules, host-owned authentication, and credential non-persistence shall apply as for HTTPS leases. HTTPS leases shall keep their current validation, including HTTPS-only and loopback rejection.
- **AC-EXECUTORS-NATIVE-WORKER-004.5:** Agentctl HTTP and WebSocket traffic, including terminal, files, Git, and gateway editor and preview proxies, shall use the tunnel for a `host_tunnel` lease.
- **AC-EXECUTORS-NATIVE-WORKER-004.6:** Canceling a request, closing a client, or releasing an environment shall close its streams and the provider shall be notified of the cancellation. After release no stream for that environment shall remain open.
- **AC-EXECUTORS-NATIVE-WORKER-004.7:** When the plugin restarts, is disabled, or its dispatch generation changes, all of its streams shall close. The next request shall obtain a new lease and open a new stream for the same environment without reprovisioning, and in-flight mutating HTTP requests shall not be replayed.
- **AC-EXECUTORS-NATIVE-WORKER-004.8:** The host shall limit open streams to 64 per environment and frames to 64 KiB. Beyond the limit a new stream shall fail as `rate_limited`, leaving existing streams unaffected.
- **AC-EXECUTORS-NATIVE-WORKER-004.9:** A provider can declare `runtime_api_url` as `host_public` (default) or `provider_forwarded`; `provider_forwarded` requires `api_url_forward`. For `provider_forwarded` the host shall skip the public HTTPS reachability check, pass the backend's own API base URL in the bootstrap as `host_api_url`, open no new listener, and still run the bounded authenticated callback probe, rolling back provisioning when it fails.
- **AC-EXECUTORS-NATIVE-WORKER-004.10:** For `host_public` the existing HTTPS, non-private runtime API URL requirement shall be unchanged.

### REQ-EXECUTORS-NATIVE-WORKER-005: Drain receipt before destroy

**Intent:** Authorized destruction stops managed processes first and gives the provider evidence of it, without giving the provider agentctl credentials.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-005.1:** When a provider requires `drain_receipt`, an authorized destroy of an environment whose runtime is recorded shall have the host ask the exact recorded runtime to seal and drain, then persist the receipt bound to environment, generation, runtime identity, and cleanup claim, then call destroy with the receipt, in that order.
- **AC-EXECUTORS-NATIVE-WORKER-005.2:** Agentctl shall expose an authenticated seal-and-drain operation that verifies the runtime identity, refuses new work, stops managed process groups, and returns a receipt. Repeating it under one claim shall return the same receipt.
- **AC-EXECUTORS-NATIVE-WORKER-005.3:** Ordinary agent Stop, backend shutdown, and plugin disable or crash shall not seal or drain an environment.
- **AC-EXECUTORS-NATIVE-WORKER-005.4:** When the drain reply is lost, times out, or the runtime is unreachable, the host shall not create a receipt. The destroy request shall carry `drain_outcome` of `drained`, `unreachable`, or `uncertain`, the host shall persist that outcome before destroy, and `uncertain` shall keep the record `cleanup_pending` until the provider confirms absence.
- **AC-EXECUTORS-NATIVE-WORKER-005.5:** The host shall reject a receipt whose environment, generation, runtime identity, or claim differs from the active cleanup, and a receipt from an earlier claim shall not satisfy a later one.
- **AC-EXECUTORS-NATIVE-WORKER-005.6:** A drain receipt shall not be evidence that the provider's resource is absent. Confirmed absence shall still come only from the provider's destroy response.
- **AC-EXECUTORS-NATIVE-WORKER-005.7:** A provider requiring `drain_receipt` can declare `destroy_timeout_seconds` from 30 to 300 (default 30), and the host shall use it as the destroy deadline for that provider. Drain shall use its own deadline of 60 seconds.
- **AC-EXECUTORS-NATIVE-WORKER-005.8:** For a provider requiring `drain_receipt`, the bootstrap shall set the agentctl idle timeout to disabled so a retained environment is never reaped by the agentctl idle reaper. Other providers shall keep the current default.

### REQ-EXECUTORS-NATIVE-WORKER-006: Destroy- and recover-scoped checkpoints

**Intent:** Provider progress during cleanup and recovery is recorded durably under the same fences as launch.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-006.1:** When a provider requires `destroy_checkpoints`, the host shall accept `CheckpointExecutorResource` while that provider holds an active cleanup claim or recovery claim, with phases `cleanup_progress` and `recovery_progress`. The phase `drain_receipt` shall be writable only by the host.
- **AC-EXECUTORS-NATIVE-WORKER-006.2:** The host shall validate each such callback against the claim token, operation ID, environment, ownership generation, dispatch generation, and plugin envelope revision. A callback from a released claim, a replaced generation, or a superseded dispatch shall be rejected and shall not change inventory.
- **AC-EXECUTORS-NATIVE-WORKER-006.3:** Provider state in a checkpoint shall pass the closed resource-state schema, shall fit the 32 KiB envelope, and shall contain no secret field.
- **AC-EXECUTORS-NATIVE-WORKER-006.4:** The phase sequence shall be monotonic. A checkpoint that lowers the phase shall be rejected.
- **AC-EXECUTORS-NATIVE-WORKER-006.5:** When two callbacks observe the same revision, exactly one shall commit and the other shall receive `conflict`. This shall hold on SQLite and PostgreSQL.
- **AC-EXECUTORS-NATIVE-WORKER-006.6:** The host shall settle an environment absent only after the provider confirms absence and the latest accepted checkpoint has been persisted. A missing checkpoint shall never be read as absence.
- **AC-EXECUTORS-NATIVE-WORKER-006.7:** After a crash between a provider side effect and its checkpoint, recovery shall re-issue destroy idempotently from the recorded state, and the provider shall return progress or confirmed absence. The host shall not allocate a replacement.

### REQ-EXECUTORS-NATIVE-WORKER-007: Durable bootstrap continuation and ready publication

**Intent:** A backend restart during launch resumes the admitted operation instead of destroying or duplicating it.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-007.1:** When a provider requires `durable_bootstrap`, the host shall persist the bootstrap nonce as a host-side secret reference before the provision call, and the agentctl authorization the same way before instance creation. Neither value shall appear in the inventory envelope, a checkpoint, progress, a log, provider state, or a browser response.
- **AC-EXECUTORS-NATIVE-WORKER-007.2:** When the backend restarts with a persisted nonce and no agentctl authorization, recovery shall repeat the authenticated handshake under the original operation, create or adopt the session's instance by verifying owner, workspace descriptor, and port, and continue to readiness without allocating a second environment.
- **AC-EXECUTORS-NATIVE-WORKER-007.3:** The host shall publish an environment from `creating` to `ready` in one guarded transaction under the recovery claim, after verifying the original operation, task and session, environment owner and generation, runtime identity, workspace descriptor, and repository inventory. Repeating publication shall be idempotent, and a mismatch shall publish nothing, adopt nothing, and retain the environment with a named reason.
- **AC-EXECUTORS-NATIVE-WORKER-007.4:** A canceled recovery or a temporary transport failure shall leave the environment `creating` and retryable while its owner exists.
- **AC-EXECUTORS-NATIVE-WORKER-007.5:** When the nonce was spent but the authorization was not persisted, and agentctl rejects a second handshake as consumed, recovery shall run cleanup with reason `bootstrap_consumed`. It shall not allocate a replacement and shall not publish `ready`.
- **AC-EXECUTORS-NATIVE-WORKER-007.6:** When archive, delete, or reset races recovery for one environment, exactly one shall hold the claim; the other shall no-op, and no cleanup state shall be lost.
- **AC-EXECUTORS-NATIVE-WORKER-007.7:** When a provider does not require `durable_bootstrap`, a non-ready record without saved authorization shall be cleaned up as `incomplete_bootstrap` exactly as today.

### REQ-EXECUTORS-NATIVE-WORKER-008: Provider-neutral crash barrier and fixture proof

**Intent:** Every capability is proven through the real plugin process boundary without a vendor.

#### Acceptance criteria

- **AC-EXECUTORS-NATIVE-WORKER-008.1:** The host shall expose named crash barriers at: after provider side effect before checkpoint, after drain before destroy, after handshake before authorization persistence, before ready publication, and after destroy confirmation before owner release. Names shall be provider-neutral.
- **AC-EXECUTORS-NATIVE-WORKER-008.2:** Barriers shall be armable only when `KANDEV_E2E_MOCK=true`. With any other profile, arming shall be refused and barrier points shall have no effect.
- **AC-EXECUTORS-NATIVE-WORKER-008.3:** An armed barrier shall fire at most once per arming and shall either block until released or terminate the backend process, as the test selects. After termination, a restarted backend shall recover under REQ-EXECUTORS-NATIVE-WORKER-006 and 007.
- **AC-EXECUTORS-NATIVE-WORKER-008.4:** The packaged fixture provider shall declare every capability in AC-EXECUTORS-NATIVE-WORKER-001.1 and a conformance test through the managed plugin process shall exercise each capability and each barrier.
- **AC-EXECUTORS-NATIVE-WORKER-008.5:** The fixture shall use a local test transport and require no network, cluster, or credentials.

## Out of scope

- Removing a built-in executor. Migration is specified in [operations and migration](remote-executor-plugin-operations-migration.md); a built-in stays until a plugin passes its live scenarios.
- A per-node repository mirror, autoscaling, fleet management UI beyond the status read, cost dashboards, and hostile-workload isolation.
- Shared workspaces, admission, and queued placement: see [shared environments, admission, and placement](remote-executor-plugin-shared-admission-placement.md).
- A new public relay service or any change to HTTPS lease validation for `https` leases.
- Durable event replay and workspace backup. Publishing the provider SDK as a versioned module.

## Implementation plans

- [Native worker capabilities for executor plugins](../../../plans/executor-plugin-native-workers/plan.md)
