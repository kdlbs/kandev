---
id: "02-render-responsive-automation-indicators"
title: "Render responsive automation indicators"
status: completed
wave: 2
depends_on:
  - "01-project-sidebar-automation-state"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.11
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.1
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---
# Task 02: Render Responsive Automation Indicators

## Summary

Render the two corner dots on the task-row pull-request icon and expose the
enabled per-pull-request settings through the existing desktop and mobile
status disclosure patterns.

## In scope

- Add independent yellow auto-fix and purple auto-merge dots with a contrasting
  ring and stable selectors.
- Extend accessible icon text with enabled automation state.
- Lazily hydrate and deduplicate per-pull-request automation details when the
  icon disclosure opens.
- Extend the desktop tooltip for pointer and keyboard users.
- Add a touch-sized mobile trigger and compact drawer with the same information.
- Add localized copy in all supported catalogs.
- Add component, hydration, desktop E2E, and mobile E2E tests before production
  implementation.

## Out of scope

- Automation mutation controls in the tooltip or drawer.
- New task-row settings or presentation preferences.
- GitLab merge-request indicators.

## Acceptance

- The PR glyph remains readable with either dot or both dots, and each dot stays
  inside its local icon box.
- Pointer hover, keyboard focus, and touch activation expose the same enabled
  settings and active pull-request identities without eager row requests.
- Mobile preserves the task row's primary navigation, uses a 44-pixel trigger,
  returns focus after dismissal, and creates no document horizontal overflow.

## Verification

```bash
cd apps/web && pnpm test -- components/github/pr-task-icon.automation.test.tsx hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx
cd apps/web && pnpm run lint
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-status-badge.spec.ts -- --grep "sidebar automation indicators"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts
```

## Files likely touched

- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-status-summary.tsx`
- `apps/web/hooks/domains/github/use-task-pr-tooltip-hydration.ts`
- `apps/web/components/task/task-contribution-icons.tsx`
- `apps/web/components/task/task-item-leading-badges.tsx`
- `apps/web/src/locales/*/github.json`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`

## Dependencies

- Task 01 supplies the bounded automation flags to every task row.

## Risks

- The mobile icon interaction is nested inside a navigable row and must not
  activate both the disclosure and task navigation.
- Lazy PR and option responses must retain workspace-generation guards so a
  task switch cannot apply stale data.

## Parallelism

`sequential`

## Inputs

- Integration acceptance criteria 002.10 and 002.11.
- Task 01 bounded summary fields.
- Existing PR task tooltip, touch drawer, and automation matching helpers.

## Results

Implemented and verified. The PR icon renders independent yellow and purple
corner dots, exposes localized active-PR automation details through a lazy
desktop tooltip or touch drawer, and preserves row navigation and focus
behavior on mobile.

Verification: 94 focused web tests passed, lint/typecheck/i18n checks passed,
and the managed Chromium and Mobile Chrome sidebar E2E scenarios passed.
