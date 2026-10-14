---
id: "01-admission"
title: "Admit inherited Kubernetes workspaces before session publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
  - REQ-EXECUTORS-KUBERNETES-TASK-POD-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.3
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4
  - AC-TASKS-MCP-WORKSPACE-MODE-004.2
  - AC-TASKS-MCP-WORKSPACE-MODE-004.3
  - AC-EXECUTORS-KUBERNETES-TASK-POD-001.6
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
  - ../../specs/executors/system-design/kubernetes-task-pod.md
---

# Task 01: Admit inherited Kubernetes workspaces before session publication

## Summary

Move unsupported cross-task Kubernetes admission before session creation and
publication. Return the documented typed conflict with the supported separate
workspace alternative, while retaining all lifecycle ownership checks.

## In scope

- Test-first service predicate and common prepare/ensure/start integration.
- Existing-session launch/resume defense; parent/group fallback coverage.
- API/WS and MCP error mapping, Kind API regression, public how-to guidance.
- Preserve a pending browser ensure request through same-task hydration so its
  rejection reaches the existing recovery banner. Guard task changes, retries,
  and unmount without changing layout or recovery controls.

## Out of scope

Creation-default changes, cross-task compute sharing, frontend controls, archive
race repair, persistent follow-up tasks, and ownership or credential migrations.

## Acceptance

1. `TestKubernetesInheritedWorkspaceAdmission` fails before the correction because
   the old path persists a session and reaches late lifecycle ownership failure.
   After correction prepare/ensure/start reject without publishing a session or
   changing the owner. Existing legacy sessions still fail safely before allocation.
2. Typed conflict mapping names unsupported reuse and directs the caller to a
   task with `workspace_mode="new_workspace"`. Deferred creation continues to
   succeed; no automatic executor or workspace-mode fallback occurs.
3. Same-task Kubernetes sessions and supported inheritance remain usable. Kind
   verifies the independent-workspace control and preservation of the parent pod,
   dirty-file sentinel, and environment inventory; cleanup runs after readiness.
4. Same-task hydration cannot leave ensure stuck in `preparing` or discard its
   typed rejection. Deferred hook tests cover success, failure, navigation away
   and back, unmount, and Strict Mode effect replay. Existing banner and mobile
   feedback tests cover the unchanged presentation.

## Verification

Run from repository root after implementing the new tests. Install dependencies
once in a fresh worktree before the E2E command. Kind requires host Docker.

```bash
(cd apps/backend && GOMAXPROCS=4 GOGC=20 go test -trimpath -p 1 -tags fts5 -race ./internal/orchestrator ./internal/orchestrator/handlers ./internal/orchestrator/executor ./internal/task/handlers ./internal/mcp/handlers ./internal/mcp/server ./internal/agent/runtime/lifecycle -run 'KubernetesInheritedWorkspace|Inherit|WorkspaceReuse|SpawnSession|KubernetesTaskPod' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && VITEST_MAX_WORKERS=1 pnpm exec vitest run hooks/domains/session components/task/ensure-session-error.test.tsx components/task/task-page-mobile-feedback.test.ts)
GOMAXPROCS=2 GOGC=20 GOFLAGS='-p=1' MAKEFLAGS='VERSION=0.0.0' KANDEV_E2E_CONTAINERS=1 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project containers tests/kubernetes/kubernetes-inherited-workspace.spec.ts --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/handoff_inheritance.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/session_ensure.go`
- New `apps/backend/internal/orchestrator/kubernetes_inherited_workspace_test.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go` and `executor_resume.go`
- New `apps/backend/internal/orchestrator/executor/kubernetes_inherited_workspace_test.go`
- `apps/backend/internal/task/models/workspace_binding.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go` and ensure handler
- New handler regression tests alongside those handlers
- `apps/backend/internal/mcp/handlers/spawn_session.go` and `spawn_session_test.go`
- Existing MCP creation/default tests, if needed for paired deferred creation
- `apps/backend/internal/orchestrator/handoff_inheritance_test.go` (seed the
  selected executor in the supported-inheritance fixture)
- `apps/web/hooks/domains/session/use-ensure-task-session.ts`
- `apps/web/hooks/domains/session/use-ensure-task-session-pending.test.ts`
- New `apps/web/e2e/tests/kubernetes/kubernetes-inherited-workspace.spec.ts`
- `docs/public/k8s.md` and `docs/public/automation-and-mcp.md`

