---
id: "03-detach-and-files-roots"
title: "Root menu, detach, and Files root markers"
status: complete
wave: 3
depends_on:
  - "02-automatic-detection"
plan: "plan.md"
requirements:
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-003
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-004
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-005
acceptance_criteria:
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.2
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.4
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.5
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.6
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.7
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.2
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.4
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.5
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-005.6
system_design:
  - ../../specs/tasks/system-design/agent-worktree-adoption.md
---

# Task 03: Root menu, detach, and Files root markers

## Summary

Right-clicking a root, on its Changes group header or its marked Files folder, opens one shared
root menu: copy paths and branch, open in editor or terminal, jump between panels, push and PR, and
Remove from task for non-primary roots. Removal keeps everything on disk and on the Git host, and stops detection from adopting
the same path again until the agent reports it. The Files panel marks each root and reveals hidden
ancestors.

## In scope

- `task_detached_worktrees` table, repository methods, replay and gated PostgreSQL tests.
- `Service.DetachTaskRepository`, primary-root rejection, post-commit rescan.
- WS action `task.repository.detach` and `DELETE /api/v1/tasks/:id/repositories/:task_repository_id`
  with authorization.
- `AdoptWorktree`: suppression for `detected`, clearing for `reported`.
- Web: store root list; shared `root-context-menu-items.tsx` driven by one root descriptor;
  Changes group header as `ContextMenuTrigger` (no overflow button); Files root marker, with root
  rows rendering the root menu instead of `FileContextMenuItems` and delete/rename shortcuts
  blocked on roots; long-press bottom sheet on touch; Shift+F10 and context-menu key; shared
  confirmation (dialog on desktop, drawer on phone); seven locale catalogs.
- "Open terminal here": confirm the user terminal API passes a working directory to agentctl
  `shell` (`WorkDir`), adding the field if it is missing.

## Out of scope

- Deleting directories, worktree registrations, branches or pull requests.

## Acceptance

- Detach removes the root from both panels without any filesystem or Git change and without
  touching the agent; a later detection of the path is suppressed and a later report re-attaches.
- The primary root has no Remove from task entry and a direct request for it fails unchanged.
- Both surfaces open the same root menu by right-click, long-press, and keyboard; the Files root row
  never offers Delete or Rename; copy actions yield the absolute path, the task-relative path, and
  the branch name.

## ASCII UI preview

See [plan.md](plan.md#ascii-ui-preview) for the full views.

UI-01 excerpt: right-click on a root's Changes header (Files root rows show the same items, with
"Show in Changes").

```text
v kandev · fix-login  <right-click>
  +--------------------------------+
  | Open in editor               > |
  | Open terminal here             |
  | Show in Files                  |
  |--------------------------------|
  | Copy full path                 |
  | Copy relative path             |
  | Copy branch name               |
  |--------------------------------|
  | Push                           |
  | Create PR   (or Open PR #123)  |
  |--------------------------------|
  | Remove from task...            |
  +--------------------------------+
```

UI-02 (Files root), UI-03 (confirmation) and UI-04 (phone long-press sheet) are in the plan.
AC-004.2, AC-004.5, AC-004.6, AC-004.7, AC-005.1, AC-005.2.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/task/service/ -run 'TestDetachTaskRepository|TestAdoptWorktree' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite/ -run 'TestDetachedWorktrees' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/controller/... ./internal/gateway/websocket/... -run 'Detach' -count=1)
(cd apps/web && pnpm exec vitest run components/task/changes-panel-repo-groups components/task/task-files-panel components/task/remove-root-dialog components/task/root-context-menu-items components/task/file-context-menu)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
bash scripts/lint-go-changed $(git ls-files -mo --exclude-standard -- 'apps/backend/*.go')
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_schema.go`, `base_migrations.go`, new
  `detached_worktrees.go`
- `apps/backend/internal/task/service/service_worktree_adoption.go`
- `apps/backend/internal/task/controller/`, `apps/backend/pkg/websocket/actions.go`
- `apps/web/components/task/changes-panel-repo-groups.tsx`
- `apps/web/components/task/task-files-panel.tsx`, `file-context-menu.tsx`,
  `file-tree-editor-menu.tsx`, new `root-context-menu-items.tsx` and `remove-root-dialog.tsx`
- User terminal creation path (web API client and backend handler) if `WorkDir` is not exposed
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 02.

## Risks

- Hidden-folder reveal must reuse the existing hidden-folder browsing behavior, not bypass it for
  unrelated folders.

## Parallelism

`sequential`

## Inputs

- Design sections: Detach, Persistence, Frontend, Security.
- `/mobile-parity` for the drawer and touch menus.

## Results

Added `task_detached_worktrees`, `DetachTaskRepository` with primary-root rejection, WS and HTTP detach routes, Files root markers, shared root context menu, confirmation surfaces, Remove from task, live stale-root clearing, and terminal working-directory support. Covered by service, repository, dialog, root-menu, and files-root tests.
