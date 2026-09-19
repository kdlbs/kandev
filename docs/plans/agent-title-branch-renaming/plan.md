---
spec: docs/specs/tasks/requirements/agent-generated-titles.md
created: 2026-08-04
status: complete
---

# Implementation Plan: Agent Title Branch Renaming

## Overview

Extend the existing owner-only `set_task_title_kandev` transaction with a per-repository
rename of branches Kandev generated from the provisional title. The runtime layer first gains a
repository-scoped rename operation and durable snapshot update; the MCP handler then invokes it only
after the title compare-and-set succeeds and returns explicit outcomes. Direct remote checkouts,
Local-executor branches, manual titles, and non-owner sessions remain unchanged.

Relevant decisions:
[agent-generated task titles](../../decisions/2026-07-31-agent-generated-task-titles.md),
[single-owner title handoff](../../decisions/2026-08-02-single-owner-agent-task-titles.md), and
[configurable worktree branch names](../../decisions/0032-configurable-worktree-branch-names.md).

---

## Backend

### Owner-session branch rename runtime

- Add an orchestrator contract and result types in
  `apps/backend/internal/orchestrator/task_title_branch.go` for
  `RenameGeneratedBranchesForTaskTitle(ctx, taskID, sessionID, title)`. Authorize the task/session pair
  before loading workspace state. The MCP handler may invoke it only after the task service has accepted
  that same session as the persisted title owner; the owner metadata is cleared by that acceptance.
- Load the owner session's `TaskSessionWorktree` rows, task-repository bindings, repository branch
  settings, task environment, and executor profile. Use the existing branch identity plans to match
  worktrees to task repositories in single- and multi-repository tasks.
- Treat a non-empty `TaskRepository.CheckoutBranch` as an explicit checkout and emit a
  `remote_checkout` preservation result without Git mutation. Emit `local_executor` for all Local
  executor branches. These checks happen before rendering or invoking agentctl.
- For each remaining worktree, call `worktree.RenderTaskBranchName` with the accepted title, task ID,
  ticket/issue metadata, repository template, and a deterministic executor-specific suffix. Do not
  special case templates without `{suffix}`; collision behavior remains the Git layer's responsibility.
- Add a narrow lifecycle operation in
  `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_branch.go` that resolves or restores
  the session execution, selects the repository subpath, and delegates to the existing agentctl
  `GitRenameBranch` call. Keep the operation repository-scoped for multi-repository workspaces.
- After each successful Git rename, update the matching `task_session_worktrees` row and the
  corresponding `TaskEnvironmentRepo.WorktreeBranch`. Update the legacy primary
  `TaskEnvironment.WorktreeBranch`, running-executor snapshot, and live execution metadata only when
  they describe that same primary repository. Add repository methods rather than relying on the
  Changes-panel callback, so completion is synchronous before the MCP response and survives restart.
- Return per-repository renamed, preserved, and failed outcomes. Continue after an individual Git
  or snapshot failure; never rename or delete a remote branch and never roll back a previously
  successful rename.

### Title tool integration

- Add a small `TaskTitleBranchRenamer` dependency to
  `apps/backend/internal/mcp/handlers/handlers.go`, wired to the orchestrator in
  `apps/backend/internal/backendapp/helpers.go`. Keep task-title ownership and compare-and-set logic in
  `task.Service.SetPendingAgentTitle` unchanged.
- In `handleSetTaskTitle`, call the branch renamer only when `SetPendingAgentTitle` returns
  `accepted: true`. The persisted task title and cleared pending state are authoritative before Git is
  attempted; a Git error must not undo them.
- Extend the success payload with `branch_rename.status` and repository-scoped `renamed`, `preserved`,
  and `failed` arrays. Preserve the existing `accepted: false` responses exactly, and return accepted
  title data with `partial` or `failed` branch status instead of turning a Git failure into an MCP
  transport error.
- Update the tool description in `apps/backend/internal/mcp/server/server.go` to state that accepted
  auto-titles also rename eligible Kandev-generated branches while preserving explicit remote
  checkouts. Do not expose the tool in any additional MCP mode.

### Public documentation

- Update `docs/public/automation-and-mcp.md` to explain that the default-on prompt-first option also
  renames the owner session's Kandev-generated branch after the agent chooses the final title, while an
  opt-out or manual title leaves branch behavior unchanged.
