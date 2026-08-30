---
status: active
system: platform
created: 2026-07-19
updated: 2026-08-27
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
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.7:** Existing diff limits remain in force: 10 MiB maximum source file size, 256 KiB maximum per emitted diff representation, and a 2 MiB enrichment threshold per status snapshot. Flattened compatibility diffs and layer-specific diffs all participate in the same snapshot threshold. Because the threshold is checked before enriching each representation, the final accepted representation may preserve the existing overshoot of up to the 256 KiB cap. Existing skip reasons remain unchanged.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.8:** Large changed sets retain every path and its status metadata. Once the total diff budget is exhausted, files that are not enriched retain `budget_exceeded` as their diff skip reason.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.9:** When one path has both index and working-tree changes, the workspace snapshot preserves the staged and unstaged change facets independently, including each facet's status, line totals, rename origin, diff content, and diff skip reason when applicable.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.10:** Changes lists a mixed path in both Staged and Unstaged, with facet-specific status and line totals. Opening either row shows only that layer's diff. Overall changed-file totals continue to count the repository and path once.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.11:** Staging or unstaging a mixed path operates once on its repository-qualified path and the next snapshot removes the consumed facet while preserving any facet that remains.
- **AC-PLATFORM-WORKSPACE-GIT-STATUS-001.12:** Desktop and mobile Changes surfaces expose the same mixed-path sections, facet-specific diff selection, and staging actions without replacing their established platform-native layouts.

## System design

The migrated technical source is split into [part 1](../system-design/workspace-git-status.md).
