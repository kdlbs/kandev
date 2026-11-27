---
status: current
system: ui
requirements:
  - REQ-UI-CHANGES-LOADING-001
---

# Changes Loading Feedback Design

## Purpose and boundaries

The Changes summary presents passive loading in its fixed toolbar.
This design changes presentation and local ownership of an existing commit-detail controller.
It does not change backend APIs, Git mutations, or persistence.
Platform owns [automatic refresh recovery](../../platform/system-design/changes-refresh-recovery.md).

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| 001.1, 001.4 | Shared loading projection |
| 001.2, 001.5, 001.7 | Toolbar and responsive presentation |
| 001.3, 001.6 | Content and failure states |

## Shared loading projection

`useChangesPanelData` already returns `gitStatusPresentation` from `useChangesPanelGitStatus`.
Use its `loading` and `detailsPending` fields for passive Git work.
Keep `git.isLoading` and `loadingOperation` for existing mutation controls.

Extract `useInlineCommitDetails` from `changes-panel-timeline-content.tsx` into
`use-changes-inline-commit-details.ts`.
Move its invocation to `useChangesPanelData`, once per mounted panel.
Return the existing controller, version, and task/session/environment context identity to the timeline.
Preserve controller retirement, generation guards, local/provider target identity, and bounded collapsed caching.
Do not create a second controller for the header.

Add a read-only pending snapshot projection to `ChangesInlineCommitState`.
Derive it from snapshots with `status === "loading"`, using the existing version subscription.
An in-flight request remains pending even after its commit collapses.
Settlement or context retirement removes its contribution.

The toolbar status combines Git refresh, pending enrichment, commit-detail loading, and Git unavailability.
Use these priorities: active Git recovery shows loading, unresolved Git failure shows warning, other pending details show loading, otherwise hide.
Pass the status to `ChangesPanelHeader` from both `ChangesPanel` and `MobileChangesPanel`.
Pass the same inline-detail owner to `ChangesPanelTimelineContent` through body props.
No global store, callback registry, or loading counter is necessary.

## Toolbar and responsive presentation

Use `@kandev/ui/spinner` and `@kandev/ui/tooltip` in a small Changes status component.
Match adjacent icon size and muted color, with no border, background, or visible text.
Override the shared spinner's English accessible label with translated copy.
Use `task:loadingChanges` for the loading tooltip and a single accessible status.
Use a small muted amber `IconAlertTriangle` in the same slot for warning.
The warning tooltip uses new localized copy: "Changes did not refresh. Retrying automatically."
With prior data, add: "Showing the last available changes."
Use one polite live region for both states, with no repeated announcement on an unchanged rerender.
Avoid duplicate live announcements from the decorative SVG and its wrapper.

Append the status after all actions in both header-left compositions, before the flexible toolbar spacer.
Keep it at the right edge of the left action group, rather than between actions or after the right toolbar group.
When Review is the final left action, the status sits directly to the right of its eye button.
Preserve responsive action ordering, including Walkthrough's existing narrow-panel order.
Render it independently of `showDiffReview`, so initial loading can appear without action buttons.
`PanelHeaderBarSplit` selects one left composition through `leftWhenOverflow`.
Keep the status visible there, rather than inside `ChangesPanelHeaderOverflowActions`.
Retain the fixed shell and neighboring action dimensions.

Fine-pointer hover and keyboard focus on a focusable status wrapper reveal the localized "Loading changes..." tooltip.
Wrap the passive SVG with `TooltipTrigger asChild` rather than attaching the trigger to a non-focusable icon.
Wrap the Review eye button with its own `Tooltip` in both `ChangesPanelHeaderLeft` compositions.
Use `TooltipTrigger asChild` on the existing button and `t(REVIEW_LABEL_KEY)` for its tooltip content.
`REVIEW_LABEL_KEY` already resolves to `task:filterStateReview`, so no new translation key is necessary.
Keep the existing accessible name, handler, focus target, and touch geometry.
Phone activation opens Review directly, without requiring a tooltip interaction.
The status does not initiate an action or open a drawer.
Phone users receive the same passive cue and accessible status without a tap.
The phone exemplar is `mobile/mobile-changes-panel.tsx`, within `session-mobile-layout.tsx`.
It already shares the summary header and body in a focused Changes destination.
Keep its bottom navigation, safe-area treatment, and content scroll owner.

## Content and failure states

Remove `GitStatusNotice` from the body, including its manual Retry control.
Retain failure and failed-repository state for the toolbar warning and its tooltip.
Name failed repositories in the tooltip when multiple repositories are involved.
While waiting between recovery attempts, show warning even if unrelated commit details remain pending.
The active recovery request replaces warning with spinner.
Accepted live status clears the matching warning without hiding another repository's failure.
Do not translate raw Git output or expose it in the tooltip.
Continue to suppress `EmptyChangesPanel` until `membershipReady` is true.

`FileRowStats` omits the pending detail label and unknown counts without a retained display value.
Keep file status icons and unavailable detail labels.
Displayed counts and diff continuity follow the [Platform design](../../platform/system-design/git-refresh-continuity.md).

`changes-timeline-history-model.ts` stops emitting `commit-status` rows for idle or loading details.
Remove the corresponding loading branch from `CommitStatusHistoryRow` and narrow its row type.
Keep commit headers, error rows with Retry, and completed empty rows.
Do not return null for a loading row that still consumes virtualizer height.

Remove pure loading flags from `beforeLayoutKey` after their notice no longer changes content height.
Retain keys for workspace states that still affect geometry.
Preserve existing virtualizer measurement and scroll-anchor behavior.

## Verification and compatibility

Unit/component evidence covers overlapping requests, retirement, both toolbar compositions,
pending membership, file stats, commit descriptors, and retained inline-commit error actions.
Desktop and mobile refresh-recovery E2E tests retain real enrichment gates and recovery evidence.
Add held commit-detail scenarios beside them, including overlapping requests and context switching.
Measure toolbar height and glyph position during pending and ready states.
Check narrow-panel containment and phone document overflow.

This presentation specializes the loading states referenced by
[bounded Changes rendering](bounded-changes-rendering.md#failure-states-and-validation).
Its source restrictions, virtualization, and failure contracts remain applicable.
Warning tooltip copy requires English and all six supported locales, plus regenerated pseudo copy.
Generate Traditional Chinese through the repository tool.
No new telemetry, dependencies, or ADR are necessary.

## Implementation plan

- [Changes loading feedback](../../../plans/changes-loading-feedback/plan.md)
