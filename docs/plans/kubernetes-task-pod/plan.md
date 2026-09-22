---
created: 2026-09-21
status: done
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Kubernetes task pod

## Overview

Replace per-session pod allocation with one task-owned pod containing independent
agentctl instances. Deliver additive durable inventory first, integrate lifecycle
ownership second, and prove the full flow with Kind plus operator documentation.
All three work orders are complete.

## Scope

### In scope

Task-owned Pod/PVC inventory and control credentials, safe session attachment,
concurrent creation/recovery, session-only termination, task-owned cleanup,
status projection, and non-destructive legacy compatibility.

### Out of scope

Cross-task pod pooling, automatic merging of existing pods/workspaces, new UI
controls, and changing workspace inheritance or executor-transition semantics.

## Confirmed evidence

Source trace, not a reproduction on the user's cluster:
`KubernetesExecutor.CreateInstance` locks by execution ID and chooses
`createFresh` without recorded pod metadata. `newKubernetesFreshLaunch` calls
`kubernetesResourceNames(req.InstanceID)`, so different sessions create different
pods. The foundation explicitly requires one pod per session and excludes
concurrent sharing. `StopInstance` also owns terminal pod deletion. The requested
behavior therefore changes physical ownership, not just a reuse lookup.

Smallest reproduction: create session A and then session B in the same ready
Kubernetes task environment with distinct execution IDs; compare Pod UIDs and
managed PVCs. The regression must assert one pod, one claim, two agentctl
instances, and unchanged workspace content.

## Technical approach

Follow the [design](../../specs/executors/system-design/kubernetes-task-pod.md)
and [requirements](../../specs/executors/requirements/kubernetes-task-pod.md).
Add `task_environment_kubernetes` with durable operation/generation exclusion and
secret references through the existing task repository and lifecycle adapters.
Keep `executors_running` per-session. Integrate create, attach, reconnect,
checkpoint, refresh, stop, cleanup, and session-status resolution together so no
intermediate shared-resource path retains destructive session cleanup.

## Tests

Proposed regression files/methods (names are implementation targets):

| Criteria | Evidence |
| --- | --- |
| AC-EXECUTORS-KUBERNETES-TASK-POD-001.1, .2 | `executor_kubernetes_task_pod_test.go`: `TestKubernetesTaskPodAdditionalSession`, `TestKubernetesTaskPodConcurrentAttach`; independent env/auth and unchanged files |
| AC-EXECUTORS-KUBERNETES-TASK-POD-001.2, .4, .5 | `environment_kubernetes_test.go`: `TestKubernetesEnvironmentClaimExclusion`, `TestKubernetesEnvironmentRestartReplay`; PostgreSQL multi-connection counterparts |
| AC-EXECUTORS-KUBERNETES-TASK-POD-001.3, .5 | `executor_kubernetes_task_pod_test.go`: `TestKubernetesTaskPodStopPreservesSibling`, `TestKubernetesTaskPodTerminalCleanup`; live + failed mixed rows |
| AC-EXECUTORS-KUBERNETES-TASK-POD-001.4, .6 | `executor_kubernetes_task_pod_test.go`: `TestKubernetesTaskPodConcurrentRecovery`, `TestKubernetesTaskPodLegacyAmbiguity`; foreign UID, partial failure, retained zero-session inventory |
| AC-EXECUTORS-KUBERNETES-TASK-POD-001.7 | `sessions_task_pod_test.go`: `TestSessionsSharedTaskPod`; lifecycle profile pinning and instance environment tests |

## E2E tests

Add `apps/web/e2e/tests/kubernetes/kubernetes-task-pod.spec.ts` in the
`containers` project using the existing disposable Kind fixture. Cover criteria
.1 through .5 and .7 with sequential and concurrent second-session creation,
shared files, stop-one, backend restart/resume, and final archive cleanup.
Criterion .6 uses exact fake-client/repository tests, with a Kind ownership
mismatch scenario if the existing fixture can inject it safely.
No rendering change is planned; desktop/mobile consume the same status contract.

## Work orders

- [x] [Task 01: Durable environment inventory](task-01-durable-inventory.md)
- [x] [Task 02: Shared pod lifecycle](task-02-shared-lifecycle.md)
- [x] [Task 03: Kind evidence and operator docs](task-03-kind-docs.md)

## Verification results

SQLite and PostgreSQL inventory regressions pass with race detection. The targeted
Kubernetes lifecycle, task service, status, orchestrator executor, and backend
wiring packages pass with race detection. Eleven desktop/mobile component tests
pass. Public-doc validators, spec catalog/linter, and SQL guard pass.

The Kind regression passed (1 test, 6.9 minutes), proving three concurrent/shared
sessions in one Pod UID and one managed PVC, distinct per-session auth files,
shared untracked data, force-stop isolation, a real response after backend
restart, healthy status rows, and final archive deletion of the Pod/PVC.

Live runs exposed and fixed task-delete inventory retention, private HOME creation,
and missing persistent ownership markers. The passing run also exposed a cleanup
retry after local execution identity changed. A follow-up failing-then-passing
regression verifies exact remote-session connection cleanup after that rebind;
the final lifecycle race suite and lint pass. Kind was not repeated after this
narrow connection-cleanup fix.

Cold-host setup required temporary image-load/fixture deadlines. Those edits were
restored; the test itself uses a supported 120-second executor request timeout for
cold PVC provisioning. All disposable Kind clusters were removed by teardown.

Mobile state-only change: use a task/session composite row identity so multiple
retained task Pods without sessions stay distinct. Existing desktop table and
mobile linked cards keep their composition and touch behavior. Targeted component
coverage exercises both layouts; no new viewport interaction is introduced.

## PR remediation

Review fixes add typed agentctl not-found errors, request-scoped canonical
inventory/Pod caching, required-table upgrade probes, and concurrent stop/attach
coverage. Credential persistence recovery retains the issued token until its
canonical encrypted record is saved, using an encrypted recovery secret across
backend restart. Cleanup detaches cancellation with a one-minute budget. Updated backend guidance distinguishes
physical environment ownership from per-session inventory.

CI remediation also incorporates the current base and fills 32 missing Japanese
SSH strings. The base's preview-feedback subscription requires an updated exact
subscription count; no WebSocket production behavior changes for that fix.
Post-fix verification: targeted lifecycle/status race regressions, the full
agentctl race suite, SQLite store conformance and previous-stable upgrades pass.
The WebSocket subscription regression passes 20 race-enabled runs. Full i18n
checks and three current component tests pass; harness/spec checks pass.
Remote CI/review completion remains pending. The security review identifies
shared-UID credential access across siblings; resolution requires an explicit
trust/isolation policy decision and is not claimed complete here.

## Risks

Control authentication and bootstrap files currently assume one session; sharing
must not replace sibling credentials. Legacy tasks can contain genuinely different
workspaces. Durable claims must recover after crashes without duplicate API creates.
Public docs reflect the implemented and verified behavior. Persistence coverage requires a disposable
PostgreSQL database; Kind coverage requires Docker and the pinned worker image.
