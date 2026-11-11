---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001
---

# GitHub PR CI Bubble System Design

## Ownership and existing contracts

The integrations system owns check interpretation and its task presentation.
This design extends the existing task summary projection and PR glyph, rather than adding another provider reader.

Dependencies:

- [Workflow attention](github-workflow-attention.md): current-head eligibility, unavailable observations, and retained stale evidence.
- [Selected check results](github-pr-check-results.md): current workflow attempts and cancellation semantics.
- [Task disclosure](../../ui/system-design/pr-task-status-summary.md): existing desktop and phone disclosure structure.
- [Bounded task delivery](../../platform/system-design/bounded-task-status-delivery.md): bounded persisted task projections.
- [Replayable migrations](../../../decisions/0027-replayable-schema-migrations.md).

The requirements and design were promoted to active and current after delivery. The completed approval-badge package remains historical evidence.
During implementation, reconcile its active placement clauses with this package. Do not reset completed work orders or rewrite historical results.
The base icon's existing color, review precedence, and merge permissions remain independent.
No new ADR is needed for this local display extension.

## Requirement mapping

All suffixes reference `AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001`.

| Criteria | Design section |
| --- | --- |
| .1-.4, .7-.8, .14, .17 | CI classification |
| .5-.6, .13, .17 | Selected check summary |
| .8, .12-.14 | Compact projection and freshness |
| .9-.11, .15-.16 | Shared presentation |

## CI classification

Add the display-only `PRCIState` union to `lib/types/github-checks.ts`:
`none`, `failed`, `approval_required`, `attention`, and `in_progress`.
`none` means no CI bubble. It does not certify successful checks.

A pure `components/github/pr-ci-state.ts` helper derives and aggregates this value from full `TaskPR` records.
The backend summary projector uses the same ordered decision table, covered by equivalent fixtures.

| Priority | Evidence on an open PR | State | Upper-right marker |
| --- | --- | --- | --- |
| 1 | Explicit `checks_state === failure` | `failed` | Red filled circle with white cross |
| 2 | Eligible current-head `approval_required` workflow observation | `approval_required` | Amber filled lock |
| 3 | Eligible `action_required` observation, or explicit `checks_state === unstable` | `attention` | Amber exclamation mark |
| 4 | Explicit `checks_state === pending` | `in_progress` | Amber progress ring |
| 5 | Success, empty, unavailable, or unsupported check state | `none` | None |

Filter terminal PRs before aggregation. Select the highest-priority state over every open PR.
Break ties with the existing deterministic repository/PR identity ordering.
Do not infer CI from review state, draft status, queue membership, mergeability, or `checks_total - checks_passing`.
In particular, count differences do not establish running jobs.
Preserve the selected-check reducer's active-workflow signal when jobs have not appeared yet.

Reuse `getTaskPRWorkflowAttention` and `isWorkflowApprovalRequired` for workflow eligibility.
An approval-only workflow does not become a failed check. If real failure and approval coexist, show the cross and retain both explanations.
Unexplained provider attention remains attention, rather than an invented approval gate.
Same-head stale workflow positives retain the existing qualified explanation.

This display reducer never calls or replaces `isPRReadyToMerge`, `canAttemptPRMerge`, or automatic repair predicates.

## Selected check summary

`computeOverallCheckStatus` and `countCheckResults` already ignore completed skipped checks.
Keep those action and pass-rate semantics. Existing frontend bucket totals also continue to exclude skipped checks.

`TaskPR` currently retains only check rollup and counted totals. Those values cannot distinguish all-skipped checks from an empty observation.
Add one optional `check_summary` object to `PRStatus` and `TaskPR`:

```text
check_summary: null | {
  head_sha: string,
  observed: nonnegative integer,
  skipped: nonnegative integer
}
```

`observed` counts the selected check records, including neutral, cancelled, and stale records.
`skipped` counts only records with `status === completed` and `conclusion === skipped`.
Reject or ignore malformed summaries, including `skipped > observed` or a head mismatch.
A nonempty snapshot is all-skipped only when `skipped === observed`. Mixed skipped/cancelled or skipped/neutral observations are not all-skipped.

Populate this summary from the same selected checks passed to `newPRStatusWithCheckState`.
Do not recount unselected runs or add GitHub requests. A populated empty snapshot is `{observed: 0, skipped: 0}`.
Missing metadata remains `null`, rather than an assumed empty snapshot.

`SyncTaskPR` carries the metadata through its existing sync-state resolution, change detection, update, and event paths.
Detailed observations replace it, including an explicit zero-skipped observation.
Lightweight rollup-only updates preserve the last detailed observation only for the same head.
A head change clears unavailable old-head metadata. Provider errors preserve the existing same-head baseline.
The feedback persistence path must preserve its existing ordering guard and must not restamp cached feedback as a new provider observation.

The metadata describes the last selected detailed snapshot. It cannot override a newer explicit failure, pending rollup, or workflow attention.
Fallback text always qualifies persisted skipped counts as the last detailed observation.
This qualification must not imply that an old skipped snapshot describes a newer rerun.
Use existing refresh paths to replace it when detailed results become available.

Persist the object as bounded JSON in an additive nullable `github_task_prs.check_summary` column.
Use the existing `workflow_attention` JSON storage pattern for encoding and decoding.
Update migration replay, `taskPRColumns`, qualified reads, inserts, updates, and table-rebuild copies.
Cover fresh SQLite, migrated SQLite, same-database replay, and environment-gated PostgreSQL replay.
No historical backfill or startup provider scan is needed. Legacy rows remain unknown until a detailed observation arrives.

