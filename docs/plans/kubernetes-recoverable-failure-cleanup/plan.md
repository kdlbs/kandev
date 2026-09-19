---
created: 2026-09-19
status: blocked
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
system_design:
  - ../../specs/executors/system-design/kubernetes-failure-recovery.md
legacy_specs:
  - ../../specs/kubernetes-executor/spec.md
---

# Implementation Plan: Kubernetes Recoverable Failure Cleanup

## Overview

Preserve the runtime needed by Resume after a recoverable Kubernetes agent
failure. One sequential work order covers the cross-boundary regression and fix.
Implementation was authorized by the user after the design handoff.

## Evidence and root cause

Read-only backend diagnostic bundle `84da3c3c3d03360d63cbfda4f5e1e579`
reported ready with no warnings or lost log entries. Running version:
`v0.94.0-247-ge3ed7bfd5`; inspected checkout: `794773dd6`.
Affected task: `73dca427-4819-4ac5-9b44-4ac65a53a486`, session:
`c4a5774f-a68e-4f16-91f3-286901e75f8b`.

Retained `backend-logs.log` timestamps on 2026-09-19, UTC+01:00:

1. 13:10:41: backend shutdown stops initial Kubernetes execution `30c4651b...`
   without force. At 13:19:27, execution `3767871c...` resumes successfully.
2. 14:30:03: agent reports `Internal error`; orchestrator handles it as
   recoverable, then stops execution with `reason=agent completed, force=true`.
3. 14:30:04: stop finishes successfully. At 14:31:11 idle reclaim preserves
   the runtime row for resume.
4. 14:35:58: relaunch fails resolving `env_secret_id_AGENTCTL_AUTH_TOKEN`,
   referencing the original `kandev-runtime:30c4651b-7325-4e22-b2e4-eb8d1363ebd6:agentctl-auth`.

Source trace: `handleRecoverableFailureLockedState` ->
`cleanupAgentExecution` -> forced `StopExecution` ->
`StopAgentWithReason` -> `KubernetesExecutor.StopInstance` -> exact resource
deletion and `deleteKubernetesRuntimeSecrets`. The old resource-instance ID is
expected across resume; its reuse alone is not the defect. Destructive cleanup
while promising recovery is the defect.

The bundle proves the cleanup request and subsequent missing secret. It does
not establish the PVC storage mode, current cluster state, or recoverability of
files. The original provider `Internal error` remains unexplained by these logs.
No live runtime or database was modified during diagnosis.

## Scope

### In scope

Recoverable established-session failure teardown, exact-execution concurrency,
credential/resource retention, and real error-to-Resume verification.

### Out of scope

Repairing this historical session's data, masking missing secrets, changing
provider routing, fresh launch rollback, or changing other runtimes. UI cleanup
is sibling task `e8757499-cfd8-4715-8b04-81fb7c1f3749`, created without a parent.

## Technical approach

Implement the [design](../../specs/executors/system-design/kubernetes-failure-recovery.md)
using a specific recoverable-failure stop reason and lifecycle-owned Kubernetes
retention. Keep generic forced cleanup and explicit archive/delete behavior.
No schema or public API changes. Reuse existing teardown claims rather than
introducing a second cleanup coordinator.

## Tests

New `event_handlers_kubernetes_recovery_test.go` in orchestrator:
`TestKubernetesRecoverableFailurePreservesResume`, covering criteria .1-.2;
`TestKubernetesRecoverableFailureCleanupBeforeWorkflowResume`, covering .3,
including publisher return and immediate workflow-triggered resume. Duplicate
cleanup with persisted inventory is covered at the lifecycle boundary.

New `manager_kubernetes_recoverable_failure_test.go` in lifecycle:
`TestKubernetesRecoverableFailurePreservesRuntimeAndSecrets`, covering .1-.2;
destructive-stop and non-Kubernetes controls for .4; existing auth resolution
tests for .5. Use fresh and resumed established executions in the matrix.

## E2E tests

Add `apps/web/e2e/tests/kubernetes/kubernetes-failure-recovery.spec.ts` in
`containers` with `preserves Kubernetes workspace after recoverable agent error`.
Use the existing mock error scenario, a sentinel file, existing Resume control,
exact Pod/PVC identities, restart, and archive assertions. Covers .1-.4.
No component markup or mobile composition changes are planned.

## Work orders

- [ ] [Task 01: Preserve Kubernetes recoverable runtimes](task-01-preserve-recoverable-runtime.md)

## Verification results

Initial orchestrator and lifecycle regression failures reproduced destructive
cleanup. A blocked-stop regression reproduced workflow Resume preceding cleanup;
a publisher-return assertion reproduced synchronous cleanup blocking publication.
The retained-inventory duplicate regression reproduced fallback into destructive
persisted cleanup. Those focused tests now pass. The existing dynamic successor
failure test exposed a session-guard deadlock; its caller now defers cleanup.
Final race-enabled tests passed for both packages, including a rerun after the
last production change. Kind verification is blocked by local fixture setup:

- Initial managed build was rejected as stale after a production edit. Rebuilt
  the required Linux artifacts and reused the completed Vite/plugin builds.
- First refreshed run failed before the test body: `kind load docker-image`
  exceeded the fixture's 180-second timeout.
- One retry with `E2E_DEBUG=1` reused image layers but failed creating the Kind
  node: no log line matched `Reached target .*Multi-User System.*|detected cgroup v1`.
- Both attempts discovered one test. Neither executed the recovery assertions.
  Fixture cleanup removed their owned clusters and image tags. Do not count
  either run as evidence for Pod/PVC retention or backend-restart recovery.

Focused Go race checks, targeted Go lint (0 issues), ESLint, Prettier,
architecture lint, and documentation
validation passed. The remaining gate is a successful Kind scenario on a
working local/CI container runner. The user explicitly requested a ready PR
with this known integration-verification blocker disclosed.
Read-only lifecycle trace: confirmed the destructive failure-cleanup path.
`python3 scripts/list-docs.py validate`: passed (291 decisions, 1040 specifications).
`python3 scripts/lint-spec-files.test.py`: passed (36 tests).
`python3 scripts/lint-spec-files.py --all`: passed.
`git diff --check`: passed.

## Risks

- Already-deleted credentials and managed workspace data cannot be restored by
  this patch. Current affected-session recovery requires separate evidence.
- Late cleanup can race immediate Resume; preserving secrets alone is insufficient.
- A global change to force cleanup can leak runtimes or weaken explicit cleanup.
- Kind validation requires Docker and disposable resources; never use the
  affected live cluster for reproduction.
