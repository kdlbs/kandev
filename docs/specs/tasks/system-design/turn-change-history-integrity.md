---
status: current
system: tasks
created: 2026-10-27
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
owners:
  - kandev
---

# Turn change history integrity

## Boundary and existing contracts

This design refines [turn changed-files](turn-changed-files.md).
Tasks owns the accepted comparison, writer attribution, and historical review.
Agentctl performs executor-local Git operations. Runtime owns capture fences
and cleanup. The existing subscription design still owns live reconciliation.

The [decision](../../../decisions/2026-10-27-turn-change-history-integrity.md)
separates raw evidence from a derived comparison.
The existing capture preference, authorization, task state, workflow gates,
autopilot, and executor ownership remain authoritative.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-TURN-CHANGES-002 | Provenance; Comparison selection; Compatibility |
| REQ-TASKS-TURN-CHANGES-003 | Writer intervals; Orphan reconciliation; Runtime integration |
| REQ-TASKS-TURN-CHANGES-004 | Accepted comparison; Resumable export; Persistence |
| REQ-TASKS-TURN-CHANGES-005 | Projection and presentation |
| REQ-TASKS-TURN-CHANGES-006 | Projection and presentation; Historical navigation |
| REQ-TASKS-TURN-CHANGES-007 | Bounds; Diagnostics and verification |

## Evidence and defect classification

Session `836f1f16-2bf1-4051-a9e7-2e237464d0be` retained change set
`83fd1a80-d0b7-57a7-9492-7b4f540352af`.
Every stored path and line count matches the retained start/end Git diff.
The 320-file result is a semantic limitation, not a database count error.

ACP and reflog evidence shows a rebase onto `origin/main`, a soft reset, and
an amended commit. The final branch patch has five documentation files.
Of the captured paths, 317 are outside that patch and match the updated main.
This comparison does not establish which individual files the agent touched.

The snapshot commit writer currently creates parentless commits with a fixed
message. `CheckpointResult` has no real HEAD or comparison-target provenance.
`CompareTurnCheckpoints` compares the complete captured trees directly.

Sibling change set `9687e213-bdd0-59af-8052-67eaf5f288bd` remains pending,
although its canonical turn ended at 13:32:27 UTC.
The investigated turn started at 14:06:08 UTC.
`recordAdmittedOverlaps` currently treats unfinished capture rows as writers.
`ReconcileUnfinished` uses processing time when no terminal claim exists.

Content is ready for 191 files and unavailable for 129.
The contiguous unavailable tail and timing suggest the 15-second export deadline.
Retained logs do not prove that per-file cause.
The export loop discards resumable progress and maps several failures to
`content_unavailable`. Regression tests must establish the timeout path.

History exclusion adds missing product behavior under AC-002.10 through .13.
Writer attribution repairs AC-002.9 and AC-003.6, with explicit recovery criteria.
Batch preservation refines AC-004.5 and AC-007.1 without promising unlimited export.

## Components and source seams

| Component | Responsibility |
| --- | --- |
| `common/turnchanges/contract.go` | Versioned provenance, accepted comparison, and batch DTOs |
| `agentctl/server/process/turn_checkpoint_capture.go` | Capture provenance with the snapshot and retain its source objects |
| `agentctl/server/process/turn_checkpoint_compare.go` | Select direct, adjusted, or unavailable comparison |
| `agentctl/server/process/turn_checkpoint_export.go` | Export bounded batches from the accepted pair |
| `task/changes/coordinator*.go` | Accept comparison once; reconcile writer intervals; schedule bounded preservation |
| `task/changes/content.go` | Validate batches and commit content with durable progress |
| `task/repository/sqlite` | SQLite/PostgreSQL migrations, conditional acceptance, interval reads, export claims |
| `backendapp/turn_changes_runtime.go` and `backendapp/orchestrator.go` | Wire runtime capture and task-owned preservation lifecycle |
| `agent/runtime/lifecycle/manager_turn_changes.go` | Preserve generation fences and bounded stop/cleanup |
| `task/service/service_turn_changes.go` and `task/dto/turn_changes.go` | Project the accepted basis and corrected attribution |
| Web turn-change types, hooks, projection, card, and historical viewer | Render the same comparison and its availability on desktop and phone |

