---
id: "02-lifecycle"
title: "Kubernetes lifecycle and recovery"
status: completed
wave: 2
depends_on: ["01-backend-foundation"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 02: Kubernetes lifecycle and recovery

- **Acceptance:** CreateInstance provisions/verifies storage and Pod, injects
  agentctl/config/auth through exec, binds a loopback forward, and completes the
  existing health/nonce handshake.
- **Acceptance:** restart/reconnect verifies exact Pod/PVC UIDs and ownership
  labels, rotates local forwarding, and surfaces deterministic remote status.
  Exact-Pod reconnect is independent of the current profile row; replacement
  of a missing retained Pod uses the recorded non-secret workload snapshot,
  even after that profile is edited or deleted.
- **Acceptance:** ordinary stop preserves resources; terminal/forced cleanup is
  idempotent, fails closed, and never deletes an existing claim.
- **Verification:** `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle/...`
- **Files likely touched:** `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes*.go`,
  `executor_backend.go`, `manager_launch.go`, liveness/status files, and adjacent tests.
- **Dependencies:** Task 01.
- **Parallelism:** parallel-safe with Tasks 03 and 04 after Task 01; owns
  lifecycle files and metadata allowlists.
- **Inputs:** spec State machine/Persistence/Failure modes; ADR exact identity
  rules; Docker and SSH reconnect/cleanup patterns.

## Results

Implemented the real Kubernetes lifecycle behind the existing
`ExecutorBackend` boundary. Fresh launch now composes and admission-validates
the exact Pod/PVC, persists provisional and admitted identities before later
fallible phases, materializes agentctl/auth/config/skills through exec, starts
a loopback-only WebSocket-primary/SPDY-fallback forward, and completes the
managed health/nonce handshake. The streaming transport honors Kubernetes REST
auth, proxy, TLS, and custom dial hooks; cancellation closes a blocked
handshake without coupling a successful upgraded stream to its deadline.

The live Kind launch also established the secret-store boundary used by that
bootstrap: lifecycle internals receive the raw store so private
`kandev-runtime:*` nonce/auth records can be materialized, while SSH, Sprites,
credential providers, and other user-visible consumers receive the filtered
`UserVisibleStore`. This keeps runtime-only records inaccessible through public
secret surfaces without blocking managed Kubernetes startup.

Reconnect, restart, active refresh, model switch, and sibling-session reuse use
the recorded six-field resource identity and immutable workload snapshot.
Exact UID/resource-version and complete ownership-label checks guard deletion.
Ordinary stop retains resources, terminal cleanup preserves durable inventory
when teardown fails, existing claims are never deleted, and restart recovery
can reconstruct cleanup without an in-memory execution.

Verification completed on 2026-08-24:

- `cd apps/backend && golangci-lint run ./...` passed with `0 issues` after
  behavior-preserving complexity, duplication, nesting, constant, and test-file
  refactors.
- `cd apps/backend && go test -race ./internal/agent/runtime/lifecycle/...`
  passed (`lifecycle` 43.627 seconds; `lifecycle/skill` 1.052 seconds).
- `cd apps/backend && go test -race ./internal/agent/kubernetes -count=1`
  passed, including blocked WebSocket/SPDY cancellation, post-upgrade survival,
  REST custom-dial preservation, and cancellation/detach race coverage.
- `cd apps/backend && go test ./internal/orchestrator/...` and
  `go test ./internal/backendapp -run 'Test.*Kubernetes' -count=1` passed.
- `cd apps/backend && go test ./internal/backendapp -run
  'TestLifecycleSecretStoresSeparateRuntimeAndUserCredentialSecrets|Test.*Kubernetes'
  -count=1` passed after the live Kind secret-store correction.
- `gofmt`, `go mod tidy -diff`, `go mod verify`, and `git diff --check` passed.
