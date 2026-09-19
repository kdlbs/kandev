---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001
created: 2026-08-11
owners:
  - tbd
---
# GitLab MR Status Chip System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Scenarios

Rendering and absence:

- **GIVEN** a task with no linked MRs, **WHEN** its session view renders,
  **THEN** no element with testid `mr-status-chip` exists.
- **GIVEN** a task whose only linked MR has `state: "merged"`, **WHEN** its
  session view renders, **THEN** no element with testid `mr-status-chip` exists.
- **GIVEN** a task with two linked MRs, both terminal (one `merged`, one
  `closed`), **WHEN** its session view renders, **THEN** no element with testid
  `mr-status-chip` exists.
- **GIVEN** a task with one open MR and one merged MR, **WHEN** its session view
  renders, **THEN** `mr-status-chip` is present with `data-mr-count="1"` and
  `data-mr-iid` equal to the open MR's iid.
- **GIVEN** a task with one open MR and no linked PR, **WHEN** the chat status
  bar renders, **THEN** `mr-status-chip` is present inside `chat-status-bar`.
- **GIVEN** a task with one open MR, **WHEN** the passthrough toolbar renders,
  **THEN** `mr-status-chip` is present inside `passthrough-status-row`.
- **GIVEN** a task linked to **both** an open GitHub PR and an open GitLab MR,
  **WHEN** the chat status bar renders, **THEN** both `pr-status-chip` and
  `mr-status-chip` are present inside `chat-status-bar`, and `pr-status-chip`
  precedes `mr-status-chip` in DOM order.

  This is the only ordering assertion. **The Azure DevOps chip is not part of
  any assertion**, and the requirement that `MRStatusChip` be rendered *before*
  `AzureDevOpsTaskPullRequestChip` (see Selection and ordering) stands as a
  source-order requirement on the two mount points, verified by reading the
  diff rather than by a test.
  That is a deliberate trade, stated honestly rather than dressed up as a
  technical limit: the Azure chip has no `data-testid`, and although its
  hardcoded `aria-label` would make a role-based locator possible, this spec
  declines to build an AC on a string owned by a component this card may not
  modify. See Selection and ordering for the full reasoning. A stated
  code-review check that everyone can see is better than an AC whose failure
  mode is an unrelated team renaming their label.

Status derivation:

- **GIVEN** an open MR with `pipeline_state: "failure"` and `draft: true`,
  **WHEN** the chip renders, **THEN** `data-status` is `failed`.
- **GIVEN** an open MR with `draft: true` and `pipeline_state: "pending"`,
  **WHEN** the chip renders, **THEN** `data-status` is `draft`.
- **GIVEN** an open MR with `approval_state: "approved"` and `pipeline_state:
  "success"`, **WHEN** the chip renders, **THEN** `data-status` is `ready` and
  the glyph carries the emerald colour class.
- **GIVEN** an open MR with `approval_state: "pending"` and `pipeline_state:
  "success"`, **WHEN** the chip renders, **THEN** `data-status` is
  `awaiting_approval`.
- **GIVEN** an open MR with `approval_state: ""` and `pipeline_state: "pending"`,
  **WHEN** the chip renders, **THEN** `data-status` is `running`.
- **GIVEN** an open MR with `approval_state: "pending"` and `pipeline_state:
  "pending"`, **WHEN** the chip renders, **THEN** `data-status` is
  `awaiting_approval`, not `running`, because row 6 carries no pipeline
  condition.
- **GIVEN** an open MR with every status field empty, **WHEN** the chip renders,
  **THEN** `data-status` is `neutral`.
- **GIVEN** an open MR with `unresolved_discussions: 7` and otherwise `ready`
  fields, **WHEN** the chip renders, **THEN** `data-status` is still `ready`,
  because unresolved discussions do not feed chip status.
- **GIVEN** any `TaskMR`, **WHEN** `getMRStatusColor` is called before and after
  the refactor, **THEN** it returns the identical class string. A table-driven
  test SHALL cover every branch of the priority table.

Aggregation and selection:

- **GIVEN** a task with two open MRs, one `running` and one `failed`, **WHEN**
  the chip renders, **THEN** `data-status` is `failed`, `data-mr-count` is `2`,
  and `data-mr-iid` is the failed MR's iid.
