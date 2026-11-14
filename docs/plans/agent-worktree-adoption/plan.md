---
created: 2026-11-10
status: complete
requirements:
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-001
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-002
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-003
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-004
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-005
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-006
system_design:
  - ../../specs/tasks/system-design/agent-worktree-adoption.md
legacy_specs: []
---

# Implementation Plan: Agent worktree adoption

## Overview

Agent-created Git worktrees inside the task workspace become ordinary task branches, visible in the
Changes and Files panels with commit and pull-request actions. Delivery runs in four sequential
slices:

1. **Agent-reported adoption.** The adoption service, the `worktree_path` MCP input, and the
   prompt guidance. This alone makes reported worktrees appear, and every later slice reuses the
   service.
2. **Automatic detection.** The agentctl observer and the backend subscriber. They feed the same
   service, and also handle removal from disk.
3. **Detach and Files roots.** User removal, its suppression of detection, and the Files panel
   root markers.
4. **End-to-end proof.** A mock-agent scenario and desktop plus phone Playwright coverage.

Requirements: [agent worktree adoption](../../specs/tasks/requirements/agent-worktree-adoption.md).
Design: [agent worktree adoption design](../../specs/tasks/system-design/agent-worktree-adoption.md).

## Scope

### In scope

- `add_branch_to_task_kandev` `worktree_path` adoption for host Worktree and Local executors.
- Detection of linked worktrees of attached repositories inside the task workspace root.
- Reconciling detection with reports, and removing roots whose worktree was deleted.
- A shared right-click root menu (copy paths and branch, open in editor or terminal, cross-panel
  navigation, push and PR, Remove from task) on Changes headers and Files root folders.
- Remove from task for every non-primary root, with suppression of re-detection.
- Files panel repository-root markers and Changes group labels and menus.
- Executor-gated agent guidance in the first-turn Kandev context.

### Out of scope

- Worktrees outside the task workspace root, remote and container executors, fresh clones.
- Deleting worktrees or branches from Kandev; a user "add worktree by path" action.
- Moving the agent working directory.

## Technical approach

| Boundary | Change |
| --- | --- |
| Schema (`internal/task/repository/sqlite`) | `task_environment_repos.worktree_origin` via `runMigrations()`; new `task_detached_worktrees` table; repository methods with SQLite replay and gated PostgreSQL tests. |
| Task service (`internal/task/service`) | New `service_worktree_adoption.go`: `AdoptWorktree`, adoption materializer for `commitWorkspaceSourceBatch`, nested-checkout exclusion, `DetachTaskRepository`, and a coalescing worker fed by `WorkspaceWorktreesObserved`. |
| MCP (`internal/mcp/server/server.go`, `internal/mcp/handlers/handlers.go`) | `worktree_path` input on `add_branch_to_task_kandev`; handler routes to `AdoptWorktree` when set. |
| agentctl (`internal/agentctl/server/process`) | `workspace_worktree_observer.go` called from `gitPollTick`; new `types.WorkspaceMessageTypeWorktreeInventory`; adopted tracker names use the canonical task-relative path. |
| Lifecycle (`internal/agent/runtime/lifecycle`) | Translate `worktree_inventory` into `events.WorkspaceWorktreesObserved` with execution identity. |
| Prompt (`internal/sysprompt`, `config/prompts/kandev-context.md`) | `IncludeWorktreeAdoption` option and `{worktree_adoption_section}`. |
| Web (`apps/web/components/task`) | Root label, shared root context menu, Files root marker, shared confirm dialog, store root list, seven locale catalogs. |

Executor compatibility:

| Executor | Report | Detect | Detach |
| --- | --- | --- | --- |
| Worktree (host) | yes | yes | yes |
| Local (host) | yes | yes | yes |
| Docker, SSH, Kubernetes, Sprites | actionable error | ignored | yes (Kandev-created roots only) |

## ASCII UI preview

Structural requirements: one Changes group per root with its branch in the label; one shared root
menu opened by right-click on the Changes group header or the marked Files folder (long-press on
touch, Shift+F10 or the context-menu key on keyboard); no overflow button; Remove from task last and
only on non-primary roots; no Delete or Rename on root folders. Spacing and icons are illustrative.

UI-01: Changes panel, right-click on an adopted root's group header (desktop).

```text
+-- Changes ---------------------------------------------+
| v kandev  (primary)                [Stage all] [Commit]|
|     M apps/web/src/app.tsx                             |
| v kandev · fix-login  <right-click> [Stage all][Commit]|
|   +--------------------------------+                   |
|   | Open in editor               > |                   |
|   | Open terminal here             |                   |
|   | Show in Files                  |                   |
|   |--------------------------------|                   |
|   | Copy full path                 |                   |
|   | Copy relative path             |                   |
|   | Copy branch name               |                   |
|   |--------------------------------|                   |
|   | Push                           |                   |
|   | Create PR   (or Open PR #123)  |                   |
|   |--------------------------------|                   |
|   | Remove from task...            |                   |
|   +--------------------------------+                   |
+--------------------------------------------------------+
```

UI-02: Files panel, right-click on a marked root folder (desktop). Same items; "Show in Changes"
replaces "Show in Files". Rename and Delete are absent for the root row; folders inside it keep
the existing file menu.

```text
+-- Files ----------------------------+
| v .claude                           |
|   v worktrees                       |
|     > [repo] fix-login  (fix-login) |  <right-click> -> root menu (UI-01 items)
| > apps                              |
| > docs                              |
+-------------------------------------+
```

UI-03: Confirmation (desktop dialog; phone bottom drawer with full-width buttons).

