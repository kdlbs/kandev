---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-NOMAD-001
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
  - REQ-EXECUTORS-NOMAD-004
  - REQ-EXECUTORS-NOMAD-005
---

# Native Nomad executor system design

## Purpose and boundaries

Executors own environment identity, resource admission and cleanup. Implement one
built-in Nomad executor that runs agentctl and agent workloads directly as
native host processes. The existing Mac worker is the first validation target,
not a definition of native execution or a claim of other-platform support.
Reuse Kandev's ExecutorBackend and SSH transport primitives; Nomad alone launches agentctl.
No Docker, VM, scheduler, supervisor, public gateway, plugin or new UI is needed.

The first environment supports one isolated session and trusted workloads whose
descendants remain in agentctl-managed process groups. It is not a sandbox or
hard macOS CPU/memory containment. Bare-metal Linux exec/cgroups is an optional
later support boundary. The [ADR](../../../decisions/2026-10-24-native-nomad-cleanup-evidence.md)
records the built-in route and conditions for considering a plugin later.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-NOMAD-001 | Components and extension seam; native job and capacity; lifetime |
| REQ-EXECUTORS-NOMAD-002 | Durable identity; retries and disconnection |
| REQ-EXECUTORS-NOMAD-003 | Sealed drain; terminal cleanup and quarantine |
| REQ-EXECUTORS-NOMAD-004 | Native bootstrap and secure transport; security and diagnostics |
| REQ-EXECUTORS-NOMAD-005 | Architecture comparison; verification and distribution |

## Architecture comparison and source grounding

Source baseline is Kandev `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`.
Names marked **new** below are proposed, not existing APIs.

| Boundary | Built-in route | External SDK route |
| --- | --- | --- |
| Lifecycle | `ExecutorBackend` in `runtime/lifecycle/executor_backend.go`: Name, HealthCheck, CreateInstance, StopInstance, RecoverInstances; `ExecutorRegistry` registers built-ins | Seven typed `ExecutorProviderPlugin` operations already exist in `pkg/pluginsdk/executor_provider.go` |
| Remote connection | Same package can call existing SSH helpers and loopback-forwarded `agentctl.Client` | Cannot delegate internal SSHExecutor; `ExecutorConnectionLease` requires non-loopback HTTPS and verified TLS, plus managed plugin transport |
| Runtime artifact | `AgentctlResolver`, `ensureAgentctlOnHostWithProgress`, native SSH upload already exist | In-process SDK `ReadExecutorRuntimeArtifact` callback must be bridged to the worker; it is not an HTTP route |
| Cleanup | Internal executor can obtain and checkpoint a generic runtime drain receipt directly | Current Destroy request has no completed-drain evidence field/callback; new SDK/manifest compatibility work would be required |
| Packaging | Existing Kandev binary plus operator's native Nomad/SSH | Dedicated production repository, manifest, executable, managed gRPC and install artifact |
| Cost | Runtime/profile taxonomy and Nomad API integration, generic drain plus focused SSH reuse | Provider wiring, artifact bridge, TLS gateway and cleanup protocol extension in addition to Nomad logic |

The built-in seam fits cleanly but is not drop-in. Add **new** `NomadExecutor`
implementing `ExecutorBackend`, register it in `internal/backendapp/agents.go`,
and add **new** RuntimeNomad/NameNomad/ExecutorTypeNomad consistently in
`internal/agentruntime/runtime.go`, `internal/agent/executor/executor.go` and
`internal/task/models`. Audit runtime predicates, resumability, executor config
validation, environment preparation, inventory checkpoint/recovery/cleanup
routing and bootstrap/profile projections. It is remote host-mode, not
containerized. Unknown/disabled/invalid config must never fall back to local.

Keep the adapter and Nomad client in cohesive `executor_nomad*.go` and a small
`internal/agent/nomad` package if needed. Do not export internal executors or
create a generic framework. No provider SDK/protobuf/manifest change belongs to
this implementation. A plugin would be a separate future design with its own
transport and cleanup contract, not a parallel implementation of this executor.