- **GIVEN** a task with two open MRs that are both `failed`, with iids 12 and 7,
  **WHEN** the chip renders, **THEN** `data-mr-iid` is `7`.
- **GIVEN** a task with two open MRs that are both `failed`, sharing iid 7
  across projects `group/a` and `group/b`, **WHEN** the chip renders, **THEN**
  the selected MR is the one in `group/a`.
- **GIVEN** a task with three open MRs whose backend response order is reversed
  between two renders, **WHEN** the chip renders each time, **THEN**
  `data-mr-iid` is identical both times.
- **GIVEN** a task with two open MRs at rank 0 with different statuses, one
  `draft` with iid 9 and one `neutral` with iid 3, **WHEN** the chip renders,
  **THEN** `data-mr-iid` is `3` and `data-status` is `neutral` (the status of
  the MR the tiebreak selected), for either input array order.
- **GIVEN** any list of MRs, **WHEN** `aggregateMRChipStatus(mrs)` is called,
  **THEN** its result equals `mrChipStatus(selectChipMR(mrs))`, or `neutral`
  when `selectChipMR(mrs)` is null. A property test SHALL assert this over
  shuffled input orders.
- **GIVEN** any two `MRChipStatus` values `a` and `b`, **THEN**
  `MR_CHIP_STATUS_RANK[a] < MR_CHIP_STATUS_RANK[b]` if and only if
  `STATUS_RANK[colour(a)] < STATUS_RANK[colour(b)]`, asserted by a table-driven
  test over all 8 statuses.
- **GIVEN** any list of MRs, **WHEN** `aggregateMRStatusColor` is called before
  and after this change, **THEN** it returns the identical class string. This
  includes all-terminal lists, where its existing first-in-input-order tie
  behaviour is preserved: `[merged, closed]` returns `text-purple-500` and
  `[closed, merged]` returns `text-muted-foreground`, both before and after.

Disclosure:

- **GIVEN** a fine-pointer viewport and a task with one open MR, **WHEN** the
  user hovers `mr-status-chip`, **THEN** `mr-topbar-popover-inner` becomes
  visible and its bounding box `y` is greater than or equal to 0.
- **GIVEN** that popover is open, **WHEN** the pointer crosses the gap between
  trigger and content, **THEN** the popover stays open.
- **GIVEN** that popover is open, **WHEN** the user clicks the chip itself,
  **THEN** the popover stays open and no navigation occurs.
- **GIVEN** a coarse-pointer viewport and a task with one open MR, **WHEN** the
  user taps `mr-status-chip`, **THEN** `mr-status-chip-drawer` becomes visible
  and contains `mr-topbar-popover-inner`.
- **GIVEN** the drawer is open, **WHEN** the user activates
  `mr-status-chip-drawer-close`, **THEN** the drawer closes and focus returns to
  the chip trigger.
- **GIVEN** a coarse-pointer viewport, **WHEN** the chip renders, **THEN** no
  hover popover is mounted.
- **GIVEN** a fine-pointer viewport, **WHEN** the user moves keyboard focus onto
  the chip trigger, **THEN** the popover opens and focus stays on the trigger.
- **GIVEN** the popover was opened by keyboard focus, **WHEN** focus moves off
  the trigger without any pointer entering the trigger or the content, **THEN**
  the popover closes, because `onBlur` is wired to `onTriggerLeave` and the
  scheduled close finds the pointer over neither region.
- **GIVEN** the popover was opened by keyboard focus and the pointer is then
  moved onto the popover content, **WHEN** focus moves off the trigger, **THEN**
  the popover stays open, because the scheduled close re-checks both hover
  regions at fire time and the pointer is over the content.
- **GIVEN** a fine-pointer viewport and a task with one open MR, **WHEN** the
  user clicks the chip trigger while its popover is closed, **THEN** no
  navigation occurs and no panel opens; the popover opens only when the 150ms
  hover-open delay elapses with the pointer still over the trigger.
