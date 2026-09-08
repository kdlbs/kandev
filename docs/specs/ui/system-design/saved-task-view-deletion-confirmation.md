---
status: current
system: ui
requirements:
  - REQ-UI-SAVED-TASK-VIEW-DELETION-001
---

# Saved Task View Deletion Confirmation System Design

## Purpose and boundaries

This design adds one reusable confirmation presentation contract to all
Kandev-owned saved task view and saved integration query delete actions. The UI
system owns the pointer-responsive surface, copy hierarchy, accessibility, and
overlay containment. Surface-specific stores and hooks remain authoritative for
whether a view can be deleted and what happens after deletion.

No backend endpoint, settings schema, saved-view model, provider operation, or
persistence queue changes. Confirmation state is ephemeral and never becomes a
second saved-view source of truth.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-SAVED-TASK-VIEW-DELETION-001` | [Shared confirmation shell](#shared-confirmation-shell), [Surface adapters](#surface-adapters), [Control flow](#control-flow), and [Responsive and accessibility behavior](#responsive-and-accessibility-behavior) |

## Surface inventory

| Owning surface | Existing delete entry | Existing mutation boundary |
| --- | --- | --- |
| Task sidebar | Active-view actions in `ViewHeaderRow` | `deleteSidebarView` |
| Threads | `EditorActions` in the desktop editor or mobile drawer | `deleteThreadView` |
| GitHub, GitLab, and Azure DevOps shared scope bar | Saved-query row in `IntegrationScopeBar` | Each wrapper's `onDeleteSaved` callback |
| GitHub mobile filter sheet | Saved-query row in `PresetsSidebar` | GitHub `onDeleteSaved` callback |
| GitLab mobile filter sheet | Saved-query row in `PresetsSidebar` | GitLab `onDeleteSaved` callback |
| Jira ticket list | Custom row in `ViewsDropdown` | `useSavedViews.removeView` through `onDeleteView` |

The shared scope bar is a Kandev host component. A plugin that uses that host
component receives the confirmation without defining another contract. UI that
exists only inside an external plugin repository is outside this design.

## Components and responsibilities

### Shared confirmation shell

`apps/web/components/confirmation/saved-task-view-delete-confirmation.tsx`
adds a small presentation-only adapter over `ActionConfirmPopover` and
`InlineConfirmActions`. Its stable input is the target visible to the user, not
the surface's complete saved-view model:

```ts
type SavedTaskViewDeleteTarget = {
  id: string;
  label: string;
};

