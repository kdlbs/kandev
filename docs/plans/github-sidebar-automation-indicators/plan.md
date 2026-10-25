---
created: 2026-08-28
status: completed
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
legacy_specs: []
---
# Implementation Plan: GitHub Sidebar Automation Indicators

## Overview

Show active GitHub auto-fix and auto-merge settings directly on the task-row
pull-request icon. Extend the bounded task summary first so every visible row
has reliable state without an HTTP request. Then add the two corner indicators,
detailed desktop and touch disclosure, and end-to-end coverage.

## Scope

### In scope

- Aggregate active per-pull-request auto-fix and auto-merge state into the
  bounded task summary.
- Update the summary after an option change, pull-request lifecycle change,
  unlink, restart, or list hydration.
- Show a yellow top-left auto-fix dot and a purple top-right auto-merge dot on
  the task-row pull-request icon.
- Let both indicators appear together without obscuring the pull-request glyph
  or its status color.
- Identify enabled settings and their active pull requests through pointer,
  keyboard, and touch disclosure.
- Preserve the shared desktop sidebar and mobile task-switcher row behavior.

### Out of scope

- Editing automation settings from a task row.
- Showing automation indicators for merged or closed pull requests.
- Adding indicators to GitLab merge-request icons.
- Loading full automation options for every visible task row.
- Changing the colors or precedence of pull-request lifecycle status.

## Technical approach

### Bounded automation projection

Add `auto_fix_enabled` and `auto_merge_enabled` to
`statussummary.PullRequestSummary` and the frontend `TaskStatusSummary` type.
Add both switches to the internal keyed pull-request observation. Derive each
summary flag with an `any active PR enabled` rule and omit false values.

Extend `githubTaskStatusSummaryPRReader` to batch the linked pull requests and
their stored `TaskPRAutomationOptions`. The status-summary projector subscribes
to `GitHubTaskCIOptionsUpdated` and refreshes authoritative pull-request
observations for the affected task before publishing a replacement summary.
Boot hydration, restart rehydration, compare-and-set rebases, and PR lifecycle
events continue through the same loader and projector boundary.

Pipe the two bounded flags through the desktop and mobile task item models into
the pull-request icon. A task row does not mount the eager automation-options
hook and does not issue one request per task.

### Icon and disclosure

Keep the existing 14-pixel pull-request glyph and status color. Wrap the glyph
in a relative inline box. Render a small yellow dot at the top-left for
auto-fix and a small purple dot at the top-right for auto-merge. Give each dot
a surface-colored ring and a stable test ID. Extend the icon's accessible name
with localized automation state so color is supplementary.

Fine pointers and keyboard focus keep the existing tooltip. Its Automation
section uses lazily hydrated `TaskCIOptionsResponse.pr_options` to identify the
enabled settings for each active pull request. Coarse pointers use the existing
compact task-status drawer pattern. The mobile trigger has a 44-pixel touch
target, stops row navigation only for the disclosure gesture, returns focus on
dismissal, and keeps one internal scroll owner without horizontal overflow.

Extend the current tooltip hydration boundary to fetch pull-request details and
automation options together on first disclosure. Deduplicate concurrent loads
per workspace generation and task, discard stale workspace responses, and
reuse the existing GitHub store and matching helpers.

## Tests

- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.10`: status-summary model,
  projector, rebuild, adapter, mapper, and icon tests prove independent and
  combined flags, active-only aggregation, live updates, and restart hydration.
- `AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.11`: component and hydration tests
  prove per-pull-request details, accessible text, pointer/focus behavior, and
  touch disclosure.
- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.1-.3`: projector and task-row
  tests prove the state is bounded, rebuildable, delivered through task-summary
  updates, and does not add per-row session or automation subscriptions.

## E2E tests

- Desktop Chromium extends `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`.
  Seed independent and combined settings, reload the task list, assert both dot
  positions remain within the icon box, hover the icon, and verify the named
  per-pull-request automation details.
- Mobile Chrome adds
  `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`.
  Open the task-switcher drawer, assert the same dots, tap the touch-sized PR
  target, verify the automation drawer content, dismiss it, and assert no
  document-level horizontal overflow.
- Both projects prove merged and closed pull requests do not show active dots
  and an option update changes the row without opening the task.

## Work orders

- [x] [Task 01: Project sidebar automation state](task-01-project-sidebar-automation-state.md) — completed
- [x] [Task 02: Render responsive automation indicators](task-02-render-responsive-automation-indicators.md) — completed

## Verification results

Completed. Backend projection, frontend row mapping, responsive disclosures, and regression
coverage are implemented.

- `go test -tags fts5 ./internal/task/statussummary ./internal/backendapp ./internal/github` — passed (2,473 tests).
- `pnpm test -- components/github/pr-task-icon.automation.test.ts components/github/pr-task-icon.render.test.tsx components/github/pr-ci-automation-controls.test.tsx components/github/pr-ci-popover.automation.test.tsx components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet.test.tsx hooks/domains/github/use-task-pr-tooltip-hydration.test.tsx hooks/domains/github/use-task-ci-options.test.tsx lib/api/domains/github-api.test.ts lib/api/domains/github-auto-merge-api.test.ts` — passed (94 tests).
- `pnpm --filter @kandev/web lint` — passed.
- `pnpm run typecheck` — passed.
- `pnpm run i18n:check` — passed.
- Managed Chromium E2E for the sidebar automation scenario — passed.
- Managed Mobile Chrome E2E for the sidebar automation scenario — passed.

## Risks

- Joining per-pull-request options incorrectly can attach settings to the same
  PR number in another repository. Matching must retain repository identity.
- Parsing only the option-update event payload can lose sibling PR state. The
  projector must refresh the complete authoritative observation set.
- Mounting the existing eager options hook in every task row would create an
  unbounded request fan-out and violate the task-summary contract.
- A touch disclosure inside a navigable task row can trigger both actions. The
  icon gesture must stop row navigation without changing desktop row behavior.
- Tiny overlay dots can disappear against status colors without a contrasting
  ring or can obscure the glyph if positioned outside its local icon box.