Existing patterns establish responsibilities rather than copyable launch code:
SSH has per-session connections, pinned host keys, process identity and liveness;
Kubernetes records UID/create nonce/launch snapshot and exact cleanup; remote
Docker retains resource identity. The dedicated plugin repository rule applies
only if a plugin is actually selected.

## Components and lifetime

Kandev retains task/session semantics, selected agent runtime, ACP, workspace
materialization, credentials, progress/results and cleanup authorization. Nomad
owns reservations, node eligibility, allocation and whole-job stop. The new
executor only connects these existing responsibilities.

Compute belongs to the canonical task environment and persists beyond an
individual turn or ordinary agent Stop. Start with one isolated session per
environment; reject shared-workspace additional sessions. Backend shutdown closes
transport without stopping compute. Recovery reattaches the exact stored
allocation/control server/workspace and never disguises replacement as resume.
Task completion alone does not destroy the environment. Archive/delete/reset and
failed-launch rollback use existing authorized terminal cleanup semantics.

Store non-secret typed **new** `NomadEnvironmentState` v1 in existing durable
runtime/environment inventory, using `ExecutorCreateRequest` checkpoint hooks
and `executors_running` as the authoritative execution inventory. Persist the
canonical environment's ownership generation and launch snapshot independently
of whichever request currently has the live SSH connection. Audit existing
Kubernetes/plugin checkpoint wiring and add the Nomad branch explicitly; do not
assume their runtime-specific hooks automatically cover a new runtime. No new
vendor database or independent fleet inventory is introduced.

### Persistence and worker authority clarification

No SQL NodeID claim, worker table, capacity ledger or schema migration is
required for the first implementation. Per-execution metadata CAS does not
serialize different sessions and must not be represented as doing so. That is
intentional: different environments may record provisioning intent and queue
Nomad jobs. The non-expiring worker-local atomic start fence below is the
authoritative cross-session exclusion before any agentctl/workload launch.
Nomad Variables CAS operation/quarantine markers and node eligibility preserve
control-plane safety; none substitutes a second SQL placement authority. Even a
registration/quarantine race cannot bypass the worker start fence.

The schema already has `executors_running.metadata` (added in
`repository/sqlite/base_migrations.go`) and string runtime/executor-type values.
`TaskEnvironment` owns task ID and ownership generation but has no generic
metadata field. Store the typed versioned `nomad` envelope in the existing
`ExecutorRunning.Metadata`, referring to that canonical environment; do not add
an assumed environment JSON column or copy its ownership into a new inventory.
With the initial single isolated session/environment, keep that row and its
secret references for the full retained environment lifetime, including
ordinary Stop and cleanup-pending. Do not rotate/delete it on a turn boundary.

Implement a narrow **new** Nomad inventory checkpoint writer on the existing
repository, following `CheckpointPluginExecutorInventory` in
`repository/sqlite/executor_plugin_inventory.go` as a concurrency pattern, not
calling its plugin-only validators. It must transactionally verify current
session/task/environment ownership generation and recovery-claim availability;
compare the execution ID and observed Nomad envelope revision; and update only
the Nomad envelope while preserving unrelated metadata, resume/conversation
fields and status. First insert is allowed only for a current complete identity
at the pre-submit phase. Return conflict on stale revision/generation/execution,
not a broad `UpsertExecutorRunning` overwrite. An observed revision argument or
transient Go field needs no database column. Add a matching claim-authorized
exact delete/release guard for a positively absent envelope; the general
timestamp/execution delete alone is not the full Nomad cleanup authorization.

Wire `ExecutorCreateRequest.CheckpointRuntimeInventory` and
`ReleaseRuntimeInventory` on both launch entry points in `manager_launch.go`
and `manager_execution.go`, using the existing
`manager_plugin_executor_inventory.go` bridge as the bounded pattern. Resolve
current ownership through `GetTaskEnvironment`, read the current row through
`GetExecutorRunningBySessionID`, and persist intent before the first Nomad
register or worker-fence mutation. Persist pending quarantine before returning
an uncertain outcome. Startup/retry/cleanup must read retained Nomad envelopes
even when the ordinary execution status is stopped/dead: the existing
`ListLiveRemoteExecutorsRunning` filters terminal rows and is insufficient by
itself. Add a narrow retained Nomad lister or equivalent explicit branch in
`persistence.go`/`manager_lifecycle.go`, route through the existing remote
recovery guard/retry pipeline, and reload/revalidate the current envelope at
each mutation. Existing `remote_recovery.go` runtime/owner checks and cleanup
routing need the Nomad branch. Unknown inventory reads block admission; a row
scan can detect a pending quarantine but cannot claim atomic NodeID ownership.

