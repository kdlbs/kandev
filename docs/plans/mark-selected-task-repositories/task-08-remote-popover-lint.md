---
id: "08-remote-popover-lint"
title: "Reduce Remote popover function size"
status: done
wave: 6
depends_on: ["07-lint-cleanup"]
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 08: Reduce Remote popover function size

## Acceptance

- `RemoteRepoPopoverContent` complies with the configured 100-line function limit after normal formatting through the smallest cohesive extraction.
- Repository search, URL validation/paste/blur/Enter behavior, provider tabs, option markers, and mobile rendering remain unchanged.
- Focused Remote tests and full web lint pass with zero warnings/errors.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-identity.test.ts components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`
- `make fmt`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `docs/plans/mark-selected-task-repositories/task-08-remote-popover-lint.md`

## Inputs

- Final verifier lint failure: `RemoteRepoPopoverContent` is 102 lines after formatting, with a 100-line maximum.

## Dependencies

Task 07.

## Completion notes

- Extracted provider-tab selection and visible-repository derivation into `visibleProviderRepositories`, leaving search, validation, URL commit event handling, markers, and rendering in the popover component.
- Focused Remote tests pass 43 tests. `make fmt` and full web lint both complete successfully with zero warnings.

## Output contract

Update only this task file's status, not `plan.md`. Report the extraction, files changed, exact format/test/lint results, blockers, and risks. Follow `apps/web/AGENTS.md`; do not spawn subagents or broaden scope.