Names introduced below are proposed contracts. The listed files and existing
methods are current source seams. Extract focused files rather than extending
large coordinators past repository limits.

## Provenance at capture

Extend each endpoint receipt with `provenance_version = 2` and a bounded
provenance object:

- Change-set, checkout, and boundary identity.
- Real HEAD commit and symbolic HEAD reference, or explicit unborn state.
- Configured comparison-target identity and its exact resolved commit.
- The unique merge base of that target and real HEAD.
- Resolution status, capability versions, and observed Git-operation state.

Resolve the configured target through `WorkspaceTracker.ComparisonResolution`
and its existing stored-base resolution. Consume the provider-qualified
`ComparisonTarget` when supplied.
Do not introduce a second origin/default-branch heuristic.
An explicitly unavailable target must never fall back to a same-named branch.
Do not reuse mutable status-cache SHAs or the session's initial base SHA.

Resolve target and HEAD before and after snapshot construction.
If either changes, reject that capture's provenance as raced.
Capture locks still coordinate Kandev captures only; unobserved external writes
remain outside an exclusive-authorship guarantee.

Store the canonical provenance in the checkpoint commit body.
Idempotent receipt reads parse that body rather than observing the checkout again.
Keep accepted commit/tree IDs unchanged on retry.
For v2 checkpoints, retain observed HEAD and target commits as parents of the
service snapshot commit, deduplicating identical parents.
An unborn endpoint has no HEAD parent.
These are source-history parents, never previous checkpoint parents.
They retain merge-base ancestry and survive a later rebase or Git GC.

Keep service author metadata fixed. Bound provenance to 16 KiB.
Do not persist credentials, remote URL secrets, or host filesystem paths.
A malformed or mismatched receipt is unavailable, not permission to fabricate metadata.

## Comparison selection

Let `S0` and `S1` be captured working-tree trees.
Let `H0/H1` be real HEAD commits and `B0/B1` be the captured unique merge bases.
The selected target identity must remain the same across endpoints.

A comparison has `comparison_version = 2` and one basis:
`direct`, `upstream_adjusted`, `history_unavailable`, or `legacy_raw`.
Comparison requires a stable symbolic branch binding, including when real HEAD stays unchanged.
A verified title rename can preserve that binding through the receipt extension below.
A branch switch is changed context; detached history needs the same explicit base proof.

### Generated-branch continuity

The [binding decision](../../../decisions/2026-10-30-turn-checkpoint-branch-binding.md)
defines the planned extension for AC-002.14 and AC-002.15.
It adds optional `Provenance.HeadBindingID` (`head_binding_id`) to v2 receipts.
The captured `HeadRef` remains the actual symbolic ref.

Each cached `GitOperator` keeps one bounded branch binding: a UUID and its observed ref.
Capture creates a fresh binding when no binding exists or the observed ref differs.
Detached HEAD has no binding ID. Receipt retries read the original checkpoint body.
They never restore live binding state from an old receipt.

Only the title owner's verified generated-branch path can carry a binding across ref names.
The orchestrator passes the recorded old branch through the
`RenameGeneratedBranchForSession` method and an optional `expected_generated_branch`
field on the existing agentctl rename request.
Ordinary rename callers omit that field.
Agentctl validates the expected branch against actual symbolic HEAD under the
checkout capture lock before Git mutation.
The rename uses explicit old and new branch operands.
Post-operation checks require the expected new ref and unchanged real HEAD.
An existing binding transfers only when its recorded ref matches the observed old ref.
Otherwise, a fresh binding prevents continuity with an earlier checkout state.

