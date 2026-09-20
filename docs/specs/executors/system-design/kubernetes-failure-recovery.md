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
No new storage, credential rotation, public endpoint, or resource ownership rule
is needed. The foundation's inventory and identity checks remain authoritative.

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
workload configuration cannot replace the recorded runtime snapshot.

The environment definitions flow through `resolveLaunchEnvironment` to the
existing lifecycle launch checkpoint. Literal and secret-reference definitions
remain distinct; this helper does not resolve secrets or change precedence.
Current profile environment edits therefore apply to resumed processes.
`ErrExecutorProfileNotFound` or an absent profile returns no definitions;
other repository failures and executor ownership mismatches return an error.
No missing profile is replaced with a different profile.

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
