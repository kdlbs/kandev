---
status: current
system: tasks
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-003
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-004
created: 2026-09-21
updated: 2026-10-18
owners:
  - cfl
---

# Managed Worktree Task Archive System Design

## Purpose and boundaries

Tasks owns archive authorization, runtime stop ordering, cleanup jobs, and
archive-generation fencing. The worktree manager owns audited physical removal.
The replacement design implements immediate removal under requirement 004.
Requirement 001 and the superseded portions of 002 and 003 remain historical
traceability; they no longer require cleanliness for archive removal.

The change removes a retention policy, not ownership checks. It uses the
existing asynchronous archive cleanup attempt, with its deadline and retries;
“immediate” means no dirtiness deferral, not a new synchronous API guarantee.
Delete consent, ordinary local executors, independent-checkout adoption consent,
remote executor teardown, session deletion, reset, and storage schedules retain
their existing contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| 001; 002.1-.4 and .6 (superseded) | Replacement policy and compatibility |
| 002.5 | Storage boundary |
| 003.1-.4, subject to 004 | Task-root completion |
| 004.1-.2 and .4 | Archive admission and manager removal |
| 004.3 and .7 | Ownership and runtime boundaries; Task-root completion |
| 004.5 | Existing retained work and restart |
| 004.6 | Confirmation and documentation |

All abbreviated numbers in this table use the
`REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-` / corresponding `AC-` prefix.

## Replacement policy and compatibility

[Immediate archive removal](../../../decisions/2026-10-17-immediate-archive-worktree-removal.md)
replaces dirty-checkout retention while preserving the lifecycle/storage split
from the [reclamation decision](../../../decisions/2026-09-24-archived-worktree-reclamation.md)
and the terminal-root operation from the
[task-directory decision](../../../decisions/2026-10-13-archived-task-directory-cleanup.md).
Prior implemented plans remain delivery history and are not reopened.

There is no new feature flag, discard parameter, consent flow, schema, or job
format. Existing archive entry points share `cleanupDestructiveTaskResources`.
API acknowledgement still archives the task while physical cleanup proceeds in
the background. Failures retain their existing diagnostic/retry behavior.

## Archive admission and manager removal

`Service.cleanupDestructiveTaskResources` keeps eligibility filtering for
ownership, shared references, and runtime stops. In its `preserveBranches`
branch, pass the entire resulting eligible set to
`CleanupWorktreesPreservingBranches`. Remove
`filterDirtyWorktreesForArchive` and the archive-only suppression of dirty
cleanup errors. Delete-path `InspectDirtyWorktrees` remains intact.

`Manager.CleanupWorktreesPreservingBranches` and
`Manager.CleanupArchivedWorktree` stop setting `RequireCleanCheckout`; remove
that now-unused option and its archive-only audit gates. Keep
`removeBranch=false` and the expected owner/path tuple used by reclaim.
Registered managed Git worktrees already support force removal through the
audited cleanup path; no new recursive deletion primitive is needed.
Do not switch archive to `CleanupWorktreesWithOptions`, which uses different
branch-removal semantics, or globally set `DiscardWorktreeChanges`, which also
authorizes independent adopted checkouts. Search other callers of the two
archive manager methods and verify that their archive intent remains valid.

`RequireCleanCheckout` currently checks cleanliness before and after repository
cleanup scripts. Removing the archive option disables both archive checks.
Staged, unstaged, untracked, ignored, and script-created files disappear with
the admitted checkout. Identity validation, Git registration verification,
locks, pinned directory handles, and branch-compaction audits remain active.
Cleanup errors are ordinary retryable failures, with no successful dirty-skip
outcome. Remove unused archive-only dirty helpers after checking all callers.

## Ownership and runtime boundaries

Original cleanup continues stopping every required runtime before destructive
work. `cleanupEligibleWorktrees`, ownership-generation checks, and shared
references decide each repository independently. A mixed clean/dirty set is
fully eligible by cleanliness; a mixed eligible/shared set removes only the
eligible members and protects the root.

Reclaim continues loading the current task and worktree, checking the exact
`archived_at`, owner, expected path, and active references. Before removing a
checkout it also lists the owning task's runtime rows. Any retained runtime
defers reclaim as in use, including after the original archive stop fails;
missing or failed runtime inventory keeps the job retryable. Manager mutation
checks retain `ExpectedTaskID`, `ExpectedWorktreePath`, and
`ExpectedRepositoryPath`. Removing the dirty predicate must not turn inspection,
identity, reference, or stop failures into successful cleanup.

Inventory-repair admission follows the same archive policy after identities
are repaired. Repair itself preserves local files and Git refs; a later
authorized archive removes eligible clean or dirty checkouts while retaining
the audited branch-identity and recovery-ref checks.

## Existing retained work and restart

`processArchiveReclaimJob` and `archiveReclaimWorktreeOutcome` no longer inspect
Git dirtiness or wait on `ErrDirtyWorktreeCleanup`. Remaining in-use outcomes
continue to defer with bounded scheduling. Keep persisted `archive_reclaim`
triggers and `waiting_for_clean` values readable for upgrade compatibility;
the historical state name does not imply a live cleanliness predicate.

