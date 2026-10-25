---
status: draft
system: ui
requirements:
  - REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001
created: 2026-08-24
owners:
  - Kandev
---
# Merge Queue Recovery Controls System Design

## Purpose and boundaries

This design extends the existing PR automation popover and phone drawer. The
two existing switches control repair and queue submission.

The UI does not own provider classification or retry policy. It renders the
state from `TaskPR`, per-PR options, and per-PR automation state.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-CI-PR-MERGE-QUEUE-RECOVERY-001` | [Control composition](#control-composition), [Responsive behavior](#responsive-behavior) |

## Control composition

`PRCIAutomationControls` keeps this order:

1. A contextual section title and selected PR subtitle.
2. `Auto-fix CI and address comments`.
3. `Auto-merge or requeue when ready`.
4. One non-interactive merge-queue status line when queue context exists.
5. The existing `Review follow-up` disclosure.
6. The existing error row.

No queue-recovery switch appears. Auto-fix owns agent repair work. Auto-merge
owns direct merge, initial enqueue, and safe requeue.

The title and subtitle use these contexts:

| Context | Title | Subtitle example |
| --- | --- | --- |
| No active queue or removal | `Automation` | `Applies to PR #3000 only` |
| Active queue entry | `Merge queue automation` | `PR #3000 is in the merge queue` |
| Removed with a classified cause | `Merge queue recovery` | `PR #3000 was removed: checks failed` |
| Removed with no classified cause | `Merge queue recovery` | `PR #3000 was removed from the merge queue` |

The switch labels do not change with these contexts. They are the stable names
of saved per-PR preferences. Short supporting text changes to explain the next
effect:

| Context | Auto-fix supporting text | Auto-merge supporting text |
| --- | --- | --- |
| Active queue entry | `Will repair failed queue checks or conflicts` | `Current queue attempt is already active` |
| Removed, actionable, auto-fix off | `Enable to send this queue failure to the agent` | `Will requeue after a new commit passes checks` |
| Removed, repair accepted | `Repair requested for this removal` | `Waiting for a new commit before requeue` |
| Removed, not actionable | `No automatic repair for this removal` | `Will requeue after a new commit passes checks` |
| New head, checks pending | `The fix created a new commit` | `Waiting for required checks` |

When auto-merge is off, the last two auto-merge descriptions instead state
that automatic requeue is off. Supporting text is not part of the switch's
accessible name.

`CIAutomationOptionRows` uses one pure presentation helper. The helper derives
these states:

- `queued`: An active queue entry exists.
- `removed_actionable`: An actionable removal exists, but no repair was
  accepted for it.
- `removed_not_actionable`: A manual, branch-protection, or unknown removal
  exists. Kandev shows the event without offering automatic repair as its next
  result.
- `repair_requested`: The latest removal has an accepted auto-fix round, and
  the pull-request head is unchanged. The UI does not claim that the agent is
  running because durable acceptance does not prove execution state.
- `waiting_for_commit`: The latest queue attempt left the queue, auto-merge is
  enabled, and the pull-request head is unchanged.
- `waiting_for_checks`: The head changed after removal, auto-merge is enabled,
  and one or more normal merge gates have not passed.
- `none`: No active or retained recovery state needs a line.

The helper can combine `repair_requested` and `waiting_for_commit` into
`Repair requested. Waiting for a new commit.` A disabled option changes the
supporting text, not the provider state.

The status line uses translated labels and a muted semantic style. A destructive
error continues to use `CIAutomationErrorRow`.

The header information control explains these combinations:

- Auto-fix only repairs an actionable queue removal.
- Auto-merge only submits an eligible head.
- Both controls form the repair-and-requeue loop.
- Kandev never requeues the same head after removal.

When automation is enabled on an already queued pull request, the visible
active entry becomes the baseline. The UI updates the saved option immediately,
but it does not show a new queue submission. Auto-fix watches for a later
actionable removal. Auto-merge adopts the current attempt.

The prompt dialog states that `{{pr.feedback}}` can include queue-removal
context and available failed merge-group checks.

## Data contracts

`apps/web/lib/types/github.ts` adds the queue-recovery snapshot fields to
`TaskPR`. It adds `last_queue_attempt_head_sha` and
`last_queue_fix_event_id` to `TaskCIPRAutomationState`. It also adds the
normalized `last_queue_removal_cause` enum.

The subtitle and status use this normalized enum. They never insert the raw
provider reason into a translation key or accessible name. If the enum is
`unknown`, the UI uses the generic removal copy.

The UI reads only existing boot, HTTP, and WebSocket data. It does not add a
fetch from the component.

## Responsive behavior

### Desktop outcome and mobile entry point

A fine pointer opens the existing hover popover. A phone user taps the existing
PR status chip and uses the existing drawer.

### Nearest mobile exemplar

`PRStatusChipDrawer` is the nearest shipped surface. It supplies the inset
drawer, internal scroll region, and safe-area behavior.

### Information hierarchy and primary action

The two switches remain the primary controls. The status line explains their
current queue outcome. The PR status chip remains the mobile entry point.

### Presentation rationale

Queue recovery is short status content inside an existing temporary surface.
It does not need a new route, drawer, or nested disclosure.

### Geometry and shared logic

`CIAutomationRow` keeps its 44-pixel coarse-pointer height. The status line can
wrap inside the drawer, but it cannot create document-level horizontal scroll.
The existing drawer remains the only vertical scroll owner.

Desktop and mobile share state derivation, copy, and action handlers. Existing
responsive wrappers continue to own presentation.

## Accessibility

The updated auto-merge label becomes the switch accessible name. The queue
status line uses status text, not color alone.

The information control remains reachable by keyboard and touch. Its copy
names the same-head retry guard.

## Localization

All new labels and explanations use the `github` namespace. The change updates
English, Portuguese, Simplified Chinese, Hong Kong Chinese, and Taiwan Chinese
catalogs. The Traditional Chinese catalogs use `pnpm run i18n:zh-hant`.

User-facing copy does not use a Unicode em dash.

## Verification

Focused component tests cover label changes, state derivation, and help copy.
Desktop Playwright coverage uses the current hover popover. Mobile coverage
uses a tap and the `mobile-chrome` project.

The mobile test proves the 44-pixel switch rows, status visibility, internal
drawer scroll, and no document-level horizontal overflow.