- **GIVEN** a fine-pointer viewport and a task with one open MR, **WHEN** the
  viewport width is any value from 320px to 1920px, **THEN** exactly one
  disclosure branch renders and the chip is never blank. No width between
  breakpoints leaves it rendered by neither branch.

Popover content and actions:

- **GIVEN** an open MR with 6 of 10 pipeline jobs passing, **WHEN** its chip
  popover opens, **THEN** the pass-rate row reads `6/10 (60%)`.
- **GIVEN** an open MR whose pipeline has not started (`pipeline_jobs_total: 0`
  and no fetched jobs), **WHEN** its chip popover opens, **THEN**
  `mr-pipeline-empty` is rendered.
- **GIVEN** an open MR and a workspace with GitLab authenticated **whose status
  request has already resolved**, **WHEN** its chip popover opens, **THEN**
  `mr-popover-link-another` is rendered. (The pre-resolution window is a
  separate AC below; the qualifier is what keeps the two from disagreeing.)
- **GIVEN** an open MR and a workspace with GitLab not configured, **WHEN** its
  chip popover opens, **THEN** `mr-popover-link-another` is not rendered.
- **GIVEN** an open MR and a workspace with GitLab authenticated, **WHEN** the
  chip mounts and its popover is opened before the mount-time GitLab status
  request resolves, **THEN** `mr-popover-link-another` is absent at first and
  appears once the status resolves, without the chip unmounting or remounting
  and without any error being shown.
- **GIVEN** a task with exactly one open MR, **WHEN** the user unlinks it from
  the chip popover and the request succeeds, **THEN** the association is removed
  from the store and `mr-status-chip` unmounts.
- **GIVEN** the unlink request fails, **WHEN** the user unlinks from the chip
  popover, **THEN** an error toast is shown and `mr-status-chip` remains.
- **GIVEN** an MR that satisfies `isMRReadyToMerge`, **WHEN** its chip popover
  opens, **THEN** `mr-merge-button` is rendered and `data-mr-ready-to-merge` on
  the trigger is `"true"`.
- **GIVEN** a task with two open MRs, **WHEN** the multi chip renders, **THEN**
  `data-mr-ready-to-merge` is present on the trigger and equals
  `isMRReadyToMerge(acted-on MR)`.
- **GIVEN** a fine-pointer viewport and an open chip popover, **WHEN** the user
  activates `mr-popover-link-another`, **THEN** the popover closes and the
  task-MR link dialog opens.
- **GIVEN** a coarse-pointer viewport and an open chip drawer, **WHEN** the user
  activates `mr-popover-link-another`, **THEN** `mr-status-chip-drawer` is no
  longer visible and the task-MR link dialog is open, so the two dialogs are
  never nested.
- **GIVEN** the link dialog opened from the chip, **WHEN** the user links a
  second open MR that outranks the current selection, **THEN** the dialog
  closes, the new association is in the store, and the trigger's
  `data-mr-count` and `data-mr-iid` update to include and name it.
- **GIVEN** two independent fixtures of the same task and association — one
  where the user unlinks from the chip popover, one where the user unlinks from
  `MRTopbarButton` — **WHEN** the request succeeds in each, **THEN** both remove
  the same association from the store and neither toasts.
- **GIVEN** those same two independent fixtures, **WHEN** the request fails in
  each, **THEN** both show the error toast built from
  `gitlab:failedToUnlinkMergeRequest` and
  `gitlab:theMergeRequestIsStillLinked`, and both leave the association in the
  store.

  These are two separate fixtures, not a sequence: the second unlink of one
  association would fail for a different reason (the row is already gone) and
  would test nothing about parity. Parity holds by construction because both
  surfaces call the one extracted `useUnlinkTaskMR` and no second copy of that
  closure exists; these ACs pin that the extraction actually happened.

Automation badges:

- **GIVEN** task MR automation with `auto_fix_enabled: false` and
  `auto_merge_enabled: false`, **WHEN** the chip renders, **THEN** neither
  `mr-status-auto-fix-chip` nor `mr-status-auto-merge-chip` is rendered.
- **GIVEN** `auto_fix_enabled: true`, `auto_fix_max_rounds: 5`, and a lifecycle
  state for the selected MR with `auto_fix_round_count: 2`, **WHEN** the chip
  renders, **THEN** `mr-status-auto-fix-chip` reads `2/5`.
