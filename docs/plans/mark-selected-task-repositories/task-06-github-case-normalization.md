---
id: "06-github-case-normalization"
title: "Normalize GitHub repository case"
status: done
wave: 4
depends_on: ["05-remote-identity-review-fix"]
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 06: Normalize GitHub repository case

## Acceptance

- GitHub HTTPS and SSH identities normalize owner and repository segments case-insensitively, so case-variant URLs match the same provider picker repository.
- GitLab and Azure DevOps identity semantics remain unchanged.
- A focused case-variant regression fails before the production change and all Remote identity tests pass afterward.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-identity.test.ts components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/components/task-create-dialog-remote-repo-identity.ts`
- `apps/web/components/task-create-dialog-remote-repo-identity.test.ts`
- `docs/plans/mark-selected-task-repositories/task-06-github-case-normalization.md`

## Inputs

- Final code-review finding at `task-create-dialog-remote-repo-identity.ts` GitHub SSH/HTTPS identity construction.

## Dependencies

Task 05.

## Completion notes

- GitHub HTTPS and SSH identities now both use the same lowercasing helper for owner and repository segments; GitLab and Azure DevOps parsing is unchanged.
- RED: mixed-case GitHub HTTPS and SSH URLs produced `url:github:AcMe/SiTe` rather than the picker identity. GREEN: focused Remote tests pass 43 tests.
- The required lint command was run but exits on seven existing/concurrent warnings in forbidden row-level and workspace files; Task 06's owned identity module and test produced no lint warning.

## Output contract

Update only this task file's status, not `plan.md`. Report RED/GREEN evidence, files changed, exact tests/lint results, blockers, and risks. Follow `apps/web/AGENTS.md` and `/tdd`; do not spawn subagents or broaden scope.