## Durable identity and operation recovery

The exact pre-dispatch job snapshot, server-default normalization and index
authority follow the [job identity contract](nomad-native-job-identity.md).
Created-session retention and guarded attach follow the
[startup continuation contract](nomad-native-startup-continuation.md).

State declares version, namespace/region/datacenter, immutable job ID/create
index, original operation/input digest, environment ID/generation, execution
identity, allocation UUID/create index, NodeID, platform, admitted SSH target and
pinned host fingerprint, exact allocation/workspace roots, reserved control and
instance ports, agentctl ServerIdentity, phase and drain evidence.
Optional allocation/runtime fields stay absent until observed.
Persist credential references in the secret store, never secret values or SSH
private keys in metadata. Profile edits cannot mutate a retained snapshot.

A job ID hashes environment ID + generation + original operation + installation
identity. Names are never reused. Job metadata holds the
non-secret identity/input digest. Register with enforce-index and zero index;
validate an existing job/spec on conflict. Stop lacks compare-and-delete,
so immutable names complement pre-stop identity checks. Allocation UUID and NodeID, not reusable node names/IPs/labels, establish
continuity. More than one allocation, or a successor, is unknown.

Checkpoint ownership and operation intent before dispatch, job identity after
register, allocation identity before bootstrap, then bootstrapping/ready.
An ambiguous register response can have committed. Recover the original job by
ID/digest, never by blindly registering a replacement. Same operation
with changed input conflicts. Missing job after a possibly committed request,
empty allocation list, GCed history and unavailable control plane are unknown.
Only positive evidence that no submit was attempted establishes pre-allocation
absence. Keep small CAS-protected Nomad operation fences/tombstones and node quarantine
markers as safety interlocks, not another inventory; history GC cannot erase them. The inventory row stays as the `absent`
tombstone with its drain receipt until environment deletion; a missing row
is never absence.

### Exact environment workspace

Profile `WorkspaceRoot` is a base directory. Persist it as admitted base in
EnvironmentState plus a separate immutable `WorkspacePath`, derived once as
`<base>/environments/<safe-environment-id>/g<ownership-generation>`. One helper
rejects unsafe path components/escape; do not use lossy sanitization. Bind both
to the launch snapshot/digest. Job KANDEV_WORKSPACE_ROOT, agentctl --workdir,
buildSSHCreateInstanceRequest, instance GET validation and ExecutorInstance
WorkspacePath all use the exact state.WorkspacePath. Recovery uses that stored
path, never an edited profile or a newly derived successor path. Different
environments/generations must not share a checkout even when execution is
serialized. No SQL migration is needed for the versioned JSON field.

SSH's helper takes an exact taskDir after prepareRemoteTaskDir derives
<workdir>/tasks/<task-dir>; passing Nomad's profile base skips that ownership
step. Pre-release retained fixtures missing WorkspacePath fail closed for new
continuation. Preserve their original job/root/fence for exact cleanup; do not
relocate or rewrite a live operation to repair isolation.

Nomad has no SSH primary-repository prepare script. Reuse manager_launch.go's
materializeWorkspaceRepositories with primary inclusion and minimumRepos=1,
as for plugin_remote. Update launchRepositoryProjectionKey consistently for
Nomad's branch-scoped primary destination. A one-repository launch must not
silently skip clone/checkout, and retained continuation must not reset it.
RequiresCloneURL alone does not materialize a repository. Verify a selected
real test repository through this path before claiming Git proof. An empty
no-repository workspace must not inherit another environment's checkout/base;
clearing a stored base SHA or manually creating replacement history is not a
substitute for correct workspace materialization and base capture.