```text
+-- Remove "kandev · fix-login" from this task? ------+
| The folder, its branch, and any pull requests stay. |
| Kandev stops showing it in Changes and Files.       |
|                              [Cancel] [Remove]      |
+-----------------------------------------------------+
```

UI-04: Phone, long-press on a root (Changes or Files) opens the root menu as a bottom sheet.

```text
+-- Changes --------------------+
| kandev                        |
|   M app.tsx                   |
| kandev · fix-login  (pressed) |
|   A login.go                  |
+-------------------------------+
| kandev · fix-login            |  <- bottom sheet, scrolls if needed
| Open terminal here            |
| Show in Files                 |
| Copy full path                |
| Copy relative path            |
| Copy branch name              |
| Push                          |
| Create PR                     |
| Remove from task...           |
+-------------------------------+
```

Open in editor is omitted on phone, where no external editor can be launched.

Maps to AC-004.1, AC-004.2, AC-004.5, AC-004.6, AC-004.7, AC-005.1, AC-005.2.

## Tests

| AC | Evidence |
| --- | --- |
| 001.1-001.5 | `internal/task/service/service_worktree_adoption_test.go`: `TestAdoptWorktree_Reported*`; `internal/mcp/handlers` `TestAddBranchToTask_WorktreePath*` |
| 002.1-002.5 | `internal/agentctl/server/process/workspace_worktree_observer_test.go`; `internal/task/service/worktree_observed_subscriber_test.go` |
| 003.1-003.6 | `TestAdoptWorktree_DetectedThenReported`, `TestAdoptWorktree_ReportedThenDetected`, `TestWorktreeObserved_RemovedFromDisk`, restart replay test in subscriber suite |
| 004.1-004.4 | Web unit tests for root labels and the files root model; E2E below |
| 004.5-004.7 | `root-context-menu-items.test.tsx` (items, disabled reasons, copy values), `file-context-menu.test.tsx` (root rows omit Delete/Rename, shortcuts blocked), keyboard and long-press tests; E2E below |
| 005.1-005.6 | `TestDetachTaskRepository*`; web dialog and menu tests; E2E below |
| 006.1-006.2 | `internal/sysprompt` `TestFormatKandevContext_WorktreeAdoption*` |

## E2E tests

- `apps/web/e2e/tests/task/agent-worktree-adoption.spec.ts` (`chromium`): mock agent creates a
  worktree without reporting; group appears; commit; right-click the group, copy relative path,
  Remove from task; worktree not re-adopted;
  mock agent reports it; group returns. AC-002.1, 003.3, 004.1, 004.3, 004.4, 005.2-005.4.
- `apps/web/e2e/tests/task/mobile-agent-worktree-adoption.spec.ts` (`mobile-chrome`): reported
  worktree visible, long-press opens the root sheet, remove, no horizontal overflow. AC-001.1, 004.4,
  004.7, 005.1.

## Work orders

- [x] [Task 01: Agent-reported worktree adoption](task-01-reported-adoption.md)
- [x] [Task 02: Automatic worktree detection](task-02-automatic-detection.md)
- [x] [Task 03: Root menu, detach, and Files root markers](task-03-detach-and-files-roots.md)
- [x] [Task 04: Prove adoption end to end](task-04-prove-end-to-end.md)

## Verification results

Implemented and verified:

- Backend unit and integration suites pass for `internal/task/service`, `internal/task/repository/sqlite`, `internal/task/handlers`, `internal/mcp/...`, `internal/agentctl/server/process`, `internal/agent/runtime/lifecycle`, `internal/agent/runtime/agentctl`, `internal/gateway/websocket`, `internal/terminal/...`, `internal/backendapp`, `internal/orchestrator`, `internal/sysprompt`, and `cmd/mock-agent`.
- `bash scripts/lint-go-changed` reports no issues on the changed Go files.
- `go run ./cmd/sqlguard ./internal` and `go test -race ./internal/persistence/storeconformance` pass.
- Web `pnpm run typecheck`, changed-file Oxlint with `--deny-warnings`, and `pnpm run i18n:check` pass.
- Playwright passes on `chromium` for `tests/task/agent-worktree-adoption.spec.ts` and on `mobile-chrome` for `tests/task/mobile-agent-worktree-adoption.spec.ts`.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` pass.
- Review remediation: adopted tracker identity is the canonical task-relative path on both sides; Push/Create PR are disabled when a non-primary root has no matching tracker; linked PRs match by branch; nested checkouts receive managed outer Git excludes; detection runs on a coalescing worker; detach is atomic and retains cleanup-visible inventory; reported adoption authorizes the task; row matching is exact; and `worktree_path` rejects additional repository locators.

## Risks

- **Rescan scope.** The design adds the adopted path to the rescan's source roots. Task 01 must
  confirm that `RescanRepositoriesWithSourceRoots` creates a tracker for a nested linked worktree
  under the backend repository name; if it does not, extend the tracker graph builder there rather
  than adding a second discovery path.
- **Duplicate key.** Two roots on the same branch name of one repository cannot coexist under the
  existing `task_repositories` unique key. Git already forbids the same branch checked out twice,
  so this should only affect stale rows; the service must fail with the typed duplicate error.
- **Primary identity.** "Primary" is the lowest position row today. Single-repository tasks rooted
  inside the repository must never offer removal of the checkout that holds the agent CWD.
- **Open terminal here.** agentctl `shell` sessions accept a working directory, but the user
  terminal API may not expose it yet; Task 03 confirms the path and adds the field if missing.
- **Observer cost.** Listing worktrees is gated by a stat of `<common_dir>/worktrees`; a repository
  with many worktrees owned by other tasks still pays the list cost on each change.
