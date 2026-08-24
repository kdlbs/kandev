---
status: active
system: platform
created: 2026-07-19
updated: 2026-08-19
owners:
  - kandev
---
# Workspace Git Status Requirements

## Overview

Users opening or focusing Changes and Review need a current workspace snapshot without a large generated or untracked tree monopolizing agentctl. Repeated requests for the same repository must not amplify expensive Git and filesystem work, and the initial session-hydration path must remain within its two-second live-status budget by falling back when necessary.

## Requirements

### REQ-PLATFORM-WORKSPACE-GIT-STATUS-001: Workspace Git Status

**Intent:** Users opening or focusing Changes and Review need a current workspace snapshot without a large generated or untracked tree monopolizing agentctl. Repeated requests for the same repository must not amplify expensive Git and filesystem work, and the initial session-hydration path must remain within its two-second live-status budget by falling back when necessary.

#### Acceptance criteria

- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.1:** Cached reads return the latest workspace-tracker snapshot. When no cached snapshot exists, the tracker performs a live observation.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.2:** Fresh reads observe the live worktree and do not themselves replace the polling cache.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.3:** Overlapping live observations for the same repository share one underlying observation. Different repositories in a multi-repository task may still be observed in parallel.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.4:** Every non-cancelled caller receives the same completed snapshot or error from a shared observation. A caller whose own context is cancelled returns promptly without cancelling or otherwise poisoning the result for other callers.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.5:** Tracker shutdown or the bounded shared-observation deadline cancels the underlying work. Cancelled work does not publish or cache a partial snapshot.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.6:** After Git output is parsed, changed-file and synthetic untracked-diff enrichment performs work proportional to the number of changed entries plus the bounded content processed.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7:** Existing diff limits remain in force: 10 MiB maximum source file size, 256 KiB maximum emitted diff per file, and a 2 MiB enrichment threshold per status snapshot. Because the threshold is checked before enriching each file, the final accepted file may preserve the existing overshoot of up to the 256 KiB per-file cap. Existing skip reasons remain unchanged.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8:** Large changed sets retain every path and its status metadata. Once the total diff budget is exhausted, files that are not enriched retain `budget_exceeded` as their diff skip reason.

## System design

The migrated technical source is split into [part 1](../system-design/workspace-git-status.md).
