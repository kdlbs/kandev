---
id: "01-root-file-discovery"
title: "Discover and publish workspace root files"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKSPACE-ROOT-FILES-001
  - REQ-TASKS-WORKSPACE-ROOT-FILES-002
acceptance_criteria:
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.2
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.3
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.4
  - AC-TASKS-WORKSPACE-ROOT-FILES-001.5
  - AC-TASKS-WORKSPACE-ROOT-FILES-002.2
system_design:
  - ../../specs/tasks/system-design/workspace-root-files.md
---

# Task 01: Discover and publish workspace root files

## Summary

Give the bare multi-repository root tracker a bounded, read-only listing of
regular files outside every checkout, and expose it from agentctl to the
backend WS action `workspace.root_files.list`. Make the root tracker's monitor
tick emit `file_change` notifications when that listing changes.

## In scope

- `WorkspaceTracker` gains checkout roots (set by the manager) and
  `ListWorkspaceRootFiles(ctx)`, with the exclusions and 500 cap from the
  system design.
- Wire types `WorkspaceRootFile`, `WorkspaceRootDestination`, and
  `WorkspaceRootFiles` in `internal/agentctl/types`.
- `Manager.WorkspaceRootFiles(ctx)` returns `Supported=false` unless the root
  tracker has `suppressGitPublication`. It fills `Destinations` from the
  repository-scope trackers and skips submodules. The manager sets checkout
  roots in `buildWorkspaceTrackerGraph`, rebind, and rescan (`manager_rescan.go`).
- `observeWorkspaceRootFiles` runs on the bare root's monitor tick. The first
  tick only seeds the snapshot. Later ticks emit create/write/remove
  notifications per changed path.
- agentctl route `GET /api/v1/workspace/root-files`, the runtime client method
  `ListWorkspaceRootFiles`, and the WS action constant
  `ActionWorkspaceRootFilesList = "workspace.root_files.list"` with its handler
  next to `wsGetFileTree`.

## Out of scope

Frontend, moves (the existing rename already covers them), Git status changes,
and metrics.

## Acceptance

1. In a temporary bare root with two `git init` checkouts, the listing returns
   `notes.md` and `dir/report.json`. It omits checkout contents, `.hidden/x`,
   `node_modules/y`, a nested `other/.git` repository, symlinks (including a
   symlink to a directory outside the root), and the ownership marker. With 501
   eligible files it returns 500 and `truncated=true`. A Git-repository root
   returns `supported=false`.
2. Observer test: seed, then create, modify (size or mtime), and delete a root
   file. Each tick emits exactly one notification with the expected operation
   and an empty repository name. Changes inside a checkout emit none from this
   observer.
3. The handler test returns the agentctl body for a session, and rejects a
   missing `session_id` with a validation error. A test-only command hook
   asserts that the listing path starts no Git subprocess.

## Verification

```bash
(cd apps/backend && go test -race -trimpath ./internal/agentctl/server/process -run 'WorkspaceRootFiles' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/agentctl/server/api -run 'WorkspaceRootFiles' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/agent/handlers -run 'WorkspaceRootFiles' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/agentctl -run 'WorkspaceRootFiles' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/agentctl/server/process -count=1)
bash scripts/lint-go-changed $(git diff --name-only -- '*.go')
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- New `apps/backend/internal/agentctl/server/process/workspace_root_files.go` and `workspace_root_files_test.go`
- `apps/backend/internal/agentctl/server/process/workspace_tracker.go` (checkout-roots field and observer state)
- `apps/backend/internal/agentctl/server/process/workspace_monitor.go` (bare-root tick hook)
- `apps/backend/internal/agentctl/server/process/manager_submodules.go`, `manager_rescan.go`
- `apps/backend/internal/agentctl/types/types.go`
- `apps/backend/internal/agentctl/server/api/workspace.go`, `server.go`, plus a new API test file
- `apps/backend/internal/agent/runtime/agentctl/` client file and test
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/agent/handlers/workspace_file_handlers.go` and a new test file

## Dependencies

None. Read `observeWorktreeInventory` (`workspace_worktree_observer.go`) and
`buildWorkspaceTrackerGraph` before you start.

## Risks

- A rescan replaces the root tracker. Set checkout roots on the replacement
  before it starts ticking, or the first walk lists checkout contents.
- Keep `workspace_tracker_multi_repo_test.go` green. Root-file notifications
  must not create an unnamed Git repository group.

## Parallelism

`sequential`

## Results

- Implemented bounded, symlink-safe root-file discovery, checkout exclusions,
  and snapshot-delta notifications without Git commands.
- Added the agentctl HTTP route, runtime client method, and session-keyed WebSocket
  action.
- Passed the focused race-enabled process, API, handler, and runtime-client
  tests; passed the full process package race test (531.373s).
- Added regressions for unreadable and vanished directories, bounded scans,
  symlink replacement, checkout roots under a symlinked parent, and refreshes
  when the truncation state changes. The targeted tests passed.
- Passed `bash scripts/lint-go-changed` for all changed Go files.
- Passed `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
