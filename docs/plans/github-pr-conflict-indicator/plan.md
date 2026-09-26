---
created: 2026-09-25
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001
system_design:
  - ../../specs/integrations/system-design/github-pr-conflict-indicator.md
legacy_specs: []
---

# Implementation Plan: GitHub PR Conflict Indicator

## Overview

Preserve GitHub's conflict observation independently from the existing
effective PR status, project it into compact task rows, then render a warning
bubble through one GitHub PR status glyph used by task rows and the top bar.
The data contract comes first so every surface uses the same confirmed value.

## Scope

### In scope

- Add and synchronize an optional GitHub conflict observation on linked PRs.
- Aggregate it in bounded task-status summaries and carry it to the web icon.
- Render one top-right warning for any open conflicted PR alongside existing
  status color, count, and automation dots.
- Reuse one glyph component in the sidebar, Home Kanban card and pipeline row,
  rich task list, task top bar, and multi-PR menu rows.
- Cover desktop and phone presentation and accessibility.

### Out of scope

- GitLab and plugin change-request indicators.
- Conflict repair actions and changes to CI or merge rules.

## Technical approach

### GitHub data and task projection

In `apps/backend/internal/github/service_pr_watch.go`, keep
`nextMergeableState` draft normalization and separately derive an optional
conflict observation from the raw `status.MergeableState`. Add the field to
`TaskPR` in `models.go`, the SQLite schema/migration and read/write paths in
`store.go`, association writes, and change-event payloads. Preserve unknown
versus confirmed false. Extend the existing store and sync tests.

Carry the observation through `PullRequestInput`, the PR event projector, and
`PullRequestSummary` in `apps/backend/internal/task/statussummary/`, including
the authoritative rebuild adapter in
`apps/backend/internal/backendapp/status_summary_adapter.go`. Aggregate
over open PRs. Keep legacy `dirty` compatibility and ensure a terminal PR does
not contribute a warning to an open sibling. Update the TypeScript wire types
in `apps/web/lib/types/github.ts` and `task-status-summary.ts` in the same
contract work order.

### Shared GitHub PR glyph

Map compact summary data through `taskPRInfoFromSummary`. In
`PRTaskIcon`, derive the warning from full PRs when available and otherwise
from compact `prInfo`. Extract the presentational PR shape and corner badges
from `PRTaskIconGlyph` into `apps/web/components/github/pr-status-glyph.tsx`.
The component accepts status color, confirmed conflict, optional automation
flags, and task/top-bar size. It owns no store subscriptions, provider calls,
or click handlers. Reserve top-right for the warning and move auto-merge to
lower-right. Reuse the existing `github:conflicts` translation in the complete
indicator's accessible name. Keep existing hover tooltip and touch drawer
behavior.

`PRTopbarButton` uses the same glyph for its single-PR button, aggregate
multi-PR button, and individual multi-PR menu rows. Extend
`ChangeRequestTopbarContent` with an optional glyph slot; its default icon
remains for other providers. Remove the single-PR trailing status icon. Keep
only the leading shared glyph and `#number`, with localized status in the
button's accessible name. The multi-PR button keeps its count, aggregate color,
and chevron. Keep the existing button, menu, popover, and panel actions. The
GitHub integration requirement and design own this top-bar presentation along
with the task-row glyph.

## ASCII UI preview

`UI-01: Task PR indicator`, sidebar, Home Kanban card and pipeline row, and rich-list rows; full data or
compact projection. `!` is a small warning bubble at the glyph's top-right,
not a separate button. `o` marks automation dots.

```text
Current, desktop row:                  Proposed, desktop row:
Task title              PR(red)       Task title              PR(red)!
Task title              PR(muted)     Task title              PR(muted)!
Task title              PR(green)     Task title              PR(green)!

Glyph detail, both viewports:
  o  !    auto-fix upper-left; conflict upper-right
   PR
     o    auto-merge lower-right; all three can coexist
```

`UI-02: Phone task switcher`, existing drawer. The row remains the primary
navigation target; the PR indicator retains its existing disclosure target.

```text
Tasks                                  [existing drawer]
  Task title                  PR(red)!
  Task title                  PR(muted)!
```

