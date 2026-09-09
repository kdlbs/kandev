---
id: "01-launch-durations"
title: "Record Kubernetes launch durations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-TIMING-001
  - REQ-EXECUTORS-K8S-TIMING-002
acceptance_criteria:
  - AC-EXECUTORS-K8S-TIMING-001.1
  - AC-EXECUTORS-K8S-TIMING-001.2
  - AC-EXECUTORS-K8S-TIMING-001.3
  - AC-EXECUTORS-K8S-TIMING-001.4
  - AC-EXECUTORS-K8S-TIMING-002.1
  - AC-EXECUTORS-K8S-TIMING-002.2
  - AC-EXECUTORS-K8S-TIMING-002.3
system_design:
  - ../../specs/executors/system-design/kubernetes-startup-timing.md
---

# Task 01: Record Kubernetes Launch Durations

## Summary

Add passive stage and total duration records around fresh Kubernetes environment creation.
Provide enough evidence to diagnose a slow or failed attempt without exposing its inputs.

## In scope

- Implement the design's local recorder, monotonic clock, correlation, fixed outcomes, and stage boundaries.
- Cover success, all stage failures, finalization, rollback, concurrent calls, and secret exclusion through TDD.
- Add one real Kind log assertion to the existing successful launch case.
- Add a diagnostic reference section to `docs/public/k8s.md` with log fields and measurement exclusions.

## Out of scope

- Generic lifecycle tracing, metrics endpoints, persistent timing rows, UI progress, or retries.
- Changes to `KubernetesPreparer` or the meaning of existing launch events.

## Acceptance

- Every returned fresh attempt emits one total and at most four stage records with truthful outcome and duration.
- Observer and action-count tests prove privacy, unchanged return errors, and no extra cluster operations.
- The real launch scenario produces correlated records while reconnect paths produce no fresh-start records.

## Verification

From `apps/backend`:

```bash
rtk go test ./internal/agent/runtime/lifecycle -run 'TestKubernetesLaunchTiming' -count=1
rtk go test ./internal/agent/runtime/lifecycle -run 'TestKubernetes' -count=1
```

From `apps`, once for a fresh worktree:

```bash
rtk pnpm install --frozen-lockfile
```

From `apps/web`:

```bash
rtk env KANDEV_LOG_LEVEL=info KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/kubernetes/kubernetes-executor.spec.ts -- --grep 'launches through kubeconfig'
```

From the repository root:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk git diff --check
```

The timing tests are new outputs. Use a Zap observer and deterministic time,
not sleeps or timing thresholds. Read the isolated backend's retained file log
rather than assuming Info records appear on stdout.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_timing.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_timing_test.go` (new)
- `apps/web/e2e/tests/kubernetes/kubernetes-executor.spec.ts`
- `apps/web/e2e/helpers/kubernetes.ts` only for an isolated log reader if needed
- `docs/public/k8s.md`

## Dependencies

None. The Kind verification needs host Docker and the existing pinned tools.
Coordinate sequential edits with the other Kubernetes plans because they share tests and documentation.

## Risks

- Record finalization failure separately from an entered stage failure.
- Register the summary defer before rollback so it sees the final result.
- Do not log request metadata, raw errors, tokens, nonces, or scripts.

## Parallelism

`sequential`

## Inputs

- [Timing requirements](../../specs/executors/requirements/kubernetes-startup-timing.md)
- [Timing design](../../specs/executors/system-design/kubernetes-startup-timing.md)
- `executor_kubernetes_fakes_test.go`, `executor_kubernetes_checkpoint_test.go`, and `executor_kubernetes_restart_test.go` in the lifecycle package
- `manager_stream_disconnect_log_test.go` for the existing Zap observer pattern

## Results

Passed:

- `rtk go test ./internal/agent/runtime/lifecycle -run 'TestKubernetesLaunchTiming' -count=1`
  (8 tests)
- `rtk go test ./internal/agent/runtime/lifecycle -run 'TestKubernetes' -count=1`
  (115 tests)
- `rtk env KANDEV_LOG_LEVEL=info KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --project containers tests/kubernetes/kubernetes-executor.spec.ts -- --grep 'launches through kubeconfig'`
  (1 test passed in 50.9 seconds; default text log format)
- `rtk node --test scripts/validate-public-docs.test.mjs`
- `rtk node scripts/validate-public-docs.mjs`
- `rtk git diff --check`