Capture and generated rename serialize through `acquireTurnCheckpointLock`.
Binding reads and writes also obey the operator's memory synchronization.
Capture verifies its binding ID with the other before/after provenance fields.
Generic renames and external branch switches cannot transfer a binding.
An ordinary rename clears the live binding, so a recreated branch name cannot
reuse an earlier checkout identity.
The next capture observes the different ref and creates a new ID.
Failed, mismatched, or raced renames never transfer the previous binding.

Comparison accepts different nonempty symbolic refs only when both immutable
receipts contain the same valid, nonempty binding UUID.
When the symbolic ref names match, two known binding UUIDs must still match:
a branch switch can recreate the previous name at the same commit.
A receipt without an ID keeps the legacy equal-ref fallback; differing names
still require valid shared proof.
The same binding predicate also applies where unborn-first-root eligibility
requires branch continuity. Other history eligibility checks retain their meaning.
An ID match never overrides target changes, unsafe operations, missing ancestry,
normalization conflicts, or invalid object evidence.

Once both receipts exist, comparison needs no live binding state.
The existing provenance JSON and checkpoint commit bodies retain the IDs.
No SQL migration, separate journal, new capability version, or public setting is needed.
Agentctl replacement before the end receipt loses unrecorded binding continuity.
A renamed interval then fails closed; recovery never infers proof from reflogs or current refs.
Backend restart with retained agentctl preserves its live binding.
Old receipts without IDs retain the strict ref check when names differ.
Accepted historical comparisons are never recalculated to repair this defect.

Desktop and phone use the existing projection: a ready zero-file result has no card.
A renamed turn with edits uses the existing card and historical diff surface.
Unproven history retains its unavailable state, including the zero-entry exception.

### Direct comparison

Use `S0 -> S1` when valid provenance proves unchanged history context.
This includes ordinary commits, amendments, staging, and resets with unchanged
base and target identity.
Unchanged real HEAD also permits a direct comparison when the target is unavailable.
Two unborn endpoints can compare directly.

A transition from unborn to its first root commit can compare directly when
the ending repository contains no imported predecessor history.
Other unsupported base-free HEAD movement is unavailable.
A missing target is not permission to classify arbitrary new history as local work.

### Upstream-adjusted comparison

All eligibility conditions must hold:

1. Both endpoint receipts are valid v2 receipts for the same checkout.
2. Both have one merge base with the same independently resolved target.
3. The target is distinct from the active writable branch.
4. `B0` is a strict ancestor of `B1`, and both bases belong to their captured HEAD ancestry.
5. Capture observed no active merge/rebase operation or provenance race.
6. The executor supports the controlled tree-merge capability.
7. The symbolic branch binding stayed continuous under the receipt rules above.

The target commit may already be ahead at turn start.
Use the captured merge base, not the target's tip, as the starting branch baseline.

Construct a private three-way merge with base `B0`, upstream side `B1`,
and local side `S0`. Its clean result `N0` is the starting content carried
onto the new upstream base.
Compare `N0 -> S1`.

Use `git merge-tree --write-tree --merge-base=<B0>` in a controlled Git context.
Probe the capability before advertising it.
Use captured attributes, controlled merge configuration, and the original
object store. Reject unsupported custom merge attributes/drivers.
Never run repository-defined merge commands, hooks, checkout, stash, reset,
or user-index mutations to compute this baseline.

Accept only Git's clean merge result.
A conflict result contains a tree OID too; its nonzero conflict status forbids acceptance.
Do not choose ours/theirs, strip conflict markers, or retry with another strategy.
Persist `N0` under an owned comparison ref before returning its receipt.

This preserves the complete starting local state, including dirty edits and
earlier task commits. An upstream change and a local edit within one file can
both survive a clean merge. Removing a starting local edit remains a real change.

### Unavailable history

Use `history_unavailable` for conflicting normalization, changed target identity,
rewound/divergent bases, multiple merge bases, unsupported Git capabilities,
raced provenance, or unsafe operation state.
Changed HEAD without sufficient base evidence is also unavailable.
Use closed reasons such as `history_conflict`, `history_context_changed`,
`history_provenance_unavailable`, and `history_unsupported`.

