---
status: draft
system: tasks
created: 2026-10-07
owners:
  - kandev
---

# Turn changed-files presentation requirements

## Overview

These requirements own how captured turn changes appear and how readers
navigate historical diffs. Capture identity, comparison, and retained content
are defined in [turn changed files](turn-changed-files.md).

### REQ-TASKS-TURN-CHANGES-005: Transcript card

**Intent:** Readers can inspect each terminal turn without opening collapsed tools.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-005.1:** One card shall appear beneath the final assistant reply with the matching durable turn identity, outside tool groups.
  Without a final reply, a stable terminal-turn row shall expose captured changes.
- **AC-TASKS-TURN-CHANGES-005.2:** A ready card shall show a pluralized file count, green plus additions, red minus deletions, and a labeled Open diff action.
  Unknown textual counts shall not appear as zero. Status labels or icons shall supplement color.
- **AC-TASKS-TURN-CHANGES-005.3:** The tree shall show root entries and initially collapsed folders with aggregate counts.
  Multiple contributing checkouts shall have distinct repository roots.
  File rows shall show names, actual kinds, rename details, binary labels, and available counts.
- **AC-TASKS-TURN-CHANGES-005.4:** Folder toggles shall expand without opening the diff.
  Expand/collapse all shall appear when folders exist.
  Header navigation shall open all changes; file navigation shall select the exact checkout and path.
- **AC-TASKS-TURN-CHANGES-005.5:** Expansion and file selection shall persist per session/turn across panel switches and reloads.
  Keyboard navigation, visible focus, accessible expansion names, and full-path disclosure shall work.
  Phone and coarse-pointer actions shall have touch targets of at least 44 pixels.
- **AC-TASKS-TURN-CHANGES-005.6:** Active turns, ready zero-change turns, disabled visibility, uncaptured turns, and turns without eligible Git checkouts shall have no card.
- **AC-TASKS-TURN-CHANGES-005.7:** Terminal pending capture shall show "Preparing changes" independently of the agent-running indicator.
  Settled summaries with zero recorded file entries shall have no card, including failed/unavailable captures.
  Retained failure status shall remain distinct from a successful zero-change capture and available through explicit history selection.
  Partial totals shall say "available changes" and identify affected repository states.
  An unavailable history comparison is an exception: it shall use the existing unavailable state even without recorded file entries.
- **AC-TASKS-TURN-CHANGES-005.8:** Expired content shall preserve counts and explain why Open diff is unavailable.
  Known shared-checkout overlap shall have a visible attribution note.
- **AC-TASKS-TURN-CHANGES-005.9:** Streaming, pagination, reconnect, and checkpoint updates shall preserve card placement and the reader's scroll position.
  Existing bottom-follow behavior shall remain authoritative.
  When bottom-follow owns the viewport, delayed summaries and file rows shall stay in view after completion and reload.
  Manual reading, disabled auto-scroll, and explicit message navigation shall retain their positions.

- **AC-TASKS-TURN-CHANGES-005.10:** File and folder line counts shall use the same addition and deletion colors as the Changes panel. File kind labels shall use matching status colors, including amber for modified files. Text and signs shall remain visible independently of color.
- **AC-TASKS-TURN-CHANGES-005.11:** File rows shall use the same filename-specific icons and unknown-file fallback as the file tree.
- **AC-TASKS-TURN-CHANGES-005.12:** A card shall leave at least 16 CSS pixels before the next message on desktop and phone. Expanded rows, long paths, and status text shall remain readable without document horizontal overflow.
- **AC-TASKS-TURN-CHANGES-005.13:** History correction shall preserve the existing card layout, controls, and styling.
  Normal cards shall add no history-adjustment or export-progress explanations.
  Legacy counts shall use repository wording in the existing count position.
  Unavailable, loading, and partial results shall reuse existing status areas; detailed reasons belong in the selected historical diff.

### REQ-TASKS-TURN-CHANGES-006: Existing desktop and mobile diff surfaces

**Intent:** Historical navigation uses familiar review surfaces with stable content.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-006.1:** Desktop shall reuse the docked diff surface. Phone shall reuse the full-height diff drawer.
  Both shall open the exact turn and optional checkout/file from the card.
