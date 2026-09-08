---
spec: ../../specs/kubernetes-executor/spec.md
created: 2026-08-24
status: completed
---

# Implementation Plan: Kubernetes Executor

## Overview

Add Kubernetes as a remote containerized executor through the existing
`ExecutorBackend` boundary. Establish the typed cluster/template contract and
Kubernetes dependency graph first, then implement lifecycle, authorization/API,
and backend parity in disjoint work packets. Build the responsive settings UI
after the API shape is stable, then add real Kind lifecycle coverage, public
documentation, and final integration checks.

The durable ownership and cleanup rules are recorded in
[ADR-2026-08-24-kubernetes-executor-resource-ownership](../../decisions/2026-08-24-kubernetes-executor-resource-ownership.md).

## Backend

### Runtime taxonomy and Kubernetes core

- Add aligned Kubernetes modules to `apps/backend/go.mod` / `go.sum`.
- Extend `apps/backend/internal/agentruntime/runtime.go`,
  `internal/agent/executor/executor.go`, and `internal/task/models/models.go`
  with the `k8s` remote/containerized runtime.
- Create `apps/backend/internal/agent/kubernetes/` for typed executor/profile
  config, strict `PodTemplate` parsing, reserved-field composition, PVC
  synthesis, client loading, and narrow exec/port-forward seams.
- Register a lazy backend/preparer in `internal/backendapp/agents.go`; invalid
  Kubernetes config fails closed rather than falling back to standalone.

### Lifecycle and recovery

- Implement `KubernetesExecutor` under
  `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes*.go`.
- Create/verify storage, create and watch a Pod, inject agentctl/config/auth
  through `pods/exec`, signal the managed bootstrap, bind loopback-only
  port-forward, and reuse existing agentctl handshake/client behavior.
- Persist namespace, names, UIDs, storage ownership, platform, port, hashes,
  and the validated workload launch snapshot (Pod template, platform, main
  container, and storage config; never Kandev-resolved credentials/profile
  env, injected files, or scripts) in `executors_running` metadata.
  Reconnect validates UID plus full ownership labels and creates a new local
  forward; missing-Pod replacement uses the snapshot rather than a later
  profile edit.
- Ordinary stop preserves resources; terminal/force cleanup deletes only the
  exact Pod and Kandev-created PVC. Existing claims are retained.
- Map Pod/container conditions into `RemoteStatusProvider` without probing a
  local PID.

### Service authorization and settings API

- Make executor/profile validation type-aware in
  `apps/backend/internal/task/service/service_resources.go`.
- Require an administrator for Kubernetes executor/profile create, update,
  delete, and diagnostic tests at the service boundary so HTTP and WebSocket
  mutations agree. Keep read/use available to members.
- Add `apps/backend/internal/kubernetes/handlers.go` for connection/RBAC/dry-run
  testing and exact active-session status, registered from
  `internal/backendapp/helpers.go`.

### Remote/container parity

- Audit exhaustive executor switches in orchestrator credentials/execution,
  workspace-source materialization, scripts/placeholders, skill delivery,
  Office CLI injection, embedded editor capability, liveness/status, and
  metrics.
- Keep `ContainerID` Docker-specific and carry Kubernetes resource identity in
  lifecycle metadata.

## Frontend

### Executor and profile settings

- Add the Kubernetes card and dedicated creation route under
  `apps/web/app/settings/executors/`.
- Add a cluster connection page for auth mode, kubeconfig path/context,
  namespace, connection/RBAC testing, and sanitized active sessions.
- Extend the profile page with focused Pod-template and workspace cards rather
  than expanding one monolithic form.
- Add pure config parse/serialize/baseline logic with Vitest coverage, API
  client types/calls, icons/descriptions, breadcrumbs, and all executor
  capability/filter switches.

### Mobile contract