## Dependencies

None. Reuse accepted task-pod ownership and shared-task credential trust ADRs.

## Risks

Resolve the effective executor/profile rather than trusting an omitted argument.
Do not treat a missing owner as supported reuse. Binding can change after
preflight, so retain the final canonical owner/generation check. Do not advise
restoring a healthy parent workspace as a fix for unsupported cross-task reuse.

## Parallelism

`sequential`

## Inputs

- [Plan and confirmed root cause](plan.md)
- [Live reproduction evidence](evidence.md)
- Referenced requirements and system designs, especially unsupported reuse and
  the Kubernetes cross-task-sharing exclusion.
- `handoff_inheritance_test.go`, `session_ensure_test.go`,
  `executor_kubernetes_task_pod_test.go`, and Kind task-pod fixture patterns.

## Results

Implementation resumed after pod replacement on 2026-10-13. The final Kind run
confirmed the typed conflict, then exposed a same-task hydration race that
discarded the response before the existing banner rendered. Deferred hook tests
reproduced stuck `preparing` states for both failure and success. Request
ownership now survives same-task hydration while excluding stale tasks and
unmounted consumers. This is state handling within the existing surface; mobile
layout, touch, navigation, and viewport behavior are unchanged.

Final verification on 2026-10-13:

- The exact targeted seven-package Go command above passed with `-race` after
  completing the selected-executor fixture. Log:
  `/tmp/k8s-admission-final-backend.log`.
- The session-hook directory and existing banner/mobile-feedback tests passed:
  93 files, 915 tests. Log: `/tmp/k8s-admission-web-tests.log`.
- The rebuilt Kind command above passed: one test, no retries, including the
  rendered conflict details, independent-workspace control, parent pod UID,
  environment record, and dirty-file preservation. Log:
  `/tmp/kandev-run.e2e.pHQcMIR4.log`. Disposable resources were removed.
- Focused ESLint, i18n ratchet, E2E sleep ratchet, specification catalog,
  specification lint, public-doc validation (47 pages), public-doc validator
  tests (63), local PR documentation coverage, and diff checks passed.

The first resumed concurrent compilation exceeded the pod's 8 GiB limit and
lost a Go compiler to OOM. Final validation serialized heavy builds, used one
Go package and one Vitest worker, and reduced Go's GC target. The fixture uses
`VERSION=0.0.0` because its compact helper package requires a SemVer build.
No live parent or quick-terminal task was changed.

Hook lint also required reusing the existing
`models.WorkspaceModeInheritParent` constant. The backend race checks and rebuilt
Kind regression were rerun successfully after that final source edit.

Public docs updated: Kubernetes how-to recovery guidance in `docs/public/k8s.md`
and workspace-mode reference guidance in `docs/public/automation-and-mcp.md`.
Authentication was restored and ready PR #4485 was published on 2026-10-14.

PR remediation preserves the new workspace-preparation error from `main` and
adds transactional shared-group Kubernetes admission. Its regression covers a
materializer becoming ready after preflight, supported non-Kubernetes sharing,
same-task sessions, and preservation of the canonical environment.

Post-merge verification on 2026-10-14:

- The eight-package targeted Go race check (adding task models and the
  `WorkspacePreparing` selector) passed.
- `PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 GOGC=20 go test -trimpath -p 1
  -tags fts5 -race ./internal/task/repository/sqlite
  -run 'TestKubernetesSharedGroupBindingAdmission|TestCreateTaskSessionWithSharedGroupWorkspaceBindingElectsOneMaterializer|TestWorkspaceBindingRetryOwnership'
  -count=1` passed from `apps/backend`. The Kubernetes ordering case first
  failed because it persisted the unsupported session; PostgreSQL cases are
  environment-gated and were skipped locally without a DSN.
- The session-hook and recovery/mobile-feedback tests passed: 101 files, 967
  tests. Focused web/E2E ESLint, harness validation and its 19 tests,
  specification lint and its 36 tests, and the documentation catalog passed.
- `NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck` passed from
  `apps/web`, run separately from compilation. Smaller local heap limits
  exhausted memory; CI configures an 8 GiB heap.
- The same managed rebuilt Kind command above passed with no retries: one
  test in 2.2 minutes. Log: `/tmp/kandev-run.e2e.nn7B8xCa.log`.
