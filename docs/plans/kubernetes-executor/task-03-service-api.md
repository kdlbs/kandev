---
id: "03-service-api"
title: "Kubernetes service and API"
status: completed
wave: 2
depends_on: ["01-backend-foundation"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 03: Kubernetes service and API

- **Acceptance:** Kubernetes executor/profile CRUD validates typed config and
  requires admin identity for mutation across HTTP and WebSocket service paths,
  while members retain read/use access.
- **Acceptance:** connection testing reports discovery, namespace, mandatory
  RBAC, streaming, and optional dry-run admission with sanitized step results;
  admitted Pod/PVC responses are checked against the same owned-invariant
  contract as real lifecycle creation.
- **Acceptance:** active-session status starts from `executors_running` and
  fetches exact Pods without broad namespace inventory.
- **Verification:** `cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/kubernetes ./internal/backendapp`
- **Files likely touched:** `apps/backend/internal/task/service/service_resources.go`,
  focused service tests, `internal/kubernetes/handlers*.go`,
  `internal/backendapp/helpers.go`, route-registration tests.
- **Dependencies:** Task 01.
- **Parallelism:** parallel-safe with Tasks 02 and 04; owns service mutation
  authorization, Kubernetes settings handlers, and handler wiring only.
- **Inputs:** spec API surface/Permissions/Failure modes; SSH handler and
  authn service-boundary patterns.

## Results

Implemented typed Kubernetes executor/profile validation, administrator-only
mutation and connection testing across HTTP and WebSocket paths, optional
profile admission checks, exact existing-claim and server-UID validation,
disposable exec/port-forward probing, and sanitized active-session projection
from `executors_running` with exact Pod identity labels.

Verification completed on 2026-08-24:

- `cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/kubernetes ./internal/backendapp`
  passed.
- `golangci-lint run ./internal/task/handlers/... ./internal/kubernetes/...`
  passed with zero issues.
- Focused `internal/agent/kubernetes`, handler authorization/error-mapping, and
  complete handler suites passed.
- `git diff --check` passed.