The existing candidate reconciliation creates missing jobs for archived active
worktree rows. Extend startup reconciliation with a bounded wake of existing
`archive_reclaim` jobs in `waiting_for_clean`, so a future `next_attempt_at`
from the old policy does not postpone removal by 24 hours. `WakeWaitingArchiveReclaimJobs` in
`task/repository/sqlite/resource_cleanup.go` supplies the bounded repository
operation through a service-local contract. Use an ID cursor and a startup timestamp cutoff;
conditionally change only matching waiting rows last updated before that
cutoff to pending. Do not modify snapshots, archive identities, attempt counts,
ordinary retry deadlines, cancelled/succeeded jobs, or running claims.

Process bounded pages through the existing worker until the startup cursor is
exhausted. Do not re-wake an in-use job deferred by this process. Waking grants
no destruction authority: the normal claim and archive/ownership checks still
run, and unarchive can cancel the pending operation under the existing fence.
A restart safely repeats the bounded admission; no persistent migration marker
or schema change is needed. SQL must use the shared dialect bindings and work
with both SQLite and PostgreSQL. Database failures remain visible and retryable.

Do not rerun attachments or unrelated task cleanup from a reclaim job. Do not
reopen succeeded original archive jobs or discard a saved root obligation when
an individual checkout has already been removed.

## Task-root completion

Keep `captureArchiveTaskDirectory`, `captureReclaimTaskDirectory`,
`cleanupArchiveTaskDirectory`, and `finishArchiveReclaimDirectory` as the
terminal task-root operation. Before destructive I/O, retain the root target,
immutable creation marker, current owner, admitted worktree paths, and native
filesystem identity in the existing cleanup snapshot.

Root eligibility is universal: every recorded worktree must have confirmed
removal. Any active borrowed, failed, unknown, or otherwise retained sibling
prevents recursive root removal. Dirtiness alone no longer retains a sibling.
After the final eligible checkout disappears, remove loose files, nested
folders, and the marker in that same cleanup attempt.

Reuse no-follow directory handles, path/repository locks, marker and identity
validation, and unarchive fencing. Internal symlinks are removed without
traversing targets. Replaced roots, symlinked roots/parents, foreign markers,
base-directory targets, and inconsistent paths fail closed. Individual
worktree/session removal keeps the empty-only `tryRemoveEmptyTaskDir` behavior.

Root failures remain durable across restart even after the worktree row becomes
deleted. A busy root with no active checkout remains a retry obligation.
Missing evidence cannot be treated as proven absence. Legacy unmarked roots
retain audited individual-checkout cleanup without gaining recursive authority.
Removed-root recording for orphan-process cleanup continues to include the
successfully removed root and worktree paths.

## Storage boundary

`storageInventory.activeWorktreePaths` continues protecting every active
physical worktree row, including archived owners. `workspaces.buildProtectedSet`,
`PermanentDelete`, and `PermanentDeleteForce` keep their inventory checks for
quarantine and purge. A retained shared or failed workspace is still protected;
a deleted historical row alone does not protect an absent checkout. Storage
maintenance does not acquire archive destruction authority.

## Confirmation and documentation

Update the existing archive-specific locale keys
`archiveSingleWorktreeCleanup` and `archiveBulkWorktreeCleanup_one/_other` in
`src/locales/*/task.json`. Proposed single-task copy:
“Git worktrees and local changes are deleted on archive.” Keep
`archiveWorktreeBranchNote`, runtime-stop text, local-executor summaries, and
delete summaries accurate and separate. No new dirty-file list or consent flag.

`task-cleanup-summary.ts` supplies full, compact inline, and bulk archive
consumers. `task-archive-confirm-dialog.tsx` uses the same summary in its desktop
AlertDialog and phone `MobileActionConfirmation` bottom drawer. Reuse those
surfaces and the task confirmation primitives; change copy only. Maintain
Cancel/Archive order, focus return, existing cascade controls, confirmation
bypass, safe-area behavior, scrolling, and phone touch targets.

Add the updated copy in English, Portuguese, Simplified Chinese, both
Traditional Chinese locales, Japanese, and Korean; use `i18n:zh-hant` for the
Traditional pair and regenerate pseudo through the repository tooling. Existing
component and desktop/phone Playwright checks must assert destruction wording
and absence of the old retention promise. Extend the mobile confirmation
preference spec with a confirmation-enabled worktree case.

At implementation, update the how-to sections in
`docs/public/tasks-and-workflows.md`, `docs/public/git-operations.md`, and
`docs/public/operations.md`, and inspect README/screenshot guidance for old
wording. Public docs must distinguish lost local files from retained committed
branch recovery and from guarded cleanup failures. Public wording ships with
the production behavior.

## Observability and validation

Keep task, worktree, and job identifiers in existing cleanup logs; do not add
new metric labels or log file contents. Successful dirty-checkout removal uses
normal cleanup success and branch receipts. There is no new dirty-deferral log
for an eligible archive checkout.

Service integration tests exercise the real ArchiveTask, manager, SQL jobs,
and filesystem. Manager tests cover dirtiness created by cleanup scripts and
retained identity/branch rules. Repository tests cover bounded startup wakes
and claim/cancellation races. Desktop and phone rendered checks prove truthful
copy and archive interaction. The linked work order owns exact commands.

## Implementation plan

[Immediate archive removal](../../../plans/immediate-archive-worktree-removal/plan.md).
