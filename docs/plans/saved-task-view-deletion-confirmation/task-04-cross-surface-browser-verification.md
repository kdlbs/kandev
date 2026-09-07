---
id: "04-cross-surface-browser-verification"
title: "Cross-surface browser verification"
status: done
wave: 3
depends_on:
  - "02-task-saved-view-surfaces"
  - "03-integration-saved-query-surfaces"
plan: "plan.md"
requirements:
  - REQ-UI-SAVED-TASK-VIEW-DELETION-001
acceptance_criteria:
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.1
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.2
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.3
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.4
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.5
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.6
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.7
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.8
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.9
  - AC-UI-SAVED-TASK-VIEW-DELETION-001.10
system_design:
  - ../../specs/ui/system-design/saved-task-view-deletion-confirmation.md
---

# Task 04: Cross-Surface Browser Verification

## Summary

Exercise the real desktop Radix nesting and every distinct mobile composition
after the shared shell and all adapters are integrated. Prove cancellation is a
no-op, explicit confirmation preserves existing deletion outcomes, and phone
surfaces remain touch reachable and contained.

## In scope

- Update the sidebar filter page object so tests can begin, cancel, and confirm
  deletion explicitly.
- Extend desktop task-sidebar, Threads, GitHub, GitLab, Azure DevOps, and Jira
  journeys with named confirmation and no-mutation-before-confirm assertions.
- Extend mobile task-sidebar, Threads, GitHub, GitLab, and Azure DevOps journeys
  and add a Jira saved-view journey.
- Assert 44px controls, parent surface retention, one overlay/scroll owner,
  safe-area reachability, focus/selection isolation, long-name containment, and
  zero document-level horizontal overflow.
- Record exact focused runner results in this work order and the plan.

## Out of scope

- New production behavior, general responsive redesign, or broad E2E cleanup.
- Full-suite verification or unrelated provider journeys.

## Acceptance

- Every listed desktop and mobile surface proves that trigger and cancellation
  do not delete, while final confirmation invokes the existing outcome once.
- Desktop tests prove anchored confirmation, parent-overlay retention, target
  naming, focus/event isolation, and nondeletable guards.
- Mobile tests prove in-place confirmation with visible 44px actions, no nested
  drawer/modal, one scroll owner, safe-area reachability, and zero overflow.

## Verification

```bash
cd apps/web
pnpm e2e:run --project chromium \
  tests/task/sidebar-filter.spec.ts \
  tests/task/threads-view.spec.ts \
  tests/github/github-scope-bar.spec.ts \
  tests/gitlab/gitlab-issue-milestone-filter.spec.ts \
  tests/integrations/azure-devops.spec.ts \
  tests/integrations/jira-status-filter.spec.ts
pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-sidebar-views.spec.ts \
  tests/task/mobile-threads-view.spec.ts \
  tests/github/mobile-github-sidebar.spec.ts \
  tests/gitlab/mobile-gitlab-issue-milestone-filter.spec.ts \
  tests/integrations/mobile-azure-devops.spec.ts \
  tests/integrations/mobile-jira-saved-view.spec.ts
```

## Files likely touched

- `apps/web/e2e/pages/sidebar-filter-popover.ts`
- `apps/web/e2e/tests/task/sidebar-filter.spec.ts`
- `apps/web/e2e/tests/task/threads-view.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`
- `apps/web/e2e/tests/task/mobile-threads-view.spec.ts`
- `apps/web/e2e/tests/github/github-scope-bar.spec.ts`
- `apps/web/e2e/tests/github/mobile-github-sidebar.spec.ts`
- `apps/web/e2e/tests/gitlab/gitlab-issue-milestone-filter.spec.ts`
- `apps/web/e2e/tests/gitlab/mobile-gitlab-issue-milestone-filter.spec.ts`
- `apps/web/e2e/tests/integrations/azure-devops.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-azure-devops.spec.ts`
- `apps/web/e2e/tests/integrations/jira-status-filter.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-jira-saved-view.spec.ts`

## Dependencies

Tasks 02 and 03, including their focused component tests.

## Risks

- Focused files still share server and seed state; scenarios must use existing
  fixture isolation and must not assume execution order.
- Mobile pages can keep hidden desktop variants mounted, so geometry and action
  locators must be scoped to the visible parent surface.
- The managed E2E runner must run sequentially by project to respect the
  repository's memory and worker guardrails.

## Parallelism

`sequential`

## Inputs

- All requirement acceptance criteria and the system design's Observability and
  verification section.
- Existing E2E fixture-state, cleanup, responsive geometry, and page-object
  conventions.
- Completed unit-tested adapters from Tasks 02 and 03.

## Results

- Updated the task-sidebar page object and all six desktop plus six mobile
  surface journeys to separate trigger, cancellation, and final confirmation.
- The exact desktop command passed 44/44 on `chromium`, covering anchored
  confirmation and each existing deletion outcome.
- The exact mobile command passed 24/24 on `mobile-chrome`, covering inline
  confirmation, touch geometry, overlay containment, long names, and overflow.
- Captured and inspected fresh desktop and phone confirmation screenshots from
  the task-sidebar journeys, then compressed both assets for PR publication.
