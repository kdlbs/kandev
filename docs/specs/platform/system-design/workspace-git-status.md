---
status: current
system: platform
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
created: 2026-07-19
updated: 2026-09-30
owners:
  - kandev
---

# Workspace Git Status System Design

## Purpose and boundaries

Platform owns repository observation, bounded computation, and publication. Tasks owns the canonical environment and workspace binding.
This design specifies progressive refresh publication while preserving those ownership boundaries.
The [environment-owned design](../../tasks/system-design/environment-owned-git-status.md) remains authoritative for source eligibility and persistence selection.
The protocol and lifecycle rules below describe the current system.

## Requirement mapping

`REQ-PLATFORM-WORKSPACE-GIT-STATUS-001` is implemented by [Progressive refresh](#progressive-refresh), [Publication and ordering](#publication-and-ordering), [Foreground recovery](#foreground-recovery), [Frontend and mobile](#frontend-and-mobile), the generated and mixed-path subsections, [Comparison-target Git execution](#comparison-target-git-execution), and [API surface](#api-surface).

### Generated untracked dependency trees

Agentctl owns the complete workspace snapshot. It collects tracked and untracked state through separate Git queries so a generated dependency tree cannot force Git to emit every package file.

The tracked query uses `git status --porcelain --untracked-files=no`. It preserves index and working-tree changes for every tracked path, including a tracked path below `node_modules`. The untracked query uses `git ls-files --others --exclude-standard --exclude=node_modules/ -z`. Git applies the extra exclusion while it walks the worktree. The `-z` result preserves path names without quoting or line splitting. Agentctl adds each returned path to the existing untracked status projection before diff enrichment.

For each full observation, agentctl pins both queries to a temporary snapshot of the Git index. Git writes the real index atomically, so the snapshot keeps tracking membership stable across the pair without blocking concurrent user Git operations. The snapshot is removed after the observation.

The extra exclusion applies to directories named `node_modules` at any repository depth. It also covers pnpm's nested `node_modules/.pnpm` store. It does not cover `.next`, `dist`, `build`, or other generated names. Those paths continue to follow Git's repository, global, and command-level ignore rules.

The full status observer and the untracked monitor fingerprint use one shared argument definition. The monitor therefore does not stat dependency-tree files, and changes limited to an untracked dependency tree do not start a full refresh. Filtering parsed status output is not sufficient because Git and agentctl have already paid the enumeration cost at that point.

The untracked exclusion and paired queries remain unchanged. The progressive refresh contract adds quality and ordering metadata to the existing status routes and events. This contract follows [ADR-2026-08-30-bound-untracked-dependency-enumeration](../../../decisions/2026-08-30-bound-untracked-dependency-enumeration.md).

### Mixed index and working-tree changes

`GitStatusUpdate.Files` stays unique by repository-relative path.
`FileInfo` retains flattened compatibility fields and optional `staged_change`/`unstaged_change` facets for mixed paths.
Each facet preserves status, line totals, rename origin, diff, and skip reason.
Single-layer paths retain the legacy flattened shape.

For mixed paths, the compatibility diff compares captured HEAD to the worktree.
The staged facet compares captured HEAD to the retained index. The unstaged facet compares that index to the worktree.
All three representations share the snapshot byte budget and per-representation cap.
Budget exhaustion never removes facet membership.

Changes projects two temporary rows but counts and mutates each repository/path once.
Desktop diff targets and the mobile full-height drawer use `(repository_name, path, layer)`.
Review's combined source stays one uncommitted file unless selection requests a layer.
Equality, editor signatures, and Changes fingerprints include facets and their phase fields.
See [Mixed Git change facets](../../../decisions/2026-08-27-mixed-git-change-facets.md).

### Base-commit staleness and refresh

Commits and cumulative diff use the stored base unless it is a strict ancestor of the current integration merge-base.
Resolve integration candidates in order: `origin/main`, `origin/master`, `main`, `master`.
Prefer upstream refs over stale local refs. A stored base equal to or newer than the merge-base remains valid.
A live non-default upstream comparison, such as `origin/develop`, retains its own merge-base.
Do not replace it merely because the default integration merge-base is newer.
If neither a candidate nor usable merge-base resolves, retain the established stored-base/branch-tip fallback.
This read-time correction does not rewrite `base_commit_sha` or retarget the stored `base_branch`.

### Repository-qualified comparison targets

A task-repository attachment can persist one versioned, credential-free `comparison_target` in metadata.
It binds provider change, validated head repository/branch, and target repository/branch.
The provider head must match the attachment repository and live normalized checkout branch.
Historical PRs and sibling attachments cannot retarget that scope.

Same-repository comparisons retain `origin/<base_branch>`.
Cross-repository comparisons materialize a deterministic comparison-only remote and fetch only the validated target branch.
They never rewrite `origin`, checkout upstream, or push routing.
The exact ref governs task totals, divergence, commits, cumulative diff, and Review expansion.
An explicit target cannot fall back to a same-named `origin` or local branch.

Association or retarget clears the matching sessions' stored base, refreshes comparison state, and invalidates client commit/diff caches.
Launch/resume reconstructs the binding without requiring a session restart for later updates.
**Compare against** atomically updates `base_branch` and removes the provider target.
Detachment removes only a target naming that change. Closing or merging the change retains its comparison target.

Target validation, remote collision, fetch, and merge-base failures are fail-closed.
Working-tree files remain usable while comparison-derived values report unavailable.
Changes names the intended target. Task summaries suppress unverified numeric totals.
Credentials remain limited to the execution's effective Git scope. Public or already readable private targets are supported.

### Comparison-target Git execution

The instance's effective Git environment is authoritative for Kandev-owned workspace Git commands. This environment contains executor credentials and managed credential-helper entries. The process manager gives a detached environment copy to each workspace tracker. This rule applies to root, repository, submodule, rescan, and lazy trackers.

Each tracker starts Git with its instance environment. Final execution preparation follows
[Managed Git execution](git-subprocess-execution.md), including deny-only askpass,
SSH option preservation, finite execution budgets, and bounded helper cleanup.
The shared boundary applies controls after caller environment assembly. Tracker-specific
index selection and optional-lock suppression remain owned by the tracker.

The process manager sets an explicit target to `pending` before it starts network work. It then materializes the target with the manager's lifetime context. Initial materialization, live updates, rescan, and lazy tracker creation do not wait for the fetch. A successful fetch publishes the exact comparison ref and starts a detached status refresh. An error publishes a bounded unavailable state. Manager shutdown cancels unfinished materialization.

Kandev selects the transport before it starts a comparison-target command. Canonical comparison targets remain HTTPS. An HTTPS authentication or transport error does not start an SSH retry. This rule prevents a command from changing identity, host trust, or credential scope after the first error. Kandev does not rewrite `origin`, the checkout upstream, or push routing.

An explicit fresh status request may re-evaluate an unavailable comparison target once. A moving target refresh may force-update only its deterministic internal comparison ref, so a stale cache does not block recovery; user remotes and push routing are unchanged.

This contract follows [ADR-2026-08-31-deterministic-noninteractive-git-transport](../../../decisions/2026-08-31-deterministic-noninteractive-git-transport.md).

## Progressive refresh

`GetGitStatus(ctx, true)` starts or joins a complete basic observation in the interactive admission class.
Background monitor and Git poll requests keep their separate singleflight key.
A basic snapshot has complete membership, but can have pending details.

The basic phase collects branch and HEAD identity, paired tracked/untracked status, mixed facets, rename origins, and symlink/submodule identity.
It moves merge-base resolution, divergence counts, per-file diff content, and branch totals into enrichment.
Split `getGitBranchInfo` so comparison probes cannot hold file membership behind secondary work.
Do not change the existing dependency-tree exclusion or porcelain interpretation.
A bare multi-repository root emits no successful root snapshot.
An invalid repository emits unavailable status, never synthetic clean status.

Keep one pinned index through the paired status reads and the accepted enrichment job.
`snapshotGitIndex` already resolves linked-worktree indexes outside the task directory.
Transfer cleanup ownership explicitly when a job is accepted. Release discarded snapshots immediately.
Do not retain an index per subscriber or waiter.

A successful basic observation passes through one publication function inside tracker-owned work.
That function updates `currentStatus` and broadcasts the immutable accepted snapshot before releasing successful waiters.
`updateGitStatusClass`, `RefreshGitStatus`, and stream attach stop making independent cache writes.
Their return values describe acceptance, so monitor retry bookkeeping still knows whether a refresh succeeded.
The publication function checks tracker lifetime and ordering under one lock.
It sends to subscriber channels without blocking and without holding locks across Git commands.

Attach registers the subscriber before requesting refresh.
It replays only an accepted snapshot, with its real timestamp and quality metadata.
It cannot manufacture a timestamp for an empty cache.
A completed refresh broadcasts to existing subscribers too.
This intentionally replaces the attach test that prohibits broadcasts to other subscribers.
Duplicate snapshots remain harmless through revision checks.

### Enrichment lifecycle

Each tracker has one lazily started worker and one replaceable pending job slot.
The worker runs existing sequential enrichment through Git background admission.
Interactive basic observations never wait behind this worker or `updateMu` held across enrichment.
The worker has a tracker-owned deadline no greater than 60 seconds for each job.
The basic singleflight deadline remains 60 seconds. Individual Git commands retain their ten-second execution bound.
The worker deadline starts before pre-validation and remains active through post-validation. Every Git validator and digest uses background admission and that same deadline; validation reads the live index, while enrichment commands use the pinned index snapshot.

Jobs use the full observation fingerprint, not caller identity.
An unchanged accepted basic observation retains current details or joins the same pending job.
It cannot restart enrichment or downgrade ready details to pending.
A changed fingerprint cancels the obsolete job and replaces the pending slot with only the latest accepted snapshot.
The worker drains cancellation before executing the successor. No per-file goroutine or unbounded queue is added.
Both admission classes can have one basic flight, but share the single enrichment worker.

The worker deep-copies maps and facet pointers before adding details.
It publishes one enriched result after validation, rather than one large event for every file.
The basic result already makes every file visible.
Skipped representations keep existing reasons and count as settled details, including binary and oversized files.
Command failures have an explicit unavailable detail state. They do not become successful empty diffs.
The failed file or facet is unavailable while successful siblings keep ready detail. Overall detail quality remains unavailable when a requested diff or secondary statistic failed.
An unchanged explicit refresh retries unavailable enrichment through the same bounded worker.
Details can be carried forward only for an identical validated fingerprint.

`Stop` prevents further lifecycle admissions, cancels both phases, wakes the worker, and waits for owned work to exit.
The existing observation WaitGroup barrier also owns enrichment and its index cleanup.
Caller cancellation only ends that waiter's wait.
Background admission revocation can end enrichment with unavailable details while preserving accepted basic membership.
A later explicit retry can restart failed enrichment once. Ordinary unchanged observations do not create an automatic retry loop.

### Callers that require details

Changes requests basic membership through the multi-status path.
Completion snapshots, archive base capture, launch base capture, and server-side Review consumers require enriched values.
Keep these consumers explicit. Add `details=wait` to the existing status routes for callers that need completed details.
The runtime client methods used by these consumers request that mode. Foreground Changes requests do not.

The wait joins the current fingerprint's bounded enrichment completion, without another job or caller-owned publication.
If a newer fingerprint replaces that job, return a superseded/unavailable result instead of unrelated details.
Caller cancellation ends only its wait. Tracker cancellation still drains the worker.
If detail generation fails, these consumers receive an error or explicit unavailable result under their existing retry policy.
They cannot persist pending values as a completed snapshot or confuse pending base resolution with a repository-less task.
Preserve existing caller budgets. This mode never extends the normal two-second Changes probe.

## Publication and ordering

Use three small identities rather than timestamps as the sole ordering authority within one tracker:

- `tracker_id`: a unique opaque identity for one repository tracker's lifetime across agentctl processes.
- Internal observation ordinal: allocated when a singleflight body starts. A lower ordinal cannot replace a newer accepted basic result or failure.
- `snapshot_revision`: a monotonic publication number within `tracker_id`. It advances for accepted membership, quality, or detail changes.

The legacy numeric `tracker_epoch` remains available during compatibility migration. It is process-local and never orders different tracker IDs.

A captured snapshot key identifies the content fingerprint independently of publication revision.
An unchanged observation can advance the freshness watermark without discarding or duplicating enrichment.
An older enrichment result can serve the latest snapshot only when its full fingerprint still matches under the publication lock.
Otherwise it is rejected. A newer failed refresh preserves prior data and marks it as prior.

The fingerprint includes canonical repository/workspace identity, resolved Git directory and index digest, literal HEAD OID and branch, and comparison generation/ref OID.
It also includes the complete parsed membership and facets, plus file identity/type/size/change stamps for eligible worktree paths.
Include rename origins, symlink targets, submodule boundaries and observed submodule HEADs.
Do not use `getWorkspaceState` unchanged: it omits HEAD and comparison identity and only checks worktree mtimes.
Do not use the poller's status-entry hash as the index-content digest.

Basic capture validates its start/end identity before publication.
Enrichment uses the retained index and literal observed HEAD instead of mutable `HEAD` arguments.
It checks identity before work and again before publication.
The worker validates content of files it enriches through bounded reads or digests before and after their diff command.
Stat evidence must still match basic capture. Any changed evidence rejects the enrichment result.
Content validation follows the existing per-file source-size gate and the tracker-owned enrichment deadline. It cannot read every ignored or oversized file. Evidence hashing is separate from the per-file and total diff-output budgets, so a larger supported source file cannot consume the patch output budget before its diff is generated.
Comparison-derived values use the captured exact comparison ref OID and fail closed after retargeting.

If validation detects a change, keep the last accepted basic data marked pending or unavailable.
Schedule at most one corrective basic observation for the changed job, without recursive retry.
The ordinary focused monitor or an explicit retry handles further churn.
Never merge individual diffs into the current map based only on a path match or equal HEAD.
A busy worktree can therefore show current file membership while details await a stable observation.

Filesystem capture is an observation, not an atomic transaction with external writers.
The implementation must reject all observable identity/content changes across its capture and publication boundaries.
Preserving timestamps alone cannot defeat content validation for enriched files.
Continuously mutating files can remain pending or unavailable. They must not receive unrelated old details.

Runtime delivery captures immutable execution identity, environment binding, workspace identity, and stream generation.
Revalidate these before publishing a delayed callback, initial-subscribe read, or HTTP result. Workspace callbacks are checked against the execution currently registered for their session.
Tracker epochs from different sibling executions are not numerically ordered.
Preserve requested-session-first source probing. Eligible siblings remain valid sources for their common environment.
Within one current `tracker_id`, snapshot revisions order HTTP responses and stream events identically.
Across eligible tracker IDs, retain capture-time ordering and backend validation of execution/workspace generation.
A new source cannot replace a newer environment observation with an older capture merely because its local revision is larger.
The backend validates source lifetime before forwarding stream updates. A replaced stream cannot restore its retired state.
Keep these source watermarks bounded by current eligible executions, not an unbounded history of epochs.

Legacy payloads retain strict timestamp ordering through the repository timestamp parser.
Once ordered state is accepted, an unordered legacy frame cannot override it.

## Status quality and wire contract

Extend the existing Go stream, HTTP result, runtime DTO, WebSocket, and TypeScript status types together.
Use optional fields for decoding compatibility:

| Field                                | Meaning                                                                                                     |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
| `status_state`                       | `ready`, `loading`, or `unavailable`. Only `ready` carries an accepted observation.                         |
| `files_complete`                     | Complete file membership, including a successful empty map. False means summary-only or unknown membership. |
| `detail_state`                       | `pending`, `ready`, or `unavailable` for diffs and secondary Git statistics.                                |
| `error_code`                         | Closed, sanitized failure code. Never raw Git output, credentials, or workspace paths.                      |
| `tracker_id`, `snapshot_revision`  | Opaque source lifetime and ordering within that lifetime. `tracker_epoch` is legacy and process-local.      |

Each file/facet adds optional `diff_state` with `pending`, `ready`, or `unavailable`.
Existing `diff_skip_reason` remains separate. A skipped binary diff can be ready with reason `binary`.
A ready empty diff still differs from a pending diff.
Snapshot-level pending details also means line totals and divergence values are unknown, not confirmed zero.
Consumers suppress unknown numbers or preserve explicitly marked prior values.
Audit task summaries, upstream contribution policy, editors, Review, and commit/cumulative-diff hooks for zero-value assumptions.
Do not infer push readiness from pending divergence counts.

`status_update` remains the existing event type.
Loading/unavailable events have repository scope, source ordering, and request correlation, but no authoritative file replacement.
The frontend gates complete snapshots and quality transitions against the accepted snapshot and refresh watermark.
An aborted foreground attempt is removed synchronously; cleanup can only settle state carrying that attempt's request ID.
HTTP per-repository `Success=false` cannot disappear in backend projection.
The multi-repository envelope still permits healthy repositories alongside failed ones.
A whole transport error emits an environment-scoped unavailable result for the known repository inventory.
If inventory is unknown, use an environment-level state. Do not create a fake root repository.

New producers always set `files_complete` explicitly.
Legacy detailed completed snapshots remain readable. A known compact `live_monitor` row is always summary-only.
An empty array of filenames from a compact row is insufficient to claim clean status.
Persist snapshot quality in compact metadata if needed, but do not add full live diff persistence or a schema migration.
Runtime ordering tokens are not durable ordering tokens across restarts.
The database keeps its existing environment/repository selection and timestamp authority.

## Foreground recovery

Keep the normal backend live-source probe at two seconds.
Preserve `resolveGitStatusSources`, environment/workspace matching, sibling source selection, and the prohibition on persisted fallback after live failure.
Report unavailable status when the probe cannot supply usable membership.
A useful late tracker observation still publishes and fills its cache independently.

Extend `session.git.refresh` to return the bounded per-repository snapshot/state in its correlated control response.
An ACK saying that the action was accepted is not refresh completion.
Keep compatibility notifications for existing consumers. Both delivery paths use the same ordered projection.
Return environment identity and repository scope in every response.
Revalidate source and requested-session bindings after asynchronous work.

Use optional refresh modes on that existing action:

- `fresh` (default): the normal two-second live basic probe. Response contains snapshots and per-repository failures.
- `recover`: one foreground recovery probe that joins or starts interactive basic observations with a total deadline of 60 seconds.
- `replay`: a two-second cached read from an eligible live tracker. It returns latest accepted state without starting new enrichment.

Only `recover` changes a waiting budget. It is not a larger timeout for every original request.
A recovery response is usable even if the corresponding notification was dropped.
Cache replay retains observed timestamps and does not claim a new live observation.
No mode substitutes persisted data for a failed authoritative live source.
All modes retain ordinary session subscription authorization.
The long recovery runs as tracked cancellable work, not inside a lock or the socket reader loop.
A disconnected client cancels its waiter, while tracker-owned work follows tracker lifetime.

Reuse existing desktop panel activation, mobile mount, connection, and focus ownership.
Coalesce requests by environment, repository scope, and connection/workspace generation across sibling consumers.
The active surface starts one initial snapshot request if subscription delivered no complete snapshot within the normal probe budget.
If the fresh response fails to supply complete membership, run `recover` once for that foreground attempt.
If details remain pending past the worker deadline, use `replay` once to recover a missed completion or failure frame.
If replay still reports pending with no live job, expose unavailable details.
Every attempt terminates in accepted data or an unavailable state with Retry.
There is no interval refresh loop. Timers clear on scope replacement, unfocus, disconnect, or unmount.
Retry and later activation can begin a new attempt. A same-state rerender cannot.

## Frontend and mobile

Keep `gitStatus.byEnvironmentRepo` and shared-environment ownership.
Add a small refresh-state companion map for pending/error/request identity. Do not replace the store infrastructure.
`applyGitStatus` accepts ordering and quality before clearing files or invalidating derived caches.
Metadata-only entries count as dirty membership even without a diff string.
Equality, editor signatures, and Changes focus fingerprints include meaningful phase/facet changes.
Enrichment-only arrival must not steal focus through an empty intermediate state.

`deriveSessionGitValues` exposes membership readiness and refresh/detail state separately from `hasAnything`.
`ChangesPanelBody` owns one scroller and selects the following states:

| Condition                                         | Presentation                                                                 |
| ------------------------------------------------- | ---------------------------------------------------------------------------- |
| No complete snapshot and a request is active      | Loading status, without the clean empty message.                             |
| No complete snapshot and request failed           | Unavailable status and Retry.                                                |
| Complete clean snapshot, no other Changes content | Existing clean empty message.                                                |
| Dirty basic snapshot                              | File rows immediately, pending line totals and diff state.                   |
| Valid prior snapshot during refresh/failure       | Keep rows, show refreshing or last-observed notice with Retry on failure.    |
| Partial multi-repository failure                  | Healthy rows remain, with a named failure notice for each failed repository. |

Workspace restoration and comparison-target notices remain distinct from Git observation failure.
Known PR or commit content remains available even while worktree status loads or fails.
A successful empty worktree does not hide independent PR/commit content.
Only complete accepted membership can prove file or facet removal.
`shouldCloseFileDiffPanel` must check repository/path/layer membership and readiness, not `file.diff` truthiness.
Review source projection and diff headers retain pending files.
Review hashes and editor diff models cannot treat a pending empty string as fresh content.

Desktop uses an inline status row inside the existing Changes body and localized diff placeholder.
Phone uses the same state row inside `MobileChangesPanel` and a placeholder inside `MobileDiffSheet`.
This shared composition follows the shipped phone Changes exemplar. No new overlay or navigation destination is required.
The existing full-height diff drawer retains Back/dismiss, focus return, dynamic viewport, and safe-area behavior.
Changes keeps one vertical scroller. Retry targets are at least 44px on phones/coarse pointers and 28px on desktop.
Loading uses `role=status` with restrained announcements. Failures use an accessible named Retry control.
All new copy uses `t()` and complete locale catalogs. Generate Traditional Chinese through `pnpm run i18n:zh-hant`.

## Observability and verification

Use existing structured logging and debug facilities for observation, publication rejection, enrichment replacement, and recovery outcomes.
Do not log diff content, credentials, or workspace paths. No new metrics subsystem is required.
Test with disposable repositories and controlled barriers. Use public tracker reads, stream delivery, HTTP results, and rendered browser outcomes.
Race tests cover publication and lifecycle ownership. Browser tests cover both shipped compositions and transport recovery.
The [delivery plan](../../../plans/changes-panel-git-refresh/plan.md) maps every new criterion to exact validation commands.

## Related publication decision

See [Tracker-owned progressive Git refresh](../../../decisions/2026-09-30-progressive-workspace-git-refresh.md).

## API surface

Existing Git-status routes remain in place. Their result and stream payloads add optional comparison state so old clients continue to decode them:

- `GET /api/v1/git/status?repo=<subpath>&fresh=<bool>` returns the existing `GitStatusResult` shape.
- `GET /api/v1/git/status/multi?fresh=<bool>` returns the existing `MultiRepoGitStatusResult` shape containing `PerRepoGitStatus` entries. `mode=replay` reads only accepted cache state; it never starts observation or enrichment.
- The `fresh` query parameter continues to select a live observation rather than a cached tracker snapshot.
- `details=wait` on either status route joins the current snapshot's bounded enrichment and returns an explicit unavailable result if that detail job is superseded or fails.
- A repository status MAY include `comparison_target` with its credential-free display identity and `comparison_error_code` when the explicit target is unavailable.
- A mixed-path `FileInfo` MAY include `staged_change` and `unstaged_change`. Each facet uses the same bounded status, line-total, rename, diff, and skip-reason vocabulary as the flattened file fields. Their absence preserves the legacy single-layer shape.
- Commit and cumulative-diff requests use agentctl's configured repository-qualified ref when present. A caller-supplied branch name cannot override it.
- The internal agentctl control API adds a per-repository comparison-target update alongside the existing base-branch update. It validates identities and refs, materializes the exact ref, replaces tracker state, and triggers a refresh.
- The bounded task Git summary adds `comparison_unavailable`; when true, additions/deletions are not rendered as authoritative task-card statistics.

## Failure handling

Each failure is defined at its owning phase above. Accepted membership survives detail failure, partial repository errors retain healthy entries, caller cancellation is isolated, and tracker shutdown drains owned work. Missed completion uses bounded replay; source replacement rejects old results. Failed live sources never use persisted summaries as complete status.

## Out of scope

- Replacing Git-status routes or subprocesses, changing multi-repository fan-out, raising existing diff limits, redefining conflict states, or changing whole-file stage/unstage/discard semantics.
- Rewriting persisted `base_commit_sha` for staleness detection. Read-time correction changes the base used for commits and diffs; capture and explicit "Compare against" resets still own persistence.
- Changing the stored task `base_branch` when a stacked parent merges. Detecting a stale base and selecting a live integration ref remain in scope.
- Adding credential scope or transport fallback for private comparison refs. Moving targets refresh on association, retarget, launch/resume, and explicit comparison refresh, not every status poll.
- Changing checkout, push remote, upstream, or `origin` when the comparison target changes.
- Redesigning the desktop Changes panel or compressing it into a mobile layout.
- Hiding generated untracked paths outside Git ignore rules or adding client-side dependency-tree filtering.

## Implementation plan

Implementation and verification are tracked in [Changes panel Git refresh](../../../plans/changes-panel-git-refresh/plan.md). Earlier related packages remain historical records in `docs/plans/`.
