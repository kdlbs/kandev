---
status: draft
system: tasks
created: 2026-10-07
owners:
  - kandev
---

# Turn changed-files requirements

## Overview

Users can inspect repository changes from a completed agent turn directly beneath its final reply.
Tasks owns this contract because durable turn identity binds capture policy, historical content, and transcript placement.
Workspace repository identity, runtime execution, and reusable diff rendering remain dependencies.

The count describes repository changes during the turn. Concurrent writers can contribute changes in a shared checkout.
The card does not claim exclusive agent authorship.

## Terms

- **Turn:** One admitted prompt/response cycle, including interrupted, canceled, or failed outcomes.
- **Checkout:** One attached repository working directory in a specific task environment.
- **Change set:** Retained summary and content availability for the interval between a turn's endpoints.
- **Capture policy:** The initiating settings user's saved preference, resolved when the turn is admitted.
- **Viewer preference:** The current reader's saved preference, which controls transcript visibility.
- **Complete:** All admitted repositories and required entries have usable summaries and historical content.
- **Partial:** Some repositories, entries, or required content are unavailable.

## Requirements

### REQ-TASKS-TURN-CHANGES-001: Persistent capture preference

**Intent:** Users can control capture and presentation without losing retained history.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-001.1:** Missing saved values shall resolve to enabled for new and existing users, including upgrades and frontend hydration.
- **AC-TASKS-TURN-CHANGES-001.2:** Explicit disabled values shall survive unrelated saves, reloads, backend restarts, and upgrades.
- **AC-TASKS-TURN-CHANGES-001.3:** Settings > General shall show the switch beside chat preferences through the existing Save/Discard flow.
  Its label shall be "Show changed files after each turn".
  Its description shall be "Show a file summary and diff beneath completed agent replies."
- **AC-TASKS-TURN-CHANGES-001.4:** Capture policy shall use the initiating user's authoritative settings context for direct, queued, deferred, and automated launches.
  Human assignment and arbitrary connected viewers shall not select that policy.
  Synthetic settings contexts shall retain the established settings-resolution behavior without inventing a human actor.
- **AC-TASKS-TURN-CHANGES-001.5:** A turn shall retain its admitted capture policy throughout completion and restart handling.
  A preference change during that turn shall affect subsequent turns only.
- **AC-TASKS-TURN-CHANGES-001.6:** Disabled subsequent turns shall perform no new checkpoint writes or content exports.
  Saving disabled shall immediately hide the current viewer's transcript cards without deleting any retained history.
- **AC-TASKS-TURN-CHANGES-001.7:** Re-enabling shall restore retained cards and enable subsequent capture.
  It shall not manufacture history for uncaptured turns or delete another user's history.

### REQ-TASKS-TURN-CHANGES-002: Repository interval accuracy

**Intent:** Changes reflect repository content during the turn, regardless of the agent's tool protocol.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-002.1:** Each enabled turn shall use a fresh start and end endpoint for each eligible attached Git checkout.
  Old endpoints, current HEAD, and current workspace content shall not replace missing endpoints.
- **AC-TASKS-TURN-CHANGES-002.2:** Unchanged pre-existing dirty edits and edits between turns shall not appear unless the turn changes their content.
- **AC-TASKS-TURN-CHANGES-002.3:** Shell commands and generators shall contribute tracked and nonignored untracked changes without write-tool events.
  Repeated edits shall count one entry. A complete content revert shall remove its entry.
- **AC-TASKS-TURN-CHANGES-002.4:** Staging and commits during a turn shall not change the historical content interval.
  Capture shall preserve the user's staging contents, index, HEAD, branch, and merge/rebase state.
- **AC-TASKS-TURN-CHANGES-002.5:** Added, deleted, renamed, copied, binary, mode-only, and type changes shall preserve path identity and applicable metadata.
  Tabs, newlines, Unicode, and literal backslashes shall not corrupt selection.
- **AC-TASKS-TURN-CHANGES-002.6:** Matching paths in separate checkouts shall remain separate entries.
  The file count shall count distinct checkout/path pairs, including distinct worktrees of the same repository.
- **AC-TASKS-TURN-CHANGES-002.7:** Sparse exclusions and uninitialized submodules shall not become false deletions.
  Unregistered nested repositories shall not imply recursive content coverage.
  Unsupported shapes shall carry explicit availability reasons.
- **AC-TASKS-TURN-CHANGES-002.8:** Canonical textual additions and deletions shall use raw whitespace semantics.
  Binary and unavailable counts shall remain distinguishable from known zero counts.
- **AC-TASKS-TURN-CHANGES-002.9:** Known overlapping executions in a shared checkout shall remain visible in the retained attribution.
  Unknown external writers shall not produce an exclusivity claim.

### REQ-TASKS-TURN-CHANGES-003: Ordered capture lifecycle

**Intent:** Capture boundaries belong to the exact admitted turn and cannot move into a successor's writes.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-003.1:** Start capture and endpoint persistence shall precede provider dispatch.
  Bounded capture failure shall allow dispatch with affected history marked unavailable.
