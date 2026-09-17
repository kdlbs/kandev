---
id: "02-coordinator-page"
title: "Central Coordinator page"
status: done
wave: 2
depends_on:
  - "01-task-observations"
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-001
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-002
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-003
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-004
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-005
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-006
acceptance_criteria:
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.4
  - AC-ORCHESTRATION-COORDINATOR-VIEW-002.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-003.4
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-005.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-005.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-006.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-006.3
system_design:
  - ../../specs/orchestration/system-design/coordinator-view.md
---

# Task 02: Central Coordinator page

## Summary

Expose the workspace task observations beside the existing persistent chat.
Preserve assignment identity and provide the same actions through mobile tabs.

## In scope

- New workspace Coordinator route, gated navigation, grouped rows/counts,
  coverage labels, filters, task/PR links and explicit setup/error states.
- Reusable conversation content, existing transports/streaming/recovery, assignment
  selector, hide/show chat and workspace/assignment-scoped draft preservation.
- Mobile Tasks/Chat tabs, keyboard navigation, focus stability, accessible labels
  and translations using the current Orchestration namespace.

## Out of scope

New chat/runtime ownership, new permissions, automatic input resolution, new
schedulers, phase/dependency editors or replacing native task controls.

## Acceptance

- Desktop shows task groups plus the selected coordinator chat; existing
  conversation URLs, configuration and native task links continue to work.
- Opening/selecting/filtering creates no delivery task or run; drafts and chat
  content never transfer to another workspace/assignment, and failed setup has
  an actionable explanation.
- At 390-pixel width all essential actions remain reachable through accessible
  Tasks/Chat controls; disabled features and private ownership stay enforced.

## Verification

From repository root (dependencies installed in task 01):

```bash
pnpm --dir apps/web exec vitest run app/coordinator/coordinator-page.test.tsx src/spa-routing.test.ts components/app-sidebar/app-sidebar-primary-nav.test.tsx components/task/simple/task-chat-identity.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec eslint app/coordinator app/settings/orchestration components/app-sidebar/orchestration-nav.tsx
git diff --check
```

Browser proof is owned by task 03 and must pass before this package is complete.

## Files likely touched

- `apps/web/app/coordinator/coordinator-page.tsx` and focused child components/tests (new).
- `apps/web/app/settings/orchestration/conversation-route.tsx`, `conversation-pane.tsx`.
- `apps/web/hooks/domains/orchestration/use-orchestrator-conversation.ts`.
- `apps/web/src/spa-routes.tsx`, `spa-routing.ts` and routing tests.
- `apps/web/components/app-sidebar/orchestration-nav.tsx`, mobile navigation and
  workspace section links; existing tests for changed entry points.
- `apps/web/src/locales/en/orchestration.json` and generated/required locale parity files.

## Dependencies

Task 01's scoped observations and tested grouping contract.

## Risks

Extracting shared chat can introduce duplicate scroll owners, lose drafts or
break stream/recovery identity. Preserve one conversation data owner and use
the existing rendering/transport contracts.

## Parallelism

`sequential`

## Inputs

- System design: Conversation and navigation; Presentation; Authority and privacy.
- Existing conversation route/pane, `TaskChat` and workspace/mobile navigation.
- Read `mobile-parity`, `e2e` and scoped frontend guidance before implementation.

## Results

Completed on 2026-09-17 in the private v0.94.0 integration branch. The new route
joins canonical workspace tasks with the existing conversation renderer. It adds
scope/group/workflow/repository/search filters, known coverage totals, paging,
owner/workspace isolation, profile diagnostics and remembered coordinator choice.
Drafts stay separate by conversation; mobile has touch tabs, collapsed filters,
44-pixel controls and a first task visible within the initial viewport.

Observed regressions before fixes: new route fell through to Kanban; switching
conversations dropped a draft; changing owners retained a previous result; an
idle summary retained a stale running badge; mobile filters/empty groups pushed
the first task below the viewport. Each regression now has a passing focused test.
The first expanded browser run failed in fixture cleanup; using the existing
confirmed-name workspace deletion helper repaired it without weakening assertions.

Final checks:
- Frontend focused suite: 7 files, 41 tests. Coverage includes canonical grouping,
  paging/refresh races, scoping/owner changes, draft identity and route matching.
- Full web typecheck, targeted ESLint with zero warnings, complete real-locale and
  pseudo checks passed. Imported Office/orchestration translation gaps were filled.
- Managed fresh build, then all three desktop orchestration flows together:
  3 passed, 1 worker, 0 retries (50.9 seconds). The new flow covers a native
  clarification summary arriving while the page is open, review/done/queued
  groups, server search, mobile tab switching, no execution from observation,
  generic message/reply, separate drafts for two coordinators, selected-task
  filtering and a second workspace rejecting the first workspace's selection.
- Pixel 5 mobile project: 1 passed, 1 worker, 0 retries (4.7 seconds); real touch
  actions, hit-target/44-pixel geometry, first-row viewport fit, draft retention,
  no horizontal overflow and native task navigation.
- Public docs validation: 48 pages; validator tests passed (61 tests).
- Specification lint and whitespace checks passed.

[Media provenance and artifacts](../../review/orchestration/media/coordinator-view/README.md)
include four genuine screenshots and a 1.4-second silent VP9 typing preview. All
seven video frames and all screenshots were reviewed; only disposable fixture
records and generic example requests are visible. No real prompts, credentials,
production data, public publication or live deployment were involved.

Public docs were updated as a how-to guide and API reference. The
[focused PR draft](../../review/orchestration/coordinator-pr-draft.md) keeps the
broader assistant integration out of its proposed contribution. This completes
the central-view work package; it does not complete the remaining assistant,
PostgreSQL qualification, deployment or upstream-extraction work orders.
