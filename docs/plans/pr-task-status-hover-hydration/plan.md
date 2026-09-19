---
created: 2026-08-26
status: complete
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
legacy_specs: []
---

# Implementation Plan: PR Task Status Hover Hydration

## Overview

Make every GitHub task PR indicator disclose details consistently. First add a
task-scoped hydration path for compact indicators. Then add the author identity
to the shared summary and the existing mobile drawer.

This order makes the data available before the presentation and end-to-end
work depends on it.

## Scope

### In scope

- Open a tooltip for compact GitHub PR indicators on mouse hover and keyboard
  focus.
- Load persisted full PR records for only the disclosed task.
- Deduplicate concurrent loads, reuse store data, guard HTTP and WebSocket
  races, and permit retry after an error.
- Preserve workspace ownership for live TaskPR updates. The browser must reject
  missing or foreign workspace IDs, and typed backend events must route by their
  owning workspace without falling back to a global broadcast.
- Show the GitHub author login in the task summary.
- Show the same author login in the existing coarse-pointer PR-status drawer.
- Add focused component, desktop Playwright, and mobile Playwright coverage.

### Out of scope

- Upstream GitHub refreshes from the task-row indicator.
- Workspace-wide PR loading for the sidebar.
- Changes to task-summary persistence or projection fields.
- New task-row touch targets, drawers, navigation, or provider associations.
- Author support for GitLab or registered provider summaries.

## Technical approach

### Task-scoped disclosure hydration

Add a hook under `hooks/domains/github` that uses `listTaskPRs([taskId])` when
`taskPRs.byTaskId[taskId]` is empty for the active workspace context. Scope the
in-flight registry to the Zustand store and key it by workspace, workspace
context generation, and task. Preserve deletion tombstones so a late response
cannot restore an association removed while it was pending.

Update `PRTaskIcon` and `TaskContributionIcons` so compact and full data use one
tooltip trigger. Compact content shows localized loading or unavailable text.
Successful data updates replace the content without unmounting the trigger or
closing the tooltip.

Before response application, compare the active workspace and current store
records. Preserve matching records that arrived from WebSocket events. Add only
missing PR identities from the HTTP result.

### Workspace-scoped live PR updates

Treat `TaskPR.workspace_id` as the authoritative owner of a live update. The
frontend handler applies an update only when that ID is present and matches the
active workspace, preserving the current scoped cache for missing or foreign
payloads. The backend exposes `TaskPR.WorkspaceID` to the notification
broadcaster and routes typed PR updates and detachments through the fail-closed
workspace path when authentication is enforced, so an unattributed event is
dropped instead of broadcast globally.

### Author presentation

Add optional author data to `ChangeRequestTaskStatusSummaryData`. Derive the
value from `TaskPR.author_login` and show localized `task:byAuthor` copy below
the title.

Extend `ChangeRequestPopoverHeader` with an optional author value. Pass the
GitHub author from `PRCIPopover` so desktop status content and the existing
mobile drawer use the same text.

### Localization

Add localized loading and unavailable copy to the GitHub namespace in English,
Portuguese, Simplified Chinese, Hong Kong Chinese, and Taiwan Chinese. Reuse
the existing localized `task:byAuthor` key for author presentation.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-UI-PR-TASK-STATUS-SUMMARY-001.13` to `.16` | A new hydration-hook test covers task scope, one in-flight request, cache reuse, workspace and task-context changes, deletion during an HTTP request, empty/error retry, and HTTP/WS ordering. Render tests cover mouse and keyboard disclosure states, including focus continuity. |
| `AC-UI-PR-TASK-STATUS-SUMMARY-001.17` | Summary derivation and shared render tests cover a present and missing author login. |
| Existing `.1` to `.12` | Existing PR icon and shared summary suites remain green. |
| `AC-UI-PR-TASK-STATUS-SUMMARY-001.18` | Existing status-chip component tests and mobile Playwright coverage prove drawer author visibility and unchanged task-row navigation. |
| `AC-UI-PR-TASK-STATUS-SUMMARY-001.19` | `github.test.ts` proves missing and foreign live updates leave the active scoped cache unchanged; `task_notifications_test.go` proves typed events reach only the owning workspace and unattributed events are dropped when auth is enforced; the singleton panel test preserves taskPR scope metadata during task switching. |

## E2E tests

- `apps/web/e2e/tests/pr/pr-sidebar-hover-hydration.spec.ts` uses the `chromium`
  project. It opens an unrelated task, hovers an inactive task PR indicator,
  observes the task-scoped request, and checks the author and structured rows.
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts` uses the
  `mobile-chrome` project. It taps the row, opens the current PR-status drawer,
  checks the author, and checks for horizontal overflow.

