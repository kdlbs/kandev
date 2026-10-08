---
status: draft
system: tasks
created: 2026-10-07
requirements:
  - REQ-TASKS-TURN-CHANGES-001
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
owners:
  - kandev
---

# Turn changed-files system design

## Boundary and source evidence

Tasks owns the immutable interval and transcript anchor.
Runtime provides ordered executor access; agentctl owns Git capture; user settings owns policy resolution.
Frontend consumers reuse the existing diff renderer and responsive workbench.

The [source inventory](../../../plans/turn-changed-files/plan.md#source-inventory) records inspected symbols and existing contracts.
It explains why start-event subscribers and mutable Git status cannot establish immutable interval ordering.
New package/type/API names below are proposed contracts unless the inventory identifies them as existing symbols.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-TASKS-TURN-CHANGES-001 | Settings and authority |
| REQ-TASKS-TURN-CHANGES-002 | Typed model; Git capture; Comparison |
| REQ-TASKS-TURN-CHANGES-003 | Admission and terminal fences; Recovery |
| REQ-TASKS-TURN-CHANGES-004 | Persistence and retention; Application reads |
| REQ-TASKS-TURN-CHANGES-005 | Transcript projection; Presentation |
| REQ-TASKS-TURN-CHANGES-006 | Historical navigation; Mobile contract |
| REQ-TASKS-TURN-CHANGES-007 | Bounds; Observability and measurements |

## Settings and authority

Use `show_turn_changed_files` in storage and API; use `showTurnChangedFiles` in frontend state.
The backend update request uses `*bool`. Serialization always includes the effective boolean, including false.
Defaults start at true. JSON hydration overlays a supplied pointer only when present.
Frontend normalization uses `?? true`, never `|| true`.
There is no release flag and no `profiles.yaml` change.

Add the field to models, DTO conversion, service patch/serialization, and store JSON encoding/decoding.
An idempotent data migration inserts true only into payloads where the key is absent.
It preserves explicit false and existing user-settings revisions through the normal migration convention.
Reads remain correct before the migration because missing values resolve to true.
Cover SQLite JSON and PostgreSQL JSONB behavior, fresh databases, upgrades, and replay.

Expose a narrow user-service method that returns effective settings user ID, enabled value, revision, and resolution kind.
It must reuse `settingsUserID`; do not implement a second fallback table in the orchestrator.
Persist the result as immutable turn policy before provider dispatch.
A settings read error records `policy_read_failed`, disables that turn's capture side effects, and allows prompt dispatch.
It must not guess true after a failed read that could conceal an explicit false.

Authority at each entry point:

| Entry | Authoritative context | Required treatment |
| --- | --- | --- |
| Browser/PAT prompt | Authenticated request identity | Persist that identity before losing request context |
| Human queue entry | Authenticated enqueuer, stored independently from display `QueuedBy` | Restore the admitted enqueuer identity at drain; reject browser spoofing |
| Deferred initial launch | Validated launch intent `UserID` | Preserve its authenticated identity through workflow wake and launch |
| Provider retry/continuation of the same turn | Persisted turn policy | No settings re-resolution or new baseline |
| New automated/workflow turn | Established launch identity or synthetic context | Use user-service synthetic/default rules; actor can remain absent |
| Dynamic provider activity without admitted start | No provable start boundary | Mark uncaptured/unavailable; no late baseline |

Existing prompt-attempt initiator evidence is process-local and can be dynamic or unknown.
It is useful corroboration, but cannot replace persisted policy.
Reserved queue labels such as workflow or system are not user IDs.
The implementation must retain validated initiating identity through queue/deferred boundaries where current request context does not survive.
It must not infer identity from the human assignee, task owner, or a connected socket.

Add a small General chat preference component, following `TodoListPanelSettings` and `useSettingsSaveContributor`.
Keep drafts, save revisions, canonical response hydration, discard, and in-flight edit preservation.
Only the saved value controls cards and subsequent capture; toggling an unsaved draft does not change policy.
Settings discovery links to the owning General chat group.

## Typed model

New task model names are proposed contracts, not existing symbols.
Create `TurnChangeSet`, `TurnRepositoryChangeSet`, `TurnFileChange`, and `TurnChangeAvailability` in a separate model file.

`TurnChangeSet` includes:

- Server-owned ID, task ID, session ID, turn ID, task environment ID, and a monotonic summary revision.
- Runtime execution ID, startup attempt identity where available, prompt generation, execution profile ID, and route generation.
- Capture enabled, settings user ID, actor identity when known, settings revision, and resolution kind.
- Availability: `pending`, `ready`, `unavailable`, `failed`, or `expired`, with a typed reason.
- `complete`, `summary_complete`, and `content_complete` booleans; `partial` is a projection of those booleans.
- Stable session turn ordinal, terminal time/outcome, final assistant message ID, and fallback anchor `turn-changes:<turn-id>`.
- File count, known textual additions/deletions, binary count, unknown-count count, and admitted repository count.
- Retention time, expiry reason, content bytes, and known overlap intervals.

`TurnRepositoryChangeSet` includes:

- Server-owned repository change-set ID and checkout ID.
- Attached repository ID, environment checkout binding, worktree identity where present, display name, and persisted repository subpath.
- Exact start/end commit and tree object IDs, hash algorithm, endpoint capture times, and internal reachability ref identities.
- Capture/comparison/content availability and typed reason per checkout.
- Enumeration completeness, overlap intervals, and capture/export timestamps.

Checkout identity follows the durable environment/source/worktree binding, not URL or branch name.
Assign a server-owned checkout ID to bindings without one; persist its resolved execution-local checkout association.
A replacement clone or worktree gets a new binding identity.
Canonical filesystem and Git-dir identity are executor-internal lock keys, not browser inputs.
Two bindings for the same actual working directory share capture admission and overlap tracking.
Distinct worktrees sharing a common Git object directory remain distinct checkouts.

`TurnFileChange` includes:

- Opaque server-owned entry ID; checkout ID; exact new path or deleted path; optional old path.
- Kind: added, deleted, modified, renamed, copied, mode_changed, or type_changed.
- Old/new blob IDs and modes, submodule/gitlink marker, binary marker, and nullable textual counts.
- Canonical patch and filtered-patch content keys; optional old/new rendering blob keys.
- Content availability, skipped/truncated flags, sizes, and typed reason.

Count distinct `(checkout_id, path)` entries; a rename counts once at its destination with the old path retained.
Do not normalize backslashes in Git paths. Git directory separators are `/` even when a filename contains a literal backslash.
The presentation escapes control characters while selection retains exact bytes/strings for supported UTF-8 paths.
Invalid UTF-8 paths receive an explicit unsupported entry reason rather than a lossy identity.
Binary counts remain null. Mode-only counts can be known zero while the file count is nonzero.
Known-text totals and binary/unknown markers coexist; the UI does not suggest total textual coverage when counts are unknown.

## Components

- Proposed `task/changes` service owns policy admission, immutable endpoint acceptance, summaries, export, and retention coordination.
- Task repository owns SQL records, compare-and-set updates, and conversation projection.
- Proposed neutral `common/turnchanges` package owns wire DTOs and strict NUL parsers used by backend and agentctl.
- A narrow runtime capability exposes admitted-turn capture and export access without adding direct lifecycle imports to new consumers.
- Proposed agentctl checkpoint service owns private index capture, object verification, checkout admission, diff metadata, and bounded export.
- Proposed domain hooks/API client own historical queries and cancellation of stale requests.
- Transcript/card components and diff surface wrappers consume shared projections; they do not fetch directly.

Do not add checkpoint logic to the mutable monitor or large existing orchestrator files beyond small calls into these seams.

## Admission and terminal fences

### Start

1. Reserve or adopt the durable turn under existing turn ownership.
2. Resolve the initiating settings identity and persist policy once.
3. Prepare the real executor and attached source manifest; resolve actual checkout bindings.
4. After prompt generation admission and before `triggerPrompt`, call a new synchronous runtime capture callback with immutable ownership evidence.
5. Capture a fresh start for every eligible checkout. Persist exact accepted object IDs before external dispatch.
6. If bounded capture fails, persist per-checkout unavailability and continue dispatch. Never manufacture replacement start content.

Reuse initial-launch dispatch callbacks, but add the generation-aware seam inside runtime's production send path.
The existing pre-admission callback alone cannot provide the admitted generation.
Provider setup probes and session initialization are not turns; only admitted provider prompts receive capture.
Workspace preparation scripts before dispatch form baseline content and do not count as provider-turn changes.
Steering within the same turn retains its endpoints; a separately admitted replacement turn requires its own fresh baseline.

### End

Use a construction-supplied synchronous runtime terminal interceptor, not an event-bus acknowledgement.
At the accepted completion boundary, claim immutable turn/execution/generation evidence and block successor dispatch for that ownership.
Run Git and SQL I/O outside `executionStore` and `promptLifecycleMu` locks.
Retain the outer startup callback lease only where required; never recursively acquire that lease.
A small capture fence supplements the existing prompt barrier; it does not replace lifecycle completion authority.

1. Validate terminal evidence and claim finalization for the exact change set.
2. Freeze accepted terminal ownership while provider writes stop.
3. Capture endpoints, compare exact objects, and export bounded historical rendering data.
4. Persist summary, content manifest, and final capture status using compare-and-set revision updates.
5. Release capture admission. Existing readiness, task/workflow transitions, and prompt completion proceed through their current owners.
6. Publish revisioned summaries. Replayed terminal events reuse the accepted result.

The interceptor precedes completion-channel release and READY fan-out.
It covers foreground-idle, dispatch failure after effects, exit, error, cancellation escalation, and cleanup.
Startup failures without dispatched work have no fabricated interval.
Runtime cleanup invokes the same idempotent finalizer before stopping agentctl or deleting ephemeral compute.
Hard loss of the executor records unavailability instead of attempting a current-workspace diff.

Agent completion can project a terminal turn with pending checkpoint processing.
The capture fence delays successor writes, not the agent-running indicator or task-state authority.
Persist the pending change-set row before fan-out; summary publication can follow existing completion publication.
The final assistant anchor can update after READY if the matching COMPLETE output arrives later.
That update changes only the anchor/revision, never accepted endpoints.

Stop/Cancel requests issue provider cancellation immediately and cancel an in-progress start capture.
After provider stop acknowledgement, terminal capture receives a short independent deadline before destructive cleanup.
A timeout records unavailable/partial and permits cleanup. Stop cannot wait behind an unbounded Git command.

## Git capture

Capture runs in the executor. Resolve a checkout through the registered source manifest and `GitOperatorFor` conventions.
Use the shared Git admission framework in `common/subproc`, with exact safe argv additions in `securityutil` where needed.
Inherit executor-owned environment and clear conflicting Git binding variables before explicitly setting the private index.

Serialize capture per canonical worktree/Git-dir identity, across agentctl instances sharing that checkout.
Use a cooperative executor-local lock with bounded acquisition and owner-death release, plus process-local keyed admission.
Do not use the user's `index.lock`. Different checkouts can run within the executor concurrency bound.
A capture lock coordinates captures only. It does not stop external edits or imply an atomic filesystem-wide snapshot.

Create a unique private writable index under the checkout's resolved Git directory.
Copy the source index only after validating timestamps, racy stat data, assume-unchanged, skip-worktree, split/sparse index, and unmerged stages.
Never hard-link an index that capture will mutate.
If optimization is unsafe, build a fresh private index with explicit tracked membership and valid sparse exclusions.
For an unborn repository, start with an empty index and capture eligible files.
For conflicts, capture working-file content into the private index without rewriting the real conflict stages or operation files.

Stage tracked content and ordinary nonignored untracked files into the private index.
Preserve tracked-but-ignored files, actual deletions, executable modes, symlinks, and gitlinks.
Avoid whole-checkout copies. Unchanged blobs reuse existing Git objects.
Do not run hooks, reset, checkout, stash, or mutate user branches/configuration.
Bound Git filters through existing subprocess/process-group control; surface failure or timeout.

Handle sparse exclusions deliberately. A safe copied sparse index retains absent excluded entries.
A rebuilt cone index preserves excluded membership while observing materialized outside-cone files.
For an unsafe non-cone rebuild, return `sparse_capture_unsupported` rather than false deletions.
Unsupported Git versions or index shapes carry explicit reasons.
Registered child repositories capture separately; parent captures only their gitlink or declared nonrecursive boundary.
Unregistered nested repositories do not receive recursive capture. Report scope exclusions internally.
An uncommitted embedded repository can be excluded only with a typed incompleteness reason.

Run `write-tree`, then `commit-tree` with fixed service author metadata and no parent-chain growth.
Use a unique ref namespace `refs/kandev/turn-changes/<change-set>/<checkout>/<start-or-end>`.
Create refs with compare-and-set semantics; do not overwrite an accepted endpoint on retry.
Flush object/ref writes through supported Git durability controls before reporting capture success.
Store exact commit/tree OIDs and their hash algorithm. Historical queries never resolve mutable HEAD or branch names.
Verify an existing idempotency ref against stored ownership and OID before reusing it.
Clean private index and its lock after success, error, timeout, or cancellation.
Remove only provably owned stale temporary files during recovery.

## Comparison

Run immutable-object comparisons with built-in Git output, no color, no external diff, and no text conversion.
Use consistent rename detection and explicit diff options for raw metadata, `--numstat -z`, and patches.
Freeze actual kinds from `--raw -z` or equivalent NUL-delimited status/mode/blob records.
Use a strict parser that retains both rename paths and binary `-` fields.
Join metadata/counts by exact path identity; mismatches produce comparison failure or partial completeness.
Do not reuse the existing parser's binary-to-zero conversion.

Compute canonical summaries without whitespace-ignore flags.
Export per-file canonical patches and an explicitly labeled ignore-all-space patch variant from the same endpoint pair.
Keep summary totals unchanged for both variants.
Preserve quoted patch framing and exact path selection using literal pathspec input, never shell interpolation.
A file-entry ID selects its server-owned old/new path pair.

Registered submodule content contributes only through its separate checkout capture.
Parent gitlink changes retain `is_submodule` metadata without claiming recursive textual changes.
Limit breaches preserve known entries and unknown coverage; they never return a successful empty result.

## Persistence and retention

Use new relational tables under the task repository's replayable schema conventions:

- `turn_change_sets`: unique turn identity, policy, ownership, anchor, status, revision, and expiry metadata.
- `turn_repository_changes`: unique change-set/checkout relation with write-once start/end OIDs and per-repository states.
- `turn_file_changes`: unique repository-change/path entries and compact file metadata.
- `turn_change_contents`: content-addressed compressed patch/rendering payloads with codec, digest, and uncompressed size.
- `turn_change_content_links`: file-to-content variant relations for authorization and shared-content lifetime.

Use SQLite BLOB and PostgreSQL BYTEA through the existing dialect layer.
Start/end acceptance and finalization use conditional writes; PostgreSQL uses scoped advisory transaction locks before reads where needed.
Never hold a database transaction across executor I/O.
Replayable migrations add new columns before dependent indexes/backfills.
Add deletion cascades and E2E reset cleanup for all side tables.

Executor Git objects retain full snapshots while capture/export is pending.
Before terminal capture is ready, materialize bounded per-file patches and required old/new rendering data into database content records.
Content-addressing deduplicates unchanged rendering blobs. This export copies changed rendering data, not the checkout.
Binary entries retain metadata and the renderer's binary notice; do not promise a binary image preview.
Old/new contents support context expansion after executor loss.
Historical components must not call the existing live-workspace expansion fetcher.

Canonical and whitespace-filtered variants allow immutable reads without restarting an archived executor.
Generation of these durable variants occurs at finalization; their transfer to the browser remains on demand.
A truncated patch or missing required text blob marks that entry partial.
Counts remain canonical even when only a bounded preview is available.

Proposed initial retention policy:

- Retain rendering content for 30 days from terminal capture.
- Bound compressed retained content to 1 GiB per installation and 128 MiB per task.
- Bound accepted export to 32 MiB per turn, with explicit file limits described under Bounds.
- Evict oldest terminal content first, excluding active capture/export leases.
- Preserve summary rows and expiry reasons until conversation deletion removes their owner.
- Apply the same policy to archived tasks; archive itself does not expire history.

These limits are proposed defaults, not measurements. Use typed policy constants; public retention controls remain outside scope.
Serialize global byte admission and eviction through existing maintenance/storage mutation conventions.
Expiry atomically clears content links, marks affected entries expired, and updates the summary revision.
Delete shared payloads only when no retained link references them.
An in-flight read holds a short content lease so pruning cannot remove its payload mid-response.

Release executor refs after verified export or explicit unavailability/expiry.
Reachability persists until all promised content is durable; never delete a sole source before export acknowledgement.
Persist cleanup intent for unavailable executors and drain it when they reconnect.
Bound owned loose-object growth and orphan refs through retention and existing executor cleanup, without running destructive user-repository GC.
A failed export preserves available counts and an explicit missing-content reason before cleanup proceeds.
SQLite backup and PostgreSQL database backup include durable content because it resides in the database.

## Application reads and live projection

Proposed authenticated endpoints, registered through normal task handlers:

- `GET /api/tasks/:task_id/sessions/:session_id/turn-changes?cursor=...` returns a paged summary catalog.
- `GET /api/tasks/:task_id/sessions/:session_id/turn-changes/:change_set_id` returns one summary and paged file metadata.
- `GET /api/tasks/:task_id/sessions/:session_id/turn-changes/:change_set_id/files/:file_entry_id/diff?ignore_whitespace=false` returns one historical patch/rendering payload.

The URL nesting validates ownership; the browser supplies no repository path or Git ref.
Require normal task read authorization on every summary/content request, including cache hits.
Resolve file-entry IDs through the selected change set. Reject cross-task, cross-session, and cross-checkout mismatches.
Do not expose the content-addressed store as an unauthenticated blob endpoint.

The diff response contains typed availability, canonical counts, patch variant, old/new modes, binary state, and truncation details.
Return `410` with an expired body for removed historical content; use typed pending/unavailable/failed states for other missing content.
A partially available turn can open retained files, with affected checkout states visible.
A lost historical target never silently becomes current changes or the latest turn.

Add optional compact `change_set` projection to turn history and boot/conversation hydration.
Reuse conversation revision ordering and the event bus/gateway fan-out.
Introduce `session.turn.changes.updated` carrying turn/change-set IDs, summary revision, compact availability/totals, and bounded root preview data.
Large file catalogs are paged through the summary endpoint; full patches and rendering blobs never enter transcript WS payloads.
Old clients ignore the optional field/event. New clients hide cards for old rows with no captured policy/endpoints.
Merge changes by revision and immutable identity, protecting live updates from stale pagination responses.

## Recovery

Persist terminal intent and ownership before accepted end capture.
After restart, reuse accepted object IDs or a capture receipt only after verifying referenced objects and checkout incarnation.
If a still-running original executor proves the same admitted generation and no successor writes, finish its original capture.
If it cannot prove that boundary, mark `boundary_lost`; do not capture the current checkout as the old endpoint.
A retained start alone is insufficient proof of a historical end.
No automatic relaunch or `GetOrEnsureExecution` can substitute a new checkout for historical content.
Historical reads use database content and therefore do not need live execution recovery.

Generation-zero synthetic turns receive capture only when an explicit pre-dispatch boundary is available.
Legacy uncorrelated terminal events cannot acquire new endpoints from a mutable active-turn lookup.
They can settle availability against already persisted ownership, without altering existing lifecycle compatibility behavior.

## Transcript projection and presentation

Persist the final assistant message ID by querying messages with the exact `turn_id`.
Final text can arrive after readiness; revision the anchor when that message becomes durable.
The fallback row stays stable until the matching assistant anchor is available.
Never attach to the session's latest message or move a card onto a later turn.
Partial pagination cannot nominate an earlier assistant message as final.

Add a separate terminal changes render item to `use-processed-messages` or an equivalent shared projection.
Its output sits immediately after the final assistant row and outside any `turn_group` tool disclosure.
Keep its stable key and measurement in native/virtualized transcript handling.
Pending/failure rows use the same anchor. No-final-reply cancellation gets a terminal row with its outcome.
Existing unread, bottom-follow, and reader-position claims remain authoritative.
Capture updates invalidate only the affected row and preserve the reader's viewport anchor.

The shared tree groups exact paths under checkout roots and aggregates known counts plus binary/unknown markers.
Folders start collapsed; root files remain visible. Status icons have accessible labels.
Full paths appear on focus/hover, with touch disclosure on phones.
Open diff, folder toggles, and expand-all remain independent actions.

Store expansion and selected entry by session/change-set ID in existing UI persistence conventions.
Namespace user-private state; prune it when its owning session is removed.
Do not store patch contents in localStorage or mutate the live Git cache.
Use nested semantic disclosure buttons, or a tree with complete roving-focus behavior.
Do not add `role=tree` without its keyboard contract.

Availability projection:

| Persisted condition | Presentation |
| --- | --- |
| Active turn | No completed card |
| Terminal pending | Preparing changes row |
| Complete ready with nonzero file count | Card with navigation |
| Complete ready with zero entries | Hidden card, retained successful zero record |
| Disabled viewer, uncaptured turn, no eligible Git checkout | Hidden |
| Failed/unavailable interval | Turn changes unavailable row; no successful zero |
| Partial summary/content | Available changes totals, repository reasons, usable retained-file navigation |
| Expired nonzero summary | Counts, History expired explanation, disabled Open diff |
| Known overlap | Shared checkout note with interval detail |

## Historical navigation

Add `HistoricalTurnDiffTarget` to dependency-neutral `diff-target-types.ts`:
`{ kind: 'turn', taskId, sessionId, changeSetId, checkoutId?, fileEntryId?, navigationToken }`.
Extend `DiffSheetMode` with that typed target. Keep existing live source discriminants separate from historical scope.
Add a Changes scope selector: Current changes, Latest captured turn, and explicit Turn N entries.
Latest captured turn resolves once to an exact change-set ID on selection; new capture does not silently move an open historical view.
Selecting Latest again resolves the newer retained turn.

Reuse Dockview panel activation and the desktop Changes panel shell.
A small historical content adapter supplies existing `FileDiffViewer`/diff adapters with retained file data.
It uses historical content expansion, never mutable workspace reads.
Keep selected turn, checkout/file, and whitespace filter in shared navigation state.
Historical panels are read-only; live staging, commit, PR, and commit-detail controls retain their current scopes.
Show a turn label and timestamp from stable server order, not the loaded-page index.
Display unavailable/expired selection explicitly instead of falling back to another turn.

## Mobile contract

Nearest shipped exemplars are `MobileDiffSheet`, `MobileChangesPanel`, and `MobilePickerSheet`.
Reuse the existing full-height diff Drawer because diff content requires focused reading and frequent return to chat.
The card stays inline in the transcript. Its narrow header uses two rows: counts first, labeled actions second.
Folder and file actions have separate touch targets of at least 44 pixels.

A file tap opens the diff drawer directly at its historical turn/checkout/file.
The fixed header shows Turn N, Close, and the visible context/whitespace control.
A picker opens a compact inset bottom drawer for turn or file choice; do not stack master/detail columns on the phone.
Reuse shared target/query/tree logic. Mobile wrappers own only composition and geometry.

The diff drawer uses dynamic viewport height, safe-area clearance, and one internal vertical scroll owner.
Code-region horizontal scrolling stays inside the renderer; the document never scrolls horizontally.
Dismissal returns focus to the originating card/file action.
A phone visit does not overwrite saved desktop layout, source, or display preferences.
Test phone fine-pointer width and coarse-pointer tablet behavior as well as the standard mobile project.

Localize all copy with count pluralization across en, pt-pt, zh-cn, zh-hk, zh-tw, ja, ko, and pseudo.
Use the existing Traditional Chinese conversion workflow.

## Bounds

The following defaults are proposed operational bounds, subject to measured validation before implementation is marked complete:

| Work | Initial bound | Limit behavior |
| --- | --- | --- |
| Start capture across attached checkouts | 10 seconds total, including admission and retries | Persist failed checkout reasons; dispatch proceeds |
| Normal terminal capture/comparison/export | 15 seconds total | Preserve accepted endpoints/available entries; release successor fence after typed settlement |
| Stop/Cancel terminal preservation | 2 seconds after provider stop acknowledgement | Mark unpreserved content unavailable; permit cleanup |
| Git capture concurrency | 2 per executor; 1 per actual checkout | Deadline includes queue time; no unbounded waiting |
| Transient Git retry | At most 2 retries, 75/150 ms backoff inside outer deadline | Retry failed command on same private index; do not move accepted endpoints |
| Raw enumeration | 8 MiB and 20,000 entries per checkout | Incomplete summary with reason; no deceptive ready zero |
| New endpoint object bytes | 256 MiB per endpoint | Preflight eligible blob sizes and track written bytes; reject excess with size reason |
| Retained text rendering | 8 MiB per blob, 4 MiB per patch variant, 32 MiB total export per turn | Explicit per-entry missing/truncated content; canonical counts retained |
| HTTP content response | One file; 16 MiB uncompressed total | Typed limit response or bounded preview; decompression limit enforced |
| Summary/file page | 200 entries; compact root preview at most 32 entries | Cursor continuation; omitted coverage stated |

Honor shared Git process admission and managed process-group termination.
Deadlines include filters, executor transport, SQL persistence, and ref/export verification.
Do not retry authentication, unsupported index shapes, malformed output, or output-limit failures as transient errors.
End capture cannot remain pending after the bounded terminal attempt settles.
Accepted endpoints never change on comparison/export retry.

## Observability and measurements

Record `turn_changes_capture_duration`, `turn_changes_compare_duration`, `turn_changes_export_bytes`,
`turn_changes_capture_failures_total`, and `turn_changes_retained_bytes` through existing metrics/log conventions.
Use closed labels: endpoint, executor kind, result, and reason.
IDs and paths belong only in structured logs, not metric labels. Do not log contents or credentials.
Known overlap records come from admitted execution intervals on the same checkout; external writer coverage remains unknown.

Work order 09 owns representative benchmarks, environment reporting, ten-observation samples, storage growth, and limit qualification.
No source-only latency claims are acceptable.

## Related decisions and designs

- [Immutable turn intervals](../../../decisions/2026-10-07-immutable-turn-change-intervals.md)
- [Prompt generation identity](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)
- [Session turn settlement](session-turn-settlement.md)
- [Prompt completion ownership](../../platform/system-design/prompt-completion-ownership.md)
- [Git diff metadata](../../platform/system-design/git-diff-file-metadata.md)
- [Implementation plan](../../../plans/turn-changed-files/plan.md)