- **GIVEN** `auto_fix_enabled: true`, `auto_fix_max_rounds: 5`, and **no
  lifecycle state at all for the selected MR** (`findMRAutomationStateForMR`
  returns `undefined`), **WHEN** the chip renders, **THEN**
  `mr-status-auto-fix-chip` IS rendered and reads `0/5`, and carries
  `data-auto-fix-exhausted="false"`.

  This is the ordinary steady state of an enabled MR that has not needed a fix
  yet, so it is the badge's most common appearance and it is pinned rather than
  left to inference. The value follows from `autoFixRoundForState`, which is
  total: given `undefined` it returns `{current: 0, max, exhausted: false}`.
  Hiding the badge until round >= 1 is specifically NOT the behaviour — the badge
  communicates that auto-fix is on, which is true at round 0.
- **GIVEN** `auto_fix_enabled: true` and a lifecycle state with
  `auto_fix_exhausted_at` set, **WHEN** the chip renders, **THEN**
  `mr-status-auto-fix-chip` carries `data-auto-fix-exhausted="true"`.
- **GIVEN** `auto_fix_enabled: true` and `auto_fix_max_rounds` absent or not a
  finite number, **WHEN** the chip renders, **THEN** the badge's denominator is
  `10`.
- **GIVEN** two open MRs where one is exhausted at round 3 of 5 and the other is
  at round 4 of 5, **WHEN** the multi chip renders, **THEN** the badge shows the
  exhausted MR's `3/5`.
- **GIVEN** two open MRs whose auto-fix rounds are **fully tied** — neither
  exhausted, both at round 2 of 5 — with iids 12 and 7, **WHEN** the multi chip
  renders, **THEN** the badge is the one belonging to iid `7`, resolved by the
  same `mr_iid` / `project_path` / `id` order that `selectChipMR` uses. This
  pins the tiebreak leg of the round-aggregation rule, which the exhausted-beats-
  non-exhausted and higher-`current`-wins legs above do not reach.
- **GIVEN** `auto_merge_enabled: true`, **WHEN** the chip renders, **THEN**
  `mr-status-auto-merge-chip` is rendered.
- **GIVEN** automation options have not loaded, **WHEN** the chip renders,
  **THEN** the chip still renders its status glyph and no badges.

Idempotency and re-render:

- **GIVEN** unchanged store state, **WHEN** the chip re-renders any number of
  times, **THEN** its rendered output and `data-*` attributes are identical.
- **GIVEN** the task's MR automation options are already in the store, **WHEN**
  the chip mounts, **THEN** no request is issued.
- **GIVEN** the task's MR automation options are absent from the store, **WHEN**
  a single chip mounts, **THEN** exactly one options request is issued for that
  task.
- **GIVEN** the task's MR automation options are absent from the store, **WHEN**
  two chips for the same task mount in the same React commit, **THEN** each may
  issue one request, and once both settle the store holds exactly one options
  object for that task and both chips render identical badges. Cross-mount
  request dedupe is explicitly NOT asserted; see Sync and freshness.
- **GIVEN** the chip is mounted, **WHEN** no user action occurs, **THEN** it
  issues no further request: it neither polls nor re-fetches on an interval.
- **GIVEN** the chip's popover is open on a task with two open MRs, **WHEN** a
  store update makes the other MR the higher-ranked one, **THEN** the popover
  keeps showing the MR it opened with, the trigger's `data-mr-iid` still names
  that MR, `data-selection-frozen` is `"true"`, and the trigger's `data-status`
  updates to the new live selection's status.
- **GIVEN** that same popover, **WHEN** the user closes it, **THEN** in the
  render after close `data-selection-frozen` is `"false"` and `data-mr-iid`
  names the new live selection.
