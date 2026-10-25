---
id: "01-backend-foundation"
title: "Kubernetes backend foundation"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 01: Kubernetes backend foundation

- **Acceptance:** `k8s` is a registered remote/containerized runtime that never
  falls back to standalone; typed executor/profile config supports kubeconfig
  and in-cluster auth plus all three storage modes.
- **Acceptance:** strict PodTemplate merge preserves operator-owned fields and
  rejects every Kandev-owned collision with stable field paths.
- **Acceptance:** Kubernetes clients, PVC synthesis, and exec/port-forward seams
  are deterministic, redacted, and unit-testable without a real cluster.
- **Verification:** `cd apps/backend && go test ./internal/agentruntime ./internal/task/models ./internal/agent/kubernetes ./internal/backendapp`
- **Files likely touched:** `apps/backend/go.mod`, `apps/backend/go.sum`,
  `internal/agentruntime/runtime.go`, `internal/agent/executor/executor.go`,
  `internal/task/models/models.go`, `internal/agent/kubernetes/*.go`,
  `internal/backendapp/agents.go`, and adjacent tests.
- **Dependencies:** None.
- **Parallelism:** sequential foundation; it owns the shared Go module files
  and public runtime/config types.
- **Inputs:** spec What/Data model/Failure modes; ADR ownership split; Docker
  and SSH registration/config patterns.

## Results

- Added the `k8s` remote/containerized runtime taxonomy, fail-closed lifecycle
  registration seams, typed cluster/profile configuration, strict PodTemplate
  validation and merge logic, managed-PVC synthesis, and injectable Kubernetes
  client/exec/port-forward boundaries.
- TDD covered runtime classification, both authentication modes, all workspace
  modes, strict YAML and reserved-field rejection, merge preservation, warning
  generation, PVC validation, client loading/redaction, loopback-only
  forwarding, and fail-closed registration.
- Integration review tightened that boundary further: mounts at or below
  `/opt/kandev`, `/run/kandev`, and `/workspace` are reserved; managed-PVC
  storage-class names receive Kubernetes DNS validation; and explicitly
  enabled service-account token automount produces a stable warning. Composed
  Pods now own both `spec.os.name: linux` and the Linux node selector, while
  `HOME` plus all explicit `AGENTCTL_*` / `KANDEV_*` template env keys are
  reserved for the managed bootstrap.
- `cd apps/backend && go test ./internal/agentruntime ./internal/task/models ./internal/agent/kubernetes ./internal/backendapp` passed.
- `cd apps/backend && go test ./internal/agent/executor` passed.
- `cd apps/backend && golangci-lint run ./internal/agentruntime ./internal/agent/executor ./internal/task/models ./internal/agent/kubernetes ./internal/backendapp --timeout=5m` passed with zero issues.
- `gofmt -l` over the changed Go files produced no output, and
  `git diff --check` passed.
- Kubernetes modules are aligned at `v0.35.5`; only their required existing
  dependency upgrades remain after `go mod tidy`.
