---
status: draft
system: ui
requirements:
  - REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006
---

# Bulk task colors system design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-SIDEBAR-AUTOMATIC-TASK-COLORS-006` | [Bulk manual color editing](#bulk-manual-color-editing) |

## Bulk manual color editing

This draft extends the [current personal-color design](sidebar-automatic-task-colors.md).
Delivery is tracked in the
[bulk color plan](../../../plans/bulk-task-colors/plan.md). UI remains the owner
because colors are personal presentation preferences, not shared task metadata.

### Selection and surfaces

`BulkSelectionMenuItems` in `task-switcher-context-menu-items.tsx` adds Color
beside Pin in its mark group, using the existing `actingIds` targeting rule.
The mark group must remain visible when pin callbacks are absent. Single-task
menus retain their existing targeting and automatic-source explanation.
`TaskMultiSelectToolbar` adds Color for its `selectedIds`; mixed workflows do
not disable it. No new task color markers are introduced on board cards.

Generalize the palette in `task-switcher-color-menu.tsx` to accept a task-ID
set and share its selection/value derivation with a toolbar picker. Deduplicate
IDs and capture an immutable set at option activation. Do not derive targets
from the active task or reuse workflow-move eligibility. A personal color does
not require a workflow or mutate task lifecycle. Existing selection cleanup
removes stale rows; an empty target set is a no-op. No descendant expansion.
Read manual values from `sidebarTaskColors`, treating absent and null as None.
Check a color only for a unanimous non-null value. Mixed state checks nothing;
None is disabled only when all values are empty. Keep selection after submit.

### Mutation and recovery

Extend the existing `useSetTaskColor` mutation implementation with a batch entry
point; keep the single-task API delegating to the same mutation path. Do not
call the single-task setter once per ID. For up to 500 unique IDs send one
`sidebar_task_color_patch` with `if_missing: false`; clearing uses null entries.
For larger sets, send sequential chunks of at most 500 IDs. The existing backend
validation, CAS merge, settings response, and personal authorization apply.
No new endpoint, task mutation, migration, or settings key is needed.

Expose pending state and a completion result to the picker. Apply optimistic
colors for the captured IDs. Serialize this operation's chunks, track confirmed
chunks, and stop after the first error. Reconcile responses through the existing
settings mapper and revision guards. Restore only this operation's outstanding
optimistic entries, preserving later per-ID changes and unrelated settings;
a whole-map rollback must not overwrite concurrent edits. Tests must cover
separate hook instances, late responses, a settings event during saving, and
single-task edits overlapping a batch. Keep a confirmed baseline per affected
ID rather than assuming a local optimistic value is confirmed.

After partial failure, retain successful chunks, restore failed/unattempted
entries, report saved and remaining counts, and allow the same selection to
retry. Reapplying a color is idempotent. Existing total settings capacity errors
use this failure path. Pending state prevents duplicate submission from the
initiating picker; it does not globally lock unrelated settings controls.

### Phone composition and accessibility

Add a phone-visible Select tasks button in `kanban-board.tsx`, immediately
above `KanbanSwimlanes`, independently of the hidden `SwimlaneHeader`. Wire it
to the existing `multiSelect.toggleMultiSelect` action. It remains reachable
with zero selected tasks and changes to Cancel selection while selection mode
is active. Cancel exits through the existing selection reset; tapping task
cards in selection mode uses the existing toggle behavior, without navigating.
The button has a localized label, an accessible pressed state, and a 44px touch
target. Cover entering, cancelling, and re-entering with zero selected tasks in
phone E2E before testing any color action. No modifier key or long press is
required. The board toolbar below
768px becomes a safe-area-aware compact row with selected count, Color, Actions,
and Clear. Existing move/archive/delete actions remain reachable inside Actions;
desktop keeps its inline actions. This prevents the added control widening the
phone viewport. Preserve desktop preferences across breakpoint changes.

Color opens a short inset `MobilePickerSheet`, the nearest shipped picker
exemplar. Its fixed title gives the selected count and its single scrolling
body contains the seven labeled colors and None. Reuse its dynamic-height bound
and bottom safe-area padding. Desktop uses an anchored menu. Tablet/coarse
pointer controls retain 44px targets and contained menus; desktop ordinary
buttons retain 28px sizing. Dismissal performs no write and returns focus.
Do not add multi-selection to the separate mobile task-switcher drawer in this
package: the phone board is the equivalent multi-task entry point.

Show localized guidance that automatic rules can override the visible manual
color. Reuse existing palette translations; new count, mixed, saving, and error
copy must cover all five locales, with proper plural forms. Tests exercise
keyboard choice, touch choice, persistence, and actual rendered geometry.

## Dependencies

Reuse the existing personal-color backend settings patch, revision guards,
validation limits, and authorization documented in the linked current design.
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md)
continues to own portable settings persistence. No new backend API is proposed.
