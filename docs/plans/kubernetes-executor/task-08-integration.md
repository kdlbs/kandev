---
id: "08-integration"
title: "Kubernetes integration verification"
status: completed
wave: 4
depends_on: ["02-lifecycle", "03-service-api", "04-backend-parity", "05-frontend-settings", "06-kind-e2e-ci", "07-docs-rbac"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 08: Kubernetes integration verification

- **Acceptance:** all child diffs are reconciled, every task's actual files and
  targeted results are recorded, and the plan/spec statuses match reality.
- **Acceptance:** backend build/lint/tests, frontend lint/typecheck/i18n/tests,
  focused production E2E, docs validation, and diff hygiene pass after the last
  production edit, or each concrete external blocker is documented.
- **Acceptance:** manual Kind acceptance confirms no credentials in Pod spec,
  exact restart/resume/cleanup boundaries, and desktop/mobile admin/member flows.
- **Verification:** task-defined commands from Tasks 01-07, followed by `make -C apps/backend build && make -C apps/backend lint && git diff --check`.
- **Files likely touched:** integration fixes across previously owned files plus
  this plan/task status package; no speculative refactor.
- **Dependencies:** Tasks 02-07.
- **Parallelism:** primary-session integration only.
- **Inputs:** all task results and complete feature diff.

## Results

Completed on 2026-08-25. The task-owned and cross-task diffs were reconciled
against the shipped spec, including fresh per-create ownership nonces,
administrator-only global session-impact counts, exact authorized session
inventory, full settings Reset behavior, and mobile task/session identity
projection. No commit or push was created.

### Backend

- The focused race command passed for `internal/agent/kubernetes`,
  `internal/agent/runtime/lifecycle`, `internal/kubernetes`,
  `internal/backendapp`, `internal/task/service`, `internal/task/handlers`, and
  `internal/orchestrator/executor`.
- From `apps/backend`, `GOFLAGS='-p=2' make test`, `make lint`, and `make build`
  all passed; lint reported zero issues. An earlier unconstrained all-package
  run exhausted the retry budget in one concurrency stress test under full
  machine load. That exact test passed 10/10 in isolation before the complete
  constrained suite passed.
- `go mod tidy -diff` produced no diff and `go mod verify` reported all modules
  verified.

### Web and responsive UI

- From `apps/web`, `pnpm run lint`, `pnpm run typecheck`,
  `pnpm run i18n:check`, and `pnpm run i18n:ratchet` all passed.
- `VITEST_MAX_WORKERS=2 pnpm test` passed 1,551 files: 13,092 tests passed and
  4 were skipped, with no unhandled errors. The integration audit added a
  Chrome user-agent contract to the shared Vitest environment so happy-dom no
  longer makes Monaco install its persistent Safari clipboard workaround and
  emit cancelled promise rejections after otherwise-green suites.
- Desktop and Pixel 5 settings flows cover administrator create/test/save,
  reload persistence, member read-only behavior, touch-visible actions, YAML
  containment, and no document horizontal overflow. A final targeted
  mobile-chrome active-session projection passed 1/1 without retries.

### Live Kubernetes

- With a fresh source-checked build, pinned Kind 0.32.0 and Kubernetes 1.36.1
  passed exactly 12/12 browser lifecycle cases on the first attempt. Coverage
  includes kubeconfig and in-cluster auth, credential nonleakage, exec and
  loopback-only port-forward, backend reconnect, main-container re-handshake,
  same-session managed-PVC stop/resume and terminal deletion, existing-claim
  retention, foreign-resource protection, and causal RBAC, scheduling,
  image-pull, and PVC failures.
- The separate API-only compatibility project passed exactly 1/1 on both
  Kubernetes 1.34.8 and 1.36.1 without retries or freshness bypasses.
- The frozen backend/web/workflow source digest was unchanged after every run.
  Final teardown found no owned Kind clusters, runtime images, containers,
  ownership markers, Playwright/backend/Vite processes, or port-forwards.
  [Task 06](task-06-kind-e2e-ci.md) records the exact pins, digests, commands,
  timings, artifacts, and teardown proof.

### Documentation and hygiene

- `node --test scripts/validate-public-docs.test.mjs` passed 61/61 and
  `node scripts/validate-public-docs.mjs` validated all 41 published pages.
- `kubectl apply --dry-run=client --validate=false -f k8s/executor-rbac.yaml`
  accepted the ServiceAccount, namespaced Role, and RoleBinding.
- Changed Go files were gofmt-clean and `git diff --check` passed after the
  final documentation/status edits.
