---
id: "01-observations"
title: "Preserve and classify executor observations"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-FAILURE-VISIBILITY-001
acceptance_criteria:
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.1
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.2
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.6
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.10
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.12
system_design:
  - ../../specs/executors/system-design/executor-failure-visibility.md
---

# Task 01: Preserve and classify executor observations

## Summary

Add typed safe observations to runtime and provider adapters, with Kubernetes
terminal preflight and separate Pod/container evidence. Write the supplied eviction
fixture first and demonstrate the current cause-loss and exec-before-preflight defects.

## In scope

Provider observations and compatibility fallback, reason selection, restart baselines,
expected-stop identity, sanitizer, reconnect/refresh preflight. Include local process,
Docker inspect and supported remote status adapters; do not expand plugin protocols.

Include Running Pod with unready required agent container, healthy Docker sidecar,
CrashLoopBackOff/last exit 2, absent volatile bootstrap state and retained disk marker.
Normalize optional correlated reboot evidence without extra cluster privileges;
SandboxChanged alone must not become a confirmed reboot diagnosis.

## Out of scope

No durable episode tables, UI, environment repair, worker upgrades or storage settings.

## Acceptance

- The supplied eviction remains actionable alongside both exit137 records; unknown facts and explicit OOM evidence remain distinct.
- Known terminal Pods fail before control exec and preserve typed evidence; stale identity and transient transport never masquerade as terminal loss.
- All adapter fallbacks and sanitizer boundaries have focused regression coverage; no provider receives automatic retry authority.

## Verification

Commands run from repository root; new test files named here are implementation
outputs, not existing checks. Demonstrate each regression failing before its fix.
Run the affected baseline tests as well if implementation touches additional suites.

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'Test(ExecutorFailure|Kubernetes|PollOneRemoteStatus|PollRemoteStatuses)' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/lifecycle -race -run 'Test(ExecutorFailure|PollOneRemoteStatus)' -count=1)
(cd apps/backend && go test ./internal/agent/runtime/routingerr ./internal/agent/kubernetes -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/runtime.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_status.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_reconnect.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_refresh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_shared_control.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_{docker,remote_docker,ssh,sprites}.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_failure_observation_test.go (new)`
- `apps/backend/internal/agent/runtime/routingerr/ (sanitizer and tests)`

## Dependencies

None. Refresh open admission/compatibility changes before coding.

## Risks

Identity and prompt-generation races must fail closed. Inspect/recheck cannot mutate
resources or cause prompt replay. Tests must use owned disposable data and cluster;
if container prerequisites are absent, record the exact blocker and leave completion
pending rather than substituting production. Reuse existing database test harnesses
for supported dialects and ensure a regex command actually discovers the new tests.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirements](../../specs/executors/requirements/executor-failure-visibility.md)
- [System design](../../specs/executors/system-design/executor-failure-visibility.md)
- [Plan, fixture matrix, source baseline and coordination](plan.md)
- Scoped AGENTS.md, /tdd, and /mobile-parity plus /e2e for rendered changes.

## Results

Implemented read-only Kubernetes/Docker/local observations, exact source identity,
Kubernetes terminal/container precedence and restart baselines, disconnect grace,
confirmed-loss retirement, and primary/secondary cleanup cause propagation.

Validation passed after the final source change:

- Lifecycle observation/polling regressions and race tests.
- Full lifecycle suite with `KANDEV_BUNDLE_DIR=''` (120.872s). The inherited
  variable pointed to another checkout and invalidated a missing-binary test;
  clearing it changed only the verification process, not runtime behavior.
- Full `internal/agent/runtime/routingerr` and `internal/agent/kubernetes` suites
  in the broader affected-package run.
- Go lint against HEAD: zero issues.

RED/GREEN evidence additionally covers source-row replacement with the same
execution ID, legacy source revision capture, transient/unknown disconnects,
provider snapshot ownership, bounded secret sanitization, and cleanup timeout
remaining secondary while preserving `errors.Is`. Kubernetes real-resource
acceptance belongs to task 03 and remains blocked at fixture setup.

### Worker connectivity regression results

`executor_kubernetes_unreachable_test.go` covers responsive API with stale container
readiness, explicit unavailable-worker conditions, deletion/disruption evidence,
generic Pod-not-ready uncertainty and preserved crash-loop termination. The owned
Pod/Bound-volume fixture rejects control operations without issuing exec requests.
Disconnect coverage verifies immediate cached unknown status, durable admission
after the grace window, retained execution and turn ownership, and typed launch
errors. Cleanup coverage retains the primary uncertainty and secondary blocker.
Affected lifecycle/orchestrator tests and focused worker race tests pass. Go static
analysis reports zero issues. No node-level permission or production test is used.
