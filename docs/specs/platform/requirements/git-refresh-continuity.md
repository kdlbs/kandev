---
status: active
system: platform
created: 2026-10-07
owners:
  - kandev
---

# Git Refresh Continuity Requirements

## Overview

During workspace Git refreshes, users must be able to keep reading eligible existing content and retain their position when that content changes.

## Requirements

### REQ-PLATFORM-GIT-REFRESH-CONTINUITY-001: Git Refresh Display Continuity

**Intent:** Preserve Changes and Review content across refresh phases and keep the reading location stable when ready content changes.

#### Acceptance criteria

- **AC-PLATFORM-GIT-REFRESH-CONTINUITY-001.1:** While refresh is pending or fails, Changes and open diff views shall retain previous counts and patches for files and layers that remain in the same checkout and comparison. The interface shall identify retained data as stale; without a prior patch it shall show loading or unavailable. Retained patches shall not authorize patch mutations or new line-based review actions.
- **AC-PLATFORM-GIT-REFRESH-CONTINUITY-001.2:** When refresh returns identical displayed content, the view shall preserve scroll position, selection, folding, and expanded context. Pending refresh shall not reset these states or replace readable content. Desktop and phone shall provide the same behavior.
- **AC-PLATFORM-GIT-REFRESH-CONTINUITY-001.3:** When refresh changes displayed content, the view shall keep the same file and surviving line at its previous viewport offset. If the line is removed, it shall restore to nearest surviving same-side content, then clamp the previous offset if no line survives. Confirmed file removal or checkout, environment, repository, layer, or comparison replacement shall not reuse unrelated content.

## Out of scope

- Backend Git publication and wire formats.
- Refresh scheduling and retry policy.
- Persistent historical diff storage.

## System design

See the [Git refresh display continuity design](../system-design/git-refresh-continuity.md) and [implementation plan](../../../plans/git-refresh-continuity/plan.md).
