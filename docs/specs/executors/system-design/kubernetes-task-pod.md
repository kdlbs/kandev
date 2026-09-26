---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
---

# Kubernetes task pod system design

## Boundary and evidence

The executor system owns physical Kubernetes compute; task environments own
canonical workspace identity. This design implements
[task pod requirements](../requirements/kubernetes-task-pod.md).

Current `KubernetesExecutor.CreateInstance` locks by `req.InstanceID`, takes the
reconnect branch only with recorded pod metadata, and otherwise calls
`createFresh`. `newKubernetesFreshLaunch` derives pod/PVC names from that instance
ID. `kubernetesIdentity` includes the session ID. Consequently distinct session
executions allocate distinct pods even when their task/environment IDs match.
`StopInstance` can delete those resources on force/terminal cleanup.

`connectNewAgentctl` already uses the agentctl control server's `CreateInstance`
API. Reuse that multi-instance boundary instead of launching another agentctl
server or copying a sibling's execution identity.

## Requirement mapping

| Requirement | Implementation boundary |
| --- | --- |
| `REQ-EXECUTORS-KUBERNETES-TASK-POD-001` | Durable ownership, attach/recovery, cleanup, and projection below |

## Durable ownership

Add a typed Kubernetes runtime record associated with the canonical
`TaskEnvironment`, separate from `executors_running`. The latter remains the
per-session execution inventory; it must not be the sole physical resource owner.
Use a dedicated `task_environment_kubernetes` table with environment ID as its
primary key and task ID constrained to one canonical Kubernetes runtime record.
Do not overload Docker `ContainerID` or its control-secret fields.

Record executor ID, pinned workload profile/snapshot, environment ownership
generation, immutable resource identity, namespace, Pod/PVC names and UIDs,
managed-claim ownership, admission checkpoints, create nonce continuity, container
restart observation, and encrypted control-token/bootstrap-nonce secret references.
No plaintext credential enters this record or a public DTO. Environment deletion
must not cascade this inventory before remote cleanup succeeds. The inventory
has no cascading environment foreign key: task deletion removes task/environment
rows synchronously and durable cleanup runs afterward. Direct environment
deletion is guarded while inventory exists. A separate cleanup-only claim can
use the retained owner/generation only when both original owner rows are absent;
normal launch claims still require a live matching environment.

Keep resource ownership separate from session identity. New shared resources use
an environment-derived stable resource instance identity and a versioned ownership
label contract; their authorization must not depend on the current attaching
session. Preserve exact legacy ownership labels for adopted legacy resources.
Persist the ownership version and validate its complete expected label set plus
UIDs. Never relax exact UID checks or create-nonce reconciliation.

Repository operations claim creation/recovery/deletion with conditional writes
against generation and operation identity. Creation and deletion claims are
mutually exclusive with attachment. PostgreSQL uses the repository's advisory
transaction lock convention; SQLite uses its existing write serialization.
Do not hold a database transaction during Kubernetes network calls. Persist each
checkpoint before allowing another operation to advance; after restart reconcile
the recorded claim and exact create result before retrying allocation. Startup
fences interrupted operation revisions under the backend’s existing exclusive
runtime-state ownership lock before any worker starts. Incomplete bootstrap
inventory without recoverable control credentials blocks attachment and remains
available for exact task cleanup.

## Initial launch and additional attachment

The canonical environment materializer creates and bootstraps the pod once,
persists its control credentials and resource inventory, prepares repositories,
and publishes ready only after workspace inventory is complete.

An additional launch validates task authorization, canonical environment,
executor identity, generation and complete repository inventory. Read only the
canonical runtime record; a sibling `PreviousExecutionID` is never an attachment
handle. Reject a different executor/profile selection that requires changing the
retained workload. Editing the same profile does not replace its pinned snapshot.

Verify the recorded pod, connect to its control server using environment-scoped
credentials, and create a new agentctl instance with the new execution/session
IDs and canonical workspace. Retain per-instance port-forward/client lifetime.
Resolve each agent profile's environment/auth/MCP configuration for that instance;
attachment must not rewrite pod-wide runtime/auth files or restart the control
server. Audit `buildReconnectCreateInstanceRequest` and bootstrap-file consumers
so a new instance does not accidentally inherit another session's agent identity
or credentials.

