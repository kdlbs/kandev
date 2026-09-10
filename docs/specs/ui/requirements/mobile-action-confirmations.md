---
status: active
system: ui
created: 2026-09-10
owners:
  - kandev
---

# Mobile Action Confirmation Requirements

## Overview

Phone confirmations give one decision its own focused surface. A confirmation
inside an open bottom sheet becomes a step in that sheet; a confirmation from
a page opens a compact bottom sheet. Lists retain their normal row geometry.

UI owns this reusable presentation, accessibility, and dismissal contract.
Tasks, saved views, terminals, integrations, and settings retain their existing
eligibility, mutations, persistence, feedback, and recovery contracts.

This replaces phone inline presentation in
[task cleanup confirmations](confirmation-warning-hierarchy.md),
[saved task view deletion](saved-task-view-deletion-confirmation.md),
[terminal close](terminal-close-feedback.md), and
[session deletion](session-tab-delete-feedback.md). Those documents continue
to own their action-specific consequences and non-phone behavior.

## Terminology

- **Phone:** A viewport narrower than 768 CSS pixels, regardless of pointer.
- **Hosted confirmation:** A confirmation step inside the already-open surface.
- **Standalone confirmation:** A compact bottom sheet opened from a page after
  its transient action menu has closed.
- **Origin:** The task, list, picker, or editor and target that initiated the
  confirmation, including its current selection and scroll position.

## Requirements

### REQ-UI-MOBILE-CONFIRMATION-001: Focused phone surfaces

**Intent:** Make repeated confirmation interactions comfortable on a phone
without changing the action being confirmed.

#### Acceptance criteria

- **AC-UI-MOBILE-CONFIRMATION-001.1:** On a phone, task archive and existing
  inline confirmations in task pickers, file/content controls, saved artifacts,
  and settings/integrations shall show a dedicated confirmation surface rather
  than expand or replace the initiating row's content.
- **AC-UI-MOBILE-CONFIRMATION-001.2:** When the origin is an open bottom sheet,
  confirmation shall replace its visible content within that same sheet. Only
  the confirmation shall be interactive or exposed to assistive technology;
  the underlying list or form shall retain its state.
- **AC-UI-MOBILE-CONFIRMATION-001.3:** When the origin is a page, confirmation
  shall open a compact inset bottom sheet after any transient initiating menu
  closes. The page's selection and scroll position shall remain unchanged.
- **AC-UI-MOBILE-CONFIRMATION-001.4:** Cancel and the confirmation's Back control
  shall return to the origin without changing its filters, selection, draft
  values, or scroll position, except for changes caused by live domain data.
  A surviving initiating control shall receive focus; otherwise a visible
  control in the origin shall receive focus.
- **AC-UI-MOBILE-CONFIRMATION-001.5:** Dismissing the whole sheet or navigating
  away shall cancel the unsubmitted decision. Reopening the origin shall not
  resurrect the confirmation. Browser Back shall retain normal navigation
  behavior; confirmation shall not create browser-history entries.
- **AC-UI-MOBILE-CONFIRMATION-001.6:** At widths of 768 CSS pixels or greater,
  current desktop/tablet confirmation composition and pointer rules shall
  remain unchanged. A viewport change across that boundary shall dismiss an
  unsubmitted confirmation without dispatching its action.
- **AC-UI-MOBILE-CONFIRMATION-001.7:** Existing full task-delete, discard-consent,
  type-to-confirm, and system maintenance dialogs shall retain their current
  presentation. An inline decision inside an existing centered form dialog
  shall become a focused step in that dialog, preserving the form on Cancel
  and opening no additional modal surface.

### REQ-UI-MOBILE-CONFIRMATION-002: Readable content and reachable actions

**Intent:** Let a phone user identify the target, understand the consequences,
and operate both actions without scrolling the page.

#### Acceptance criteria

- **AC-UI-MOBILE-CONFIRMATION-002.1:** Each confirmation shall expose a visible
  action title and target identity, including a count for bulk actions. Task
  archive shall retain executor-specific cleanup effects, supporting notes,
  in-flight warnings, and optional subtask selection from its shared model.
