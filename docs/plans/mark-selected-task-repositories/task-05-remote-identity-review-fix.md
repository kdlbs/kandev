---
id: "05-remote-identity-review-fix"
title: "Close Remote identity review gaps"
status: done
wave: 3
depends_on: ["03-remote-identity-remediation"]
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 05: Close Remote identity review gaps

## Acceptance

- Accepted `www.github.com` repository and subresource URLs canonicalize to the same GitHub repository identity as picker options.
- The provider-collision regression uses an identical raw provider ID for two different providers and proves the option is not marked across providers.
- Pure Remote repository identity logic and its equivalence matrix live in a focused utility/test module, keeping `task-create-dialog-remote-repo-chip.test.tsx` within the configured 600-line limit while preserving rendered marker coverage.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-identity.test.ts components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/components/task-create-dialog-remote-repo-identity.ts`
- `apps/web/components/task-create-dialog-remote-repo-identity.test.ts`
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip.test.tsx`
- `docs/plans/mark-selected-task-repositories/task-05-remote-identity-review-fix.md`

## Inputs

- Second-round code-review blockers for `www.github.com` canonicalization and identical-ID provider collision.
- Code-review max-lines suggestion for `task-create-dialog-remote-repo-chip.test.tsx`.

## Dependencies

Task 03.

## Completion notes

- Added `www.github.com` handling to the pure Remote repository identity helper and moved the HTTPS/SSH/subresource equivalence matrix out of the component test.
- The provider-collision regression now uses the same raw ID (`acme/site`) for GitHub and GitLab and proves the GitLab option remains unmarked.
- RED: `www.github.com` tree identity did not equal the GitHub picker identity. GREEN: focused unit coverage passes 41 tests and web lint passes with zero warnings.

## Output contract

Update only this task file's status, not `plan.md`. Report RED/GREEN evidence, files changed, exact tests/lint results, blockers, and risks. Follow `apps/web/AGENTS.md` and `/tdd`; do not spawn subagents or broaden scope.
