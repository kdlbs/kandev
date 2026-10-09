---
status: active
system: ui
created: 2026-09-26
owners:
  - web
---

# Reverse Search Focus Requirements

## Overview

The chat composer offers Ctrl+R message-history search in task chat and Quick
Chat. UI owns the transient keyboard and focus interaction; message history and
session data remain owned by their existing systems. A user who dismisses the
search with Escape needs to continue typing without reaching for the pointer.

## Requirements

### REQ-UI-REVERSE-SEARCH-FOCUS-001: Restore composer focus after Escape

**Intent:** Keep keyboard-only prompt editing continuous after dismissing
message-history search.

#### Acceptance criteria

- **AC-UI-REVERSE-SEARCH-FOCUS-001.1:** When the user opens Ctrl+R search from a
  task chat or Quick Chat composer and presses Escape while the search field is
  focused, the search shall close and the same composer's editor shall receive
  keyboard focus, ready for typing without a pointer action.
- **AC-UI-REVERSE-SEARCH-FOCUS-001.2:** When focus has moved to another control
  inside the same Quick Chat dialog while its search remains open, Escape shall
  close that search and focus that dialog's composer editor. It shall not close
  the Quick Chat dialog or another composer's search.
- **AC-UI-REVERSE-SEARCH-FOCUS-001.3:** Escape dismissal shall leave the existing
  composer draft unchanged and shall not select a history result or submit a
  message. Dismissal by clicking outside or by a viewport change shall retain
  its existing focus behavior.

## Out of scope

- Changing history search results, pagination, selection, or Ctrl+R binding.
- Adding a touch-only entry point or changing the phone overlay layout.

## Implementation Plans

- [Restore chat focus after history Escape](../../../plans/reverse-search-focus/plan.md)
