---
status: active
system: ui
created: 2026-08-24
owners:
  - kandev
---

# Task Confirmation Surface Requirements

## Overview

Task archive and delete confirmations reuse one in-flight caution. Its visual
treatment must read as supporting warning copy, not compete with the
confirmation title or primary cleanup description. The fine-pointer archive
popover must also give its copy enough room without changing unrelated
confirmation surfaces or changing the originating sidebar row's geometry.

Task archive and delete confirmations also explain what happens to the task's
runtime environment. That copy must separate the requested task action from
its cleanup effects and from resources that remain safe, instead of presenting
every sentence as an equal paragraph.

## Terminology

- **Still-working warning:** The localized caution shown when the task or one
  of the selected tasks has generating or background activity.

## Requirements

### REQ-TASKS-CONFIRMATION-WARNING-001: Compact still-working warning

**Intent:** Make in-flight caution easy to scan while preserving its semantic
meaning and the existing archive and delete workflows.

#### Acceptance criteria

- **AC-TASKS-CONFIRMATION-WARNING-001.1:** When an archive or delete
  confirmation renders for an in-flight task, the still-working warning shall
  use compact secondary typography with readable line height and deliberate
  short-text wrapping, and its icon, spacing, and padding shall remain optically
  balanced with that typography.
- **AC-TASKS-CONFIRMATION-WARNING-001.2:** When the warning renders in a full
  dialog, desktop archive popover, archive inline confirmation, or shared delete
  dialog, it shall preserve its localized text, `role=alert`, restrained yellow
  semantic treatment, and existing in-flight visibility conditions.
- **AC-TASKS-CONFIRMATION-WARNING-001.3:** At desktop and phone confirmation
  widths, the warning shall remain contained without document-level horizontal
  overflow, and existing confirmation actions shall remain reachable. Full
  task cleanup dialogs shall follow the dedicated action contract in
  `AC-UI-TASK-CLEANUP-CONFIRMATION-001.5`; other action geometry shall remain
  unchanged.

### REQ-TASKS-CONFIRMATION-SURFACE-002: Stable fine-pointer archive surface

**Intent:** Give the fine-pointer archive confirmation readable width while
keeping the source sidebar row stable and preserving the intentional coarse
pointer inline confirmation behavior.

#### Acceptance criteria

- **AC-TASKS-CONFIRMATION-SURFACE-002.1:** The fine-pointer archive popover
  shall use an archive-only width opt-in wider than the existing 256px default
  (targeting a modest `w-72` contract after rendered inspection), with a
  viewport-aware maximum width. Unrelated `ActionConfirmPopover` consumers
  shall retain the existing `w-64` default.
- **AC-TASKS-CONFIRMATION-SURFACE-002.2:** Opening and cancelling the
  fine-pointer archive popover shall leave the originating sidebar task row's
  `getBoundingClientRect().height` stable within normal subpixel precision.
  The implementation shall remove the source of any extra flex line rather
  than mask it with fixed or minimum row height, negative margins, or a
  tolerance-only assertion.
- **AC-TASKS-CONFIRMATION-SURFACE-002.3:** The fine-pointer popover shall
  preserve its existing anchor, focus-return boundary, confirmation callbacks,
  and compact desktop/tablet viewport containment. Coarse-pointer inline
  confirmation may continue to expand its row and shall retain its existing
  44px action geometry and zero-overflow behavior.

### REQ-UI-TASK-CLEANUP-CONFIRMATION-001: Scannable task cleanup confirmation

**Intent:** Make task archive and delete consequences easy to distinguish and
operate without changing task cleanup behavior.

**User story:** As a user archiving or deleting a task, I want the confirmation
to distinguish the task action, affected runtime resources, and protected
repository resources, so that I can understand the consequence before acting.

#### Acceptance criteria

- **AC-UI-TASK-CLEANUP-CONFIRMATION-001.1:** A single-task confirmation shall
  identify the named task and state the requested archive or delete outcome
  directly. Delete shall state that the action cannot be undone without adding
  a redundant generic confirmation question.
- **AC-UI-TASK-CLEANUP-CONFIRMATION-001.2:** Cleanup effects shall appear as one
  scannable semantic group. Information about repository files, branches, or
  folders that remain untouched shall read as supporting reassurance rather
  than as another destructive effect.
- **AC-UI-TASK-CLEANUP-CONFIRMATION-001.3:** Single and bulk confirmations shall
  preserve the current executor-specific distinctions for local, worktree,
  local Docker, remote Docker, Sprites, SSH, unknown executors, and running
  agent sessions in every supported locale.
- **AC-UI-TASK-CLEANUP-CONFIRMATION-001.4:** Full dialogs, the archive popover,
  and the archive inline confirmation shall derive their cleanup content from
  the same structured model. Each surface may use its established density, but
  shall preserve the same effects, reassurance, and order.
- **AC-UI-TASK-CLEANUP-CONFIRMATION-001.5:** At phone widths, full task cleanup
  dialogs shall remain centered and inset within the visual viewport, create no
  document horizontal overflow, keep overflowing content reachable, and expose
  stacked full-width actions at least 44 CSS px high. Tablet and desktop shall
  retain compact row actions.
- **AC-UI-TASK-CLEANUP-CONFIRMATION-001.6:** Delete shall use the design
  system's semantic destructive action treatment without a competing primary
  treatment. Archive shall retain its non-destructive action treatment.
- **AC-UI-TASK-CLEANUP-CONFIRMATION-001.7:** Cancel, dismiss, focus, Escape,
  Enter default-action behavior, loading state, cascade selection, in-flight
  warning visibility, and single/bulk archive and delete callbacks shall remain
  unchanged.

## Out of scope

- Rewriting or re-translating the still-working warning.
- Widening all confirmation popovers globally.
- Changing in-flight detection, archive behavior, delete behavior, API
  contracts, focus handling, Escape handling, or safe-area handling.
- Changing compact archive popover/inline or unrelated dialog action
  dimensions.
- Replacing the centered task alerts with a drawer, sheet, or new navigation
  flow.
- Masking sidebar row layout changes with fixed/minimum heights or negative
  offsets.
- Adding animation or redesigning any confirmation surface beyond the shared
  warning density and the task cleanup hierarchy above.