## Mobile design contract

- **Desktop outcome:** Hover or keyboard focus opens and hydrates the PR task
  summary.
- **Mobile entry:** The task row remains the entry point. The user opens the
  task and then uses the existing PR-status chip.
- **Exemplars:** `session-task-switcher-sheet.tsx` owns task navigation.
  `pr-status-chip.tsx` owns the coarse-pointer drawer.
- **Hierarchy:** The task row stays primary. The PR author is secondary content
  below the title in the existing drawer.
- **Presentation:** Reuse the inset PR-status drawer. Do not add another drawer
  or a compact icon hit target.
- **Scroll and geometry:** Keep the drawer's existing internal scroll owner,
  safe-area behavior, and viewport containment.
- **Shared state:** Desktop and mobile read the same `TaskPR.author_login`.
- **Proof:** The mobile Playwright scenario checks navigation, drawer content,
  and horizontal containment.

## Work orders

- [x] [Task 01: Hydrate PR details on disclosure](task-01-hydrate-pr-details-on-disclosure.md)
- [x] [Task 02: Show PR author identity](task-02-show-pr-author-identity.md)

Task 02 depends on Task 01. The work orders are sequential and do not authorize
subagents.

## Verification results

- Focused web unit suite: 163 tests passed across 10 files after the review
  fixup, including the workspace-context generation regression test.
- Web typecheck passed.
- Web i18n check and ratchet passed.
- Full web lint passed.
- Desktop Playwright hydration scenario passed, including task-scoped request
  deduplication, structured details, author identity, and cache reuse.
- Mobile Chrome Playwright scenario passed, including task navigation, the
  existing PR drawer, author identity, and horizontal containment.
- Web E2E build passed.
- Fresh desktop and mobile captures were generated, inspected, compressed,
  and validated through `apps/web/.pr-assets/manifest.json`.
- The repository-wide E2E sleep-policy lint remains baseline-red with 188
  unrelated errors. Both changed E2E files pass the same lint directly.
- PR review fixup coverage also verifies task-id and workspace-context races,
  deletion tombstones, and keyboard focus continuity during hydration.
- Workspace-routing review fixup on 2026-08-26 added the `workspace_id` frontend
  contract, missing/foreign event guards, typed backend workspace extraction, and
  fail-closed notification tests. The focused frontend suite passed 95 tests
  across 6 files and the backend gateway/GitHub suites passed 2,176 tests. The
  full frontend Vitest run was stopped after the requested 15-minute local
  window without a failure result or terminal summary; the PR checks remain the
  authoritative broad-suite validation.

## Documentation impact

No public documentation change is required. This package updates the durable UI
requirement and adds its system design.

## Risks

- A late HTTP result can replace newer WebSocket data unless current store
  identities win during response application.
- One hook instance per row can produce duplicate requests unless the in-flight
  registry is scoped above each component.
- A request that starts on pointer entry can finish after pointer leave. The
  result must remain cached without reopening the tooltip.
- Radix can keep hidden tooltip portals mounted. E2E locators must target the
  active tooltip content.
- Adding author text can increase summary height and can wrap in translated
  layouts. Desktop and mobile tests must retain viewport containment.
- A missing or mismatched workspace ID must never be replaced with the active
  workspace or reach a global notification broadcast.
