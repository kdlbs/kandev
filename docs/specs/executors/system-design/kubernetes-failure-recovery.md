---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
---

# Kubernetes Failure Recovery Design

## Boundary

Implements [retained failure recovery](../requirements/kubernetes-failure-recovery.md)
within the existing [resource ownership decision](../../../decisions/2026-08-24-kubernetes-executor-resource-ownership.md).
No new database table, public endpoint, or resource ownership rule is needed.
Reuse the existing per-boot token generation, bootstrap handshake, and encrypted
credential recovery protocol for both attachment and stop. The foundation's inventory and identity checks remain authoritative.

## Failure teardown

`handleRecoverableFailureLockedState` currently schedules
`cleanupAgentExecution`, which calls `StopExecution` with reason `agent completed`
and `force=true`. Kubernetes interprets force as destructive Pod/PVC teardown;
`StopAgentWithReason` subsequently deletes authentication and bootstrap secrets.
The recoverable task state and retained inventory then disagree with reality.

Use a distinct semantic reason for recoverable agent-failure teardown. Keep the
existing exact-execution ownership claim and activity retirement. Do not globally
change the shared cleanup helper's force setting: it also serves terminal,
orphan, and stale-resume callers.

At the lifecycle boundary, use the recorded execution runtime to make this
specific reason non-destructive for Kubernetes, whether the established
execution originated from a fresh launch or a resumed session. Stop the failed
agent through agentctl with a bounded cleanup context, close local connections,
and release the in-memory execution slot. Preserve authoritative runtime
inventory and both secret references and values. Other runtimes retain their
existing force behavior. Explicit destructive reasons retain priority.

The existing failed-resume-bootstrap retention path in
`manager_kubernetes_resume_bootstrap_cleanup_test.go` provides the nearest
pattern. Startup authentication and managed-NPM failures carry the bootstrap
stop reason, so resumed startup retains resources while fresh-launch rollback
remains destructive.

## Concurrency and failure handling

Retain the exact `(session, execution)` teardown claim and stale-event guards.
Failure cleanup and a subsequent resume must be ordered so predecessor cleanup
cannot tear down the newly attached execution or its shared retained Pod. Cover
this ordering with a blocked-stop test, including synchronous `on_agent_error`
restart. Run cleanup and its subsequent workflow callback together after releasing
the session guard, on a service-owned worker: lifecycle failure publication can
hold the prompt lock until its subscribers return. Do not add a session-wide
lock across callbacks that reacquire it. A repeated recoverable stop for an
execution no longer tracked in memory must not enter destructive persisted
Kubernetes cleanup.

Keep secret lookup failures fail-closed in `resolveLaunchAuthToken`.
Regenerating a secret would not restore a deleted PVC or authenticate to an
already-running agentctl. Do not silently migrate broken historical sessions.
Cleanup errors release only the exact failed teardown claim so a later attempt
can retry; they suppress workflow dispatch. A successful teardown records
completion on its claim, allowing workflow redelivery without repeating cleanup.
An in-flight duplicate cannot dispatch ahead of the owning cleanup. An already
absent execution is a successful cleanup outcome.

Recovery workers share the dynamic-successor cancellation context and wait group.
Shutdown rejects new workers, cancels active work, and drains within the existing
shutdown bound. Workflow recovery checks cancellation after cleanup and receives
the worker context, so a stop finishing during shutdown cannot start Resume.

## Resume environment sources

Criteria .6-.8 use the authoritative `executors_running` profile identity.
`applyRecordedKubernetesExecutorConfigToResumeRequest` restores current executor
connection settings and immutable recorded workload metadata, then calls
`restoreKubernetesProfileEnvironment` to load only `ProfileEnvVars` from that
recorded profile. The mutable session profile selection and current profile
workload configuration cannot replace the recorded runtime snapshot. Resume also
restores the session's `ExecutorProfileID` before the existing guarded full-row
persistence, so `configureExistingWorkspace` and the lifecycle
`ExecutorProfileEnvForSession` reader use the same profile on later starts.

The environment definitions flow through `resolveLaunchEnvironment` to the
existing lifecycle launch checkpoint. Literal and secret-reference definitions
remain distinct; this helper does not resolve secrets or change precedence.
Current profile environment edits therefore apply to resumed processes.
`ErrExecutorProfileNotFound` or an absent profile returns no definitions;
other repository failures and executor ownership mismatches return an error.
No missing profile is replaced with a different profile.

## Shared stop after control-server restart