### Pre-instance continuation and credential checkpoints

A retained registered/allocated operation without a Kandev instance is an
unfinished original creation, not an initialized ACP session. Normal
`prepareExecutionCreation` allocates a new UUID. The current generic remote
recovery path requires authenticated existing agent-session evidence, then sets
sessionInitialized; failed sessions without durable pending-delivery evidence
are ineligible. Do not return a newly created pre-ACP instance into that path as
if it had already initialized or relax its eligibility checks.

The bounded continuation belongs in the manager before new-UUID allocation.
Read the exact current retained row and canonical
`workspaceInfoProvider.GetWorkspaceInfoForSession`, require current authorized
session/task/environment ownership and generation, and prepare with existing
`prepareExecutionCreateRequest` using the ORIGINAL AgentExecutionID. Retain the
original operation/job and revalidate admitted inputs. Build instance settings
from canonical authorized agent/MCP/permission/delivery configuration, not an
invented minimal request. Persist a versioned non-secret instance-input
snapshot/fingerprint before dispatch; re-resolve secret references at use. If
required original inputs cannot be reconstructed or changed, fail closed.
Never persist resolved Env, gateway credentials or arbitrary secret-bearing
CreateInstanceRequest JSON as that snapshot.

A Nomad provisioned-operation continuation validates the original job, full
allocation UUID/NodeID, worker fence and reserved ports, then checkpoints the
registered-to-allocated transition. It only reopens pinned SSH/control/instance
and reverse API transport. It never registers, stages files, generates a new
nonce or starts another controller. GET the original instance ID; reuse only
matching ID/task/session/workspace/port, and create only on exact not-found.
Reconcile an ambiguous create by GET, not a second identity. Failed attachment
must not delete a preexisting instance. Return through normal
initializeCreatedExecution/registerAndPublishCreatedExecution and first agent
initialization. Attachment never sends or replays a prompt. Existing normal
admission must authorize this retry; no direct SQL session-state repair or
initialized flag is permitted. Established-session restart remains the separate
existing remote-recovery proof path.

Persist the original nonce through the manager's existing SecretStore and
Nomad checkpoint callback BEFORE worker staging/job registration. The request
holds that one nonce; its initial checkpoint must store the vault value and
reference durably before returning success. Reuse
`persistRuntimeSecretResult`'s deterministic original-ID pattern without
registering a live execution prematurely. Retained references must be revealed
and compared, never overwritten with a new credential. After the one-use
Handshake returns a token, save its vault reference under the same current
inventory CAS immediately, before instance creation; the nonce alone cannot
recover a consumed handshake. Return both AuthToken and BootstrapNonce on
ExecutorInstance for existing lifecycle persistence. Missing secret/checkpoint
success blocks the next side effect. Only approved nonce/token reference keys
may join the typed envelope's metadata CAS; a merge replacing only the envelope
would drop new top-level references. Keep unrelated metadata/conversation/status
protected. Startup TransientAuthToken/TransientBootstrapNonce merely reveal
saved references and cannot reconstruct a missing original value.

For the disposable test-owned fixture only, an explicit operator repair may
read the ORIGINAL nonce from its exact immutable private bootstrap environment
file over pinned SSH after matching job/allocation/NodeID/fence, private owner/
mode and non-symlink path evidence. Parse only the expected key in memory,
without printing or sourcing/executing the file; no glob, logs or process-env
search. Save the original value in the existing vault and CAS only its reference
into the original unchanged generation/revision, with sanitized repair
provenance. This is not production automatic credential discovery or proof of
automatic restart recovery. If the nonce was consumed and no original token is
saved, do not rotate/restage/register; retain/quarantine and use exact owned
cleanup/external recovery. A fresh operation is allowed only after positively
clearing the original worker fence/resource. Never erase the failed record to
make the fixture appear clean.

## Native job and capacity

