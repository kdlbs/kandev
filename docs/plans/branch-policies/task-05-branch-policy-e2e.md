---
id: "05-branch-policy-e2e"
title: "Cover branch policies in desktop and mobile E2E"
status: complete
wave: 4
depends_on:
  - "04-task-create-policy-selector"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-001
  - REQ-WORKSPACES-BRANCH-POLICIES-002
  - REQ-WORKSPACES-BRANCH-POLICIES-003
  - REQ-WORKSPACES-BRANCH-POLICIES-004
  - REQ-WORKSPACES-BRANCH-POLICIES-005
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-001.1
  - AC-WORKSPACES-BRANCH-POLICIES-001.2
  - AC-WORKSPACES-BRANCH-POLICIES-001.3
  - AC-WORKSPACES-BRANCH-POLICIES-001.6
  - AC-WORKSPACES-BRANCH-POLICIES-002.1
  - AC-WORKSPACES-BRANCH-POLICIES-002.3
  - AC-WORKSPACES-BRANCH-POLICIES-002.4
  - AC-WORKSPACES-BRANCH-POLICIES-003.1
  - AC-WORKSPACES-BRANCH-POLICIES-003.2
  - AC-WORKSPACES-BRANCH-POLICIES-003.3
  - AC-WORKSPACES-BRANCH-POLICIES-003.4
  - AC-WORKSPACES-BRANCH-POLICIES-004.2
  - AC-WORKSPACES-BRANCH-POLICIES-004.4
  - AC-WORKSPACES-BRANCH-POLICIES-005.1
  - AC-WORKSPACES-BRANCH-POLICIES-005.2
  - AC-WORKSPACES-BRANCH-POLICIES-005.3
system_design: "../../specs/workspaces/system-design/branch-policies.md"
---

# Task 05: Cover branch policies in desktop and mobile E2E

## Objective

Prove the user-visible policy workflow on desktop and phone with focused,
deterministic Playwright scenarios.

## E2E scenarios

- Desktop repository settings start collapsed, show count/help when expanded,
  list and filter local/remote branch choices with refresh, seed Gitflow
  policies, and persist an edited policy after reload.
- Desktop task creation groups policies before branches, keeps each policy on
  one line with hover/focus details, enables fresh-branch mode, and creates a
  task using the expected base/template/PR target.
- Phone repository settings use tap-accessible help and a full-height drawer
  whose fields and safe-area action remain reachable at a compact viewport.
- Phone task creation shows the one-line policy marker and tap details without
  horizontal overflow, then retains the selected policy state through task
  submission.

Use API seeding for prerequisites that are not the behavior under test. Use the
mock backend/provider contracts and existing stable accessibility selectors.

## Implementation acceptance

- Desktop scenarios prove settings persistence and policy-backed task creation.
- Phone scenarios prove drawer/help reachability and selector layout without
  overflow.
- Each scenario asserts user-observable behavior and avoids timing-only waits.

## Exclusions

- Broad regression-suite execution.
- External Git provider credentials or real remote pull requests.
- Re-testing backend constraints that focused service tests already own.

## Files likely touched

- `apps/web/e2e/tests/settings/workspace-repository-branch-policies.spec.ts`
- `apps/web/e2e/tests/settings/mobile-workspace-repository-branch-policies.spec.ts`
- `apps/web/e2e/tests/task/task-create-branch-policies.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-create-branch-policies.spec.ts`
- `apps/web/e2e/helpers/` only if shared policy seeding removes duplication

## Verification

- `cd apps/web && pnpm e2e:run tests/settings/workspace-repository-branch-policies.spec.ts`
- `cd apps/web && pnpm e2e:run tests/task/task-create-branch-policies.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-workspace-repository-branch-policies.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-create-branch-policies.spec.ts`

The feature-specific scenarios must fail against the pre-feature application
and pass after Tasks 01 through 04.

## Dependencies and parallelism

Depends on Task 04. Parallel-safe with Task 06 after that dependency because
this task owns only E2E files and narrowly necessary E2E helpers.

## Output contract

Report scenario evidence, desktop and phone viewport behavior, exact Playwright
results, failure artifact paths, changed files, and unresolved risk. Mark this
task and the plan checkbox complete together.

## Results

Added deterministic desktop and mobile settings/task scenarios. They cover
collapsed settings, Gitflow CRUD, policy grouping and markers, local fresh
branch selection, task snapshot persistence, drawer help access, and compact
phone layout.

Verification:

- Desktop settings and task scenarios passed: 2 tests.
- Mobile settings scenario passed.
- Mobile task scenario passed.
- The E2E fixture plugin package was built before browser validation.

Review remediation verification:

- Desktop and mobile local-executor subtask scenarios passed with policy
  selection, fresh-branch submission, immutable snapshot fields, and the
  generated branch identity asserted.
- Desktop keyboard-focus help and mobile tap-to-open help scenarios passed.
- The targeted desktop and mobile settings and subtask Playwright runs passed
  after the backend fresh-branch persistence fix.
