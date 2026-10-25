---
id: "03-regression-documentation"
title: "Regression documentation"
status: done
wave: 3
depends_on: ["02-title-tool-integration"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/agent-generated-titles.md"
---

# Task 03: Regression documentation

## Acceptance

- Focused integration coverage proves default/custom generated-branch renaming, remote and Local
  preservation, mixed multi-repository outcomes, owner scoping, restart-safe snapshots, and failure
  semantics through the title-tool boundary.
- Public task and Git documentation explains the option gate, final-title rename, explicit remote
  checkout preservation, and surfaced Git/snapshot failure behavior without implying that remote
  branches move.
- The affected backend packages and public-document validators pass, with results recorded in this
  task and the parent plan.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/mcp/handlers ./internal/mcp/server ./internal/task/repository/sqlite ./internal/worktree -run 'Test.*(TaskTitleBranch|SetTaskTitle|Rename.*Branch|BranchName)')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- focused tests introduced or extended by Tasks 01–02
- `docs/public/automation-and-mcp.md`
- `docs/public/git-operations.md`
- `docs/public/coverage.json`, only if the existing ownership map requires an update
- `docs/plans/agent-title-branch-renaming/plan.md`
- `docs/plans/agent-title-branch-renaming/task-01-branch-rename-runtime.md`
- `docs/plans/agent-title-branch-renaming/task-02-title-tool-integration.md`
- `docs/plans/agent-title-branch-renaming/task-03-regression-documentation.md`

## Dependencies

Tasks 01–02 complete.

## Parallelism

Sequential in the primary conversation. The integration assertions and documentation must describe
the landed contract.

## Inputs

- All new branch scenarios in the spec.
- Public task-title guidance in `docs/public/automation-and-mcp.md` and managed-branch guidance in
  `docs/public/git-operations.md`.
- Exact results from Tasks 01–02.

## Risks

- Documentation must distinguish local branch rename from a remote branch mutation.
- Avoid browser E2E churn: the setting and dialog payload do not change, so backend integration is the
  correct boundary for this extension.

## Results

- Added regression coverage for final-title rendering, custom templates, remote checkout and Local
  preservation, mixed multi-repository outcomes, owner-only invocation, failed Git side effects, live
  snapshots, repeated same-repository worktrees, and repository-scoped branch-switch events.
- Updated `docs/public/automation-and-mcp.md` and `docs/public/git-operations.md` with the option gate,
  final-title rename behavior, remote/PR preservation, Local preservation, and surfaced failure
  semantics.
- `node --test scripts/validate-public-docs.test.mjs` — 58 passed.
- `node scripts/validate-public-docs.mjs` — validated 41 published docs pages.
- `git diff --check` — passed.
