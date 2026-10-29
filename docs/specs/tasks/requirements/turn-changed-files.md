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

The count describes turn changes after excluding provable upstream integration changes.
Concurrent writers can still contribute changes in a shared checkout. The card does not claim exclusive agent authorship.
Older captures retain an explicit repository-snapshot label because they lack the evidence needed for this separation.

## Terms

- **Turn:** One admitted prompt/response cycle, including interrupted, canceled, or failed outcomes.
- **Checkout:** One attached repository working directory in a specific task environment.
- **Change set:** Retained summary and content availability for the interval between a turn's endpoints.
- **Capture policy:** The initiating settings user's saved preference, resolved when the turn is admitted.
- **Viewer preference:** The current reader's saved preference, which controls transcript visibility.
- **Complete:** All admitted repositories and required entries have usable summaries and historical content.
- **Partial:** Some repositories, entries, or required content are unavailable.
- **History-adjusted comparison:** A comparison that excludes provable upstream changes while preserving the turn's starting local content.
- **Legacy comparison:** An older repository-snapshot comparison without evidence for separating Git history updates.

## Requirements

### REQ-TASKS-TURN-CHANGES-001: Persistent capture preference

**Intent:** Users can control capture and presentation without losing retained history.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-001.1:** New accounts and unsaved settings reads shall default to disabled, including frontend hydration. Existing saved preferences shall remain unchanged; no preference reset is performed.
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
- **AC-TASKS-TURN-CHANGES-002.10:** When an upstream update can be separated from local changes, the summary shall exclude that update.
  This includes rebases and merges onto the same configured comparison target.
- **AC-TASKS-TURN-CHANGES-002.11:** History adjustment shall preserve unchanged starting local edits, including uncommitted content.
  Subsequent local changes and legitimate reverts shall remain visible, including changes to paths also modified upstream.
- **AC-TASKS-TURN-CHANGES-002.12:** When history cannot be separated reliably, the affected checkout shall report unavailable turn comparison.
  It shall not present the whole repository difference as turn-edit totals or claim a successful zero-change result.
  Other checkouts with valid comparisons shall remain usable.
- **AC-TASKS-TURN-CHANGES-002.13:** Legacy captures shall retain their existing files and counts with an explicit repository-snapshot label.
  An upgrade shall not manufacture missing historical evidence from the current checkout or silently replace retained comparisons.

### REQ-TASKS-TURN-CHANGES-003: Ordered capture lifecycle

**Intent:** Capture boundaries belong to the exact admitted turn and cannot move into a successor's writes.

#### Acceptance criteria

- **AC-TASKS-TURN-CHANGES-003.1:** Start capture and endpoint persistence shall precede provider dispatch.
  Bounded capture failure shall allow dispatch with affected history marked unavailable.
- **AC-TASKS-TURN-CHANGES-003.2:** Terminal capture shall validate the turn, execution, and prompt generation.
  Accepted end capture shall precede successor dispatch. Historical content preservation can continue independently from accepted immutable endpoints.
  Executor cleanup shall wait for preserved content or an explicit final partial or unavailable result within its cleanup deadline.
- **AC-TASKS-TURN-CHANGES-003.3:** Duplicate READY/COMPLETE events and delayed events from replaced executions shall not replace accepted endpoints or capture a successor.
- **AC-TASKS-TURN-CHANGES-003.4:** Interrupted, canceled, and failed turns shall retain valid changes when their checkout remains available.
  Stop and Cancel shall remain bounded and responsive.
- **AC-TASKS-TURN-CHANGES-003.5:** After a crash, capture shall require proof of the original boundary and retained content.
  Without proof, the affected endpoint shall become unavailable; a later snapshot shall not impersonate it.
- **AC-TASKS-TURN-CHANGES-003.6:** Capture processing shall remain separate from agent completion, task completion, workflow completion gates, and autopilot policy.
  Terminal pending capture shall not extend the agent-running indicator.
- **AC-TASKS-TURN-CHANGES-003.7:** A completed turn's unfinished export shall not imply continued writing or create an overlap with a later turn.
  Overlap shall require intersecting writer intervals for the same checkout. A shared endpoint alone shall not constitute an overlap.
- **AC-TASKS-TURN-CHANGES-003.8:** Reconciliation shall correct stale overlap attribution using the exact turns' recorded boundaries.
  It shall preserve genuine overlaps and leave active turns unchanged, including when completed and active captures coexist.
- **AC-TASKS-TURN-CHANGES-003.9:** A completed turn with a lost capture boundary shall settle with explicit historical unavailability.
  Reconciliation shall not recapture its end, affect another execution generation, or change task and workflow completion authority.

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
- **AC-TASKS-TURN-CHANGES-004.8:** Transient preservation failures shall receive bounded automatic retries against the original accepted interval.
  During recovery, readers shall see pending preparation rather than a final content-loss result. Recovery shall not extend the agent-running indicator.
- **AC-TASKS-TURN-CHANGES-004.9:** After restart, pending preservation shall resume only with verified original checkout identity and accepted endpoints.
  Missing proof, permanent failure, or exhausted recovery time shall settle explicitly without replacement snapshots.
- **AC-TASKS-TURN-CHANGES-004.10:** Retrying one repository shall preserve file identities, counts, and content already retained for every other repository.
  Successful recovery shall update the original turn automatically and preserve its historical selection and transcript anchor.
- **AC-TASKS-TURN-CHANGES-004.11:** Historical rendering shall derive from the exact retained interval on demand.
  A temporary rendering error shall not permanently invalidate verified snapshots. Removal of a managed worktree shall not invalidate surviving checkpoint objects.
- **AC-TASKS-TURN-CHANGES-004.12:** The existing compaction policy shall govern removal of eligible historical content and old tool details together.
  Compaction shall retain turn/file summaries and show expired history explicitly. No separate historical-diff policy or enable switch shall be required.
- **AC-TASKS-TURN-CHANGES-004.13:** Interrupted content preservation shall retain acknowledged files and resume from durable progress while its original source remains available.
  Repeated delivery shall not duplicate entries, inflate retained bytes, or change the comparison.
- **AC-TASKS-TURN-CHANGES-004.14:** Exhausted preservation attempts and executor cleanup shall leave missing content explicitly unavailable with a specific failure reason.
  Preserved files shall remain readable. Cleanup and cancellation shall remain bounded.
- **AC-TASKS-TURN-CHANGES-004.15:** A capture that finalizes while a user opens or reconnects to its session shall appear without a manual reload.

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
- [Presentation requirements](turn-changed-files-presentation.md)
- [Actor attribution](human-assignee.md)

Presentation refinement: [plan](../../../plans/turn-changed-files-presentation/plan.md).
