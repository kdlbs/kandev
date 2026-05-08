# Tasks Without Repositories — Implementation Plan

**Spec:** `./spec.md`

## Approach

The model and schema already support repo-less tasks (`Task.Repositories` is a slice; `task_sessions.repository_id` is nullable). The lifecycle manager already knows how to launch agents in a non-git workspace via the existing `quick-chat` path (`manager_launch.go:182`, which uses `~/.kandev/data/quick-chat/{sessionID}`).

A repo-less task can launch in one of two workspace modes:

- **Scratch mode** — kandev creates an empty per-task directory at `~/.kandev/data/scratch/<task-id>/`. Used for any executor when no folder is picked.
- **Picked-folder mode** — the user supplies a host path (e.g. `~/Documents/research/`); the agent's working directory is set to that path verbatim. Local executors only.

The picked path needs to persist across sessions of the same task, so we add a nullable `workspace_path` column on `task_sessions` (or a per-task field — TBD in implementation; simplest is per-session because that's where workspace path already lives).

The rest of the work is UX exposure plus a few guards:

1. Backend: persist the picked workspace path; resolve it in `launchResolveWorkspacePath`.
2. Backend: skip the `WorktreePreparer` and the repo-cloning prepare scripts when no repo is attached.
3. Backend: force the executor to `local` when the workspace default is `worktree` and the task has no repo. Reject non-local executors when a folder was picked.
4. Frontend: add a "no repository" path to the create-task dialog with an optional folder picker; hide repo-bound panels when `task.repositories.length === 0`.

One small schema change (the `workspace_path` column). No new executor type.

## Backend

**Schema migration** (`internal/task/repository/sqlite/`)
- Add nullable `workspace_path TEXT DEFAULT ''` column to `task_sessions`. Mirror what `repository_id` already does. Migration follows the pattern in `base.go:247-274`.
- Expose on the `TaskSession` model in `internal/task/models/models.go`.

**Resolve workspace path** (`internal/agent/lifecycle/manager_launch.go`)
- Update `launchResolveWorkspacePath()`:
  1. If session has a `WorkspacePath` set (picked-folder mode) → use it directly. Validate it exists and is a directory; reject if missing.
  2. Else if task has no repos → use scratch dir `~/.kandev/data/scratch/<taskID>/` (create if missing). Note: `<taskID>` not `<sessionID>` so the scratch persists across sessions of the same task — diverges from current `quick-chat/{sessionID}` convention; confirm that's the right call.
  3. Else → existing repo-bound logic.
- Unit-test all three branches.

**Worktree preparer** (`internal/agent/lifecycle/env_preparer_worktree.go:33-120`)
- Already validates `req.RepositoryPath` and returns early. No change — but cover with a test that asserts a no-repo `EnvPrepareRequest` skips the preparer cleanly.

**Default scripts** (`internal/agent/lifecycle/default_scripts.go`)
- The `local` and `worktree` scripts reference `{{repository.clone_url}}`, `{{repository.setup_script}}`. For repo-less tasks, return an empty/no-op prepare script. Cleanest: in the script resolver, short-circuit to `""` when the repo context is empty.
- File to touch: `internal/scriptengine/` (placeholder resolver) — return empty string for the whole script when all `{{repository.*}}` placeholders resolve to empty *and* the script is the default. Or simpler: in `manager_launch.go`, detect no-repo and skip prepare-script execution entirely.
- Pick the latter — explicit branch in lifecycle manager, no resolver changes.

**Executor fallback / restriction** (`internal/agent/lifecycle/manager.go` + `manager_launch.go`)
- If the resolved executor type is `worktree` and the task has zero repos, downgrade to `local`. Log a warning.
- If `WorkspacePath` is set (picked-folder mode), require executor type `local` / `local_pc`. Reject other executor types at launch time with a clear error — the dialog should already prevent this, but enforce server-side too.

**Service validation** (`internal/task/service/service_tasks.go:102-154`)
- `createTaskRepositories()` already no-ops on empty slice — confirmed. No change.

**HTTP / WS handlers**
- Accept an optional `workspace_path` field in the create-task request alongside `repositories`. Pipe it through to the first session.
- Verify no handler rejects an empty `repositories` array on create. Add a service test creating a task with `repositories: nil` (with and without `workspace_path`).

**Folder picker endpoint** (new)
- `GET /api/v1/fs/list-dir?path=<abs-path>` — returns immediate subdirectories of `path`. Used by the frontend folder picker.
- Default starting path: `$HOME`. Refuse to traverse outside it unless an absolute path is explicitly supplied. Return entries with `name` + `is_dir` only.
- Check whether kandev's existing repo-add flow has a similar endpoint to reuse instead of creating a new one.

## Frontend

**Create-task dialog** (`apps/web/components/task-create-dialog.tsx` + `task-create-dialog-repo-chips.tsx`)
- Add a "No repository" toggle/option above the chips row. When active:
  - Hide the chip row.
  - Reveal an optional folder picker (collapsible: "Start in a folder…").
  - Submit `repositories: []` and the picked `workspace_path` (or empty string) in the create payload.
- When the workspace has at least one repo, default the toggle off.
- When the workspace has zero repos connected, default the toggle on.
- When a folder is picked, hide non-local executors from the executor picker (Docker/Sprites/remote not available).
- When no folder is picked, the executor picker behaves normally (minus `worktree`).

**Task detail / repo-bound panels**
- `apps/web/components/github/pr-detail-panel.tsx` — early-return when `task.repositories?.length === 0` (renders nothing, not an empty state).
- Diff viewer, branch picker, git status panels — same conditional. Find each via grep for usages of `task.repositories` and `gitStatus.bySessionId`.
- Centralize the check: a small `useTaskHasRepo(taskId)` hook in `hooks/domains/kanban/`.

**Folder picker component** (new — `apps/web/components/fs/folder-picker.tsx`)
- Modal or popover. Lists directories from the new `/api/v1/fs/list-dir` endpoint, starting at `$HOME`. Click to descend, breadcrumbs for ascent, "Choose this folder" button to commit.
- Reuse if a similar component already exists (check `apps/web/components/` for repo-add flow).

**Executor picker**
- When no repo is selected and no folder is picked, hide `worktree`.
- When a folder is picked, hide all non-local executors.

**Kanban**
- Treat repo-less tasks identically (per spec). No badge, no icon. Verify nothing in `apps/web/app/tasks/columns.tsx` crashes on `task.repositories?.[0] === undefined` — it already uses optional chaining (line 39-40), so should be safe.

## Phases

1. **Schema + workspace resolution.** Add `task_sessions.workspace_path` migration; update `launchResolveWorkspacePath` for the three branches (picked folder / scratch / repo); unit-test each branch.
2. **Backend guards.** Skip prepare-script when no repo; executor fallback `worktree`→`local`; reject non-local executors when `workspace_path` is set.
3. **Folder-picker endpoint.** `/api/v1/fs/list-dir`; or reuse existing if found.
4. **Frontend create flow.** Toggle in create-task dialog; folder picker component; submit empty `repositories` + optional `workspace_path`.
5. **Frontend conditional UI.** `useTaskHasRepo` hook; gate PR panel, diff viewer, git status, branch picker, executor picker.
6. **E2E.** Two Playwright tests:
   - Repo-less + scratch: create task → agent launches → verify no diff/PR UI.
   - Repo-less + picked folder: create task pointing at a temp dir → agent reads/writes file there.
7. **Verify.** `make fmt` then `make typecheck test lint` (backend); `pnpm typecheck test lint` (web).

## Risks / open points

- **Picked-folder validation.** Symlinks, missing paths, paths inside `$HOME/.kandev/`, paths containing existing kandev tasks — what do we reject? v1: require absolute path, require it to exist as a directory, no other restrictions. Trust the user.
- **Persistence of picked path across sessions.** Picked path needs to survive task restart. `task_sessions.workspace_path` per-session works if we copy it from the prior session on resume. Per-task `Task.workspace_path` is simpler but a slightly bigger schema change. Pick one in implementation.
- **Quick-chat vs scratch namespace.** Existing `quick-chat/{sessionID}` is per-session; new scratch per-task `scratch/{taskID}` diverges. Decide: rename quick-chat to scratch and unify, or keep both with clear semantics?
- **Docker / Sprites scratch dirs.** `local_docker` will need to bind-mount the host scratch dir into the container; sprites needs an internal scratch dir. Check `default_scripts.go` Docker/Sprites scripts to confirm — may need a no-repo variant for each. If scope grows, restrict v1 to `local` only and defer Docker/Sprites to v2.
- **Workspace cleanup.** Today nothing seems to clean up `quick-chat/{sessionID}` dirs. If we add `scratch/{taskID}`, accumulate even more cruft. Out of scope to fix here, but worth a follow-up issue.

## Out of scope (per spec)

Promoting a repo-less task to repo-bound after creation; detaching repos from existing tasks; cross-task scratch dirs; visual badges on the kanban; alternative tool surfaces.
