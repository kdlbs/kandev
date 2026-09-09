---
id: "01-retention-projection"
title: "Project retained Kubernetes resources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-K8S-RETAINED-001
  - REQ-EXECUTORS-K8S-RETAINED-002
acceptance_criteria:
  - AC-EXECUTORS-K8S-RETAINED-001.1
  - AC-EXECUTORS-K8S-RETAINED-001.2
  - AC-EXECUTORS-K8S-RETAINED-001.3
  - AC-EXECUTORS-K8S-RETAINED-001.4
  - AC-EXECUTORS-K8S-RETAINED-002.5
system_design:
  - ../../specs/executors/system-design/kubernetes-retained-compute.md
---

# Task 01: Project Retained Kubernetes Resources

## Summary

Extend the existing sanitized session response with recorded session state,
verified retention classification, and main-container requests.
Use only the database and Pod observations the endpoint already performs.

## In scope

- Add `session_state`, `retention_state`, and optional `main_container_requests` to `SessionRow`.
- Implement the design's precedence table at the existing authorization/identity boundary.
- Keep current HTTP and WS response fields compatible and add contract tests.
- Cover ordinary stopped inventory without filtering it through active-agent status.

## Out of scope

- UI, persistence changes, new cluster permissions, cleanup changes, or session-impact semantics.
- Pod-wide totals, metrics-server calls, or frontend classification logic.

## Acceptance

- Table tests cover every canonical session state, Pod phase, deletion timestamp, and failed observation.
- CPU/memory values come only from the verified main container, with explicit zero preserved and missing values omitted.
- Existing authorization and exact-inventory filters apply to every new field without extra cluster calls.

## Verification

From `apps/backend`:

```bash
rtk go test ./internal/kubernetes -run 'TestKubernetes(RetentionProjection|MainContainerRequests)' -count=1
rtk go test ./internal/kubernetes -count=1
```

From the repository root:

```bash
rtk git diff --check
```

The named projection tests are new outputs. TDD tests must inspect both HTTP
and WS serialization, access rejection, identity mismatch, and fake-client actions.
They must not mirror only a helper's expected mapping without exercising `sessionRow`.

## Files likely touched

- `apps/backend/internal/kubernetes/sessions.go`
- `apps/backend/internal/kubernetes/sessions_test.go`
- `apps/backend/internal/kubernetes/sessions_retention.go` (new if needed for file limits)
- `apps/backend/internal/kubernetes/sessions_retention_test.go` (new)

## Dependencies

None. Existing Kubernetes fake clients support these tests without a cluster.

## Risks

- `WAITING_FOR_INPUT` does not mean stopped. A retained Pod does not prove the absence of background work.
- Missing or foreign Pods cannot supply trustworthy resource values.
- Session-state additions must map to unknown until the classification contract explicitly includes them.

## Parallelism

`sequential`

## Inputs

- [Retained-compute requirements](../../specs/executors/requirements/kubernetes-retained-compute.md)
- [Retained-compute design](../../specs/executors/system-design/kubernetes-retained-compute.md)
- `apps/backend/internal/kubernetes/sessions_namespace_test.go`
- `apps/backend/internal/task/models/models.go` for canonical session states
- `docs/decisions/2026-08-24-kubernetes-executor-resource-ownership.md`

## Results

Passed:

- `rtk go test ./internal/kubernetes -run 'TestKubernetes(RetentionProjection|MainContainerRequests)' -count=1`
  (6 tests)
- `rtk go test ./internal/kubernetes -count=1`
  (131 tests)
- `rtk git diff --check`