Keep the raw snapshots for evidence. Do not emit their broad diff as turn edits.
Do not present unavailable counts as successful zero.
Other repositories continue independently.

### Why path filtering is insufficient

Never remove a file merely because its end blob equals an upstream blob.
A reset or restore can legitimately remove a starting local edit.
Never use only the final branch patch, which includes preceding turns.
Write-tool events and ACP strings are diagnostic evidence, not comparison authority.

## Accepted comparison

Preserve the raw endpoint columns in `turn_repository_changes`.
Add nullable provenance and comparison fields through replayable migrations:

- `start_provenance_json` and `end_provenance_json`.
- `comparison_version`, `comparison_basis`, and `comparison_reason`.
- Effective start/end commit and tree OIDs, plus the owned comparison ref.

The effective end is always the accepted raw end.
The effective start is the raw start or the retained derived baseline.
An unavailable comparison has no effective pair.
A migrated legacy row has basis `legacy_raw` and no fabricated provenance.

Add conditional acceptance for the comparison, keyed by repository-change ID,
accepted raw pair, checkout, hash algorithm, and algorithm version.
Accept it before file summaries or exports.
Retries reuse the same accepted pair and basis.
They cannot recalculate against later refs, replace files, or mix algorithms.

Comparison and export responses echo both raw identity and accepted effective
identity. The coordinator validates both.
The backend explicitly requests comparison version 2 only after capability discovery.
An absent version keeps legacy raw behavior, including for an older backend
talking to a newer agentctl. Reject unknown versions instead of silently downgrading.
Summary counts, file IDs, canonical patches, whitespace variants, and rendering
blobs must all refer to the same effective pair.
Retain the comparison ref until content acknowledgement or explicit unavailable settlement.

## Writer intervals

Writer attribution uses canonical turn identity, not capture availability.

For each checkout, derive a half-open interval:
`[start_captured_at, writer_end)`.
The end is the earliest valid terminal claim or canonical turn completion
for that exact turn and ownership.
A missing end denotes an active or unknown interval; missing start is not a valid interval.
An end before its start produces no positive writer interval.

Two rows overlap only when checkout identity matches and:
`max(startA, startB) < min(endA, endB)`.
Intervals meeting at a boundary do not overlap.
Export, compression, retries, and ref cleanup never extend these intervals.

Add a repository interval reader that joins change sets to their canonical turns.
Read only admitted checkout identities and stable cursor pages.
Do not use a global first-500 unfinished-row scan as liveness authority.

At admission, reconcile candidates' canonical ends before recording overlap.
At canonical settlement and periodic reconciliation, revise affected overlap
records for both pending and finalized change sets.
Remove empty intersections, shorten ended intersections, and preserve genuine ones.
Publish incremented revisions through the existing summary publisher.

## Orphan reconciliation

Reuse `Coordinator.ReconcileUnfinished`, its startup wiring, and the existing
task reconciliation sweep.
Read exact canonical turns in bounded pages, including completed rows whose
capture processing remains unfinished.

A canonical completed turn closes attribution immediately.
It does not authorize capturing a later end.
For capture settlement, require matching ownership and absence of an active
capture-boundary operation for that exact execution/generation.
Expose this evidence through a narrow construction callback from runtime.
Unknown runtime ownership fails closed and is retried; session idle state alone
is insufficient proof.

If an accepted end and comparison exist, resume preservation from those objects.
If the original end boundary is lost, settle historical unavailability using
the recorded terminal time. Never substitute reconciliation time for a known end.
Do not close active siblings or replace a valid terminal claim.

Startup runs this repair after canonical turn recovery can provide its result.
Periodic reconciliation catches later recovered completions.
A genuine completed overlap remains visible after both exports finish.
These changes do not complete turns, cancel agents, or advance workflows.

## Resumable export

Extend the internal agentctl contract with advertised `export_batch_version = 1`.
A batch binds change set, checkout, raw pair, effective pair, hash algorithm,
comparison version, and a deterministic inventory fingerprint.
Backend-selected files retain exact path bytes and stable entry IDs.