- **AC-TASKS-TURN-CHANGES-006.2:** Diff navigation shall offer Current changes, Latest captured turn, and captured historical turns.
  Stable session turn order and useful timestamps shall identify turns.
  Turn and file selectors shall match adjacent desktop control text and keep their chevrons inset from the edge.
  Phone and touch controls shall retain their minimum targets and input anti-zoom font rule.
  An explicit historical selection shall not follow later turns or silently fall back to current changes.
- **AC-TASKS-TURN-CHANGES-006.3:** Header totals, file lists, and patches shall use the same accepted comparison pair.
  History adjustment shall not replace the original captured endpoints.
  A visible whitespace filter shall change rendering only, preserving canonical totals.
- **AC-TASKS-TURN-CHANGES-006.4:** Current-workspace, committed, PR, and commit-detail navigation shall retain their existing behavior.
  Historical navigation shall not offer rollback, staging, restore, or undo actions.
- **AC-TASKS-TURN-CHANGES-006.5:** Mobile shall keep count and actions readable without document horizontal scrolling.
  Its drawer shall preserve internal scrolling, safe-area clearance, direct file selection, dismissal, and focus return.
- **AC-TASKS-TURN-CHANGES-006.6:** New copy shall use localization and count pluralization.
  Desktop, mobile, keyboard, light/dark themes, and required locale catalogs shall pass focused rendered checks.
- **AC-TASKS-TURN-CHANGES-006.7:** Historical diff navigation shall preserve the selected comparison and its availability after reload and reconnect.
  Detailed reasons shall use the existing selected-diff status area.
  An unavailable comparison shall not fall back to current changes or a different turn.

### REQ-TASKS-TURN-CHANGES-007: Bounded operation and measured evidence

**Intent:** Capture has explicit limits and reports what those limits omit.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-007.1:** Capture, comparison, command output, retries, concurrency, and retained content shall have explicit bounds.
  Exceeding a limit shall preserve failure, partial, binary, and truncation states.
- **AC-TASKS-TURN-CHANGES-007.2:** Capture shall reuse immutable Git content without copying the whole checkout for every endpoint.
  Summary delivery shall remain separate from full-patch loading.
- **AC-TASKS-TURN-CHANGES-007.3:** Validation shall measure small repositories, large file counts, and large changed files.
  Slow storage shall be measured where available, with its absence reported explicitly.
- **AC-TASKS-TURN-CHANGES-007.4:** Measurements shall record environment, capture/comparison duration, changed bytes, failures, and retention growth.
  Operational metric labels shall exclude task, turn, session, path, and user identities.

- **AC-TASKS-TURN-CHANGES-007.5:** Full tracked-file inventories shall not be rejected by the changed-file enumeration count limit. Within existing byte/time bounds, a valid repository with more than 30,000 tracked files shall support zero-change and one-file-change capture and comparison.
- **AC-TASKS-TURN-CHANGES-007.6:** Content preservation shall bound each batch, total attempts, concurrency, and retained bytes.
  Deadline exhaustion shall remain distinguishable from missing objects, size limits, transport failure, and cleanup interruption.

## Exclusions

Delegated-task cards, delegation tools, completion-gate changes, autopilot changes, and task state-transition changes are excluded.
Workspace rollback, restore, undo, replacement diff applications, and capture after each write-tool call are excluded.
Backfill from current HEAD or workspace content and an off-by-default release toggle are excluded.

## Related artifacts

- [System design](../system-design/turn-changed-files.md)
- [Subscription reconciliation design](../system-design/turn-changed-files-subscription.md)
- [History integrity design](../system-design/turn-change-history-integrity.md)
- [Implementation plan](../../../plans/turn-changed-files/plan.md)
- [Subscription reconciliation plan](../../../plans/turn-changes-subscription-reconciliation/plan.md)
- [Preservation design](../system-design/turn-change-preservation.md)
- [Unified compaction contract](../../system-page/requirements/tool-payload-retention.md)
- [Preservation retry plan](../../../plans/turn-change-preservation-retry/plan.md)
- [History integrity fix plan](../../../plans/turn-change-history-integrity/plan.md)
- [Content requirements](turn-changed-files.md)
- [Actor attribution](human-assignee.md)

Presentation refinement: [plan](../../../plans/turn-changed-files-presentation/plan.md).