Attach-only launch skips cloning, repository setup, bootstrap, and pod-level
prepare scripts. Preserve current task workspace inheritance admission; this
change does not grant cross-task compute sharing when a parent/group environment
cannot safely satisfy the task-owned pod boundary. Return typed unsupported reuse
rather than silently provisioning a separate workspace.

A consumed bootstrap handshake retains its issued token until encrypted canonical
storage acknowledges it. If that update fails, an environment-derived encrypted
recovery secret preserves the token across backend restart; attachment retries
finish the canonical save and remove the recovery secret before proceeding.
Request cancellation does not cancel the bounded credential persistence attempt.
Task teardown also removes the recovery secret.

## Credential trust boundary

The task is one trust boundary, as recorded in the
[credential trust ADR](../../../decisions/2026-09-23-kubernetes-task-trust-boundary.md).
Different agent profiles and credential bindings are allowed within its pod.
Per-session HOME, environment and auth files separate configuration; all agents
run as the same OS user and can read sibling files and process credentials.
File permissions and hashed session paths do not provide security isolation.

Operators must attach only mutually trusted agents and credentials. After a
suspected compromise, stop the task's agents and revoke or rotate exposed
credentials with their providers; stopping one session cannot undo exposure.
Use separate tasks with appropriately isolated executor policies when agents
must not share trust. Regression coverage verifies distinct profiles and
credentials use one pod without overwriting each other's launch settings.

## Session termination and task cleanup

Session stop/delete terminates only that agentctl instance and its local
connections. Force is not authority to delete the task's resources. Failed
attachment rolls back only resources allocated by that attachment.

Task archive/delete resource cleanup claims the environment for deletion, blocks
new attachment, settles all attached executions through the existing task cleanup
orchestrator, and verifies that no execution still needs the resource. One stopped
or failed row is insufficient when any sibling is live. Run cleanup scripts from task teardown rather than individual session stops,
then perform exact-UID/ownership-checked Pod and managed-PVC deletion.
Retain inventory and credentials for retry on partial deletion failure. External
claims are never deleted. Zero-session environments remain discoverable for
retention/status and terminal cleanup.

## Recovery and compatibility

Backend restart reconstructs environment control connections independently of
session connections. A single generation claim owns missing-pod replacement from
the pinned launch snapshot and existing managed PVC. Other resumes wait or return
a recoverable preparing result; they cannot provision replacements independently.
A pod/container restart invalidates all affected per-instance clients; refresh
projects the same physical generation while each session reconnects independently.

Migration is additive and non-destructive. At first use, adopt legacy inventory
only when all relevant retained rows agree on one exact pod/PVC identity, matching
canonical task/environment ownership, and available encrypted control credentials.
Preserve legacy resource labels and record the ownership version. Multiple pods,
conflicting snapshots, missing identity, or unreadable inventory block additional
attachment. Existing recorded legacy sessions retain their current resume/cleanup
path; do not rewrite their files, move claims, or delete a supposed duplicate.

An explicitly selected executor transition must use the existing task environment
transition contract and cannot attach to stale Kubernetes state. This work does
not add automatic executor migration.

## Status and diagnostics

`internal/kubernetes/sessions.go` keeps authorized session rows but resolves shared
physical identity through their canonical runtime record. Probe each exact pod
once per request and project its result to authorized rows; session counts remain
session counts. Retained environment visibility must survive removal of its last
session. Never expose control secrets. Existing desktop/mobile components consume
the same response shape, so this package introduces no layout or control changes.
Log task/environment and execution identifiers for attach/create/recover/delete
operations and bounded failure categories, without credential payloads.

## Verification

Use fake Kubernetes clients through real lifecycle entry points for two-session
attachment, concurrent launch/recovery, stop-one/sibling-live, forced session
cleanup, failed attachment rollback, task cleanup, retained zero-session state,
profile snapshot pinning, per-instance env isolation, and foreign UID rejection.
Repository tests cover claim exclusion, replay, and mixed legacy inventories.
Kind E2E starts two real sessions, observes one Pod/PVC and distinct instances,
checks an untracked file from both, stops one, restarts the backend, resumes, then
archives the task and checks cleanup. No production cluster reproduction is needed.

## Related decisions

[Task-owned Kubernetes compute](../../../decisions/2026-09-21-kubernetes-task-pod-ownership.md).
