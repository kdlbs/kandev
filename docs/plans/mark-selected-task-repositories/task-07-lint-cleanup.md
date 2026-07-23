---
id: "07-lint-cleanup"
title: "Clear repository marker lint warnings"
status: done
wave: 5
depends_on: ["04-workspace-coverage-remediation", "06-github-case-normalization"]
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 07: Clear repository marker lint warnings

## Acceptance

- `RepoChip` and the changed repository-marker test callbacks comply with the configured 100-line function limit through cohesive extraction/splitting without changing behavior or assertions.
- All task-owned duplicate-literal warnings are removed with local constants or clearer test helpers, without speculative abstraction.
- Focused repository-marker tests and the complete web lint command pass with zero warnings/errors.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-identity.test.ts components/task-create-dialog-repo-chips.test.tsx components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`
- `apps/web/components/task-create-dialog-repo-chips.test.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chips.test.tsx`
- `docs/plans/mark-selected-task-repositories/task-07-lint-cleanup.md`

## Inputs

- Final code-review lint finding: `RepoChip` at 101 lines and changed test callbacks at 169 and 144 lines, plus four duplicate-literal warnings in task-owned files.

## Dependencies

Tasks 04 and 06.

## Output contract

Update only this task file's status, not `plan.md`. Report files changed, exact lint warnings resolved, focused test/lint results, blockers, and risks. Follow `apps/web/AGENTS.md`; do not spawn subagents or broaden scope.
