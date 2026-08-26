---
id: "04-task-create-policy-selector"
title: "Add policies to task creation"
status: complete
wave: 3
depends_on:
  - "02-task-policy-snapshot-runtime"
  - "03-repository-settings-policies"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-003
  - REQ-WORKSPACES-BRANCH-POLICIES-004
  - REQ-WORKSPACES-BRANCH-POLICIES-005
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-003.1
  - AC-WORKSPACES-BRANCH-POLICIES-003.2
  - AC-WORKSPACES-BRANCH-POLICIES-003.3
  - AC-WORKSPACES-BRANCH-POLICIES-003.4
  - AC-WORKSPACES-BRANCH-POLICIES-003.5
  - AC-WORKSPACES-BRANCH-POLICIES-003.6
  - AC-WORKSPACES-BRANCH-POLICIES-004.1
  - AC-WORKSPACES-BRANCH-POLICIES-004.4
  - AC-WORKSPACES-BRANCH-POLICIES-004.5
  - AC-WORKSPACES-BRANCH-POLICIES-005.2
  - AC-WORKSPACES-BRANCH-POLICIES-005.3
  - AC-WORKSPACES-BRANCH-POLICIES-005.4
system_design: "../../specs/workspaces/system-design/branch-policies.md"
---

# Task 04: Add policies to task creation

## Objective

Extend the current repository and branch chip with typed, visibly distinct
policy choices and submit the selected policy through the backend snapshot
contract.

## TDD contract

Write failing component/helper tests first for grouped options, tagged selection,
closed-chip rendering, missing-base disablement, local fork transition, raw
branch compatibility, stale-policy recovery, and the exact task-create payload.

## Implementation scope

- Replace branch-string-only draft state with a tagged raw-branch/policy
  selection without changing existing wire values for raw branches.
- Extend the existing `Pill` option contract or add the smallest branch-specific
  grouped wrapper needed to show policies before branches.
- Render a one-line localized `Policy` marker and policy name with hover, focus,
  and touch details for base, template preview, target, and availability; keep
  the selected chip visibly policy-specific.
- On local executors, policy selection enables the visible `Fork a new branch`
  mode and preserves dirty-tree consent.
- Exclude policies from unsaved path, Remote URL, Quick Chat, Add Sources, and
  Add Branch flows.
- Submit `branch_policy_id`, handle stale-policy responses without closing the
  dialog, and refresh policy options.
- Use the snapshotted pull-request target as the initial desktop and mobile PR
  base while preserving user override and legacy fallback.
- Add localized copy and accessible grouped-selector semantics.

## Implementation acceptance

- One selector presents tagged policies before raw branches and never derives
  policy identity from visible text.
- Policy selection visibly enters fresh-branch mode and submits only the policy
  ID as new policy configuration.
- Raw branches and all named exclusions retain their current behavior.

## Exclusions

- Quick Chat, Remote URL, Add Sources, and post-creation Add Branch.
- Persisting a last-used policy.
- New policy management controls inside the task dialog.

## Files likely touched

- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`
- `apps/web/components/task-create-dialog-pill.tsx`
- `apps/web/components/task-create-dialog-branch-options.tsx`
- `apps/web/components/task-create-dialog-helpers.ts`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task/`
- `apps/web/components/session-mobile-top-bar-git-controls.tsx`
- `apps/web/lib/api/`
- `apps/web/src/locales/`

## Verification

- `cd apps/web && pnpm exec vitest run components/task-create-dialog-branch-policies.test.tsx components/task-create-dialog-helpers.multi-repo.test.ts components/task-create-dialog-submit.test.tsx`
- `cd apps/web && pnpm exec vitest run components/task/pull-request-target.test.tsx components/session-mobile-top-bar-git-controls.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check`

## Dependencies and parallelism

Depends on Tasks 02 and 03. Sequential: it integrates their snapshot contract
and client state into the shared task-create flow.

## Output contract

Report the RED tests, tagged-state shape, grouped selector behavior, local fork
transition, exclusions, stale recovery, desktop/mobile PR target behavior,
exact verification results, changed files, and risks. Mark this task and the
plan checkbox complete together.

## Results

Implemented grouped policy-before-branch selection, localized policy markers
and summaries, unavailable-base handling, policy-ID payloads, local fresh-branch
transition, named-flow exclusions, stale-option recovery, and snapshotted
desktop/mobile pull-request targets. Raw branch behavior remains unchanged.

Verification:

- Focused frontend Vitest suite passed: 78 tests in 5 files.
- `pnpm run typecheck`, `pnpm run lint`, and `pnpm run i18n:check` passed.
- Desktop and mobile task-creation Playwright scenarios passed.

Review remediation verification:

- Unstarted-task edits keep repository snapshots unless repository rows are
  explicitly changed, including an explicit delete.
- The policy selector keeps policies before raw branches within the combined
  control and previews base, template, and pull-request target.
- Deferred-response tests cover stale create, update, and Gitflow responses;
  stale task submissions refresh policy options without closing the dialog.
- Local-executor policy subtasks submit the fresh-branch contract and are
  covered on desktop and mobile.
- The focused regression suite passed: 80 tests in 7 files. Web typecheck,
  lint, and i18n checks passed.
