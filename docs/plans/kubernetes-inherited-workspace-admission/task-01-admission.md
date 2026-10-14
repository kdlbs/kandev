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

Additional PR review remediation:

- Shared retry guidance is one model constant used by HTTP, WebSocket and MCP.
  Their tests assert the exact external guidance before and after extraction.
  The Kubernetes predicate documents its intentional support for legacy task
  rows without workspace-policy metadata.
- The shared-group race matrix now covers both executor mismatch directions.
  The local follower initially persisted a Kubernetes binding; the regression
  failed and now verifies unsafe-reuse rejection before persistence.
- A parent becoming available after preflight initially left a foreign
  Kubernetes session bound. Propagation now checks the prepared session's
  executor before updating the binding and compensates rejected preparation.
  The regression also preserves supported Docker inheritance.
- The browser initially reloaded sessions after disable/archive/removal and
  could revive an obsolete request after re-enablement. Deferred tests failed
  before correction. Admission loss now clears response ownership and permits
  a fresh eligible attempt; same-task hydration and Strict Mode remain covered.
- This is shared state handling in the existing desktop and phone surface.
  Layout, copy, touch, scrolling and navigation are unchanged. Mobile parity
  uses the state-only exception: deferred hook tests plus the existing
  `ensure-session-error.test.tsx` and `task-page-mobile-feedback.test.ts`
  consumers. No additional mobile composition test is required.
- The session-hook directory and those consumers passed: 101 files, 972 tests.
  Focused hook ESLint passed. Logs: `/tmp/pr4485-coderabbit-web-tests.log` and
  `/tmp/pr4485-coderabbit-hook-lint2.log`.
- Backend review regressions passed from `apps/backend`:
  `PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 GOGC=20 go test -trimpath -p 1
  -tags fts5 -race ./internal/task/repository/sqlite ./internal/orchestrator
  ./internal/orchestrator/handlers ./internal/task/handlers
  ./internal/mcp/handlers ./internal/orchestrator/executor
  -run 'KubernetesInheritedWorkspace|KubernetesSharedGroupBinding|Inherit|WorkspaceBindingRetryOwnership|WorkspaceReuse|SpawnSession'
  -count=1`. All six packages passed; PostgreSQL cases remain DSN-gated.
  Log: `/tmp/pr4485-coderabbit-backend-green.log`.
- Final `NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck` passed from
  `apps/web`. Log: `/tmp/pr4485-final-typecheck.log`.
- The final managed rebuilt Kind regression passed with no retries: one test
  in 2.3 minutes. Log: `/tmp/kandev-run.e2e.fiBBeNKa.log`.

CI remediation:

- PostgreSQL 16 reproduced a fixture-only mismatch: `UpdateTaskEnvironment`
  leaves an in-memory nanosecond timestamp, while PostgreSQL stores
  microseconds. The preservation assertion now compares database readbacks
  before and after binding, retaining the full-row equality check.
- A disposable local PostgreSQL 16 instance ran
  `KANDEV_TEST_POSTGRES_DSN=<local test DSN> PATH=/usr/local/go/bin:$PATH
  GOMAXPROCS=2 GOGC=20 go test -trimpath -p 1 -tags fts5 -race
  ./internal/task/repository/sqlite
  -run '^TestKubernetesSharedGroupBindingAdmission$' -count=3` from
  `apps/backend`. All three runs passed across SQLite and PostgreSQL. The
  disposable container and volume were removed. Log: `/tmp/pr4485-pg-fixed.log`.
- The task-coordination layout fixture now supplies a valid profile and
  explicitly prepares each workspace without running an agent. It no longer
  mistakes a legitimate session-error banner for removed-control spacing.
- File drag-and-drop waits for the actual rename reply and the existing
  Changes activation, then returns to Files. Disk checks use the task's
  resolved workspace path.
- Both failed E2E titles passed three repetitions with no retries:
  `PATH=/usr/local/go/bin:$PATH GOMAXPROCS=2 GOGC=20 GOFLAGS='-p=1'
  MAKEFLAGS='VERSION=0.0.0' scripts/run-quiet e2e --summary --
  pnpm --dir apps/web e2e:run --host --no-build --shards 1
  --project chromium tests/task/task-coordination-ui-removal.spec.ts
  tests/task/file-tree-drag-drop.spec.ts
  --grep 'task details omit native coordination|drag a file into a folder'
  --repeat-each=3 --retries=0`. Six tests passed in 51.1 seconds using the
  freshly built Kind assets; only test fixtures changed afterward. Log:
  `/tmp/kandev-run.e2e.kell2jIM.log`. Focused E2E lint passed.

Latest-base integration and mobile CI follow-up:

- Integrated the upstream failed/abandoned-environment recovery change. The
  shared-group transaction keeps its materialization lock, claim takeover and
  failed-environment re-election; executor ownership and compatibility are
  checked before those mutations. MCP retains both unsupported-reuse guidance
  and the upstream reset-required response.
- The upstream retry fixture now uses the public task repository and real
  Office workspace groups. The admission regression uses that fixture in the
  external test package, retaining same-owner reuse, mismatch rejection and
  complete environment-preservation assertions.
- Focused race checks passed across task SQLite, orchestrator, executor and
  MCP handlers, including admission, retry ownership, failed-environment and
  materialization coverage. Log: `/tmp/pr4485-recovery-merge-tests2.log`.
- CI shard 9 found a cancelled Quick Chat turn with a disabled composer. The
  phone palette fixture admitted its slow turn without a confirmed opening
  baseline. It now waits for that baseline and the slow-turn
  acceptance marker before cancellation, then waits for cancellation settlement
  before its follow-up prompt. Existing touch-target and overflow checks remain.
  The merged baseline passed six runs without retries; this fixture correction
  is validated separately. Log: `/tmp/kandev-run.e2e.uym4ga2d.log`.
- The corrected mobile spec passed six runs without retries in 2.2 minutes.
  Focused ESLint passed. Log: `/tmp/kandev-run.e2e.eyks8v0B.log`.
- PostgreSQL 16 passed the admission and retry-ownership regressions twice
  with `-race` (89.8 seconds), then the upstream materialization serialization
  and shared-group failed-environment re-election cases (38.2 seconds).
  Logs: `/tmp/pr4485-merge-pg-tests.log` and `/tmp/pr4485-merge-pg-locks.log`.
  The disposable PostgreSQL container and volume were removed.
- The managed rebuilt Kind regression passed without retries against the
  integrated backend and fresh Linux mock-agent artifact. Log:
  `/tmp/kandev-run.e2e.QWuYvV3e.log`.
- Both previously failing Chromium fixture titles passed again on the merged
  assets without retries. Log: `/tmp/pr4485-integration-chromium.log`.