Initial profile pins one operator-owned dedicated NodeID/platform/datacenter and
namespace. No shared/general-purpose worker eligibility mutation is permitted.
Use one long-running batch job, one group count=1, explicit no automatic restart,
zero reschedule attempts/unlimited=false, disconnect replace=false and
keep-original reconciliation. Validate canonical API policy on the pinned Nomad
release, initially 2.0.7. If batch/disconnect policy is unsupported or can create
a replacement after loss, fail verification and revisit this bounded policy;
never silently take defaults or add a scheduler. The exact node pin and durable
admission interlock remain required through disconnected/lost states.

`Disconnect.LostAfter` is profile-set (see job-identity).

The raw_exec task first acquires the durable worker start fence, then runs a
finite bootstrap shell and `exec`s agentctl; no custom
supervising loop remains. Nomad reserves dynamic native control and instance
ports. Set `AGENTCTL_PORT` and set `AGENTCTL_INSTANCE_PORT_BASE` equal to
`AGENTCTL_INSTANCE_PORT_MAX` at that single reserved port. Reject extra instances
instead of using an unreserved port range. Launch admission must ensure raw
agentctl binds loopback, or equivalent worker firewall restriction, rather than
exposing authenticated-but-public ports accidentally.

CPU/memory requests include runtime/bootstrap overhead. Capacity derives from
the node's configured resources minus system reserve. Distinct A/B environment
jobs demonstrate real admission: A consumes over half a configured resource, B
has the same request and remains unallocated until A is safely released. Count=1
only describes that job group. macOS native resources are reservations, not hard
limits; unrelated native processes are outside Nomad authority.

## Durable worker start fence

Reactive node ineligibility cannot alone prevent this race: A's agentctl leader
crashes, its separate managed groups survive, and Nomad schedules already-queued
B after freeing A's reservation while Kandev is down. Therefore every native
bootstrap must acquire a non-expiring worker-local start fence before executing
agentctl or any agent/workspace/terminal workload. This is a safety lock, not a
placement or resource scheduler; Nomad still decides resource admission.

The first support boundary is one active managed environment per dedicated
worker, independently of Nomad's CPU/memory reservations. Under a private
worker-owned root outside allocation directories, atomic mkdir of
`<state-root>/<NodeID>/active-environment` is the linearization point. The
winning bootstrap writes the full immutable job/operation/environment-generation
and allocation identity before launch, atomically publishes an owner record,
and never treats an incomplete record as unlocked. A different bootstrap
cannot remove, overwrite, lease-expire, or steal it based on PID/port/task state.
Identical retries may inspect the owner but cannot launch a second controller;
only the original successful bootstrap crosses its launch marker. A crash in
any preparation window fails closed.

No timeout, leader exit, allocation GC, backend restart or absent job releases
this fence. Only the exact generation's authorized successful durable drain
plus whole-job/live-node stop evidence permits compare-owner cleanup of the
marker and empty-directory removal. Do not use recursive deletion or repeat
removal after another bootstrap acquires a new fence. If cleanup or ownership
is ambiguous, leave the lock, quarantine the node and require explicit externally
verified operator recovery. Dedicated worker account/NodeID/root mapping is
admitted and persisted; allocation filesystem cleanup cannot delete the fence.

B may become a Nomad allocation after A crashes, but its bootstrap must reject
before agentctl/workload starts when A's fence remains. It exits with a sanitized
blocked outcome and no automatic restart/replacement. Test this with B queued
before killing A's leader and the backend unavailable, while A's child/grandchild
remain alive. Ordinary positive A cleanup releases the exact fence before B
crosses its own start barrier. This lock deliberately serializes productive
work on the first worker; increasing concurrent native environments is separate
scope requiring a corresponding complete safety design.

## Native bootstrap and secure transport

Reuse exact current helpers in the lifecycle package:

- `ResolveSSHTarget` and `dialSSH` in `executor_ssh_connection.go`, requiring an
  admitted pinned host fingerprint (never test-mode trust-on-first-use).
- `detectRemoteInfo`, `ensureAgentctlOnHostWithProgress` and file-upload helpers
  in `executor_ssh_operations.go` for selected platform artifact integrity and
  remote byte transfer.
- `StartPortForward`/`SSHPortForwarder` for control and instance forwarding,
  `remoteControlHandshake`, and `createRemoteAgentInstance` for existing
  authenticated agentctl protocol. Extract a small common helper only when a
  method combines reusable connection work with SSH-owned launch/cleanup.
