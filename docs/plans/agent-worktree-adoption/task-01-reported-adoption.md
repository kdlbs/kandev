---
id: "01-reported-adoption"
title: "Agent-reported worktree adoption"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-001
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-003
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-004
  - REQ-TASKS-AGENT-WORKTREE-ADOPTION-006
acceptance_criteria:
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-001.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-001.2
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-001.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-001.4
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-001.5
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.4
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-003.6
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-004.3
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-006.1
  - AC-TASKS-AGENT-WORKTREE-ADOPTION-006.2
system_design:
  - ../../specs/tasks/system-design/agent-worktree-adoption.md
---

# Task 01: Agent-reported worktree adoption

## Summary

An agent that calls `add_branch_to_task_kandev` with `worktree_path` gets its existing worktree
attached as a task branch in place. A tracker is created for it, and its changes appear as a
separate Changes group with the existing per-root actions. The first-turn context tells the agent
to do this.

## In scope

- `worktree_origin` column migration with SQLite replay and gated PostgreSQL tests.
- `Service.AdoptWorktree` (reported source), candidate validation through the runtime, the
  adoption materializer, and the post-commit rescan.
- `worktree_path` on the MCP tool and handler; response fields `adopted` and `already_attached`.
- Changes group label `<repository> · <branch>` for non-primary roots.
- `KandevContextOptions.IncludeWorktreeAdoption` and `{worktree_adoption_section}`, set for host
  Worktree and Local executors.

## Out of scope

- Detection, detach, and the detachment table (Tasks 02 and 03).
- Files panel markers (Task 03).

## Acceptance

- A reported in-root worktree on a named branch becomes a task branch with an unchanged directory,
  branch and `HEAD`, and its `git_status` reaches the web under the new repository name.
- Out-of-root, non-worktree, foreign-repository, detached-`HEAD` and mismatched-`checkout_branch`
  reports fail with no rows written; omitting `worktree_path` keeps existing tests green.
- The guidance appears only for host Worktree and Local executors.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/task/service/ -run 'TestAdoptWorktree' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite/ -run 'TestWorktreeOrigin' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/... -run 'TestAddBranchToTask' -count=1)
(cd apps/backend && go test -trimpath ./internal/sysprompt/ -run 'TestFormatKandevContext' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process/ -run 'TestRescan' -count=1)
(cd apps/web && pnpm exec vitest run components/task/changes-panel-repo-groups)
bash scripts/lint-go-changed $(git ls-files -mo --exclude-standard -- 'apps/backend/*.go')
(cd apps/web && pnpm run i18n:check)
```

## Files likely touched

- `apps/backend/internal/task/service/service_worktree_adoption.go` (new) and its test
- `apps/backend/internal/task/service/service_branches.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`, `task_environment.go`
- `apps/backend/internal/worktree/worktree.go` (origin field)
- `apps/backend/internal/mcp/server/server.go`, `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/sysprompt/sysprompt.go`, `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/internal/agentctl/server/process/manager_rescan.go` (only if the rescan risk holds)
- `apps/web/components/task/changes-panel-repo-groups.tsx`, `apps/web/src/locales/*/task.json`

## Dependencies

None.

## Risks

- The rescan risk in [plan.md](plan.md#risks); confirm with a nested linked worktree test first.

## Parallelism

`sequential`

## Inputs

- Design sections: Adoption service, MCP contract, Persistence, Agent guidance, Frontend.
- Patterns: `Service.AddBranchToTask`, `commitWorkspaceSourceBatch`, `RescanWorkspaceForSession`.

## Results

Implemented `AdoptWorktree` with canonical path containment, linked-worktree and repository identity validation, in-place inventory persistence with `worktree_origin = adopted`, MCP `worktree_path`, post-commit rescan, `worktree_origin` migration, and executor-gated first-turn guidance. Covered by `service_worktree_adoption_test.go`, MCP forwarding tests, SQLite origin/migration tests, and sysprompt tests.