Desktop keeps the existing settings-card composition. Phone uses direct
settings navigation with vertically stacked cards; the raw YAML surface owns
its internal horizontal scrolling while the document has one vertical scroll
owner. The nearest shipped exemplar is the responsive settings/profile flow
combined with `mobile-menu-sheet.tsx` geometry for any contextual actions.
Primary Save/Test actions stay visible with 44 px touch targets, profile
actions are not hover-only, and member mode renders a readable administrator
boundary. Desktop/mobile share config state, validation, API calls, and save
handlers.

### Localization

Add all new copy to the six `executors.json` catalogs. Persisted enum values,
namespace names, paths, and raw YAML remain untranslated.

## Tests

- **Runtime classification and config:** table-driven Go tests in
  `internal/agentruntime`, `internal/task/models`, and
  `internal/agent/kubernetes` cover enums, auth modes, strict YAML, reserved
  collisions, merge preservation, storage modes, and redaction.
- **Lifecycle:** controllable API/exec/forward fakes in
  `executor_kubernetes*_test.go` cover launch, partial failure, reconnect,
  container restart, UID/label mismatch, ordinary stop, exact cleanup, and
  existing-claim retention. Run concurrency-sensitive forward-state tests with
  `-race`.
- **Service/API:** service and handler tests cover admin/member/synthetic
  identity, type transitions, field-path errors, RBAC result shaping, dry-run,
  and exact session inventory.
- **Parity:** focused table cases cover every explicit executor switch.
- **Frontend:** Vitest covers parse/serialize/default/dirty-state logic and API
  response normalization.
- **Integration:** Kind tests cover kubeconfig and in-cluster authentication,
  exec/port-forward launch, restart/reconnect, storage retention/deletion,
  denied permissions, scheduling/image/PVC failures, and foreign-resource
  protection.

## E2E Tests

- `apps/web/e2e/tests/settings/mobile-kubernetes-executor.spec.ts`: an
  administrator configures auth/template/storage on Pixel 5, completes Test
  and Save, reloads to prove persistence, and has no page horizontal overflow.
  The same file verifies member read-only state and touch-visible actions.
- `apps/web/e2e/tests/settings/kubernetes-executor.spec.ts`: desktop creation,
  validation feedback, and profile navigation.
- `apps/web/e2e/tests/kubernetes/*.spec.ts`: real Kind launch, reconnect,
  stop/resume, cleanup, existing-claim retention, RBAC denial, and failure
  status through the user-facing UI.

## Verification Results

Completed on 2026-08-25. Each task records its focused commands and outcomes in
its `## Results`; [Task 08](task-08-integration.md) records the reconciled broad
backend, web, documentation, hygiene, and live-cluster gates.

- The constrained full backend suite, build, lint, race-sensitive Kubernetes
  packages, module tidy/verification, and changed-file formatting all passed.
- Web lint, typecheck, complete i18n checks, the new-code ratchet, and the full
  13,096-case Vitest catalog passed without unhandled rejections (13,092
  passed; 4 skipped).
- A fresh pinned Kind/Kubernetes 1.36.1 build passed all 12 lifecycle cases
  without retries. API/agentctl compatibility smoke passed on both 1.34.8 and
  1.36.1, and the targeted mobile active-session projection passed.
- All live-test clusters, images, containers, ownership markers, forwards, and
  workspace-owned test processes were absent after teardown.
- Public-doc tests passed 61/61, all 41 published pages validated, the opt-in
  RBAC manifest passed client dry-run, and `git diff --check` was clean.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-backend-foundation](task-01-backend-foundation.md)

Wave 2 (parallel-safe after Task 01):

- [x] [task-02-lifecycle](task-02-lifecycle.md)
- [x] [task-03-service-api](task-03-service-api.md)
- [x] [task-04-backend-parity](task-04-backend-parity.md)

Wave 3:

- [x] [task-05-frontend-settings](task-05-frontend-settings.md)
- [x] [task-06-kind-e2e-ci](task-06-kind-e2e-ci.md)
- [x] [task-07-docs-rbac](task-07-docs-rbac.md)

Wave 4:

- [x] [task-08-integration](task-08-integration.md)

Tasks marked parallel-safe own disjoint files. The primary session alone
updates this plan and integrates cross-task conflicts.