type SavedTaskViewDeleteConfirmationProps = {
  target: SavedTaskViewDeleteTarget;
  presentation: "popover" | "inline";
  open: boolean;
  anchorRef: RefObject<HTMLElement | null>;
  focusBoundaryRef?: RefObject<HTMLElement | null>;
  confirmDisabled?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (id: string) => void | Promise<void>;
};
```

The component owns localized title, consequence, Cancel, and destructive Delete
content. It passes the target ID back only after explicit confirmation. It does
not decide eligibility, select a view, mutate a store, close a parent surface,
show a persistence error, or retry a failed write.

For `popover`, the component enables `ActionConfirmPopover`'s confirmation
boundary and accepts the owning surface as its focus boundary. A shared target
predicate recognizes that boundary so nested Radix menu and popover parents can
ignore focus/interact-outside events originating inside the confirmation. This
follows the existing saved-layout deletion pattern instead of adding another
overlay primitive.

For `inline`, the caller mounts the component where the saved row or editor
actions normally render. `InlineConfirmActions` uses touch density, focuses
Cancel on mount, stops row activation events, handles Escape locally, and
closes before queueing the delete callback.

### Surface adapters

Each owning surface keeps at most one local delete candidate plus the initiating
element reference. The candidate contains the stable ID and the exact localized
or user-authored label already rendered by that surface.

- `SidebarFilterPopover` coordinates the active sidebar view candidate. The
  fine-pointer popover is anchored to `view-delete-button`; the drawer branch
  replaces the view-header action region with inline confirmation.
- `ThreadsViewControls` coordinates the active Threads view candidate and
  carries it through `ThreadsViewEditor` to `EditorActions`. Confirm retains the
  current desktop/mobile behavior of closing the settings surface after the
  existing delete callback is invoked.
- `SavedMenu` in `presets-scope-bar-base.tsx` coordinates saved-query rows for
  GitHub, GitLab, and Azure DevOps. The shared dropdown stays mounted around a
  fine-pointer confirmation; its coarse-pointer row becomes the inline surface.
- GitHub and GitLab `PresetsSidebar` components keep their mobile-sheet
  candidate local and replace only that saved-query row. Their delete actions
  remain separate from row selection and GitHub's default marker.
- Jira `ViewsDropdown` coordinates custom saved-view rows. Built-in rows never
  create a candidate. Its parent popover uses the same confirmation-boundary
  handling as the shared scope-bar menu.

The task sidebar, Threads, integration scope bar, and Jira adapters read pointer
mode from `useResponsiveBreakpoint`. GitHub and GitLab mobile sidebars always
use the coarse-pointer inline presentation because their Kandev-owned mount is
the phone filter sheet.

## Data and contracts

The confirmation target is transient component state. Its `id` remains opaque;
its `label` is display content only. The shell never serializes either value,
sends a network request, or derives deletion eligibility.

Shared copy lives in the `common` namespace with a `{{name}}` placeholder for
the target title, a consequence/reassurance description, and a generic Delete
action. English, Portuguese, Simplified Chinese, both Traditional Chinese
catalogs, and the generated pseudo locale change together. The title and
destructive action accessible name include the same visible target label.

Existing trigger test IDs remain stable. The shared shell adds stable IDs for
the confirmation container and final destructive action; surface tests scope
those IDs to the owning row or overlay when hidden responsive variants are also
mounted.

## Control flow

1. The owning surface evaluates its existing eligibility and pending rules.
2. Activating an available delete trigger records the target and anchor. It
   prevents row selection or default-toggle events and performs no mutation.
3. A fine pointer opens the anchored confirmation. A coarse pointer replaces
   the relevant row or action region inside its existing parent surface.
4. Cancel, Escape, parent dismissal, target disappearance, view switching, or
   unmount clears the candidate without invoking the delete callback.
5. Explicit Delete first closes the confirmation, then invokes the owning
   callback once with the captured target ID.
6. The existing surface action applies its current fallback, persistence,
   optimistic update, rollback, error, and parent-close behavior.

Capturing the target ID prevents a later active-view change from redirecting a
confirmation to another view. The connected-anchor guard already provided by
`ActionConfirmPopover` prevents a stale fine-pointer target from being deleted.
The inline branch disappears with its keyed row and therefore also fails closed.

## Responsive and accessibility behavior

Fine-pointer triggers retain their compact row geometry. The confirmation is a
viewport-contained non-modal popover anchored to the trigger. Its parent
dropdown or popover stays mounted while focus moves between the two surfaces,
and Cancel restores focus to the connected trigger.

Coarse-pointer triggers and both confirmation actions use a minimum 44-pixel
hit area and are visible without hover. Inline content wraps long translated
copy and names without widening its parent. It remains inside the current
drawer, sheet, dropdown bottom sheet, or editor, so no second modal navigation
layer is introduced. The existing parent remains the sole vertical scroll
owner and keeps its current safe-area padding.

The confirmation is exposed as a named group or dialog. Cancel receives initial
focus. Escape is a cancellation path, the destructive action is not an implicit
Enter default, and pointer/key events within the confirmation do not activate
the saved-view row beneath it.

The GitLab and Jira coarse-pointer delete triggers become explicitly visible
and touch reachable; their fine-pointer variants may keep hover disclosure but
must also reveal on keyboard focus.

## Failure and recovery

Closing or cancelling a confirmation is always a local no-op. If live data
removes the target or disconnects its anchor, the shell closes without calling
the stale callback.

After confirmation, each owner keeps its current failure contract. Backend
user-settings failures continue to use existing optimistic rollback and sync
feedback. Integration settings failures continue to use their existing local
or provider-specific feedback. The confirmation shell neither suppresses nor
duplicates those messages and adds no new retry state.

When an adjacent GitHub default mutation is pending, the existing disabled
delete rule remains authoritative. If pending state begins while confirmation
is open, the destructive action is disabled while Cancel remains available.

## Persistence

Confirmation open state, target, and anchor are never persisted. Sidebar and
Threads views continue through backend user settings. Integration saved queries
continue through their existing workspace or portable settings owners. There
is no browser-storage fallback and no schema migration.

## Security

The shell can act only on a delete callback already provided by an authorized
owning surface. It does not fetch by target ID, expand provider permissions, or
log target names. Confirmation copy makes clear that the operation does not
delete tasks or connected-service records.

## Observability and verification

No production log or metric is added for local confirmation state. Component
tests verify that opening and every cancellation path perform no mutation, the
named destructive action calls the captured ID once, disconnected targets fail
closed, and focus/event isolation is preserved.

Surface tests cover each adapter, including built-in/last-view guards, GitHub
default-mutation disabling, and delete versus row-selection isolation.
Playwright covers the real Radix nesting on desktop and the task-sidebar,
Threads, GitHub, GitLab, Azure DevOps, and Jira phone compositions. Mobile
checks inspect 44-pixel actions, one overlay/scroll owner, reachable safe-area
actions, long-name containment, and zero document-level horizontal overflow.

## Related decisions

- [Backend-owned Portable User Settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
- [Surface-owned Saved Task Views](../../../decisions/2026-08-31-surface-owned-saved-task-views.md)
- [Task Confirmation Surface Requirements](../requirements/confirmation-warning-hierarchy.md)
- [Threads Saved Views](threads-saved-views.md)