`UI-03: Task top bar`, single and multi-PR states. The single badge has no
trailing status icon. The menu shows per-PR warnings rather than copying the
aggregate warning to every row.

```text
Single:  [ PR(red)!  #3932 ]
Multi:   [ PR(red)!  2 PRs  v ]
Menu:      PR(red)!   repo #3932
           PR(green)  repo #3933
```

The corner placement, shared glyph, simultaneous visibility, single-PR badge
layout, and retained multi-PR arrow are structural requirements. The text
labels and spacing in the drawing are illustrative. Status color rules stay as
they are. Previews map to AC-001.1 through AC-001.7; rendered checks are in
Task 02. No new scrolling region, overlay, or touch action is introduced.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| AC-001.1, AC-001.2, AC-001.4 | GitHub sync/store tests for draft plus dirty, initial association, clearing, and unknown; frontend color and warning tests. |
| AC-001.3, AC-001.5 | Task summary aggregation and compact task icon render tests. Home Kanban card and pipeline row reuse `PRTaskIcon`. |
| AC-001.6, AC-001.7 | Topbar render tests for badge, count, absent trailing icon, localized accessible status, and desktop browser interactions. |

All abbreviated `AC-001.*` references above belong to
`REQ-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001`.

## E2E tests

- Desktop `apps/web/e2e/tests/pr/pr-topbar-popover.spec.ts`, `chromium`:
  seed failing CI, changes requested, and conflict; confirm badge layout,
  accessible text, focus disclosure, and click to details.
- Phone `apps/web/e2e/tests/pr/mobile-pr-ci-chip.spec.ts`,
  `mobile-chrome`: confirm the existing touch drawer still opens and shows
  PR CI details.
- Phone `apps/web/e2e/tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts`,
  `mobile-chrome`: confirm the linked-PR record and bounded task summary both
  retain conflict through automation updates, then check the conflict bubble,
  automation dots, touch drawer, focus return, and terminal-state cleanup.

## Work orders

- [x] [Task 01: Preserve conflict observations in task status](task-01-conflict-observation.md)
- [x] [Task 02: Share the PR conflict glyph across surfaces](task-02-task-pr-badge.md)

Task 02 depends on Task 01's additive payload and compact projection.

## Verification results

GitHub sync, persistence, status-summary, and backend app tests pass. Focused
web component tests, typecheck, ESLint, i18n checks, production build, and
specification validation pass. Desktop single-PR topbar and multi-PR menu
browser scenarios pass. Phone CI drawer and task-switcher scenarios pass; the
task-switcher scenario verifies conflict in both the linked-PR record and
bounded summary, with automation dots, touch disclosure, focus return, and
terminal-state cleanup.

The PR fixup keeps compact accessibility labels within the bounded summary's
known facts. The focused tests cover neutral failure and pending buckets,
review-only pending, and an open PR whose `ready` bucket has no check evidence.
The duplicate top-bar UI requirement/design/plan was consolidated into this
GitHub-owned requirement and design; its single- and multi-PR layout, accessible
status, interactions, and phone behavior remain covered by AC-001.3, .6, and
.7 and Task 02.

Post-fixup documentation checks pass:

- `node --test .github/scripts/pr-docs.test.cjs` (81 tests).
- `python3 scripts/list-docs.py validate` (305 decisions and 1,155
  specifications validated).
- `python3 scripts/lint-spec-files.test.py` (36 tests).
- `python3 scripts/lint-spec-files.py --all`.
- `python3 .github/scripts/pr-docs-workflow-contract_test.py` (7 tests).
- `git diff --check`.

## Risks

- `mergeable_state` currently encodes draft, so deriving the warning from it
  alone loses simultaneous draft and conflict. The raw provider result must
  reach both full and compact projections.
- The top-right corner currently contains auto-merge. Moving that dot needs a
  rendered geometry check at the small glyph size.
- The top bar currently has a separate status slot. Removing it from the
  single-PR badge requires localized accessible status text on the button.
- Old persisted rows cannot recover a draft's hidden raw conflict until the
  next successful provider observation; the migration must leave them unknown.
