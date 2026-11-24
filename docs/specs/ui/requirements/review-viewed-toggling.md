---
status: active
system: ui
created: 2026-10-20
owners:
  - kandev
---

# Review viewed toggling

## Overview

Reviewers must be able to clear a viewed checkbox without navigating back to
that file. UI owns this reusable review interaction; session review persistence,
repository identity, and patch freshness remain with their existing owners.

## Requirements

### REQ-UI-REVIEW-VIEWED-001: Manual clearing and subsequent automatic review

**Intent:** Keep manual clearing useful while preserving automatic review on a
subsequent reading pass.

#### Acceptance criteria

- **AC-UI-REVIEW-VIEWED-001.1:** When a user clears a viewed file while its start
  is above the review viewport, the file shall remain unviewed at that position,
  including after unrelated rerenders and further downward scrolling that does
  not revisit that file.
- **AC-UI-REVIEW-VIEWED-001.2:** With automatic marking enabled, when the user
  scrolls back until that file's start is in or below the viewport and then scrolls
  downward past it again, the file shall become viewed again. Manual scroll
  input that cancels a pending reading-position restoration shall authorize
  that subsequent reading pass; restoration cleanup shall not revoke it.
- **AC-UI-REVIEW-VIEWED-001.3:** Initial rendering, observer reconnection,
  programmatic navigation, and layout movement shall not themselves undo a
  manual clearing. Existing initial-load and programmatic-jump suppression
  shall remain effective.
- **AC-UI-REVIEW-VIEWED-001.4:** With automatic marking disabled, manual clearing
  shall remain effective across scrolling. Stale, pending, and otherwise
  ineligible diff content shall not acquire a new automatic review mark.
- **AC-UI-REVIEW-VIEWED-001.5:** Desktop file-list controls and phone file-header
  controls shall expose the same transitions. Review counts and other controls
  for the same file shall agree, and files in different repositories or change
  layers shall remain independent.

## Out of scope

New settings, permanent opt-out per file, storage/API changes, review-dialog
redesign, and changes to patch eligibility or review hash persistence.
