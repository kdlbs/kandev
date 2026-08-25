---
status: active
system: ui
created: 2026-08-24
owners:
  - kandev
---

# Task Confirmation Warning Hierarchy Requirements

## Overview

Task archive and delete confirmations reuse one in-flight caution. Its visual
treatment must read as supporting warning copy, not compete with the
confirmation title or primary cleanup description. The fine-pointer archive
popover must also give its copy enough room without changing unrelated
confirmation surfaces or changing the originating sidebar row's geometry.

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
  overflow, and existing confirmation actions shall remain reachable with their
  current touch-target geometry.

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

## Out of scope

- Rewriting or re-translating warning text.
- Widening all confirmation popovers globally.
- Changing in-flight detection, archive behavior, delete behavior, API
  contracts, focus handling, Escape handling, safe-area handling, or action
  dimensions.
- Masking sidebar row layout changes with fixed/minimum heights or negative
  offsets.
- Adding animation or redesigning any confirmation surface beyond the shared
  warning density.
