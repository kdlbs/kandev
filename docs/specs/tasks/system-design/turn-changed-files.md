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
Frontend uses the existing diff renderer and responsive workbench.

The [inventory](../../../plans/turn-changed-files/plan.md#source-inventory) separates existing/proposed contracts. Mutable Git status and event subscribers cannot order intervals.

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
New defaults start at false. JSON hydration overlays a supplied pointer only when present.
Frontend defaults are false; partial hydration preserves current explicit values.
There is no release flag and no `profiles.yaml` change.

Add the field to models, DTO conversion, service patch/serialization, and store JSON encoding/decoding.
The existing upgrade backfill inserts true only where the key is absent; it is unchanged.
It preserves explicit false and existing user-settings revisions through the normal migration convention.
Unstored settings reads default false; saved true/false values retain their meaning.
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

Follow `TodoListPanelSettings` and `useSettingsSaveContributor` for General chat settings/discovery. Preserve drafts, revisions, canonical hydration, discard, and in-flight edits. Only saved values control cards and subsequent capture.

## Typed model

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
- Internal Git source identity for new captures; legacy SQL patch/rendering content keys for existing rows.
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

The [preservation lifecycle](turn-change-preservation.md) owns terminal capture, content retries, restart recovery, and cleanup ordering.
`CaptureTerminalEndpoints` freezes the exact turn and accepts immutable ends before successor writes.
`ProcessTerminal` compares those accepted objects and verifies a retained Git source independently from the prompt fence.
Transient content errors retain pending work until bounded retry succeeds or settles a final failure.
Runtime cleanup preserves content or explicitly settles outstanding work before destroying its executor source.
Existing completion authority, final-message anchoring, and bounded Stop/Cancel behavior remain dependencies of that lifecycle.

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
Generate per-file canonical and ignore-all-space patch variants on demand from the same accepted endpoint pair.
Keep summary totals unchanged for both variants.
Preserve quoted patch framing and exact path selection using literal pathspec input, never shell interpolation.
A file-entry ID selects its server-owned old/new path pair.

Registered submodule content contributes only through its separate checkout capture.
Parent gitlink changes retain `is_submodule` metadata without claiming recursive textual changes.
Limit breaches preserve known entries and unknown coverage; they never return a successful empty result.

## Persistence and retention

[Git history preservation](turn-change-preservation.md) is authoritative for content sources, durability, migration, and expiry.
New captures retain Git commits/trees/blobs and store compact SQL metadata.
Local worktrees use verified common-repository storage; disposable sources transfer required objects into backend-owned Git storage before destruction.
Patches and old/new context are generated on demand through authenticated historical reads.
They are not copied into SQL for every new turn.

The existing `turn_change_contents` and `turn_change_content_links` tables remain legacy readers during migration.
Existing payloads retain their gzip integrity checks and deduplication rules until verified migration or authorized expiry.
Do not infer loss from absent SQL content links for a Git-backed capture.

The [unified compaction policy](../../system-page/system-design/history-compaction.md) governs historical content and old tool details together.
There is one enabled value, task-inactivity period, analysis action, manual action, and automatic schedule.
Remove the independent fixed 30-day destructive sweep after that policy owns an installation's history.
Preserve summary rows and explicit expiry states until conversation deletion removes their owner.
Capacity admission remains bounded without silently expiring protected history.

Owned checkpoint refs remain reachable while retained, in use, or awaiting preservation.
Only proven ownership permits ref deletion. Object reclamation runs explicitly only in Kandev-owned stores.
Database and Git checkpoint backups share the expanded compaction preparation contract.

## Application reads and live projection

Proposed authenticated endpoints, registered through normal task handlers:

- `GET /api/tasks/:task_id/sessions/:session_id/turn-changes?cursor=...` returns a paged summary catalog.
- `GET /api/tasks/:task_id/sessions/:session_id/turn-changes/:change_set_id` returns one summary and paged file metadata.
- `GET /api/tasks/:task_id/sessions/:session_id/turn-changes/:change_set_id/files/:file_entry_id/diff?ignore_whitespace=false` returns one historical patch/rendering payload.

URL nesting validates ownership. Browsers supply no repository paths or Git refs.
Require normal task read authorization on every summary/content request, including cache hits.
Resolve file-entry IDs through the selected change set. Reject cross-task, cross-session, and cross-checkout mismatches.
Do not expose the content-addressed store as an unauthenticated blob endpoint.

Diff responses include typed availability, canonical counts, patch variant, old/new modes, binary state, and truncation.
Return `410` with an expired body for removed historical content; use typed pending/unavailable/failed states for other missing content.
Partial turns open retained files and show affected checkout states.
A lost historical target never silently becomes current changes or the latest turn.

Add optional `change_set` summaries to turn history and boot/conversation hydration.
Reuse conversation revisions and event bus/gateway fan-out.
`session.turn.changes.updated` carries turn/change-set IDs, revision, availability, totals, and bounded root previews.
Page file catalogs; never send patches or rendering blobs through transcript WS.
Old clients ignore optional additions. New clients hide uncaptured legacy rows.
Merge by revision and immutable identity; stale pages cannot replace live updates.
After subscription acknowledgment or reconnect, refresh page one, finishing any
pre-acknowledgment read first. Release the lease and skip obsolete refreshes and
errors on session change or unmount. Keep initial HTTP reads independent of WS.


## Recovery

Follow [preservation recovery](turn-change-preservation.md#restart-and-cleanup) for accepted endpoint pairs.
An unfinished start without an accepted end cannot establish a historical interval after restart.
Never capture a replacement end, relaunch an executor, or use current workspace contents to repair that interval.
Historical reads use the retained Git source or legacy SQL content and do not start replacement execution recovery.

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
Eligible pending/failure rows use the same anchor. Chat uses `isTurnChangeCardVisible`; history retains `projectTurnChangeSummary`. No-final-reply cancellation gets a terminal row with its outcome.
Existing unread, bottom-follow, and reader-position claims remain authoritative.
Capture updates invalidate only the affected row and preserve the reader's viewport anchor.

The tree groups paths by checkout. Folders start collapsed. Preserve known counts, binary/unknown markers, accessible labels, full-path disclosure, and independent navigation.

AC-TASKS-TURN-CHANGES-005.10 through 005.12 use `FileIcon` and `LineStat` in `TurnChangedFilesCard`, with Changes colors for textual kinds. Unknown/binary counts retain labels. Historical rows never gain live Git controls.
Add non-collapsing 16px space outside the card border at both transcript anchors. Preserve scroll ownership and phone targets.

Persist user-private expansion/selection by session/change-set; prune on session removal. Never persist patches in localStorage or mutate live Git state. Use semantic disclosure buttons unless full tree keyboard support exists.

Availability projection:

| Persisted condition | Presentation |
| --- | --- |
| Active turn | No completed card |
| Terminal pending | Preparing changes row |
| Complete ready with nonzero file count | Card with navigation |
| Complete ready with zero entries | Hidden card, retained successful zero record |
| Disabled viewer, uncaptured turn, no eligible Git checkout | Hidden |
| Settled zero-entry summary | Hidden chat card; retain status and history selection |
| Failed/unavailable interval with recorded files | Existing incomplete-state card |
| Partial summary/content | Available changes totals, repository reasons, usable retained-file navigation |
| Expired nonzero summary | Counts, History expired explanation, disabled Open diff |
| Known overlap | Shared checkout note with interval detail |

The transcript ResizeObserver handles late summaries and asynchronous file rows without new observers or layout polling. Native scroll cannot revoke bottom-follow ownership without reader intent. Explicit navigation releases reconcile viewport geometry. Resize events blocked by history loading retry on the next render under existing follow guards.

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

Proposed bounds require measurement before completion:

| Work | Initial bound | Limit behavior |
| --- | --- | --- |
| Start capture across attached checkouts | 10 seconds total, including admission and retries | Persist failed checkout reasons; dispatch proceeds |
| Normal terminal endpoint capture | 20 seconds total | Accept immutable ends or settle affected endpoints before releasing the successor fence |
| Background comparison/object preservation | 90 seconds per attempt, capped by recovery deadline | Retry transient errors against the accepted Git interval |
| Preservation recovery window | 3 minutes from the persisted terminal claim | Settle remaining content explicitly, then permit owned-ref cleanup |
| Stop/Cancel terminal preservation | 2 seconds after provider stop acknowledgement | Mark unpreserved content unavailable; permit cleanup |
| Git capture concurrency | 2 per executor; 1 per actual checkout | Deadline includes queue time; no unbounded waiting |
| Transient Git retry | At most 2 retries, 75/150 ms backoff inside outer deadline | Retry failed command on same private index; do not move accepted endpoints |
| Full-index inventory | 8 MiB output and existing deadline; no changed-entry count cap | Explicit bounded failure |
| Candidate/changed enumeration | 8 MiB and 20,000 entries per checkout | Incomplete summary with reason; no deceptive ready zero |
| New endpoint object bytes | 256 MiB per endpoint | Preflight eligible blob sizes and track written bytes; reject excess with size reason |
| Historical rendering response | Existing per-file response/output bounds | Explicit per-entry truncation; canonical counts retained |
| HTTP content response | One file; 16 MiB uncompressed total | Typed limit response or bounded preview; decompression limit enforced |
| Summary/file page | 200 entries; compact root preview at most 32 entries | Cursor continuation; omitted coverage stated |

Honor shared Git process admission and managed process-group termination.
Deadlines include filters, executor transport, SQL persistence, and ref/export verification.
Do not retry authentication, unsupported index shapes, malformed output, or output-limit failures as transient errors.
Settle terminal failure durably and release the capture fence. Unavailable,
incomplete settlement may bypass `start_accepted`, preserving policy and revision
guards. Ready results require accepted starts. Never recapture missing starts or
change accepted endpoints on retry.

## Observability and measurements

Record `turn_changes_capture_duration`, `turn_changes_compare_duration`, `turn_changes_export_bytes`,
`turn_changes_capture_failures_total`, and `turn_changes_retained_bytes` through existing metrics/log conventions.
Use closed labels: endpoint, executor kind, result, and reason.
IDs and paths belong only in structured logs, not metric labels. Do not log contents or credentials.
Overlap records cover admitted intervals on the same checkout; external writers remain unknown.

Work order 09 owns representative benchmarks, environment reporting, ten-observation samples, storage growth, and limit qualification.
No source-only latency claims are acceptable.

## Related decisions and designs

- [Immutable turn intervals](../../../decisions/2026-10-07-immutable-turn-change-intervals.md)
- [Git preservation](turn-change-preservation.md)
- [Unified compaction decision](../../../decisions/2026-10-28-git-turn-history-and-unified-compaction.md)
- [Prompt generation identity](../../../decisions/0035-version-agent-ready-events-by-prompt-generation.md)
- [Session turn settlement](session-turn-settlement.md)
- [Prompt completion ownership](../../platform/system-design/prompt-completion-ownership.md)
- [Git diff metadata](../../platform/system-design/git-diff-file-metadata.md)
- [Implementation plan](../../../plans/turn-changed-files/plan.md)

[Presentation plan](../../../plans/turn-changed-files-presentation/plan.md).