Return a bounded prefix and its continuation when the page deadline expires.
Do not fill the entire remaining inventory with generic unavailable entries.
A zero-progress timeout remains a retriable timeout, subject to attempt bounds.

Use at most 25 entries per batch.
Preserve the existing 4 MiB patch, 8 MiB rendering, and 32 MiB cumulative
uncompressed turn-export limits.
A batch cannot exceed the remaining turn budget.
An oversized entry receives an explicit size-limit result and advances once;
it must not trap the cursor.

Persist each acknowledged page and its next cursor in one transaction.
Use the current compressed content store and a new page receipt method.
A lost response can replay that page without duplicate bytes or links.
Cursor advance occurs only after durable storage.
Reject wrong pair, inventory, cursor, lease, or generation responses.

Keep export progress in `turn_repository_changes`:
state, cursor, attempt count, next-attempt time, admitted bytes, and claim owner/expiry.
Use states `queued`, `running`, `retrying`, `complete`, and `unavailable`.
Claims have a monotonic token so an expired worker cannot overwrite its successor.
Existing content leases protect preserved data from retention during an attempt.

A task-owned bounded processor resumes eligible rows.
Wire it once in `backendapp`; do not introduce Office or generic run scheduling.
The processor has at most two active attempts per installation.
Each attempt has a 20-second total deadline and each agentctl page has a
15-second ceiling within that remaining deadline.
Allow at most three attempts in total, with 75/150 ms retry delays.
Persist attempt admission before external I/O.
Execution time is bounded to 60 seconds per repository, excluding process downtime.
Restore claims only after their expiry and revalidate the original checkout identity.

Use batch subprocess reads where practical instead of one full comparison
and several independent Git processes for every file.
The already accepted inventory and comparison remain authoritative.

## Runtime integration and cleanup

The existing start fence still precedes provider dispatch.
The terminal fence ends after accepting immutable endpoints, as current source
already permits. Comparison and preservation use those objects asynchronously.

Publish the summary when its comparison is durable.
Represent pending export separately from missing content in the data contract.
Allow acknowledged files to open immediately and reuse existing viewer loading states.
Do not hold prompt, execution-store, or database locks across Git or transport I/O.

Cleanup asks the processor to drain within the existing cleanup budget.
Stop/Cancel gets at most two seconds of preservation after provider-stop
acknowledgement. No export retry can extend that budget.
If preservation cannot finish, atomically invalidate the claim and mark the
remaining content unavailable with `export_interrupted` before releasing refs.
A late response cannot restore expired, deleted, or settled rows.

A backend shutdown preserves durable cursor state and follows the established
agent-survival policy. It must not send an extra ACP cancel.
On restart, resume only accepted immutable endpoints.
Missing objects or a changed checkout settle unavailable without relaunching
an executor or capturing today's files.

Release source refs only after all promised content is acknowledged or a typed
unavailable/expiry result is durable.
Retention and task deletion cancel claims and preserve referential cleanup.

## Compatibility

| Shape | Behavior | Evidence and fallback |
| --- | --- | --- |
| New backend and capable agentctl | Provenance, history adjustment, and batches | Real Git, SQL, API, lifecycle, desktop, and phone tests |
| Retained older agentctl | Explicit legacy raw comparison; bounded single export | Capability-absent client fixture; no ignored-field pagination loop |
| Older backend and newer agentctl | Unversioned requests retain raw comparison/export | Compatibility test; additive receipts do not opt clients into adjustment |
| Existing stored captures | Preserve original files, content, and counts as legacy raw | Upgrade/replay tests; no live Git repair |
| Same HEAD, unresolved target | Direct snapshot comparison | Real Git fixture; no inferred new branch history |
| Commits/amend/reset with stable independent base | Direct comparison | Preserve turn edits and legitimate reverts |
| Rebase or merge with advancing stable base | Clean adjusted baseline | Conflict means unavailable |
| Missing/divergent/ambiguous history | Unavailable comparison | No turn-edit totals or current-workspace fallback |
| Local/worktree/container/SSH/Kubernetes/Sprites | Same executor-local capability when advertised | Unit transport contracts do not establish real executor support |
| Provider-qualified target | Use its materialized identity; never same-name origin fallback | Existing target failure fixtures plus provenance regression |
| Plugin executor without capability | Existing unsupported/legacy treatment | No host filesystem substitution |
| Unborn/detached/shallow repository | Direct only with sufficient immutable proof | Missing ancestry or unsupported movement is unavailable |