Map the optional object into `lib/types/github.ts` and preserve it through existing API/WS task-PR adapters.
Raw check arrays remain outside task summaries.

## Compact projection and freshness

Extend `statussummary.PullRequestSummary` and the matching web summary type with:

| Field | Meaning |
| --- | --- |
| `ci_state` | Explicit `PRCIState`, including `none` for clearing |
| `ci_pr_number` | One representative open PR for a visible CI state |
| `ci_repository` | Its bounded `owner/repo` identity |
| `ci_stale` | Selected workflow attention is retained stale evidence |

The existing `PullRequestInput` already carries check state and workflow evidence.
Derive the new aggregate independently from `aggregate_state`, which intentionally mutes draft PRs.
Reuse `githubTaskStatusSummaryPRReader` and `Projector.applyPREventLocked` for rebuild/event parity.
If a source cannot be read, retain the existing baseline policy.
Serialize `ci_state: none` for current payloads that have no bubble, including all-terminal associations.
Omitted fields from old payloads remain unknown.

Map these fields through `taskPRInfoFromSummary` to `TaskPRInfo`.
Extend the existing compact/full freshness selection in `pr-task-workflow-projection.ts` to choose one coherent CI observation.
A compact summary can supersede full records only when its valid timestamp is strictly newer than every full sync and workflow observation.
Missing or malformed required timestamps keep full-record authority.
Never combine an old compact positive with a newer full negative, or vice versa.

Legacy compact data can retain the existing explicit workflow approval fallback. It cannot derive CI from aggregate icon color.
Compact disclosure uses the selected CI identity before full details load. An unavailable hydration result retains that reason.
Preserve existing negative-projection behavior for terminal and non-approval details.

No new subscription, poller, batch size, or provider endpoint is introduced.
The new task summary fields are additive and remain bounded to one CI identity per task.

## Shared presentation

Extend `PRStatusGlyph` with an explicit `ciState` prop.
Use a 10px upper-right wrapper and an 8px inner symbol, with a background halo.
Reuse existing red status tokens and approval amber `#D97706` in light mode, `#FBBF24` in dark mode.
The progress ring animates only under `motion-safe`; its static shape still communicates progress.
`none` renders no upper-right element. Success never produces a green bubble.

Move the conflict triangle into a separate inline trailing slot beside the PR glyph.
Reserve its width only when a conflict exists. The PR count follows the complete glyph/conflict group.
Keep auto-fix at the upper-left and auto-merge at the lower-right.
Update wrappers that assume a fixed 14px total width so they account for the inline conflict indicator.
Keep the 14px task and 16px top-bar base glyph sizes.

Consumers:

- `PRTaskIcon` and its disclosure props: sidebar, cards, lists, task pickers, and graph rows.
- `TaskContributionIcons`: static compact fallback.
- `PRTopbarButton`: single, multi-PR, and per-PR entries.
- `SubtaskCIGlyph`: reuse the presentational glyph without nesting disclosure roots.
- `PRStatusChip`: retain its existing status composition. When its draft glyph hides CI, render the shared draft PR glyph and CI bubble.
  Existing non-draft primary status glyphs remain intact; do not add a second badge to a checkmark or failure glyph.

There is one status interpretation across these consumers. Generic PR links and dashboard type icons remain outside this task-status contract.

Add a muted `all_skipped` status and skipped-count detail to the shared summary row vocabulary.
GitHub supplies the new rows. Other providers do not change behavior.
`derivePRTaskStatusSummary` uses valid selected metadata for all-skipped and mixed skipped explanations.
Detailed check popovers also derive skipped counts directly from loaded selected feedback, which takes priority over fallback metadata.
Retain authoritative loaded-empty behavior in `PRChecksSection`.
All-skipped copy replaces the misleading unstarted empty message. Cancelled-only results retain their existing explanation.

Use `t()` and count-based plural keys for every new label, accessible name, and fallback.
Update English, Portuguese, Simplified Chinese, Japanese, and Korean catalogs.
Generate Traditional Chinese catalogs through `pnpm run i18n:zh-hant`.
Keep complete status, conflict, automation, and per-PR meaning in the existing accessible name and disclosure.

## Desktop and phone composition

The nearest shipped exemplars are `PRTaskIconDrawer`, `PRStatusChipDrawer`, and `PRTopbarButton`'s touch disclosure.
Reuse `useTouchDrawer`. A compact CI explanation fits the existing temporary drawer.
No new route, sheet, badge button, or independent scroll owner is needed.

Desktop hover/focus retains the current details. Phone tap opens the existing drawer without selecting the task.
The existing drawer body owns scrolling, dynamic viewport bounds, and safe-area clearance.
The whole PR control owns the touch target. Recalculate it for the wider conflict group and prove at least 44px on phones and coarse pointers.
Fine-pointer desktop controls retain their existing density.
Measure containment at the canonical phone viewport and just below/above 768px, including a narrow fine-pointer phone viewport.
Verify focus return, reduced motion, light/dark status meaning, and no document horizontal overflow.

## Delivery and compatibility

The [plan](../../../plans/github-pr-ci-bubble/plan.md) sequences additive metadata/projection, presentation, then browser evidence and documentation.
Current runtime flags, credentials, check selection, CI cache policy, and merge/repair authorization remain outside this extension.
No new metrics are required. Existing sync logs and the projected API state provide diagnostic evidence.
