---
id: "01-preserve-recoverable-runtime"
title: "Preserve Kubernetes recoverable runtimes"
status: blocked
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.1
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.2
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.3
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.4
  - AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.5
system_design:
  - ../../specs/executors/system-design/kubernetes-failure-recovery.md
---

# Task 01: Preserve Kubernetes Recoverable Runtimes

## Summary

Separate recoverable agent-failure teardown from destructive runtime cleanup.
Keep existing Kubernetes resources and secret values available to Resume while
stopping the failed agent and releasing its local execution slot.

## In scope

- Add the orchestrator regression first; prove the existing handler reaches
  destructive Kubernetes cleanup. Cover fresh and resumed established sessions.
- Route recoverable failure through a distinct stop reason, preserving existing
  exact-execution claims, stream retirement, and non-Kubernetes behavior.
- Apply Kubernetes retention at the lifecycle boundary and prove secret values,
  references, inventory, and remote resources survive.
- Block cleanup in a deterministic test while a replacement is requested;
  cover duplicate events and synchronous workflow restart without deadlock or
  predecessor teardown affecting the replacement.
- Add the Kind scenario named in the plan and verify archive cleanup afterward.

## Out of scope

Live data mutation, recovery-token regeneration, UI redesign, new schema, broad
cleanup refactoring, and unexplained provider failures.

## Acceptance

1. The regression fails before the correction because recoverable failure
   requests destructive Kubernetes teardown, and passes after it.
2. Recovery retains the same workspace and credentials across retry/restart;
   late or duplicate failures cannot damage the replacement execution.
3. Explicit destructive cleanup, missing-secret fail-closed behavior, and
   existing non-Kubernetes teardown remain covered and passing.

## Verification

From the repository root; install workspace dependencies before the first pnpm
command if this worktree has not been installed. Docker is required for Kind.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test(KubernetesRecoverableFailure|HandleRecoverableFailure|HandleAgentFailed|HandleAgentStartFailed|RunDetachedDynamicSuccessorLaunch|ClaimForcedExecutionCleanup|RegisterExecutionStopOwner|HandleAgentStopped)' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'Test.*(Kubernetes.*(Failure|Stop|Cleanup|Resume)|ResolveLaunchAuthToken|PersistedKubernetesInventory)' -count=1)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/kubernetes/kubernetes-failure-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/event_handlers_kubernetes_recovery_test.go` (new)
- `apps/backend/internal/orchestrator/execution_teardown_ownership_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_restart_cleanup.go`
- `apps/backend/internal/agent/runtime/lifecycle/recovery_stop.go` (recoverable stop reason)
- `apps/backend/internal/orchestrator/execution_failure_cleanup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_recoverable_failure_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_resume_bootstrap_cleanup_test.go`
- `apps/web/e2e/tests/kubernetes/kubernetes-failure-recovery.spec.ts`

## Dependencies

None. Execute after the explicit implementation request.

## Risks

The failure handler dispatches workflow callbacks that can immediately resume.
Respect existing cancellation/teardown ordering; do not hold a session guard
across a callback that needs the same guard. Do not change the shared helper's
default behavior for terminal or orphan callers.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/kubernetes-failure-recovery.md)
- [System design](../../specs/executors/system-design/kubernetes-failure-recovery.md)
- [Evidence and source trace](plan.md#evidence-and-root-cause)
- Existing `newAgentErrorTestService`, teardown-claim tests, failed-resume
  bootstrap retention tests, and Kubernetes Kind fixtures.

## Results

Implementation complete; integration verification blocked. The destructive-stop, cleanup ordering, publisher
return, and persisted-inventory duplicate regressions were observed failing and
then passing. Final checks include the existing dynamic successor failure tests,
which exposed and drove correction of another guarded cleanup caller.
Focused non-race regressions plus dynamic failure tests passed after the final
production edit. Race-enabled orchestrator/lifecycle checks passed, including
recoverable failures, event handling, teardown claims, Kubernetes cleanup and
resume controls, persisted cleanup, and auth resolution. A final race-enabled
rerun of `TestKubernetesRecoverableFailure|TestRunDetachedDynamicSuccessorLaunch`
passed for both packages.

Kind verification was attempted twice against refreshed artifacts. The first
failed during image loading (180-second fixture timeout); a debug retry failed
while creating the Kind node (missing expected systemd boot log). Neither
entered the test body. Owned clusters and image tags were cleaned up. Rerun the
same managed `containers` command on a working Kind runner before completing
this work order. The user explicitly authorized committing and opening a ready
PR with the unresolved integration verification disclosed.

The shared Go build cache lost entries during the first race attempt. Successful
runs used `GOCACHE=/tmp/kandev-recovery-go-cache`; the failures were build errors,
not test assertions.

Targeted Go lint passed with 0 issues. The E2E file passed ESLint and Prettier.
Architecture and documentation validators passed. Final Docker inspection
confirmed no owned test containers or runtime image tags remained.


## PR review remediation

- Cleanup failures now block workflow recovery and release their exact claim for
  retry. Completed teardown claims allow deferred workflow redelivery without
  repeating a stop; in-flight duplicates cannot bypass cleanup.
- Recovery workers share service shutdown ownership with dynamic successors.
  Cancellation suppresses workflow Resume, and shutdown drains tracked workers.
- Authentication and managed-NPM startup failures use bootstrap cleanup, keeping
  fresh-launch rollback destructive and resumed-bootstrap retention intact.
- Async workflow route, cancellation, reconciliation, and panic tests now await
  worker completion. SQLite-backed recovery tests use bounded real-time signals.
- Full affected-package race suites passed:
  `go test -race ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -count=1 -timeout=15m`.
  This includes every backend CI assertion that failed on the opening head.
- Container CI reached the new test and exposed an invalid failure injection:
  `/e2e:error` emits text but does not fail ACP. The test now uses the existing
  `/transport-lost` prompt error in separate fresh-session cases with and without
  backend restart. A retries-disabled local run used rebuilt backend, Linux
  helpers, and plugin artifacts, but again hit the Kind image-load timeout before
  entering either assertion path. The run was interrupted during fixture cleanup;
  CI's Kubernetes runner remains the integration gate. Docker also reported that
  it could not immediately kill the owned Kind node; it subsequently exited and
  was removed explicitly. The owned image was already removed by fixture cleanup.
  No recovery assertion is counted as passing from this local attempt.

Architecture lint rejected direct lifecycle imports in new callers; stop reasons
and missing-execution checks now use the public runtime facade. After that
adjustment, race tests for Kubernetes recovery, dynamic failure, all agent-error
workflow routes, runtime missing-execution handling, and stop paths passed across
orchestrator, runtime, and lifecycle. Architecture lint passed.

A subsequent review exposed the same already-absent outcome in transient retry
teardown. The R5 workflow-route regression failed for a missing execution, then
passed after the transient path adopted the runtime facade's `IsNotFound` check.
Success, already-absent, and real stop-error controls now run together. The race
command `go test -race ./internal/orchestrator -run 'TestDispatchKanbanAgentErrorTrigger|TestKubernetesRecoverableFailure|Test.*TransientRetry' -count=1 -timeout=180s`
passed. Cleanup ownership and synchronization helpers also received explanatory
comments in response to the standalone documentation-coverage suggestion.
