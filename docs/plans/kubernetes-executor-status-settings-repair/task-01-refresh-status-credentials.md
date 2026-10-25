---
id: "01-refresh-status-credentials"
title: "Refresh Kubernetes status credentials"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 01: Refresh Kubernetes status credentials

## Acceptance

- Active Kubernetes status/reconnect inspection uses a freshly constructed
  client from recorded connection metadata and publishes it only after exact
  Pod/PVC identity validation succeeds.
- Credential rotation clears a stale Unauthorized result without recreating the
  Pod, rotating runtime secrets, or replacing a healthy forward/agentctl client.
- A still-unauthorized client or foreign exact-name resource remains a sanitized
  fail-closed error.

## Verification

```bash
cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestKubernetesRefreshRemoteInstance.*(Credential|Runtime)|TestKubernetesRefreshRemoteInstance.*Foreign' -count=1 && go test -race ./internal/agent/runtime/lifecycle/... && git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_refresh.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_status.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes_restart_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_remote_status_test.go` (only if the cache boundary needs a direct regression)

## Dependencies

None.

## Parallelism

`sequential`. Task 02 consumes the corrected status contract.

## Inputs

- Spec: status consistency and rotated-credential scenarios.
- Plan: Backend sections.
- Existing patterns: `kubernetesExecutorConfigFromMetadata`,
  `kubernetesCleanupInventory`, `verifyRecordedPod`,
  `verifyKubernetesRecordedPVC`, and staged refresh commit/abort finalizers.

## Output contract

Report the RED failure, exact GREEN commands, runtime publication behavior,
files changed, blockers/risks, and synchronize this task plus `plan.md`.

## Results

RED reproduced both credential directions: the launch-time runtime returned
`expired test credential` even though the factory supplied the rotated client,
while a still-unauthorized or foreign fresh client was ignored. GREEN now
constructs a client from current recorded connection metadata, validates the
exact Pod/PVC, and publishes that runtime only after validation. A healthy
agentctl client and its retained forwards remain unchanged.

Verification:

- `go test -race ./internal/agent/runtime/lifecycle -run 'TestKubernetesRefreshRemoteInstance.*(Credential|Runtime)|TestKubernetesRefreshRemoteInstance.*Foreign' -count=1` passed.
- `go test -race ./internal/agent/runtime/lifecycle/...` passed (`lifecycle`
  43.788s; `lifecycle/skill` 1.057s).
- `git diff --check` passed.
