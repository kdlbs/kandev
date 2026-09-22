---
id: "02-shared-lifecycle"
title: "Shared pod lifecycle"
status: done
wave: 2
depends_on:
  - "01-durable-inventory"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
acceptance_criteria:
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.1
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.2
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.3
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.4
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.5
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.6
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.7
system_design:
  - ../../specs/executors/system-design/kubernetes-task-pod.md
---

# Task 02: Shared pod lifecycle

## Summary

Wire task-owned inventory into the complete Kubernetes lifecycle. Start additional
sessions as independent agentctl instances in the canonical pod and remove all
session-level authority to destroy shared compute.

## In scope

- Fresh create, attach-only admission, per-instance configuration, secret resolution, and pinned workload snapshot.
- Generation-guarded reconnect/replacement, checkpoints, partial-failure rollback, and restart refresh.
- Session-only termination, final task cleanup, zero-session retention, legacy compatibility, and authorized status projection.

## Out of scope

Cross-task pooling, automated legacy consolidation, and UI redesign.

## Acceptance

- The two-session regression fails before the change and passes with one Pod/PVC and distinct agentctl instances; concurrent attach never creates another pod.
- Stop/delete/force/failure of one session leaves a live sibling usable; task cleanup excludes new attachment and deletes only exact owned resources.
- Restart, legacy ambiguity, foreign identity, profile edits, independent runtime env, and status projection pass targeted regressions.

## Verification

Use TDD for changed logic: run the named regression before and after implementation.
Commands run from repository root. Read scoped guidance and applicable TDD/E2E
skills before implementation.

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/agent/kubernetes ./internal/agent/runtime/lifecycle ./internal/task/service ./internal/kubernetes ./internal/orchestrator/executor ./internal/backendapp -run 'Kubernetes|SharedTaskPod' -count=1)
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes.go`
- Kubernetes lifecycle bootstrap, identity, reconnect, cleanup, refresh, and checkpoint files
- New `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_task_pod_test.go`
- `apps/backend/internal/agent/kubernetes/compose.go` and ownership/admission tests
- `apps/backend/internal/task/service/service_resources.go` and Kubernetes resource cleanup tests
- `apps/backend/internal/kubernetes/sessions.go`, `sessions_retention.go`, and new `sessions_task_pod_test.go`
- `apps/backend/internal/orchestrator/executor/` and `apps/backend/internal/backendapp/` launch/cleanup adapters

## Dependencies

01-durable-inventory.

## Risks

Do not copy sibling execution metadata wholesale, rerun pod-wide bootstrap on attach, or treat force-stop as task deletion authority.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/kubernetes-task-pod.md).
- [System design](../../specs/executors/system-design/kubernetes-task-pod.md).
- [Plan](plan.md) regression matrix.
- Existing `executor_kubernetes_fakes_test.go`, checkpoint/restart tests, and
  task environment repository tests as patterns.

## Results

Targeted Kubernetes package tests passed with `-race -tags fts5`. Added regressions
cover independent agent environments, concurrent attachment, session-only stop,
exact task cleanup, backend/container/Pod recovery, stale-token protection,
secret/inventory/connection failure rollback, legacy conflicts and adopted-resource
protection, retained zero-session status, and executor deletion guards.

Startup fences interrupted claims under the existing exclusive runtime-state lock.
Incomplete bootstrap inventory remains available for exact cleanup and blocks
unsafe attachment. Desktop and mobile retained-row component tests passed.

Kind validation additionally exposed two integration gaps that unit fixtures did
not cover: creating private HOME before credential-less session preparation, and
preserving shared ownership/version markers through execution metadata filtering.
Both have failing-before/passing-after regressions. The final targeted lifecycle,
orchestrator executor, and status packages pass with race detection; scoped
lifecycle lint reports zero issues. The live test proves three sessions in one
Pod UID and a sibling continuing after force-stopping the creator; restart and
final cleanup subsequently passed in work order 03.

The passing Kind run exposed a transient cleanup retry after execution identity
rebinding. The final regression reproduces stopping the same remote session with
a new local execution ID and verifies immediate task cleanup. Exact task/session/
environment/remote-instance matching closes its original local connection while
leaving siblings intact. Final lifecycle race tests pass (8.512s); lint is clean.

## PR remediation

PR remediation adds typed not-found errors, encrypted credential recovery across backend restart, concurrent stop/attach coverage, and bounded cancellation-detached cleanup. Targeted race coverage passes; sibling credential trust/isolation remains a security-review decision.
