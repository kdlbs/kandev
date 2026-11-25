---
status: current
system: ui
requirements:
  - REQ-UI-DIFF-PRESENTATION-001
---

# Diff Presentation Preferences

## Boundary and owner

`apps/web/hooks/use-global-view-mode.ts` owns the existing browser-local mode.
It reads the raw `diff-view-mode` value (`split` or `unified`), with unified as
its invalid/missing/server fallback. No backend preference or migration applies.
This documents the existing owner rather than adding another state authority.

## Consumers and flow

`useGlobalViewMode` uses `useSyncExternalStore` and subscribes to
`diff-view-mode-change` and browser `storage` events. Its setter writes the raw
value and emits the existing same-window change notification.

`useReviewDialogState` in `review-dialog.tsx` and `useChangesActions` in
`task-changes-panel.tsx` derive their `splitView` boolean from this hook. Their
boolean callbacks translate directly to its setter. They must not retain a local
mode snapshot or independently write/broadcast the preference. `ReviewTopBar`
and `ChangesTopBar` retain their controlled boolean APIs. File toolbar,
`useDiffOptions`, commit controls, and Monaco consumers continue using the same
owner.

Mode events update all subscribed consumers. A subsequent topbar activation uses
the updated boolean and selects the opposite mode. Remount reads storage again;
no effect copies between two state owners.

## Responsive behavior

Review keeps its existing expanded dialog and phone file-action menu. Changes
keeps the task phone Changes entry and existing diff content/overflow actions;
no phone-only mode entry is introduced. Existing file-header mode controls are
desktop-only. Phone Review's topbar controls and any mounted Changes header use
the same state adapter. Existing scroll owners and geometry remain unchanged.
Word wrap and reviewed-file state keep their independent owners.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-DIFF-PRESENTATION-001 | Consumers and flow; Responsive behavior |

## Verification

Consumer regressions exercise the actual Review dialog and file toolbar with
only deferred WebSocket transport replaced. Focused managed desktop and phone
flows exercise actual task Changes and Review controls. Cover both directions,
the first subsequent click, saved/default values, remount, and storage events.

## Implementation plans

- [Synchronize diff mode controls](../../../plans/diff-mode-controls/plan.md)