- `openSSHRuntimeAPITunnelForRequest` in `executor_ssh_runtime_api.go` for existing
  reverse runtime-to-backend API connectivity, with recorded recovery metadata.

Do not call `SSHExecutor.CreateInstance`, `startAndForwardAgentctl`,
`startAgentctlAndHandshake` or `startRemoteAgentctl`: those launch/kill their own
controller and could create an unmanaged second process. Nomad alone starts the
job. Helpers that auto-retry by starting a new process are not reusable as-is;
auth rejection becomes an exact-resource failure/cleanup, not another launch.
Nomad owns process stop; SSH helper process-kill paths must not run behind it.

First resolve and verify the exact worker/SSH NodeID mapping, architecture,
known host key and native tools. Use `AgentctlResolver` to select the binary,
copy bytes via existing SSH transfer with digest/atomic rename and private
ownership, then submit a job referencing the verified worker-local path. A
controller-local path is never a remote path. Per-environment private bootstrap
material lives under an exact 0700 allocation/workspace root with 0600 files and
symlink/foreign-root rejection; secrets travel via SSH stdin, not Nomad job env,
templates, argv or logs. Agentctl receives the nonce only from its private
bootstrap process environment. Preserve existing managed Git credential policy.

After Nomad allocates, forward the reserved control port over the authenticated
SSH connection, handshake with the one-use nonce, establish ServerIdentity and
drain capability, create the Kandev instance on its reserved port, then forward
that port and use normal `agentctl.NewClient` with bearer/execution identity.
HTTP/WebSockets inside SSH retain existing Kandev auth. No remote public HTTPS
lease, custom gateway or provider callback route is required. Colocation uses
the same SSH boundary initially; explicit local fast-path optimization is out
of scope. If SSH is unavailable, reject the profile instead of exposing a port.

SSH connection loss closes only transport and marks unavailable. It cannot prove
process exit or authorize replacement. Reconnect forwards the same ports after
exact job/allocation/node/server identity checks; restore the reverse API tunnel
without sending a new prompt. Reuse watchdog/liveness primitives without copying
a second session supervisor.

## Sealed drain capability

Current `instance.Manager.Shutdown` already seals new creates, drains admitted
creation/abandonment, retries provisional teardown and calls `stopInstance`.
`stopInstance` closes process admission and invokes `StopForTeardown`.
`process.Manager.CloseAdmission`, `BeginOwnedOperation`, `admitStart`,
`WaitForAdmission` and `ProcessRunner` admission cover managed spawn ownership.
Current `/api/v1/ownership/shutdown` replies `shutting_down` before this work
finishes; it is not a cleanup receipt.

Add **new** `runtime-seal-drain.v1` capability and authenticated
`POST /api/v1/runtime/seal-and-drain` with `request_id`,
`expected_server_identity` and bounded deadline. Require the current control
credential, not the stale adoption-only credential accepted by ownership
shutdown. Existing `Config.ServerIdentity` is generated per process launch;
verify continuity through authenticated control, not unauthenticated identity
alone. A host-selected runtime without this capability rejects before ACP start.

The handler atomically seals control/instance create and ownership admission,
serializes with main-loop shutdown and calls the existing manager's Shutdown.
Audit all process owners: ACP start/configure, custom commands/ProcessRunner,
terminal/StartShell, LSP start/install, preview processes, Git/materialization/
polling/comparison operations, admitted instance creation and provisional work.
Each either completes under admitted ownership and joins drain, or rejects. Fix
only demonstrated admission gaps; do not create a second teardown manager.

Return a successful response only after all managed teardown succeeds: request
ID, ServerIdentity, sealed=true, drained=true, scope=managed_process_groups and
version. Keep control and the leader alive but permanently sealed until Nomad
stop. Cache successful identical request retries for that sealed launch.
Wrong identity/conflicting request reuse fails; failed teardown stays sealed
and retries retained owners. Bound WaitGroup/deadline behavior explicitly; a
detached goroutine returning early is not completed teardown evidence.

