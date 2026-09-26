---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001
created: 2026-09-25
owners:
  - kandev
---

# GitHub PR Conflict Indicator System Design

## Purpose and boundaries

The GitHub integration owns the provider conflict observation. The UI system's
existing `PRTaskIcon` and `PRTopbarButton` render that observation through a
shared GitHub glyph without changing their status-color rules or disclosure
contracts. This design covers both full `TaskPR` records and the bounded
`TaskStatusSummary.pull_request` used by inactive task rows.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-PR-CONFLICT-INDICATOR-001` | [Conflict observation](#conflict-observation), [Task-row projection](#task-row-projection), [Visual presentation](#visual-presentation), [Responsive and accessible behavior](#responsive-and-accessible-behavior) |

## Conflict observation

GitHub supplies draft status and `mergeStateStatus` separately. The current
`SyncTaskPR` path overwrites the effective `TaskPR.MergeableState` with `draft`
when the PR is draft; consumers rely on that effective state. Preserve it.
Add an optional conflict observation to `TaskPR`, sourced from the provider's
raw mergeability result before draft normalization. A raw `dirty` result means
confirmed conflict; an authoritative non-dirty result means no conflict;
missing or `unknown` means unconfirmed. The additive field is nullable in
persistence and on the API/WS payload, so old rows and partial observations do
not claim a conflict. A fresh authoritative sync updates or clears it. The
existing `mergeable_state: dirty` remains a compatibility signal for older
records until a new observation reaches them.

The observation flows through both initial association and subsequent syncs,
including mock-provider paths. Sync change detection includes it so a conflict
transition emits the existing PR update. Existing auto-fix, merge-readiness,
and queue decisions continue to use the effective mergeability contract.

## Task-row projection

The bounded `PullRequestSummary` gains an aggregate conflict boolean. The
status-summary projection receives the conflict observation from the
`backendapp` authoritative rebuild adapter and PR update events. It sets the
boolean when any open PR has a confirmed
conflict, independent of `AggregateState` and the representative PR. A compact
task row therefore shows the warning without a disclosure-triggered hydration
request. An authoritative clear, PR closure, or unlink updates the aggregate
through the existing projection path. Legacy `dirty` rows can contribute until
fresh observation replaces them.

`taskPRInfoFromSummary` carries the aggregate flag to `PRTaskIcon`. When full
workspace-scoped `TaskPR` data is present, the icon derives the flag from open
records instead. `PRTopbarButton` derives the same flag from its full PR list.
The compact and full paths must agree for the same snapshot; workspace changes
and late hydration follow the existing cache guards.

## Visual presentation

One presentational GitHub PR status glyph owns the PR shape, size variants,
conflict bubble, and optional automation dots. `PRTaskIconGlyph` uses it at the
small task-row size; `PRTopbarButton` uses it at top-bar size for single and
aggregate buttons and for each multi-PR menu row. Both surfaces supply the
same conflict derivation from full `TaskPR` records. The top-right corner is
reserved for a confirmed conflict. The existing auto-merge dot moves to the
lower-right corner wherever automation badges are supplied; auto-fix keeps its
upper-left position. The bubble is a separate semantic mark, not a new
clickable control. `getPRStatusColor`, `aggregatePRStatusColor`, PR count,
ready-to-merge attributes, and status-summary rows retain their current rules.

`ChangeRequestTopbarContent` receives an optional glyph slot so the GitHub
button can supply the shared glyph; other providers retain its default icon.
The single-PR top-bar button passes no trailing status icon. Its shared leading
glyph carries color and the conflict bubble; the adjacent number identifies
the PR. The multi-PR button retains its dropdown arrow. Multi-PR buttons
aggregate over open PRs, while each menu row uses its own PR state and glyph.
The same task-row glyph reaches the sidebar title or trailing slot, Home
Kanban cards and pipeline rows, and rich task-list rows. GitLab and plugin
change-request indicators do not change.

## Responsive and accessible behavior

The warning uses an accessible name on the complete task-row or top-bar PR
indicator, with localized conflict copy. The badge itself is decorative to assistive
technology, avoiding a duplicate announcement. On fine pointers, existing
hover and keyboard focus disclose the structured PR summary or top-bar
popover. On coarse pointers, the existing task-row target and PR drawer remain
the entry points; the bubble adds no small independent touch target. The top
bar keeps its current click and dropdown actions where rendered. The nearest
phone exemplar is the task-switcher PR indicator in
`apps/web/components/github/pr-task-icon-disclosure.tsx`.

The top-bar button's accessible name identifies the PR and summarizes its
current lifecycle, checks, review, and confirmed conflict in localized text.
It derives that text from the same PR snapshot as the glyph, so color is not
the only status cue after removal of the trailing icon. The detailed hover or
focus popover remains available.

Compact task summaries expose an aggregate status bucket without always
identifying its cause. Use neutral localized wording for `failure` and
`pending`; name a review or check state only when that aggregate distinguishes
it. An open PR's `ready` bucket can occur when check evidence is absent, so the
compact accessible name identifies it as open without claiming it is ready to
merge. Full PR details may use specific check and review labels.

## Failure and recovery

Unknown or absent conflict information does not create a warning. If an older
record reports effective `dirty`, the UI can show the warning until a later
authoritative observation updates it. A failed provider refresh leaves the
last stored observation in place, consistent with existing PR status display.
No separate fetch, retry loop, or user-facing error is added for the badge.

## Verification

Backend tests cover draft plus raw `dirty`, dirty plus failing CI, clearing,
unknown/legacy records, summary aggregation, and event propagation. Frontend
tests cover badge derivation, coexistence with automation dots, the absent
single-PR trailing icon, accessible names, and compact-to-full hydration. Desktop and
`mobile-chrome` Playwright scenarios verify the rendered shared glyph in the
sidebar, Home Kanban card, pipeline row, top bar, and phone task switcher.

## Related designs

- [PR task status summary](../../ui/system-design/pr-task-status-summary.md)
- [Bounded task status delivery](../../platform/system-design/bounded-task-status-delivery.md)
