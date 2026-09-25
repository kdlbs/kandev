---
status: active
system: ui
created: 2026-09-17
owners:
  - kandev
---

# Composer Focus Hint Requirements

## Overview

UI owns the shared composer's responsive focus guidance, independent of agent
and task lifecycle. Phone users focus the editor by tapping it.

## Requirements

### REQ-UI-COMPOSER-FOCUS-HINT-001: Responsive focus guidance

**Intent:** Keep keyboard guidance from consuming phone editing space.

#### Acceptance criteria

- **AC-UI-COMPOSER-FOCUS-HINT-001.1:** Below 768 CSS pixels, the composer shall
  omit the `/ to focus` hint and the editing space reserved solely for it,
  including when the editor is empty and unfocused.
- **AC-UI-COMPOSER-FOCUS-HINT-001.2:** At widths of 768 CSS pixels or greater,
  the composer shall retain its existing hint eligibility: empty after trimming,
  unfocused, without an active clarification or pending review comments.
- **AC-UI-COMPOSER-FOCUS-HINT-001.3:** Resizing across the phone boundary shall
  update hint visibility and reserved space without losing the draft. Tapping
  the phone editor, typing slash commands, and sending shall remain available.

## Out of scope

Keyboard bindings, tablet policy changes, executor caches, and new copy.

## Implementation Plans

- [Mobile composer focus hint](../../../plans/mobile-composer-focus-hint/plan.md)
