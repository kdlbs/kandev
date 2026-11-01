---
id: TASK-DELETE-RECOVERY-01
title: Delete tasks while retaining workspace resources
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-DELETE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-DELETE-RECOVERY-001.1
  - AC-TASKS-DELETE-RECOVERY-001.2
  - AC-TASKS-DELETE-RECOVERY-001.3
  - AC-TASKS-DELETE-RECOVERY-001.4
  - AC-TASKS-DELETE-RECOVERY-001.5
  - AC-TASKS-DELETE-RECOVERY-001.6
  - AC-TASKS-DELETE-RECOVERY-001.7
  - AC-TASKS-DELETE-RECOVERY-001.8
system_design:
  - ../../specs/tasks/system-design/task-delete-recovery.md
---

# Delete tasks while retaining workspace resources

## Summary

Deliver the complete keep-files deletion path. Use TDD for backend and UI logic.
Keep the normal deletion contract intact. The user authorized implementation
after reviewing the design package.

## In scope

Typed delete mode and confirmation binding, database-only retain preparation,
durable retention snapshots, shutdown separation, cleanup consumer guards,
HTTP/MCP outcomes, shared UI and callback propagation, all supported locales,
public documentation, and regression tests. Audit every snapshot reader and
every consumer that can remove workspace files or their storage.

## Out of scope

Repairing live incident files, changing archive policy, new cleanup queues,
automatic commit/push, new resource management screens, and releasing retained
resources. Do not create persistent tasks or sessions to implement this work.

## Implementation acceptance

1. A damaged or unreachable workspace cannot prevent an authorized retain-mode
   task deletion when its database inventory and deletion transaction succeed.
2. Resource retention survives task/workspace deletion, worker completion,
   restart, and competing cleanup attempts. Runtime shutdown remains accounted
   for without deleting file-bearing resources.
