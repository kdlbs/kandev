---
status: current
system: ui
created: 2026-09-13
requirements:
  - REQ-UI-RIGHT-PANEL-VISIBILITY-001
owners:
  - kandev
---

# Right panel visibility system design

## Purpose and current baseline

The persistent header control now toggles the actual rightmost region of the active workbench.
The user clarified this behavior on 2026-09-14 with Plan Mode and Preview Mode examples.
The prior implementation at `03fe74e955a869a30819e6d9b3f59c83bb1c3c45` still rebuilds `defaultLayout()` on Show.
That reconstruction is superseded by exact hidden-pane restoration.

UI owns this layout interaction. Reuse [layout profiles](task-layout-profiles.md) and the device-local layout exception in
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).
This is a local extension of that ownership boundary. The requirement and this design retain the rationale without another ADR.

## Requirement mapping

| Criteria                                    | Design section               |
| ------------------------------------------- | ---------------------------- |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.1, .4, .7  | Control and readiness        |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.2, .3, .8  | Geometric target selection   |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.5, .9, .10 | Hide, restore, and lifecycle |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.6          | Tablet and phone             |

## Geometric target selection

A pane is the outermost right side-by-side workbench region. It can contain a leaf group or an entire split subtree.
Default therefore hides Files/Changes and Terminal together. Plan, Preview, and VS Code hide their current right-hand regions.
For three side-by-side regions, hide only the rightmost region. Do not remove all regions with matching panel IDs.
For nested layouts, preserve the selected region's complete tree, including its stacked and nested groups.
Do not flatten a retained `LayoutColumn.tree` into `groups`; serialization gives the tree precedence.

Use live grid ordering and structure, as represented by `fromDockviewApi`, rather than `isRightColumn`, preset names, or component IDs.
The serializer capture must preserve the actual outer split direction. When the outer split is vertical, there is no separate right-hand region.
Do not treat the last member of a vertically stacked layout as rightmost.
Resolve the outer horizontal split from the live Dockview grid, then take its final child.
The existing split tree and serializer are the starting point, not a new independent geometry model.

Eligibility requires another workbench region to remain. Exclude the application navigation sidebar from this count.
Reject a target containing the active `session:<id>` Agent panel or the active `chat` placeholder.
Do not select a region to its left as a substitute when the actual rightmost region is ineligible.
A single compact group is ineligible until the user creates a separate region through normal layout actions.

## Control and readiness

Keep `TaskRightPanelsToggle` before `LayoutPresetSelector` in `task-top-bar.tsx`.
`useTaskRightPanelsToggle` remains the responsive adapter. It must expose effective state from the selected owner.
The desktop state has three cases: hideable visible target, retained hidden target, or unavailable target.
A retained hidden target takes precedence over selecting another visible target.

Use localized next-action labels such as `Hide right pane` and `Show right pane`, with `aria-expanded` matching effective visibility.
For an unavailable target, retain a disabled control with an explanation such as `No separate right pane to hide`.
Use the existing focusable disabled-tooltip wrapper and maximized-state explanation.
Disable during initialization, restoration, and maximize. Preserve 28px fine-pointer and 44px coarse-pointer hit areas.
The next user action must never create Files/Changes/Terminal merely because no hidden pane exists.

## Hide, restore, and lifecycle

Add a focused helper module, proposed `lib/state/dockview-right-pane.ts`, for selection, snapshot validation, and reinsertion.
Keep active environment state in `useDockviewStore`; avoid growing the visibility actions into another large layout subsystem.
A hidden-pane descriptor contains the captured subtree, panel/group identities and parameters, selected tabs, width,
its placement anchor, and enough layout identity to reject a stale descriptor. It contains no transcript or terminal output.
The descriptor is one reversible operation, not an undo history or a saved profile.

On Hide, capture the eligible region and its insertion anchor before removing it.
Preserve chat scroll and the remaining layout's group selections. Save the collapsed layout and descriptor together.
Do not terminate agents or user shells when hiding their UI. Block stale callbacks using environment and operation identity before any mutation.

On Show, merge the retained subtree into the current live arrangement at its anchored right-side position.
Do not restore an old whole-workbench snapshot over edits made while the pane was hidden.
Reconcile panel IDs already reopened elsewhere: keep their live instances, omit duplicates from the retained subtree,
and remove empty leaves while preserving the remaining tree and its active-tab fallbacks.
Use existing availability and session reconciliation for removed plugin panels, obsolete sessions, reviews, and ephemeral panels.
When no eligible hidden panels remain, clear the descriptor without creating a fallback region.
Clear recovery state only after successful application. Retain recoverable state after an application failure.
Restore captured geometry within current safety limits; do not apply Default-only pinned widths to Plan or Browser merely because they are rightmost.

### Environment persistence

Use the existing environment layout storage scope, which currently uses `sessionStorage` through `getEnvLayout` and `setEnvLayout`.
Persist optional versioned `kandevHiddenRightPane` metadata with the existing serialized environment layout record.
This keeps the collapsed layout and recovery descriptor in one write, without a second storage key or backend preference.
The metadata field is proposed new code. Strip it before passing the Dockview payload to `api.fromJSON`.
Legacy records without this field remain valid. Validate metadata separately from the existing grid health check.
Invalid metadata must not discard an otherwise valid visible layout.

Centralize record composition so immediate toggle saves, debounced saves, outgoing-environment saves, and unload saves preserve current recovery state.
Read it on initial mount and every environment restore path, including fast restores and maximize restoration.
An A/B task switch saves and loads each environment's own descriptor rather than copying the current descriptor to the next environment.
Do not put environment-specific recovery data in global fallback layouts or portable saved layout profiles.
Ordinary Save current layout captures the visible arrangement only, without hidden metadata.

Explicit preset application, custom layout application, and Reset Layout clear the current environment's descriptor before applying the new layout.
This prevents a hidden Plan from reappearing after the user selects Preview Mode.
Normal panel close does not populate this descriptor. Maximize does not capture its temporary overlay as a hidden right pane.
Preserve metadata when saving an unchanged regular layout beneath a maximize overlay.

## Tablet and phone

The 768-1023px coarse-pointer fallback remains `SessionTabletLayout`, using its existing session-scoped right-column state.
Its outer right region is Files/Terminal; preserve its saved two-panel sizing and active content on hide/show.
Its left Chat/Plan/Changes tab surface is not a separate right pane.
Larger tablets use Dockview, so Plan and Preview select their live geometric target exactly as on desktop.
No breakpoint or cross-store synchronization is added.

Phone composition remains `SessionMobileLayout` with `SessionMobileBottomNav`.
Files and Terminal keep full-screen destinations with panel-owned scrolling and existing dynamic viewport/safe-area behavior.
Do not add a right-pane toggle or let phone navigation write wider-layout recovery state.

## Validation and delivery

Use real capture/serialization round trips with `session:<id>` Agent identities, rather than mocked column labels alone.
Cover exact pane identity, nested geometry, no fabricated sidebar, duplicate reconciliation, reload, environment switches,
preset invalidation, malformed recovery metadata, maximize guards, and tablet/phone parity.
The [new work order](../../../plans/right-panel-visibility/task-02-contextual-right-pane.md) owns exact checks.
[Task 01](../../../plans/right-panel-visibility/task-01-persistent-toggle.md) is historical implementation evidence, not authority for superseded reconstruction.