- **GIVEN** the chip's popover is open on an MR whose `pipeline_state` is
  `"success"`, with `detailed_merge_status: "mergeable"`,
  `unresolved_discussions: 0`, `draft: false` — so `isMRReadyToMerge` is true and
  the trigger's `data-mr-ready-to-merge` is `"true"` — **WHEN** a store update
  sets that same MR's `pipeline_state` to `"failure"`, **THEN**
  `data-mr-ready-to-merge` becomes `"false"` and `mr-merge-button` is no longer
  rendered in the open popover.

  The transition is chosen so the assertion cannot pass vacuously. A
  `pending` -> `failure` transition would leave `isMRReadyToMerge` false on both
  sides (it requires `pipeline_state === "success"`), so it proves nothing about
  liveness; `success` -> `failure` is the smallest change that actually flips the
  observed value.
- **GIVEN** that same popover is open on that MR, **WHEN** the same store update
  lands, **THEN** the trigger's `data-status` changes from `ready` to `failed`,
  confirming the body and the trigger read the same live row.
- **GIVEN** the chip's popover is open, **WHEN** the MR it is showing is
  unlinked from another surface, **THEN** the popover closes.
- **GIVEN** the chip's popover is open, **WHEN** the MR it is showing
  transitions to `merged` while another open MR remains, **THEN** the popover
  closes and the trigger re-selects the remaining open MR.
- **GIVEN** two chips are mounted at once, **WHEN** the user opens one,
  **THEN** the other stays closed.
- **GIVEN** two chips are mounted at once for the same task, **WHEN** the user
  opens the chat-status-bar chip by keyboard focus and then opens the
  passthrough-status-row chip by hover, **THEN** **both** disclosures are open
  at the same time, each rendering `mr-topbar-popover-inner`, and neither closes
  the other. Simultaneous open is permitted; no cross-chip coordinator exists.
- **GIVEN** those two disclosures are both open, **THEN** each chip issues its
  own MR feedback read — two requests for the one task. Cross-chip feedback
  dedupe is explicitly NOT asserted; see Sync and freshness.
- **GIVEN** the chip's popover is open on a task with two open MRs, so
  `data-selection-frozen` is `"true"`, **WHEN** a store update makes the other
  MR the badge-selected one (it becomes exhausted while the frozen MR is not),
  **THEN** `mr-status-auto-fix-chip` updates to the newly badge-selected MR's
  round and its `data-auto-fix-exhausted` becomes `"true"`, while
  `data-mr-iid` still names the frozen MR. The badges do not freeze.

Internationalization:

- **GIVEN** the pseudo-locale is active, **WHEN** the chip and its drawer
  render, **THEN** every string this feature adds is pseudo-localized,
  including the trigger's `aria-label`, the drawer title, and the drawer's
  screen-reader description.
- **GIVEN** the pseudo-locale is active, **WHEN** the chip's popover renders
  `MRCIPopover`, **THEN** GitLab-sourced domain data inside it is NOT
  pseudo-localized: MR titles, author and reviewer names, `project_path`,
  branch names, and pipeline job and stage names render verbatim. Per
  `apps/web/CLAUDE.md` these are user/domain data, never UI copy, and this
  feature does not change how `MRCIPopover` renders them. The i18n assertion is
  scoped to chip-owned copy for exactly this reason.

## Out of scope

Each of these is a deliberate exclusion, not an oversight. Each is its own card.

- **MR conflict / merge-blocked as a distinct chip status.** GitHub's chip has
  `conflict` (`mergeable_state: "dirty"`) and `behind` states. GitLab exposes
  the equivalent through `detailed_merge_status` / `merge_status`, but
  introducing it as a chip status would add a branch to `getMRStatusColor` and
  therefore change the kanban card icon and the topbar trigger colour for every
  conflicted MR in the product. That is a change to a different surface than
  this card's, so `mrChipStatus` deliberately produces no status that alters an
  existing colour.
- **Re-tuning the GitLab MR status colour priority.** For example, making a
  green pipeline with no approval requirement read green rather than falling to
  `neutral`. The chip inherits today's priority table verbatim.
- **A GitLab multi-MR tabbed popover.** GitHub has `MultiPRCIPopover` with
  segmented per-PR tabs, backing `PRStatusChipMultiHoverCard` and
  `PRStatusChipMultiDrawer`. GitLab has no such component, and
  `mr-topbar-button.tsx` already carries this exact exclusion. The multi-MR chip
  therefore shows the aggregate status plus a count on the trigger, and exactly
  one MR (the selected MR) inside its popover.
