---
status: current
system: ui
requirements:
  - REQ-UI-CHANGES-FILE-ACTION-FEEDBACK-001
---

# File-action feedback

## Purpose and boundaries

This design keeps the existing per-file stage and unstage spinner visible after
a fine-pointer user leaves the affected Changes row. It changes only the
presentation precedence inside the shared file-action slot. Git operation
dispatch, repository scoping, status refresh, and pending-state cleanup remain
unchanged.

## Requirement mapping

| Requirement                               | Design section                                                                                                                                                                                       |
| ----------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-CHANGES-FILE-ACTION-FEEDBACK-001` | [Components and responsibilities](#components-and-responsibilities), [Visibility precedence](#visibility-precedence), [Responsive behavior](#responsive-behavior), and [Verification](#verification) |

## Components and responsibilities

- `useSessionGit` continues to own `pendingStageFiles`, keyed by repository and
  path. It records the requested stage or unstage transition before dispatch
  and clears it when refreshed status reaches that transition or the request
  fails. A stale same-repository snapshot cannot clear a newer action.
- `changes-panel-tree.tsx` and `changes-panel-timeline.tsx` continue to derive
  `FileRow.isPending` from that set. No additional local loading state is added.
- `FileRow` continues to share one implementation across the desktop Changes
  panel and the phone Changes surface.
- `TreeModeFileActionSlot` owns the fine-pointer swap between `FileIcon` and
  `StageButton`. Pending state takes precedence over the idle hover rule.
- `StageButton` continues to replace either stage or unstage control with the
  existing `IconLoader2` spinner while `isPending` is true.

## Visibility precedence

The tree-mode slot has three presentation states:

1. On a fine pointer while idle and not hovered, the file-type icon is visible
   and the stage or unstage action is hidden.
2. On a fine pointer while idle and hovered, the icon is hidden and the action
   is visible and interactive.
3. While pending, the icon remains hidden and the action layer containing the
   spinner remains visible regardless of hover. The pending layer stays
   noninteractive through the existing spinner-only branch.

The correction is expressed from `isPending` and the existing pointer mode. It
does not add timers, duplicate request state, pointer listeners, or a second
spinner. When pending state clears, the existing hover rule becomes
authoritative again or the refreshed Git status moves the file to its new
section.

## Responsive behavior

- **Desktop outcome:** the resizable Changes panel keeps the spinner in the
  leading file-action slot after pointer leave, so the acted-on file remains
  identifiable throughout the operation.
- **Mobile entry point and surface:** the existing Changes bottom-navigation
  item opens `MobileChangesPanel`; the shared row keeps its always-visible
  coarse-pointer action and 44px target.
- **Nearest shipped exemplar:** the current coarse-pointer branch in
  `TreeModeFileActionSlot` already makes the action and pending branch
  independent of hover. The desktop correction applies the same pending-state
  precedence without changing desktop density.
- **Hierarchy and primary action:** file identity and the stage or unstage
  control remain in the existing row. No drawer, route, or new navigation is
  introduced.
- **Surface rationale:** this is short, row-scoped request feedback, so keeping
  it inline preserves context better than opening a temporary surface.
- **Shared logic and geometry:** both viewport classes consume the same
  `isPending` value. Scroll ownership, safe-area handling, path truncation, and
  document overflow behavior remain unchanged.

## Failure and recovery

The UI does not infer completion from hover or elapsed time. It follows the
existing `isPending` input. Status refresh clears pending state only when the acted-on file has reached the
requested staged or unstaged state. Request failures clear it directly. The
slot then restores the idle icon/action presentation, and backend or WebSocket
errors remain surfaced through their current paths.

## Verification

- A Chromium Playwright regression pauses the outgoing `worktree.stage`
  request, moves the pointer away from the row, and proves that the spinner
  remains visible until the request is released and the file moves to the
  staged section.
- The same regression pauses `worktree.unstage`, repeats the pointer-leave
  assertion, and then proves the file returns to the unstaged section.
- A focused hook regression publishes a stale same-repository snapshot during
  each paused action and proves pending state survives until status reflects
  the requested transition.
- The test intercepts only the selected worktree request at the WebSocket
  transport boundary and forwards every unrelated frame.
- Existing coarse-pointer component and Pixel 5 Changes tests continue to prove
  that stage and unstage actions are touch-reachable. No new mobile composition
  or interaction is introduced.

## Related decisions

None. The change corrects local rendering precedence without changing a
runtime, data, or ownership boundary.
