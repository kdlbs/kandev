---
created: 2026-10-27
status: done
requirements:
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
  - REQ-TASKS-TURN-CHANGES-007
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
  - ../../specs/tasks/system-design/turn-changed-files-subscription.md
legacy_specs: []
---

# Fix plan: Turn change history integrity

## Overview

Make turn comparisons account for upstream history changes while retaining the
original snapshots. Derive overlap from canonical writer intervals and preserve
diff content through bounded, resumable export.

Deliver provenance first, then accepted comparisons, writer reconciliation,
durable export, and minimal integration with the PR #4831 desktop/phone UI. All
five work orders are done and unstaged for review; implementation results and
environment-only verification gaps are recorded below. Commit, push, live-database
repair, and historical rewrite remain outside this package.

Tasks owns this package because turn identity binds its evidence and presentation.
Use the [existing requirements](../../specs/tasks/requirements/turn-changed-files.md)
and [history integrity design](../../specs/tasks/system-design/turn-change-history-integrity.md).
The [proposed decision](../../decisions/2026-10-27-turn-change-history-integrity.md)
records the comparison boundary and rejected alternatives.

## Required base: merged PR #4831

The user requires the end-of-turn styles from
[PR #4831](https://github.com/kdlbs/kandev/pull/4831), without additional UI complexity.
The PR merged on 2026-10-27 at 16:13:24 UTC as
`f80b89ec4993d6dda5c6f69b08c6bcc0d3c40277`.
Its inspected source head was `75a7709f04850f3173a4d653973b325dd2cc1bd3`.
A fresh remote comparison confirmed main equals the merge commit.
The merged card, requirement criteria, and presentation plan were re-read;
both attached desktop/phone screenshots were inspected.

Before implementation, refresh main and verify it contains that merge.
Build the implementation branch on that base and preserve/reconcile this
unstaged design package. No branch rebase or production style change was
performed in this design turn. Recheck main before final integration.

Read the merged [presentation plan](https://github.com/kdlbs/kandev/blob/f80b89ec4993d6dda5c6f69b08c6bcc0d3c40277/docs/plans/turn-changed-files-presentation/plan.md)
and [work order](https://github.com/kdlbs/kandev/blob/f80b89ec4993d6dda5c6f69b08c6bcc0d3c40277/docs/plans/turn-changed-files-presentation/task-01-summary-presentation.md)
from the resolved base. Keep their recorded statuses and evidence.

Acceptance-ID reconciliation is explicit: PR #4831 retains AC-005.10 through .12
for counts/colors, icons, and spacing/wrapping. The earlier unaccepted .10 in this
fix draft conflicted with that ownership and is withdrawn. The minimal status
integration now uses AC-005.13. No PR styling criterion is repurposed.

## Confirmed evidence and scope

The investigated session is 836f1f16-2bf1-4051-a9e7-2e237464d0be, task
d694147a-7b14-4cfd-9fe0-53f25f0d30cf.
Its raw start/end diff contains 320 files, +13,633/-1,191.
Every stored file count matches Git.
The final branch patch is five documentation files, +125/-20.
Of the captured paths, 317 match the updated main outside that final patch.
These observations establish base movement, not exclusive authorship.

ACP records the rebase at 14:13:50 UTC, successful continuation at 14:26:58,
soft reset at 14:30:29, and amended commit 5258da4.
Raw checkpoint trees are 2ff24571450fa5e93dec27782fd5586d1c016fbe and
15aa2b2ea1e0e787b9b53a611f1580a6a0b25a66.
The latter equals the final commit tree.

A sibling turn ended at 13:32:27 UTC but its capture still had no terminal time.
The investigated turn began at 14:06:08 UTC.
The export stored content for 191 files and omitted 129.
A deadline explains the likely failure path, but its historical per-file cause
was not retained. Work order 04 must prove the timeout behavior with fault injection.

### In scope

- Versioned real HEAD, target, and merge-base provenance with immutable receipts.
- A separately accepted direct or upstream-adjusted comparison pair.
- Explicit unavailable comparison and legacy snapshot semantics.
- Canonical writer intervals and bounded repair of orphan capture attribution.
- Durable export cursors, conditional claims, typed failures, and cleanup fencing.
- Shared API/WS projections and minimal integration with PR #4831 card styles and existing diff navigation.
- Regression tests, schema replay/parity, desktop/phone E2E, and implementation-time public docs.

### Out of scope

- Guessing authorship from ACP tools, filtering paths that match main, or using the final PR patch.
- Rewriting old endpoint/file/content records or repairing the developer's live database.
- Completion gates, autopilot, agent-survival policy changes, or new runtime flags.
- A new diff application, raw/adjusted view toggle, restore actions, or whole-checkout copies.
- Additional routine badges, explanation rows, progress messages, controls, or card styling beyond PR #4831.
- Closing the original package's unrelated Kubernetes, Sprites, plugin, and storage qualification gaps.

## Technical approach

### Capture and storage

Extend common/turnchanges receipts and the agentctl capture path with v2 provenance.
Use WorkspaceTracker.ComparisonResolution and existing base resolution; pin exact
objects without fetching or inventing a default target.
Preserve explicit provider-target failures.
A checkpoint body retains its own provenance. Observed HEAD/target parents retain
source history without linking successive checkpoints.

Add provenance and comparison fields through replayable task-store migrations.
Keep new fields additive and accepted endpoints write-once.
Record missing legacy provenance explicitly.

### Comparison

For a stable independent target whose unique merge base advances from B0 to B1,
merge the starting snapshot onto B1 with explicit base B0.
Accept only a clean controlled tree merge.
Use that derived start and the captured end for every file, count, and patch.
An unavailable comparison has no numeric turn-edit claim.
The design defines direct, unborn, detached, shallow, conflict, and capability cases.

### Writer intervals

Replace unfinished-capture liveness with canonical turn boundaries.
Read half-open intervals for actual checkout identities and intersect them strictly.
Use known terminal times, never processing time, when fixing a completed turn.
Guard orphan capture settlement against an active operation for the exact runtime
generation. Correct both finalized and pending overlap projections through revisions.

### Export

Publish a durable comparison summary before content processing finishes.
Save each bounded page and cursor atomically.
Use inventory/pair identity and a leased claim to reject stale responses.
Apply one cumulative turn byte budget across all repositories.
Resume at most three 20-second attempts with two concurrent attempts per installation.
Stop/Cancel preservation stays within two seconds after provider-stop acknowledgement.
Cleanup settles missing content explicitly before releasing its sole source.

### Presentation

Build on PR #4831 and retain its card layout, controls, FileIcon, LineStat,
status colors, 16px message gap, path disclosure, and mobile wrapping.
Correct normal files/counts without adding adjustment or export messages.
Legacy count wording and existing unavailable/loading/partial slots provide the
necessary exceptions. Detailed failures stay in the selected historical diff.
Preserve exact selection, revision ordering, and landed subscription recovery.

### Compatibility matrix

| Boundary | Intended behavior | Required evidence | Fallback |
| --- | --- | --- | --- |
| Capable agentctl on local/worktree | v2 provenance, accepted comparison, batched export | Real Git, SQL, API, desktop and phone tests | Typed history/content reason |
| Older retained agentctl | Explicit legacy raw semantics and bounded one-shot export | Capability-absent client/API fixture | Never loop on ignored batch fields |
| Existing stored history | Original files/counts remain unchanged | SQLite/PostgreSQL replay and legacy rendering | Repository snapshot label |
| Provider-qualified target | Use its materialized ref and identity | Explicit-unavailable target test | No same-name origin fallback |
| ACP/native provider transports | Same filesystem/capture path | Runtime prompt-generation tests | No tool-specific authorship logic |
| Container/SSH/Kubernetes/Sprites | Capability-dependent shared protocol | Client contract tests in this package | No new real-executor qualification claim |
| Unborn/detached/shallow/custom merge driver | Apply documented proof/capability gate | Real Git failure fixtures | Direct only when proven; otherwise unavailable |
| Multiple checkouts | Independent comparisons and shared byte budget | Mixed healthy/unavailable and concurrent-page tests | Healthy roots remain usable |

## ASCII UI preview

PR #4831 supplies the visual baseline. Counts below are illustrative, not a
retroactive corrected count for the legacy incident.
The change corrects data while keeping the normal layout and information density.

### UI-01: Same normal card before and after the data correction

```text
Before, with PR #4831 styles and broad snapshot counts:
+-----------------------------------------------------------+
| 320 changed files +13633 -1191   [Expand all] [Open diff]  |
| > kandev                                                 |
+-----------------------------------------------------------+

After, with the same styles and corrected turn data:
+-----------------------------------------------------------+
| 1 changed file +6 -2            [Expand all] [Open diff]  |
| v kandev                                                 |
|   v docs/                                                |
|     [MD icon] plan.md            Modified +6 -2           |
+-----------------------------------------------------------+
             existing 16px message gap
[next message]
```

Keep filename icons, amber Modified, green additions, red deletions, and the
existing border/spacing. Folder expansion remains optional and independent.
No history-adjustment badge, explanation row, or progress text is added.
Map: AC-002.10-.11, AC-005.10-.13, AC-006.3.

### UI-02: Existing phone composition

```text
+------------------------------------+
| 1 changed file +6 -2               |
| [Expand all]        [Open diff]    |
|------------------------------------|
| v kandev                          |
|   v docs/                         |
|     [MD] plan.md   Modified +6 -2  |
+------------------------------------+
       existing 16px message gap
[next message]
```

Reuse the stacked header, row wrapping, transcript scroll owner, 44px targets,
and direct file navigation from PR #4831.
The historical destination remains its existing full-height MobileDiffSheet.
Map: AC-005.10-.13, AC-006.1 and .5-.7.

### UI-03: Exceptions reuse existing slots

```text
Legacy: only the count wording changes
[320 repository changes +13633 -1191] [Open diff]

All history comparisons unavailable: existing status slot
[This turn's change summary is unavailable.]

Mixed usable/unavailable roots: existing Partial indicator
[1 available change +6 -2] [Partial] [Open diff]

Content still pending: unchanged normal card
[selected pending file uses the existing diff loading state]
```

Use one short existing unavailable message, without an additional warning block
or a numeric zero. Specific causes appear only in the affected selected-diff
status area. Keep already saved files readable.
No routine adjustment/saving message or new controls appear.
Map: AC-002.12-.13, AC-004.8-.9, AC-005.7 and .13, AC-006.7, AC-007.6.

Structure, controls, and density follow the PR baseline. ASCII glyphs and sample
paths are illustrative. Production copy remains localized.

## Tests

All proposed tests must fail behaviorally before their correction.
A missing test or selector is not RED evidence.
Run the complete matching command after implementation; zero matching tests is failure.

| Criteria | Proposed regression test | Work order |
| --- | --- | --- |
| AC-002.1, .4, .13; AC-004.1 | TestTurnCheckpointProvenanceReceiptSurvivesHeadMove; TestTurnChangeProvenanceMigrationReplay | 01 |
| AC-002.10-.12 | TestTurnCheckpointHistoryRebaseExcludesUpstream; TestTurnCheckpointHistoryPreservesDirtyBaseline; TestTurnCheckpointHistoryRetainsRevert | 02 |
| AC-002.5-.8, .11-.12 | TestTurnCheckpointHistorySameFileChanges; TestTurnCheckpointHistoryConflictUnavailable; TestTurnCheckpointHistoryMetadataAndModes | 02 |
| AC-004.1; AC-006.3 | TestCoordinatorHistoryComparisonAcceptedOnce; TestTurnChangeHistoryComparisonPairValidation | 02 |
| AC-002.9; AC-003.7-.9 | TestTurnChangeWriterIntervalsExcludeCompletedPending; TestTurnChangeWriterIntervalsPreserveRealOverlap; TestTurnChangeOrphanReconcileMixedState | 03 |
| AC-003.4-.6; AC-004.8-.9 | TestTurnChangeExportBatchTimeoutResume; TestTurnChangeExportBatchLostReceipt; TestTurnChangeExportCleanupRejectsLatePage | 04 |
| AC-004.6; AC-007.1, .6 | TestTurnChangeExportBudgetAcrossRepositories; TestTurnChangeExportClaimTakeover; TestTurnChangeExportRetryExhaustion | 04 |
| AC-005.7, .10-.13; AC-006.7 | preserved PR styling, minimal card states, revision cases, and real browser flows | 05 |

Tests belong beside the changed source. New test-file paths in work orders are
proposed additions, not claims that regression coverage exists today.

## E2E tests

Use new desktop tests/git/turn-changes-history-integrity.spec.ts and
mobile tests/git/mobile-turn-changes-history-integrity.spec.ts.
Reuse the existing GitHelper, isolated fixtures, SessionPage, changed-files
helpers, and mock-agent command conventions.

Add a deterministic mock-agent history-change scenario in cmd/mock-agent.
It must perform the actual Git history operation between start and end capture.
Coordinate the scenario with fixture barriers; do not use fixed sleeps.
Failure-state fixtures can seed exact retained records in isolated E2E storage.
Do not add production database-edit endpoints or operate on the developer instance.

Required flows:

- Clean upstream rebase plus one local edit: card/list/patch agree, exclude upstream,
  and survive reload. AC-002.10-.11, AC-004.1, AC-006.3 and .7.
- Conflicting baseline: existing unavailable state, no raw totals, and no
  current-changes fallback. Include a healthy sibling checkout. AC-002.12, AC-005.7.
- Legacy row: unchanged counts/files and only repository count wording. AC-002.13, AC-005.13.
- Completed pending capture versus a live sibling: no false overlap; actual
  overlapping intervals still show the note. AC-003.7-.8.
- Paused content batch: saved file opens, remaining file is pending, then settles
  without another agent edit or new chat progress text. AC-004.8-.9, AC-005.13.
- Phone: preserve PR icons/colors/spacing, direct drawer navigation, safe areas, focus return,
  zero document overflow, 44-pixel touch targets, and narrow fine-pointer geometry.
  Cover light/dark and a pseudo-locale state. AC-006.1 and .5-.7.

A backend RED test can establish the defect before mock scenario/UI wiring.
Record that boundary explicitly; browser fixtures must still exercise the
final production capture, API, and shared projection paths.

## Work orders

| Order | Work order | Depends on | Status |
| --- | --- | --- | --- |
| 01 | [Capture and persist history provenance](task-01-history-provenance.md) | None | done |
| 02 | [Accept history-aware comparison pairs](task-02-history-comparison.md) | 01 | done |
| 03 | [Repair canonical writer attribution](task-03-writer-attribution.md) | None; scheduled third | done |
| 04 | [Resume bounded content preservation](task-04-resumable-export.md) | 02, 03 | done |
| 05 | [Preserve PR styling and integrate corrected data](task-05-history-presentation.md) | 02, 03, 04; PR #4831 base | done |

Execution is sequential by default. These dependencies do not authorize subagents.
Work order 03 has a separate verification boundary and needs no provenance migration.
Work order 04 consumes accepted comparison identity and the writer/export separation.

## Companion package reconciliation

The [original plan](../turn-changed-files/plan.md) retains its recorded work-order
results. Its task 09 remains in progress with its existing executor/performance gaps.
This package replaces only the raw-comparison, unfinished-row attribution, and
single-window export assumptions identified above.

The [subscription package](../turn-changes-subscription-reconciliation/plan.md)
remains done. Do not reset its status, overwrite its counts, or replace its
readiness/revision logic. Work order 05 reruns its focused hook regressions
because new export revisions use that path.

The core design is near its size limit. Keep this refinement at its history
integrity boundary and retain links rather than duplicating the entire contract.

## Verification results

The PR #4831 dependency and minimal-UI revision passed the same documentation
checks on 2026-10-27. The dependency is now merged; implementation starts on
main containing f80b89ec4993d6dda5c6f69b08c6bcc0d3c40277.
No production, permanent test, or public-doc changes were made in this design turn.

Design-only validation passed on 2026-10-27:

- Catalog validation: 406 decisions and 1,754 specifications.
- Specification-linter tests: 36 passed.
- Full specification lint: passed, including document-size limits.
- Local links and whitespace: passed for all eight new documents.
- PR-documentation preflight: the actual docs-only diff is exempt.
  A simulated implementation trigger validated all five new work orders and
  both touched companion work orders, including requirement/design references.
- Package statuses: plan draft, five work orders pending, no staged files.

Product implementation and verification cover work orders 01 through 05,
including the isolated mock-agent history scenarios and the desktop/phone
browser matrix. PostgreSQL concurrency evidence remains an environment gap.

Implementation verification on 2026-10-27:

- Go build for every backend package passed.
- Turn-change, process, agentctl API/client, task service/handler, and backendapp
  focused tests passed.
- SQLite comparison, writer-interval, provenance-replay, and export-claim tests passed.
- Store conformance and `cmd/sqlguard` passed.
- Changed-Go lint passed with zero issues.
- Frontend focused tests, TypeScript typecheck, i18n keys/ratchet, and public-doc
  validation passed.
- The core design file remains at its bare size margin; the refinement lives in
  the linked history-integrity design rather than duplicating its contract.
- PostgreSQL parity tests were not executed because `KANDEV_TEST_POSTGRES_DSN`
  is unset in this environment.
- Desktop and phone history-integrity suites, conflict/legacy/mixed/overlap/
  paused-content suites, and PR #4831 style regressions passed in this environment.
- Multi-repository browser coverage uses an isolated retained-record fixture;
  no fixture currently captures two healthy repositories in one turn.

Follow-up review remediation on 2026-10-28:

- Timeout-prefix export pages now remain retryable and resume from their durable cursor; a real coordinator-to-SQLite test covers timeout prefix, zero progress, and exhaustion.
- Restart recovery rebuilds SQL-backed summaries without a retained executor and settles missing content explicitly instead of leaving a pending row. A concrete starting execution without an attached client defers settlement until that restore finishes.
- SHA-256 baselines use the accepted object format before creating the isolated merge view; a real SHA-256 rebase and batch export test passes.
- Synthetic late-page selections no longer mask success or terminal errors; the production viewer covers loading, ready, terminal content failure, and settled-repository states without requiring pagination.
- Stop preservation propagates one absolute deadline through retries, persistence, and lease release; slow-store and blocked-RPC tests return within 2.5 seconds and assert the inherited lease deadline.
- Repeated overlap revision conflicts retry from fresh state and fail visibly if still unresolved; success and exhaustion are both covered.
- Built-in `merge`, `merge=text`, `merge=binary`, and `merge=union` attributes are allowed; custom drivers remain rejected and never execute.
- New desktop history specs run together in one worker (6 passed); the phone history spec covers the main drawer flow, dark mode, and a pseudo-locale card/drawer (2 passed); the PR #4831 desktop and phone style suites (6 and 3 passed) and both scroll suites (1 each) still pass.
- The new history specs reset the shared worker checkout before seeding so their untracked-file turns stay independent when files share one worker.
- PostgreSQL upgrade, takeover, and receipt tests are present but skipped because `KANDEV_TEST_POSTGRES_DSN` is unset. No PostgreSQL pass is claimed.

Fixup review remediation on 2026-10-28:

- Capture provenance before preparing the snapshot so a concurrent rebase cannot pair old content with new history; the post-snapshot verification test moves `HEAD` from a `post-index-change` hook and requires rejection.
- Wrap the local comparison tree in a synthetic commit before `merge-tree`; Git 2.43 rejects a bare tree as the second branch, while the wrapped form works on 2.43 and 2.47.
- Treat only an unexpired export claim as live; an expired claim from an exhausted attempt now settles unavailable instead of repeating the loop.
- Keep file-specific export failures on their entries and continue paging; the regression exports 30 files with one oversized file and verifies the remaining files stay ready.
- Retry a saved synthetic selection once its repository settles; covered in the hook and mounted-viewer tests.
- Stop and join the export processor through the application context so shutdown does not leave in-flight writes running.
- Move the new history columns, dependent index, and backfill into `runMigrations`; upgrade and replay tests exercise the boot path.
- Correct the companion `turn-changed-files` plan and Results notes so completed history-integrity work is not described as pending or proposed.
- Reconcile the concurrent capture-finalization criterion by keeping its landed AC-004.8 and moving interrupted content preservation to AC-004.10.

A disposable real-Git feasibility probe passed during planning.
It excluded 318 imported paths from a 319-path raw diff, retained the one turn
file and a subsequent legitimate revert, preserved an unchanged dirty file,
and rejected a conflicting baseline.
The temporary repository was removed.
This does not prove the proposed Go implementation, remote compatibility, or timings.

Required design checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/turn-change-history-integrity docs/plans/turn-changed-files
git status --short -- docs/plans/turn-change-history-integrity
```

The PR-documentation preflight used validateCoverage from
.github/scripts/pr-docs.cjs with the actual modified/untracked path list.
Reference validation added a simulated coordinator implementation trigger to
exercise work-order validation without changing production code.
Leave all package files unstaged and uncommitted.

## Risks

- A clean three-way merge is a comparison policy, not proof of exclusive authorship.
  Unknown external writers remain possible.
- Conflict fallback can hide numeric turn totals that a broad raw diff previously showed.
  Reuse the existing unavailable state and selected-diff reason area.
- A target that aliases the active branch cannot safely distinguish upstream
  integration from local commits. Unsupported movement remains explicit.
- Missing history or older agentctl capabilities reduce coverage.
  Legacy rows cannot receive retrospective precision claims.
- Custom merge drivers and attributes must never execute during normalization.
- Durable export claims, retention, and cleanup can race; stale responses must lose.
- Three bounded attempts can still fail on slow storage. Preserve honest partial
  results; do not claim that all 129 historical missing files can be recovered.
- SQLite/PostgreSQL parity needs real PostgreSQL evidence when configured.
  Existing skips and unrelated executor gaps remain visible.
- Public docs change with implementation. This package changes internal intent only.