- **Reaching a terminal MR from the chip.** Because there is no multi-MR
  surface, the chip renders only for open MRs, and a merged or closed
  association is not reachable from it. Unlinking a terminal association stays
  with the topbar dropdown and the MR detail panel. This is a deliberate
  divergence from `PRStatusChip`, which keeps terminal PRs in its multi-PR
  surface for exactly that reason.
- **GitLab equivalents of `PRMergedBanner` / `PRClosedBanner`.** GitHub's chip
  row shows an archive-prompt banner when a task's PR merges or closes. No
  GitLab counterpart is added here.
- **Opening the MR detail panel from the chip popover.** `MRCIPopover`'s
  `onOpenDetailPanel` prop is optional and the chip omits it, so the popover
  title renders as static text. The dockview-settle logic that makes that open
  reliable is owned by the topbar button, which remains the entry point.
- **Giving GitLab MR hydration a single provider-level owner.** It currently has
  four independent call sites, each with its own per-instance "already fetched"
  ref, so which surfaces a session has visited decides whether a given task's
  MRs are in the store. Consolidating them is its own card. Until it lands, this
  spec pins no hydration outcome and no AC depends on one. See Sync and
  freshness.
- **A shared GitLab MR feedback cache or background sync.** No store slice, no
  warmer, no polling is added.
- **Making `useGitLabStatus` cache across mounts, and stopping it blanking the
  shared status.** Its effect refetches on every mount and clears the store to
  `null` first, so each new consumer costs a request and transiently makes
  `useGitLabAvailable()` read `false` everywhere. The chip inherits this rather
  than introducing it — `MRTopbarButton` already calls the same helper on the
  same route — and cannot fix it locally, because the hook serves navigation,
  the kanban board, the settings sidebar and the integrations menu. Fixing it
  means a card that can test all of those callers. See Sync and freshness.
- **A cross-chip disclosure coordinator.** Two chips mounted for one task may
  both have an open disclosure, and each then issues its own MR feedback read.
  Serialising that would require new shared "currently open chip" state that
  neither mount point nor `MRTopbarButton` has today, and a rule for which chip
  loses. The duplicate is an idempotent `GET` into a per-instance reducer. See
  Sync and freshness.
- **A cross-mount in-flight guard for `useTaskMRAutomationOptions`.** Its lazy
  fetch gates on render-captured state, so two chips mounted in one React commit
  can each issue a `GET`. Fixing it means changing a hook `MRAutomationControls`
  also uses from the topbar, so it needs a card that can test both callers. The
  duplicate is an idempotent `GET` that converges to one stored object. See
  Sync and freshness.
- **Restoring focus when the chip unmounts underneath its own drawer.**
  Unlinking the task's last open MR from inside the drawer closes the drawer and
  unmounts the trigger in the same update, so there is nothing to return focus
  to and it falls to `document.body`. Choosing and moving focus to a surviving
  landmark is a shared problem with `PRStatusChip` rather than one this chip
  introduces, and fixing it well means deciding a focus target for the whole
  status row. See Accessibility.
- **Making `MRCIPopover`'s controls Tab-reachable from a hover popover.** Radix
  portals the content with no tab bridge from the trigger, so link, unlink and
  merge are pointer- or drawer-only on a fine-pointer device. This gap is
  pre-existing and shared with `PRStatusChip` and `MRTopbarButton`; the chip
  inherits it rather than creating it, and fixing it would change a component
  the topbar also renders. See Accessibility.
- **Changing `aggregateMRStatusColor`'s array-order tie behaviour.** Its
  first-in-input-order tie is observable on the multi-MR kanban badge today.
  This card freezes it rather than fixing it, because changing it is re-tuning
  the colour priority, already excluded above. See State machine.
- **Enriched My-GitLab list badges, per-job checks UI in the detail panel,
  mergeability explanation or conflict-fix CTA, merge-method picker, GitLab
  webhooks, poller rate limiting, and poller-side watch reconciliation.**
- **Depending on the parent card's `resolvePushRepo` fix.** This feature is
  independent in code. Its tests seed MR associations directly through the
  existing link endpoint and the GitLab mock provider, never through autolink.