3. Desktop, phone, HTTP, and MCP expose the explicit outcome. Normal cleanup,
   authorization, cascade, shared ownership, and confirmation regressions pass.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [combined preview](plan.md#ascii-ui-preview):

```text
Desktop, retain ready
Delete task
The task and conversation will be deleted.
[x] Keep workspace files
    Workspace files and branches will remain.
[ ] Also delete 3 subtasks                    (if present)
             [Cancel] [Delete task and keep files]

Phone, centered inset alert
Delete task
The task and conversation will be deleted.
[x] Keep workspace files
    Files and branches will remain.
[ ] Also delete 3 subtasks                    (if present)
[ Delete task and keep files     ]
[             Cancel             ]
```

UI-03: Mode changes start a new preview and disable submission until it returns.
Normal inspection errors leave Retry and the unchecked keep-files choice
available. Retain preview errors leave Retry available and submission disabled.
Discard consent is hidden in retain mode. The body scrolls inside the surface.
Phone footer actions remain visible and at least 44px high. Verify translated
copy without clipping. Do not redesign the shared dialog or archive surface.

## Regression matrix

| Boundary | Required proof |
| --- | --- |
| Damaged worktree | Empty/invalid marker, empty `.git`, broken ref, absent path, permission denied, and Git timeout block normal cleanup but not retain deletion. No physical inspector or HEAD capture runs in retain mode. |
| Selection | Zero-session task, multiple repositories, mixed healthy/broken bulk roots, archived descendants, noncascade child promotion, and inherited/shared environment retain exact scope. Unauthorized descendant or database inventory failure causes no premature deletion. |
| Confirmation | Mode mismatch, discard+retain, expired/replayed ticket, scope changes, stale responses, close/reopen, and switch back to normal cannot reuse consent. MCP has no implicit new authority. |
| Durability | Crash before/after snapshot and row commit, partial cascade, shutdown activation failure, stopped/unreachable runtime, and repeated recovery preserve exact results and handles. |
| Cleanup | Restart and every job state preserve retention. Storage sweep, quarantine/permanent delete, Docker cleanup, remote teardown, and branch/root cleanup cannot remove retained resources. Query/decode errors and unknown identities fail closed. |
| Race | Live inventory overlaps retained inventory, older cleanup claims are fenced, new session/resource creation respects preparation, and a scan before commit cannot authorize later deletion. |
| UI | Real corruption reproduces the old block. Desktop and phone retain deletion removes the task and preserves test files. Normal clean/dirty behavior, Retry, bulk outcomes, focus, keyboard, long locale text, and touch remain usable. |

Do not use real user tasks or the preserved incident workspace. Filesystem
assertions must include content hashes and Git registration/branch state where
applicable, not only the existence of a parent directory.

## Files likely touched

- Backend `internal/task/service/`: `task_delete_preflight.go`,
  `task_delete_confirmation.go`, `handoff_cascade.go`, `handoff_service.go`,
  `service_tasks.go`, `resource_cleanup_jobs.go`, and resource cleanup consumers.
  Add focused helpers rather than expanding large existing functions.
- Backend task handlers, `internal/mcp/handlers/config_task_handlers.go`, MCP
  tool schema declarations, task models and repositories, and corresponding tests.
- Backend `internal/backendapp/storage_inventory.go`, storage providers,
  worktree maintenance, and executor lifecycle boundaries found by the consumer audit.
- Web `hooks/use-task-delete-preflight.ts`, `lib/api/domains/kanban-api.ts`,
  `components/task/task-delete-confirm-dialog.tsx`, cleanup summaries, notification
  handling, deletion callback adapters, locales, and associated tests.
- Existing desktop/mobile deletion E2E files, the public documents in the plan,
  and this package's delivery results. Read scoped AGENTS.md files before editing.

## Verification

Run each command from the repository root. Install dependencies once before
the first pnpm command in this fresh worktree. The first guarded E2E command
builds its required artifacts. The second uses those same artifacts. Run the
commands sequentially with one shard and the runner's normal worker budget.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/... ./internal/mcp/handlers ./internal/backendapp ./internal/system/storage/... ./internal/worktree)
(cd apps/web && pnpm exec vitest run hooks/use-task-delete-preflight.test.ts lib/api/domains/kanban-api.test.ts components/task/task-delete-confirm-dialog.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium -- e2e/tests/kanban/card-menu-delete-archive.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome -- e2e/tests/task/mobile-delete-discard-consent.spec.ts e2e/tests/task/mobile-delete-task-redirect.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also run every new or changed test suite not covered by that block. Run
`bash scripts/lint-go-changed` with the actual changed Go paths and focused
ESLint from `apps/web` with the actual changed TS/TSX paths. Record those exact
expanded commands in Results. If executor lifecycle packages change, run their
focused regression tests too. Do not replace provider tests with mocks that
cannot detect a combined stop-and-destroy call.

## Dependencies and parallelism

No work-order dependencies. Sequential implementation in the primary session.
The shared deletion mode, snapshots, and consumer guards form one release unit.
The completed conditional-discard package supplies the existing confirmation
surface. It is not a pending delivery dependency.

## Inputs and risks

Read the requirement, design, ADR, and combined preview before editing.
Confirm actual cleanup consumers against this checkout, including the retained
snapshot owner. Do not assume every draft runtime-cleanup field exists in code.
Resolve any reader that cannot preserve retention before enabling the option.
Leaving resources consumes disk/provider capacity. Keep that consequence visible.

## Results

Implemented after the user authorized this package with “go for it”.
No live task, incident backup, or production instance was changed. Changes are
uncommitted and have not been deployed.

Delivered typed HTTP/MCP retention, mode-bound confirmation, database-only
preparation, durable cleanup inventory, shutdown outcomes, cleanup guards,
shared desktop/phone UI, seven locales plus pseudo, and public recovery guidance.
The inventory includes earlier cleanup snapshots when their live rows are gone.
Earlier stop handles are carried into the new retained shutdown job.

### Passed checks

- Focused Go deletion regressions: service, HTTP, MCP handlers/server, storage
  inventory, worktree cleanup, and runtime lifecycle. Retention regressions were
  rerun after adding historical inventory and shutdown-handle protection.
- Complete retention, workspace-storage, and Docker-storage package tests;
  focused SQLite cleanup-reservation tests and lifecycle stop tests.
- Retention package race detector, including nested mutation leases.
- Seven frontend unit-test files: **75 tests passed** in the final combined run.
- Type checking with a 6 GiB Node heap; changed-file Go lint reported zero
  issues; changed-file ESLint passed.
- Six desktop E2E tests and five phone E2E tests. Both damaged-workspace tests
  used real disposable checkouts with an empty ownership marker. They verified
  exact file hashes, Git registrations, and branch identities after deletion.
  The phone case repeated those assertions after a backend restart. It passed
  again after the final conversation-deletion copy update.
- Locale generation and i18n check/ratchet. Public-doc validation covered 48
  pages; its test suite passed 63 tests. Specification catalog and lint passed.
- Local PR documentation coverage accepted the complete delivery package;
  `git diff --check` passed.

The browser tests use isolated servers. This NixOS host requires Chromium's
shared libraries through `LD_LIBRARY_PATH`. The temporary runner also removes
inherited `KANDEV_*` and `AGENTCTL_*` values before setting test overrides, so
production's configured network address cannot leak into disposable servers.
It uses one shard, one worker, `GOMAXPROCS=2`, `GOFLAGS=-p=2`, and
`NODE_OPTIONS=--max-old-space-size=6144`. No host configuration was changed.

### Validation limits

The attempted broad Go sweep did not finish. MCP handlers and backendapp
reached the ten-minute timeout in SQLite fixture schema initialization;
additional handler/worktree fixture waits were stopped. Captured stacks point
to SQLite setup, not the retention mutation lease. This is not a confirmed
pre-existing defect. Focused checks passed after the edits settled, but this
work does not claim a successful full-package sweep.

Remote provider protection is tested at the shared destructive-stop boundary;
no live SSH, Kubernetes, or Sprites infrastructure was provisioned. Those
providers remain retained with shutdown pending when a safe process-only stop
cannot be proved. The regression matrix above describes the intended boundaries;
it is not a claim that each listed fault was separately injected.

### Commands and evidence

Commands below ran from `apps/backend`:

```bash
GOMAXPROCS=2 go test -p 2 ./internal/task/retention ./internal/task/service ./internal/task/handlers ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp ./internal/worktree ./internal/agent/runtime/lifecycle -run 'Test(Retain|Retained|OlderCleanup|ExplicitRetention|HTTPDeleteTask|HTTPTaskDeletePreflight|StorageInventoryProtectsRetained|DeleteTaskForwardsRetain|MutationLease|NestedMutation|HandleDeleteTask|TaskDelete|DeleteTask|PrepareTaskResourceCleanup|CleanupWorktrees_RemovesBranchByDefault)' -count=1
GOMAXPROCS=2 go test -p 2 ./internal/task/retention ./internal/task/service ./internal/backendapp ./internal/worktree ./internal/system/storage/workspaces ./internal/agent/runtime/lifecycle -run 'Test(Retain|Retained|OlderCleanup|ExplicitRetention|StorageInventoryProtectsRetained|MutationLease|NestedMutation|QuarantineRechecks)' -count=1
GOMAXPROCS=2 go test -p 2 ./internal/task/retention ./internal/system/storage/workspaces ./internal/system/storage/dockerstore
GOMAXPROCS=2 go test -p 2 ./internal/task/repository/sqlite -run 'Test(TaskResourceCleanup|CancelTaskResourceCleanup|RestoreCancelledTaskResourceCleanup|CancelArchiveTaskResourceCleanup|ListPreparedTaskResourceCleanup|HasActiveTaskResourceCleanup|PreparedCleanupSnapshot)'
GOMAXPROCS=2 go test -p 2 ./internal/agent/runtime/lifecycle -run 'Test(StopAgent|StopRunOwner|StandaloneExecutorStopInstance|RetainedWorkspace)'
GOMAXPROCS=2 go test -race -p 2 ./internal/task/retention
```

Commands below ran from `apps/web` with the environment described above:

```bash
pnpm exec vitest run hooks/use-task-delete-preflight.test.ts lib/api/domains/kanban-api.test.ts components/task/task-delete-confirm-dialog.test.tsx hooks/use-task-actions.test.ts hooks/use-task-crud.test.ts components/kanban/task-multi-select-toolbar.test.tsx components/task/task-session-sidebar-selection.test.ts
NODE_OPTIONS=--max-old-space-size=6144 pnpm run typecheck
pnpm run i18n:zh-hant
pnpm run i18n:pseudo
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --host --shards 1 --project mobile-chrome -- e2e/tests/task/mobile-delete-discard-consent.spec.ts e2e/tests/task/mobile-delete-task-redirect.spec.ts
pnpm e2e:run --host --shards 1 --no-build --project chromium -- e2e/tests/kanban/card-menu-delete-archive.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome -- e2e/tests/task/mobile-delete-discard-consent.spec.ts --grep 'deletes a damaged task'
```

Expanded lint commands from the repository root:

```bash
nix shell nixpkgs#golangci-lint --command bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/lifecycle/manager_environment.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_execution.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_idle_parking.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_environment_cleanup.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_kubernetes_restart_cleanup.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_launch.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_ssh_restart_cleanup.go \
  apps/backend/internal/agent/runtime/lifecycle/remote_recovery_stop.go \
  apps/backend/internal/agent/runtime/lifecycle/run_owner_persistence.go \
  apps/backend/internal/agent/runtime/lifecycle/workspace_retention.go \
  apps/backend/internal/agent/runtime/lifecycle/workspace_retention_test.go \
  apps/backend/internal/backendapp/storage_inventory.go \
  apps/backend/internal/backendapp/storage_inventory_test.go \
  apps/backend/internal/mcp/handlers/config_task_handlers.go \
  apps/backend/internal/mcp/handlers/task_delete_recovery_test.go \
  apps/backend/internal/mcp/server/config_handlers.go \
  apps/backend/internal/mcp/server/server.go \
  apps/backend/internal/mcp/server/task_delete_recovery_test.go \
  apps/backend/internal/system/storage/dockerstore/provider.go \
  apps/backend/internal/system/storage/workspaces/provider.go \
  apps/backend/internal/system/storage/workspaces/retention_test.go \
  apps/backend/internal/task/handlers/task_delete_authz_test.go \
  apps/backend/internal/task/handlers/task_delete_preflight.go \
  apps/backend/internal/task/handlers/task_delete_preflight_test.go \
  apps/backend/internal/task/handlers/task_http_handlers.go \
  apps/backend/internal/task/models/workspace_cleanup.go \
  apps/backend/internal/task/repository/sqlite/workspace_retention.go \
  apps/backend/internal/task/retention/gate.go \
  apps/backend/internal/task/retention/gate_test.go \
  apps/backend/internal/task/retention/inventory.go \
  apps/backend/internal/task/retention/inventory_test.go \
  apps/backend/internal/task/service/handoff_cascade.go \
  apps/backend/internal/task/service/handoff_service.go \
  apps/backend/internal/task/service/resource_cleanup_jobs.go \
  apps/backend/internal/task/service/resource_cleanup_preparation.go \
  apps/backend/internal/task/service/resource_cleanup_retained.go \
  apps/backend/internal/task/service/service.go \
  apps/backend/internal/task/service/service_tasks.go \
  apps/backend/internal/task/service/task_delete_confirmation.go \
  apps/backend/internal/task/service/task_delete_options.go \
  apps/backend/internal/task/service/task_delete_preflight.go \
  apps/backend/internal/task/service/task_delete_recovery_test.go \
  apps/backend/internal/task/service/task_delete_result.go \
  apps/backend/internal/worktree/branch_cleanup.go \
  apps/backend/internal/worktree/manager_cleanup.go \
  apps/backend/internal/worktree/manager_cleanup_taskdir.go \
  apps/backend/internal/worktree/workspace_retention.go \
  apps/backend/internal/worktree/workspace_retention_test.go

(cd apps/web && pnpm exec eslint \
  app/tasks/columns.tsx \
  app/tasks/tasks-list-view.tsx \
  app/tasks/tasks-page-client.tsx \
  components/kanban-card-menu.tsx \
  components/kanban/task-multi-select-toolbar.tsx \
  components/task/chat/messages/action-message-actions.tsx \
  components/task/mobile/session-task-switcher-sheet-dialogs.tsx \
  components/task/task-delete-confirm-dialog.test.tsx \
  components/task/task-delete-confirm-dialog.tsx \
  components/task/task-session-sidebar-dialogs.tsx \
  components/task/task-session-sidebar-selection.tsx \
  e2e/helpers/task-delete-recovery.ts \
  e2e/tests/kanban/card-menu-delete-archive.spec.ts \
  e2e/tests/task/mobile-delete-discard-consent.spec.ts \
  e2e/tests/task/mobile-delete-task-redirect.spec.ts \
  hooks/use-task-actions.test.ts \
  hooks/use-task-actions.ts \
  hooks/use-task-delete-preflight.test.ts \
  hooks/use-task-delete-preflight.ts \
  lib/api/domains/kanban-api.test.ts \
  lib/api/domains/kanban-api.ts)
```

Local logs are under `/tmp/task-delete-*.log`. The final regression, retention,
race, UI, lint, localization, desktop, and phone logs include the pass summaries.
These temporary files are diagnostic evidence for this session, not repository
artifacts or a requirement for a future test run.


### PR review regressions

Retain preparation now publishes its mode only with a complete snapshot.
Cleanup waits for runtime shutdown before acquiring the physical mutation lease,
then rechecks retention. Saved task/session/execution identity admits safe local
shutdown for shared tasks without an owned environment. Unknown or destructive
runtimes remain pending. Post-commit responses preserve the computed shutdown
status, and retention maps omit empty IDs.

Regression tests cover failed inventory capture and snapshot writes, independent
stop admission, retention published during shutdown, shared runtime identity,
and completed versus pending shutdown in HTTP responses. Missing prior-cleanup
inventory still blocks deletion because earlier stop handles must survive.
The focused eight-package regression command above passed with the added
`FailedRetain`, `CleanupShutdownDoesNotHold`, `CleanupRechecksRetention`, and
`HTTPRetainedDelete` test prefixes. The prior-inventory guard test and retention
race suite also passed. CI and review completion are tracked on the PR.