- Update `docs/public/git-operations.md` with the remote-change preservation rule, multi-repository
  behavior, and non-transactional failure outcome. Keep the existing manual **Edit branch** workflow
  documented separately.

---

## Tests

- **What:** final-title rendering uses default/custom templates and current task metadata, while an
  explicit checkout and Local executor are preserved before Git is called. **Files:**
  `apps/backend/internal/orchestrator/task_title_branch_test.go`,
  `apps/backend/internal/orchestrator/task_title_branch_service_test.go`, and existing worktree config
  tests.
  **How:** table-driven tests with fake lifecycle/repository dependencies, covering single repository,
  mixed multi-repository, custom template, direct GitHub PR checkout, and Local executor cases.
- **What:** the lifecycle operation resolves the correct execution and repository subpath and relays
  agentctl success/failure without touching another repository. **File:**
  `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_branch_test.go`. **How:** focused
  unit tests with fake execution and agentctl clients.
- **What:** successful Git renames synchronously update repository-scoped session worktrees,
  task-environment rows, the primary legacy snapshot, and applicable running/live metadata; failed
  renames leave old snapshots intact. **Files:** orchestrator and SQLite repository tests. **How:** real
  SQLite integration tests plus lifecycle fakes, followed by a reload/resume assertion.
- **What:** accepted title calls trigger branch renaming once and return `renamed`, `preserved`,
  `partial`, `failed`, or `not_applicable`; rejected/non-owner/repeated calls never invoke Git.
  **Files:** `apps/backend/internal/mcp/handlers/set_task_title_test.go` and MCP server catalog tests.
  **How:** handler-to-task-service integration with a real task repository and fake branch renamer.
- **What:** a branch failure after title persistence does not restore pending metadata or revert the
  accepted title. **File:** `apps/backend/internal/mcp/handlers/set_task_title_test.go`. **How:** fail
  the fake branch rename and reload the task after the handler response.
- **What:** only the persisted owner session is targeted; concurrent non-owner worktrees remain
  unchanged. **File:** `apps/backend/internal/orchestrator/task_title_branch_service_test.go`. **How:** seed two
  sessions and assert Git and snapshot mutations are scoped to the owner.

No new frontend or browser E2E test is planned. The existing option and creation payload are unchanged;
the new behavior begins after the task-bound MCP call and is covered at the backend integration layer.

## Verification Results

- Task 01 exact targeted backend check: 31 passed in 4 packages.
- Task 02 exact MCP/backendapp check: 12 passed in 3 packages.
- Affected-package backend check: 3469 passed in 6 packages.
- Focused race check: 11 passed in 3 packages.
- Backend lint: `make -C apps/backend lint` — 0 issues.
- Public-doc checks: 58 Node tests passed; 41 published docs pages validated; `git diff --check`
  passed.
- No generated artifacts or persistent external branches were created; HTTP agentctl calls are
  covered by local test servers and production renames remain scoped to the owner session's workspace.

### PR Fixup Remediation

- Addressed review findings for manual-branch preservation, synchronized lifecycle metadata, execution-ID
  compare-and-swap snapshots, repository-scoped branch-switch updates, nil-runtime handling, deterministic
  worktree suffixes, and surfaced snapshot failures.
- Follow-up review fixes add owner/non-owner session isolation and actual snapshot-write failure coverage,
  document the `switched_branch` preservation reason, make repeated-repository fixtures deterministic,
  and correct the recorded test paths and verification command scope.
- Remediation verification: targeted backend check — 32 passed in 6 packages; affected-package backend
  check — 3473 passed in 6 packages; focused race check — 12 passed in 3 packages; architecture lint,
  backend lint (0 issues), public-doc checks (58 tests and 41 pages), and `git diff --check` passed.

## Implementation Waves And Parallel Candidates

The default execution order is sequential in the primary conversation. These waves do not authorize
subagents.

Wave 1:
- [x] [Task 01: Branch rename runtime](task-01-branch-rename-runtime.md)

Wave 2:
- [x] [Task 02: Title tool integration](task-02-title-tool-integration.md)

Wave 3:
- [x] [Task 03: Regression documentation](task-03-regression-documentation.md)