The built-in executor creates the request under the current environment cleanup
claim and persists its receipt with resource/state fingerprint, original
operation, environment generation, runtime/execution identity, ServerIdentity,
request ID, scope and claim before any whole-job stop. The fingerprint covers
immutable resource identity and admitted launch inputs; mutable phase, retry
fields and the receipt itself are excluded so checkpointing cannot invalidate
its own proof. On restart, reuse only
that exact durable receipt. Rebinding to a newer authorized cleanup claim is an
explicit core validation; stale callers cannot replay an old claim against a
successor. No public SDK field or browser-provided proof is needed.

## Terminal cleanup and quarantine

Positive path: current task/environment cleanup authorization -> exact live
control server seal/drain -> durable receipt checkpoint -> remove only owned
sensitive bootstrap material while sealed leader is alive -> stop entire
immutable Nomad job, purge=false -> confirm all exact allocations/tasks terminal
on the original responsive NodeID, with no successor -> checkpoint absence ->
release the exact worker start fence, then release inventory. Repeated cleanup
using the matching durable receipt/stop confirmation is idempotent. Do not purge history before durable final absence.

For failed-launch rollback before runtime launch, absence requires proof that
no submit happened or the exact launch barrier was never crossed and the owned
job stopped. A missing descriptor/job is not such proof. Crashed runtime,
failed/lost drain, changed ServerIdentity, incomplete process history, lost node
or unexpected allocation leaves cleanup_pending. Try stopping the whole owned
job, but its terminal state cannot stand in for the missing managed-group proof.

The non-expiring worker fence prevents workload launch even before core reacts.
Quarantine must also prevent placement before any cleanup release/terminalization:
validate dedicated node ownership and mark it ineligible through Nomad, persist
a CAS quarantine marker and refuse new operations for that node. If the control
API is unreachable, durable local pending-quarantine inventory prevents all new
admission; on reconnect reconcile node eligibility before dispatch. Preserve
the interlock after job/allocation history GC. Disable restart/reschedule/
replacement policy so a server cannot create a successor behind the interlock.
Any observed successor is an invariant failure and retains quarantine.

Readmission is an explicit operator action after recovered matching successful
drain/stop evidence or external whole-worker process absence verification, such
as a controlled reboot with changed boot identity plus owned-workspace audit.
No automatic reboot, broad PID kill, inventory erase or node re-enable is added.
Never override a newer operator decision to keep the worker ineligible. Native
daemonization/hostile escape is outside the supported receipt scope and remains
uncertain, not successful cleanup.

## Security and diagnostics

Use admitted pinned SSH authentication and least-privilege Nomad namespace/job
permissions plus eligibility authority for the explicitly dedicated worker.
Verify actual ACL node-scope enforcement and document it. Raw_exec runs trusted
native code under the worker account and is not tenant isolation. Secrets stay
in Kandev's existing vault/transient SSH channel and private bootstrap files.
Never copy profile secrets into Nomad specs, ordinary metadata, argv or logs.

Existing progress and error projections report provisioning, artifact,
bootstrap/readiness, unavailable and cleanup_pending; no new UI is planned.
Logs identify environment/generation, operation, job, allocation and NodeID with
closed outcome/stage, never raw agent payloads, nonce, token or private keys.
Results/files/Git/terminal remain agentctl-owned. Map Nomad errors to sanitized
bounded outcomes rather than forwarding raw response bodies.

## Verification and distribution

Use the [plan](../../../plans/nomad-native-executor/plan.md) work-order checks.
Build the ordinary Kandev binary with the executor registered; configure a test
profile through existing backend APIs in an isolated database. No production
plugin package or frontend install is required. Runtime config/type additions
must preserve existing profile/bootstrap contracts without adding rendered UI.

Real proof must start inert ACP through Kandev and record its actual child and
grandchild process identities; no model/account call is needed. Cover actual
A/B capacity, committed-response loss, backend restart, actual Nomad client/server
partition, positive drain+whole-job stop and negative quarantine. Prepare-only,
health-200, mocks, skips and no-tests-to-run do not satisfy real criteria.

