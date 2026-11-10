---
status: active
system: ui
created: 2026-10-22
owners:
  - kandev
---

# Diff Presentation Preferences

## Overview

Diff presentation is a reusable browser preference shared by file viewers,
Review, and Changes. UI owns this presentation contract independently of task,
repository, and review-mark state.

## Requirements

### REQ-UI-DIFF-PRESENTATION-001: Consistent diff view mode

**Intent:** Reviewers can change split or unified presentation from any existing
mode control and continue using another control without repeating the action.

#### Acceptance criteria

- **AC-UI-DIFF-PRESENTATION-001.1:** Every mounted mode control shall reflect
  the current shared split or unified preference after another control changes it.
  Its next activation shall switch to the opposite mode on the first click.
- **AC-UI-DIFF-PRESENTATION-001.2:** Viewers and controls shall retain an existing
  saved split or unified choice on mount and remount, default to unified without
  a valid saved choice, and follow changes delivered from another browser tab.
- **AC-UI-DIFF-PRESENTATION-001.3:** Existing desktop and phone Review/Changes
  entry points shall share the same mode semantics without changing word-wrap,
  file review marks, or the saved desktop preference through a phone fallback.

## Out of scope

Account or backend settings, new controls or navigation, storage failure policy,
word-wrap policy, diff rendering algorithms, and review-mark persistence.