This package requires local/worktree browser evidence and shared transport tests.
Existing executor qualification gaps remain recorded in the original plan.
Do not claim new Kubernetes, Sprites, or plugin qualification from shared code.

## Required presentation baseline

Build implementation on main containing [PR #4831](https://github.com/kdlbs/kandev/pull/4831).
The PR merged on 2026-10-27 at 16:13:24 UTC as
`f80b89ec4993d6dda5c6f69b08c6bcc0d3c40277`.
The inspected source head was `75a7709f04850f3173a4d653973b325dd2cc1bd3`.
A fresh comparison confirmed main equals the merge commit.
The merged card, style criteria, and presentation plan were re-read.
Refresh main before implementation and final integration, and verify it contains this merge.
This design turn does not rebase the unstaged package or apply production styles.

Retain that PR's `FileIcon`, `LineStat`, file-kind palette, responsive wrapping,
and non-collapsing 16px gap after the card.
Preserve fine-pointer and coarse-pointer path disclosure and its geometry tests.
The [presentation package at the merged base](https://github.com/kdlbs/kandev/blob/f80b89ec4993d6dda5c6f69b08c6bcc0d3c40277/docs/plans/turn-changed-files-presentation/plan.md)
owns those styles. Reuse its components and controls without another visual design.

PR #4831 owns AC-005.10 through .12.
This package's earlier unaccepted draft also used .10 for extra status explanations.
That conflicting draft proposal is withdrawn. AC-005.13 now defines the minimal
presentation rule; the PR's existing IDs and meanings remain unchanged.
Reconcile all three artifacts when stacking, including both additions to the core design.

## Projection and presentation

Extend the shared repository DTO with comparison basis/reason and export state.
These fields primarily drive correctness and loading behavior; they are not new
routine labels or a second status system.
Derive aggregate `counts_available` from usable repository comparisons.
One valid repository is sufficient for available totals.
All admitted repositories must have complete comparisons for summary completeness.
All required files must be durable for content completeness.

For direct and upstream-adjusted success, preserve the PR #4831 card unchanged.
Only the corrected files and counts differ.
Do not add adjustment badges, explanatory paragraphs, extra rows, new controls,
or a visible saving/export progress message.
A complete adjusted comparison with zero changes keeps the existing hidden-card behavior.

For `legacy_raw`, change only the existing count wording to "repository changes".
Keep its files, totals, colors, controls, and row structure unchanged.
When any root is legacy, use repository wording for aggregate counts too.
The basis remains in retained metadata; no per-root basis badge or explanatory
paragraph is required on an otherwise usable card or diff.

When every comparison is unavailable, omit numeric totals and use one existing
unavailable message in the current card status area, even without file entries.
Do not duplicate it in the header and body or introduce another warning surface.
For mixed results, reuse the existing Partial indicator and repository status
positions. Keep healthy roots and available counts usable.
A selected unavailable historical diff can explain the cause in its existing
empty/error area. It never falls back to current changes.

Keep content preservation in the background.
Saved files remain usable; a selected pending file uses the existing loading state.
After bounded failure, use the existing unavailable/partial state, with a specific
reason only where the user is trying to open the affected historical content.
Do not show a shared-checkout note unless canonical writer intervals intersect.

Reuse localized unavailable/loading/partial copy where it already exists.
Only necessary count wording or selected-diff reasons need additional catalog entries.
Keep technical OIDs, cursors, capabilities, retries, and algorithm versions internal.

## Historical navigation and mobile

Use the existing shared projection, `turn-changed-files-card.tsx`, and
`historical-turn-diff-viewer.tsx` after integrating PR #4831.
Preserve the exact change-set/repository/file target and current controls.
Do not duplicate history explanations between transcript and viewer.

The closest mobile exemplars remain `MobileDiffSheet` and `MobilePickerSheet`.
Keep the PR's existing inline card, stacked header/actions, metadata wrapping,
filename disclosure, and measured 44-pixel phone/coarse targets.
Do not insert an explanation row or change fine-pointer desktop density.
Healthy file selection still opens the existing full-height drawer directly.
Preserve dynamic height, safe areas, one internal scroll owner, and focus return.

Desktop and phone share projection, identity, and revision handling.
The completed subscription reconciliation package remains authoritative for
late updates, reconnection, and older in-flight reads.

## Persistence and upgrade

Add columns through `base_migrations.go` before dependent indexes or backfills.
Mirror them in fresh-table DDL and every table-rebuild path.
Cover SQLite and PostgreSQL with the required-store and upgrade fixtures.

Default existing comparisons to `legacy_raw`.
Do not rewrite their file lists, accepted endpoints, or content IDs.
Only overlap attribution can be corrected from canonical stored turn timestamps.
That correction increments the summary revision and preserves genuine overlaps.

Use conditional, transaction-bound comparison and page receipts.
Never hold a transaction across executor access.
A page's file links, compressed-byte accounting, cursor, and claim token
commit together. Retention and export participate in the existing storage gate.

No preference, environment variable, runtime flag, or public retention setting
is introduced. Public docs change with implementation, not with this design package.

## Bounds

| Work | Bound |
| --- | --- |
| Provenance per endpoint | 16 KiB; existing capture deadline |
| Normalization | 15 seconds within processing attempt; one accepted result |
| Export page | 25 entries; at most 15 seconds and remaining byte/time budget |
| Export attempt | 20 seconds; at most three admitted attempts per repository |
| Export concurrency | Two active attempts per installation |
| Canonical patch/rendering | Existing per-file limits and 32 MiB cumulative turn export |
| Stop/Cancel preservation | Two seconds after provider-stop acknowledgement |
| Reconciliation page | 100 candidates with stable cursor; no first-500 liveness shortcut |

The 32 MiB budget spans every repository of the turn, not each batch.
Reserve byte admission atomically across concurrent repository exports.
A batch replay does not reserve those bytes again.
A bounded reconciliation invocation can yield; the next invocation continues
with a durable cursor rather than skipping the remaining candidates.

## Diagnostics and verification

Use bounded reason codes for history conflict/context/provenance/capability failures
and export timeout, transport, missing-object, size, and cleanup failures.
Keep raw commands, content, credentials, and sensitive target data out of logs.
Log task/turn/repository identity only as structured correlation fields.
Metric labels remain closed and exclude those identities.

The disposable design probe used real Git in a temporary repository.
It reduced a 319-path checkout diff to the single new turn file, preserved an
unchanged dirty file, retained a legitimate revert, and rejected a merge conflict.
This is feasibility evidence only. It is not implementation or regression-test completion.

The [fix plan](../../../plans/turn-change-history-integrity/plan.md) assigns
behavioral RED tests, exact verification commands, schema parity, rendered checks,
and failure injection. Implementers must validate custom-attribute isolation,
lost acknowledgements, stale claims, mixed active/completed turns, and cleanup races.

## Related artifacts

- [Requirements](../requirements/turn-changed-files.md)
- [Core design](turn-changed-files.md)
- [Subscription design](turn-changed-files-subscription.md)
- [Decision](../../../decisions/2026-10-27-turn-change-history-integrity.md)
- [Original delivery plan](../../../plans/turn-changed-files/plan.md)
- [Fix plan](../../../plans/turn-change-history-integrity/plan.md)
- [Generated-branch continuity fix plan](../../../plans/turn-change-title-branch-continuity/plan.md)
