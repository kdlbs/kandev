---
id: "03-remote-identity-remediation"
title: "Canonicalize Remote repository identity"
status: done
wave: 2
depends_on: ["02-remote-repository-marker"]
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 03: Canonicalize Remote repository identity

## Acceptance

- Supported HTTPS and SSH forms of the same GitHub, GitLab, or Azure DevOps repository derive the same fallback identity as the provider picker option, including GitHub issue/PR/tree/blob URLs collapsing to the repository root.
- Provider-qualified IDs do not collide across providers; current-row exclusion and live clearing after another row changes or is removed are covered by row-level regression tests.
- Marked Remote options remain selectable and all existing Remote marker/mobile tests stay green.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts`

## Files likely touched

- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip.test.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chips.test.tsx`
- `docs/plans/mark-selected-task-repositories/task-03-remote-identity-remediation.md`

## Inputs

- QA report: SSH and subresource URL fallback mismatch at `normalizeRemoteRepositoryURL`.
- Code-review blockers 1 and 2.
- Existing URL parsing patterns in `apps/web/lib/utils/github-repo-url.ts` and `apps/web/hooks/domains/github/use-branches-by-url.ts`.

## Dependencies

Task 02.

## Completion notes

- Canonical fallback identities now normalize GitHub, GitLab, and Azure DevOps HTTPS/SSH repository forms. GitHub issue, pull-request, tree, and blob links resolve to their repository root.
- Provider-qualified picker IDs remain the primary identity and cannot collide across providers. Row-level coverage proves current-row exclusion plus immediate identity clearing when a sibling changes or is removed.
- RED: the focused Remote unit command failed on six new SSH/subresource equivalence cases before the normalization change. GREEN: the same suite passes 40 tests; the assigned `mobile-chrome` Remote E2E passes all 3 scenarios.

## Output contract

Update only this task file's status, not `plan.md`. Report RED/GREEN evidence, files changed, exact tests and E2E results, blockers, and risks. Follow `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and `/mobile-parity`; do not spawn subagents or broaden scope.