- **AC-TASKS-TURN-CHANGES-003.2:** Terminal capture shall validate the turn, execution, and prompt generation.
  End capture, summary persistence, and required content preservation shall precede successor dispatch and executor cleanup.
- **AC-TASKS-TURN-CHANGES-003.3:** Duplicate READY/COMPLETE events and delayed events from replaced executions shall not replace accepted endpoints or capture a successor.
- **AC-TASKS-TURN-CHANGES-003.4:** Interrupted, canceled, and failed turns shall retain valid changes when their checkout remains available.
  Stop and Cancel shall remain bounded and responsive.
- **AC-TASKS-TURN-CHANGES-003.5:** After a crash, capture shall require proof of the original boundary and retained content.
  Without proof, the affected endpoint shall become unavailable; a later snapshot shall not impersonate it.
- **AC-TASKS-TURN-CHANGES-003.6:** Capture processing shall remain separate from agent completion, task completion, workflow completion gates, and autopilot policy.
  Terminal pending capture shall not extend the agent-running indicator.

### REQ-TASKS-TURN-CHANGES-004: Durable summaries and historical reads

**Intent:** Later edits and executor cleanup do not silently change historical review.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-004.1:** Reload, transcript pagination, reconnect, later edits, staging, commits, and later turns shall preserve the same historical summary and patch.
- **AC-TASKS-TURN-CHANGES-004.2:** Persisted and live summaries shall contain availability, completeness, repository identities, file kinds, counts, and a durable transcript anchor.
  Successful zero-change capture shall remain distinct from missing or failed capture.
- **AC-TASKS-TURN-CHANGES-004.3:** Historical patches and rendering content shall load on demand through authenticated, task-scoped reads.
  Normal transcript payloads shall exclude full patches and file contents.
- **AC-TASKS-TURN-CHANGES-004.4:** Historical reads shall validate task, session, turn, and checkout relationships.
  Browser requests shall use server-owned identities rather than arbitrary paths or Git refs.
- **AC-TASKS-TURN-CHANGES-004.5:** Required historical content shall survive backend restart, task archive, and ephemeral executor cleanup within the documented retention policy.
  Local, worktree, container, and remote executors shall use their established access boundaries.
- **AC-TASKS-TURN-CHANGES-004.6:** Retention shall bound content growth and remove expired content safely.
  Summary metadata shall survive content expiry until its owning conversation is deleted.
  Expired reads shall return an explicit expired state.
- **AC-TASKS-TURN-CHANGES-004.7:** Missing repositories, comparison failures, truncated enumeration, and missing required content shall prevent a fully ready label.
  Available entries shall remain usable with explicit partial completeness.

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
  Failure shall show "Turn changes unavailable".
  Partial totals shall say "available changes" and identify affected repository states.
- **AC-TASKS-TURN-CHANGES-005.8:** Expired content shall preserve counts and explain why Open diff is unavailable.
  Known shared-checkout overlap shall have a visible attribution note.
- **AC-TASKS-TURN-CHANGES-005.9:** Streaming, pagination, reconnect, and checkpoint updates shall preserve card placement and the reader's scroll position.
  Existing bottom-follow behavior shall remain authoritative.

### REQ-TASKS-TURN-CHANGES-006: Existing desktop and mobile diff surfaces

**Intent:** Historical navigation uses familiar review surfaces with stable content.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-006.1:** Desktop shall reuse the docked diff surface. Phone shall reuse the full-height diff drawer.
  Both shall open the exact turn and optional checkout/file from the card.
- **AC-TASKS-TURN-CHANGES-006.2:** Diff navigation shall offer Current changes, Latest captured turn, and captured historical turns.
  Stable session turn order and useful timestamps shall identify turns.
  An explicit historical selection shall not follow later turns or silently fall back to current changes.
- **AC-TASKS-TURN-CHANGES-006.3:** Header totals, file lists, and patches shall use the same endpoint pair.
  A visible whitespace filter shall change rendering only, preserving canonical totals.
- **AC-TASKS-TURN-CHANGES-006.4:** Current-workspace, committed, PR, and commit-detail navigation shall retain their existing behavior.
  Historical navigation shall not offer rollback, staging, restore, or undo actions.
- **AC-TASKS-TURN-CHANGES-006.5:** Mobile shall keep count and actions readable without document horizontal scrolling.
  Its drawer shall preserve internal scrolling, safe-area clearance, direct file selection, dismissal, and focus return.
- **AC-TASKS-TURN-CHANGES-006.6:** New copy shall use localization and count pluralization.
  Desktop, mobile, keyboard, light/dark themes, and required locale catalogs shall pass focused rendered checks.

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

## Exclusions

Delegated-task cards, delegation tools, completion-gate changes, autopilot changes, and task state-transition changes are excluded.
Workspace rollback, restore, undo, replacement diff applications, and capture after each write-tool call are excluded.
Backfill from current HEAD or workspace content and an off-by-default release toggle are excluded.

## Related artifacts

- [System design](../system-design/turn-changed-files.md)
- [Implementation plan](../../../plans/turn-changed-files/plan.md)
- [Actor attribution](human-assignee.md)
