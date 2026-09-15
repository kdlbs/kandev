---
id: "03-remote-cwd"
title: "Support remote scratch attachment"
status: completed
wave: 3
depends_on: ['02-local-cwd']
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-006
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.3
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.5
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.7
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-006.8
system_design:
  - ../../specs/tasks/system-design/current-workspace-sources.md
---

# Task 03: Support remote scratch attachment

## Summary and scope

Apply the established-root contract to runnable remote/container scratch tasks through MaterializeRepositoriesForEnvironment. Add server-authoritative capability/error projection, cloneable locator validation, executor-side destination checks, durable inventory/reuse, and compensation across RPC/database failure. Keep host folder live-attachment unavailable and explain separate upload semantics in UI-03.

Acceptance: the first remote repository appears inside the existing scratch CWD and survives resume without restart; unknown/disconnected/unsupported providers never fall back to host filesystem operations; failed batches remove only owned additions and retain prior files and tracking.

Proposed cases: TestRemoteWorkspaceSources_ScratchFirstRepository; TestRemoteWorkspaceSources_InventoryFailureCompensatesClone; TestRemoteWorkspaceSources_DisconnectedHasNoHostFallback. Use existing Kubernetes materializer tests and add matching shared-contract cases for concrete runnable providers. Update public attachment instructions with + location, Local link consequences, and the executor matrix.

Out of scope: root expansion, native-session replacement, active-turn admission, host-to-remote mounts/sync, and unrelated refactors.

## ASCII UI preview

Relevant views copied from the [combined preview](plan.md#ascii-ui-preview). Required order and capability semantics must match rendered UI; spacing is illustrative.

### UI-03: Remote/container workspace and unavailable states

```text
+------------------------------------------------------------+
| Add repositories or folders                            [X] |
| Workspace: /workspace/task-123   Executor: SSH (build-host)  |
| [+ Repository v]  [+ Folder: unavailable]                   |
| Host folders cannot be linked into this executor.          |
| Upload folder in + copies files when uploads are supported. |
|                                                            |
| Repository [payments-api v]   Branch [main v]                |
| Destination: /workspace/task-123/payments-api/               |
| Cloned on build-host. Current directory stays unchanged.    |
| Session and running processes stay unchanged.               |
|                                        [Cancel] [Add sources]|
+------------------------------------------------------------+

Loading:      Checking workspace capabilities... [Add disabled]
Disconnected: Reconnect the executor to add sources. [Retry]
Unprepared:   Start the task to prepare its workspace.
Busy:         Wait for the active turn to finish.
Collision:    reference already exists. Choose another name.
Stale:        Workspace changed. Review updated destinations.
Failed:       Could not add sources. [Retry] (rows preserved)
Submitting:   Adding sources... (prevent duplicate submission)
```

The actual executor name, supported clone path, and error are server-derived. Do not show a host folder browser remotely or promise that a local-only repository is cloneable.
Maps to AC-005.2 and AC-006.5/.6/.7/.8. Failure must not silently select another placement.

### UI-04: Phone

```text
Files   /workspace/...   [+] [...]
                         | New file
                         | Upload files
                         | Upload folder
                         | Add repositories or folders

+----------------------------------+
| Add repositories or folders  [X] | fixed header
| Local / research                 |
|----------------------------------|
| [+ Repository v] [+ Folder]      | one scroll body
| [payments-api v]                 |
| Branch [main v]                  |
| [/home/me/reference] [Browse]    |
|                                  |
| (*) Current folder               |
|     Short paths; more entries.   |
| ( ) Inside ./kandev/             |
|     Grouped; longer paths.       |
|                                  |
| Result and unchanged CWD         |
| Folder links edit original files.|
| Access rules still apply.        |
|----------------------------------|
| [Cancel]          [Add sources]  | fixed safe-area footer
+----------------------------------+
```

Use the existing full-height source drawer for the form, not a compressed desktop dialog. Dynamic viewport height, vertical-only content, wrapping paths, visible disabled reasons, >=44px touch targets, and no page overflow are required. Return focus to + after close. Remote phones use UI-03 capabilities in this same composition.
Maps to AC-005.3 and AC-006.1/.5/.6/.7. The menu/dialog transition must not steal focus or close the new surface.

## Verification

Use TDD for changed behavior. Commands run from repository root; install workspace dependencies from apps if absent. E2E requires built assets and the existing guarded runner. Record actual results and environment blockers; do not mark unavailable provider checks passed.

```bash
(cd apps/backend && go test ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api)
(cd apps/web && pnpm exec vitest run components/workspace-source-picker/executor-capabilities.test.ts components/task/add-workspace-sources)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
make build-web
make build-backend
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/docker/add-workspace-sources.spec.ts tests/ssh/add-workspace-sources.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/task/mobile-add-workspace-sources.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/workspace_source_materializer.go`
- `apps/backend/internal/backendapp/workspace_source_materializer_kubernetes_test.go`
- `apps/backend/internal/agent/runtime/lifecycle`
- `apps/backend/internal/agent/runtime/agentctl/client_workspace_sources.go`
- `apps/backend/internal/agentctl/server/api`
- `apps/web/components/workspace-source-picker/executor-capabilities.ts`
- `apps/web/components/task/add-workspace-sources`
- `apps/web/e2e/tests/docker/add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/ssh/add-workspace-sources.spec.ts`
- `docs/public`

Also update affected locale catalogs and focused tests beside changed code. Read scoped AGENTS.md before implementation. Use docs-maintainer for public documentation changes.

## Dependencies

02-local-cwd. Read the requirements, current-workspace design, and earlier placement package. Do not clear its expansion gate.

## Risks

See the package risks; verify ownership and effective root before filesystem changes. No automatic rebind is permitted for unchanged-CWD additions.

## Parallelism

`sequential`

## Inputs

[Requirements](../../specs/tasks/requirements/attach-workspace-sources.md), [design](../../specs/tasks/system-design/current-workspace-sources.md), existing attachment tests, and the assigned previews.

## Results

Implemented. Runnable remote and container workspaces add repository sources through the executor
materializer under the established executor CWD. Local Git sources require a cloneable origin, host
folders remain unavailable remotely, and database or RPC failures compensate created inventory and
filesystem state. Persisted remote inventory is reused on reconnect and resume without changing the
agent CWD.

Verification passed:

- Backend materializer, service, lifecycle, and executor tests passed, including remote locator and compensation cases.
- Docker and SSH container-project E2E flows passed: scratch-first repository, partial-batch rollback, and reconnect/resume coverage.
- Public attachment documentation was updated for the `+` entry, Local live-link behavior, remote clone requirements, and folder limits.

Kubernetes and Sprites were not run with live infrastructure in this environment. Their support
continues through the shared executor materializer contract and existing provider tests.