- **AC-UI-MOBILE-CONFIRMATION-002.2:** Consequences shall use readable body text
  distinct from supporting reassurance. Neither target names nor required
  consequences shall be hidden in a tooltip or optional disclosure. Long
  names shall wrap without introducing horizontal page overflow.
- **AC-UI-MOBILE-CONFIRMATION-002.3:** The primary action and Cancel shall be
  vertically stacked, full-width, and at least 44 CSS pixels high, targeting
  48 pixels. Back and selectable option labels shall have at least 44-pixel
  touch targets. Archive shall use its existing non-destructive treatment;
  deletion shall use semantic destructive treatment.
- **AC-UI-MOBILE-CONFIRMATION-002.4:** Short standalone confirmations shall fit
  their content. Longer confirmations shall remain inside the dynamic visual
  viewport, with one active internal body scroller and visible title/actions.
  Bottom actions shall clear the device safe area, including in landscape.
- **AC-UI-MOBILE-CONFIRMATION-002.5:** The active surface shall have an accessible
  name and description. Initial action focus shall be on Cancel. Escape shall
  cancel only the confirmation step; it shall not also dismiss its host. Plain
  Enter shall not implicitly execute a destructive phone action, and normal
  focused-button keyboard activation shall remain available.
- **AC-UI-MOBILE-CONFIRMATION-002.6:** Labels, descriptions, warnings, accessible
  names, and count-dependent copy shall use the active supported locale.
  Pseudo-locale and longer translations shall retain readable hierarchy and
  reachable controls. Light and dark themes shall retain readable contrast.
- **AC-UI-MOBILE-CONFIRMATION-002.7:** A surface transition shall never activate
  the new action from the gesture that opened it. Reduced-motion preferences
  shall suppress additional content-transition motion without delaying input.

### REQ-UI-MOBILE-CONFIRMATION-003: Stable action ownership

**Intent:** Change presentation while preserving domain behavior and preventing
stale or duplicate actions during overlay transitions.

#### Acceptance criteria

- **AC-UI-MOBILE-CONFIRMATION-003.1:** Opening, cancelling, dismissing, or changing
  presentation shall perform no domain mutation. Explicit confirmation shall
  invoke the captured target's existing action once, including rapid repeated
  taps or keyboard activation.
- **AC-UI-MOBILE-CONFIRMATION-003.2:** A removed or ineligible target, origin
  unmount, or context change shall invalidate an unsubmitted request. Late
  asynchronous results shall neither reopen it nor redirect it to another
  task, session, file, or saved artifact.
- **AC-UI-MOBILE-CONFIRMATION-003.3:** Archive confirmation shall preserve the
  user's confirmation preference, default unchecked cascade selection,
  descendant classification, bulk behavior, and existing post-archive
  navigation. Disabled confirmation shall bypass every new surface and shall
  not implicitly include subtasks.
- **AC-UI-MOBILE-CONFIRMATION-003.4:** Archive shall expose no enabled submit
  action while descendant classification is pending. Its known phone surface
  shall remain stable while results arrive, and classification failure shall
  retain the existing conservative confirmation behavior without inventing a
  descendant count or silently enabling cascade.
- **AC-UI-MOBILE-CONFIRMATION-003.5:** Each action shall preserve its existing
  pending, close timing, error, rollback, and retry behavior. Terminal close
  shall still remove local UI immediately and complete teardown in the
  background. Presentation shall add no common transport spinner or toast.
- **AC-UI-MOBILE-CONFIRMATION-003.6:** Existing eligibility and confirmation
  controls shall remain effective, including protected/built-in saved views,
  disabled actions, primary/only-session warnings, and typed/discard consent
  in the retained full dialogs. Presentation shall grant no new capability.

## Out of scope

- Undo, cleanup/recovery changes, new confirmation preferences, API changes,
  or new persistence and telemetry.
- Redesigning desktop/tablet surfaces, general mobile menus, navigation, or
  unrelated full dialogs.
- Changing destructive text-entry tokens or expanding plugin APIs; bespoke
  external-plugin UI remains owned by its repository.
- Forcing every action to wait for transport completion or changing existing
  failure feedback into a shared retry flow.

## Design and delivery

- [System design](../system-design/mobile-action-confirmations.md)
- [Implementation plan](../../../plans/mobile-action-confirmations/plan.md)