Criteria .9-.13 repair the task-owned stop path. On 2026-10-07 a container OOM
restarted agentctl with a fresh token. `stopSharedKubernetesInstance` used the
saved token and removed `sharedSessions` before DELETE returned 401. Manager
cleanup retained the execution, but subsequent refresh had lost its connection
inventory. This section supersedes the assumption that attachment-only token
recovery is sufficient. It does not change error propagation or UI diagnostics.

Keep the exact instance lock and bounded service-owned cleanup context. Read the
canonical environment runtime, verify task/executor ownership and recorded Pod
UID/labels before opening control transport. Stop must not create a replacement
Pod, PVC, or remote agent instance. Existing missing-Pod handling remains separate
and must prove absence through the Kubernetes API; connection failure is not
absence.

Extract the authenticated control-operation boundary currently used by
`connectSharedKubernetesAgentctl`. Under the existing `control:<environment>`
lock, load pending/recovery/canonical credentials, open a healthy control forward,
and attempt the requested operation. On typed authentication failure (401),
re-handshake once using only the recorded bootstrap nonce, persist the issued
token through `persistSharedKubernetesControlToken`, and retry the same operation
once. Do not treat arbitrary 403, timeout, or identity errors as permission to
bootstrap. Do not recover by minting a replacement nonce. Attachment continues
to create instances; stop only deletes its recorded instance ID. HTTP 404 from
that authenticated DELETE is success, including when the restarted server has
no instance registry.

Retain existing in-memory pending-token and deterministic encrypted recovery
secret handling for a consumed nonce. Canonical persistence must finish before
cleanup declares success. If canonical persistence fails, save the token in the
existing recovery secret with a bounded durable context and leave execution
ownership available for retry. If all durable writes fail, retain in-memory
pending state and report failure; recovery across a simultaneous backend crash
cannot be guaranteed without durable storage. Tests distinguish that limitation
from successful recovery-secret persistence. Never log nonce/token values.

Do not close/remove session inventory at the start of stop. Keep an immutable
snapshot or generation-checked recovery handle while remote deletion is pending.
Transient transport may be replaced without discarding recorded recovery inputs.
After authenticated deletion/absence and credential persistence succeed, remove
only the matching `(execution, session, environment, remote instance)` attachment
and close its clients/forwards. A late completion cannot remove a replacement
attachment. On failure retain or reconstruct a usable recovery handle; a dead
socket alone is not useful retained state. Environment credentials are shared,
while connections remain per execution. Manager cleanup releases its execution
slot only after this stop returns success; its existing stop-error retention
requires no public state contract change.

Use one lock order across attach, refresh and stop: instance lock, then existing
environment control lock. Do not acquire instance locks from inside the control
lock or hold database transactions during network operations. Follow existing
bounded persistence behavior and do not call lifecycle callbacks while either
lock is held. Task-terminal cleanup retains generation claims, sibling checks,
exact UID deletion, and recovery-secret teardown.

## Validation

Map criteria .1-.3 to orchestrator failure-handler, teardown ownership, and
lifecycle tests. Verify .4 with existing exact Kubernetes cleanup tests plus
explicit destructive-stop controls. Verify .5 with secret-store missing/error
tests. A Kind scenario must write a workspace sentinel, inject an agent error,
resume, and prove unchanged Pod/PVC identity and sentinel contents. Repeat after
a backend restart; archive at the end to prove cleanup remains available.

No rendered interface changes are required. The existing Resume control is the
user-visible entry point. The separate error-UI task owns presentation changes.

`TestKubernetesResumeRestoresRecordedProfileEnvironment` covers .6 and .8,
including a conflicting session profile, unresolved secret references, and an
unchanged workload snapshot. `TestKubernetesResumeProfileEnvironmentFailures`
covers .7 with deleted, absent, lookup-error, and foreign-executor cases.

## Implementation plan

[Kubernetes recoverable failure cleanup](../../../plans/kubernetes-recoverable-failure-cleanup/plan.md)

[Resume profile environment repair](../../../plans/kubernetes-resume-profile-env/plan.md)


[Restart-safe cleanup and isolated validation delivery](../../../plans/kubernetes-session-resilience/plan.md)
owns criteria .9-.13. Its lifecycle regressions cover stop before polling,
concurrent sibling recovery, consumed-nonce persistence failure, backend restart,
stale completion, and authenticated absence. Disposable Kind acceptance retains
Pod/PVC and workspace bytes, uses the same native conversation, and observes one
continuation execution. Production recovery is not a fault-injection test.
