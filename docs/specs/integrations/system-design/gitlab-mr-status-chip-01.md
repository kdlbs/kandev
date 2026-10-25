---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001
created: 2026-08-11
owners:
  - tbd
---
# GitLab MR Status Chip System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-MR-STATUS-CHIP-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

A task whose work lives on GitHub shows its pull request's CI, review and
automation state as a chip in the chat status bar, so the user sees it without
leaving the conversation. A task whose work lives on GitLab shows nothing
there. GitLab users have to look up at the topbar button, or open the MR detail
panel, to answer "did my pipeline pass". The GitLab merge-request equivalent of
that chip does not exist.

## What

- A task with at least one **open** linked GitLab merge request SHALL render an
  `MRStatusChip` in the chat status bar and in the passthrough status row,
  beside the existing GitHub and Azure DevOps chips.
- The chip SHALL show, at a glance: a GitLab merge glyph, a status glyph whose
  colour matches the MR's existing status colour everywhere else in the product,
  an MR count when more than one open MR is linked, and auto-fix / auto-merge
  badges when task MR automation is enabled.
- Hovering the chip on a fine-pointer device SHALL open the existing
  `MRCIPopover` body (pass rate, pipeline stage groups, approval row, unresolved
  discussions, automation controls, merge action, footer). Tapping it on a
  coarse-pointer device SHALL open that same body inside a bottom-sheet drawer.
- The chip SHALL derive its status from the one shared GitLab MR status
  derivation, not a new private copy. `getMRStatusColor` in
  `apps/web/components/gitlab/mr-task-icon.tsx` remains the single source of MR
  status colour, and its output for every input SHALL be byte-identical before
  and after this feature.
- The chip SHALL NOT own any recurring fetch, polling, cache-warming or
  background-sync responsibility. It issues exactly **four** kinds of request,
  all of them existing behaviour it inherits rather than invents:
  1. the lazy read of the task's MR automation options, on mount;
  2. the GitLab connection-status read that `useGitLabAvailable()` performs on
     mount, which the chip needs to source `canLink`. This one has a
     cross-surface side effect and is stated in full under Sync and freshness;
  3. the MR feedback read that `MRCIPopover` already owns, **only while a
     disclosure is open** (this is what the popover's `enabled` prop gates; see
     API surface);
  4. the link and unlink the user explicitly triggers.
  Nothing else: no polling, no interval, no warmer, and specifically no
  `useWorkspaceMRs`. See Sync and freshness.
- No file under `apps/web/components/github/` changes. `PRStatusChip`'s
  rendered output SHALL be identical before and after.
- All chip copy SHALL go through `t()` into
  `apps/web/src/locales/en/gitlab.json`, including accessible labels.

## Data model

No new persistent state. The chip reads the existing `TaskMR` rows
(`gitlab_task_mrs`, typed at `apps/web/lib/types/gitlab.ts`) already cached in
the store at `taskMRs.byWorkspaceId[workspaceId][taskId]`, and the existing
`TaskMRAutomationOptions` cached at `taskMRAutomation.byTaskId[taskId]`.

Fields the chip's own status derivation reads, and only these:

| Field | Type | Used for |
|---|---|---|
| `id` | string | React key, unlink target, final selection tiebreak |
| `state` | `open \| closed \| merged \| locked \| string` | terminal filtering, `merged` / `closed` status |
| `pipeline_state` | `"" \| success \| failure \| pending \| string` | `failed` / `ready` / `running` status |
| `approval_state` | `"" \| approved \| pending \| string` | `ready` / `awaiting_approval` status |
| `draft` | boolean | `draft` status |
| `mr_iid` | number | display, `data-mr-iid`, primary selection tiebreak |
| `project_path` | string | secondary selection tiebreak, automation-state lookup |
| `repository_id` | string? | automation-state lookup |

The chip SHALL NOT read `unresolved_discussions` for status. That field is
documented as populated only for automation-subscribed MRs, so using it would
make the chip report a different status for two otherwise identical MRs
depending on whether automation happens to be on. The popover body's own live
discussion fetch continues to render the unresolved count, unchanged.

The chip SHALL NOT read `merge_status` / `detailed_merge_status` for status.
See Out of scope.

Separately from status, the trigger's `data-mr-ready-to-merge` attribute and the
popover's merge button both come from the existing `isMRReadyToMerge`, which
does read `unresolved_discussions` and `detailed_merge_status` by design and
carries its own documented rationale. That helper is reused unchanged; the
restriction above applies to chip status only, and the two must not be
reconciled by editing `isMRReadyToMerge`.

## API surface

No new endpoints, no new store actions, no new WS events.

Consumed, all existing:

- `useTaskMRs(taskId)` (`apps/web/hooks/domains/gitlab/use-task-mr.ts`) for the
  linked MR list.
- `useGitLabAvailable()` for whether "link another merge request" is offered.
- `useTaskMRAutomationOptions(taskId)`
  (`apps/web/hooks/domains/gitlab/use-task-mr-automation.ts`) for automation
  flags and `mr_states`.
- `findMRAutomationStateForMR` / `autoFixRoundForState`
  (`apps/web/lib/gitlab/mr-automation.ts`) for round badges.
- `MRCIPopover` (`apps/web/components/gitlab/mr-ci-popover.tsx`) as the popover
  and drawer body, reused as-is with no new variant and no signature change.
  Its full prop contract is specified below; every prop it declares gets a
  named source, because one of them (`enabled`) decides whether the chip
  fetches.
- `TaskMRLinkDialog` (`apps/web/components/gitlab/task-mr-link-dialog.tsx`) for
  the link flow, with its existing
  `{open, onOpenChange, taskId, workspaceId, taskRepositories, repositories}`
  signature unchanged.
- `DELETE` task-MR association via `deleteTaskMR` + the `removeTaskMR` store
  action, for the unlink control the popover header already renders.

### `MRCIPopover` prop contract

`MRCIPopover`'s declared signature is
`{mr, taskId, enabled, canLink, onOpenDetailPanel?, onLink, onUnlink}`. The chip
supplies every one of them from a named source:

| Prop | Type | Source |
|---|---|---|
| `mr` | `TaskMR` | the **acted-on** MR: the live selected MR while the disclosure is closed, the frozen MR while it is open. Always the live store row for that id, never a snapshot. See Selection and ordering. |
| `taskId` | `string` | the chip's own `taskId` |
| `enabled` | `boolean` | **the chip's own disclosure-open state** (`popoverOpen` for the hover variant, `drawerOpen` for the drawer variant). `false` whenever the disclosure is closed. |
| `canLink` | `boolean` | `useGitLabAvailable()` |
| `onOpenDetailPanel` | `(() => void)?` | **omitted.** See Out of scope; the popover title then renders as static text. |
| `onLink` | `() => void` | opens this chip's own `TaskMRLinkDialog`. See Link and unlink from the chip. |
| `onUnlink` | `() => void` | a zero-argument closure that calls `useUnlinkTaskMR(workspaceId)` with the **acted-on MR's association `id`** captured from the enclosing render. See Link and unlink from the chip. |

**`enabled` is the load-bearing one.** `MRCIPopover` gates its live MR feedback
read on it (`useMRFeedbackGated(mr, enabled)`), which is the only reason the
"only network effect" clause in What can be stated at all. `MRTopbarButton`
passes `enabled={popoverOpen}` and the chip SHALL do the same for its own
disclosure. Passing a constant `true` is specifically forbidden: it would make
every mounted chip fetch MR feedback on mount, for every session view with a
linked open MR, and the What clause would be false.

Note the arity: the component declares `onUnlink: () => void`, not
`onUnlink(associationId)`. The association id is closed over, not passed as an
argument, and which id gets closed over is fixed by the freeze rule rather than
left to the call site.

New exported helpers in `apps/web/components/gitlab/mr-task-icon.tsx` alongside
`getMRStatusColor` (the file the repo already designates as the single source of
GitLab MR status):

```
type MRChipStatus =
  | "merged" | "closed" | "failed" | "draft"
  | "ready" | "awaiting_approval" | "running" | "neutral"

mrChipStatus(mr: TaskMR): MRChipStatus
selectChipMR(mrs: TaskMR[]): TaskMR | null
aggregateMRChipStatus(mrs: TaskMR[]): MRChipStatus
```

`getMRStatusColor(mr)` SHALL be re-expressed as a lookup from
`mrChipStatus(mr)`, so exactly one priority table exists.

`aggregateMRChipStatus(mrs)` SHALL be defined as the composition
`mrChipStatus(selectChipMR(mrs))`, returning `neutral` when `selectChipMR`
returns null. It is not an independent derivation and SHALL NOT have a tiebreak
rule of its own. This is what makes the trigger internally consistent: the
status the chip reports is, by construction, the status of the MR the chip
identifies and acts on. See Selection and ordering.

One extraction, in `apps/web/hooks/domains/gitlab/use-task-mr.ts` alongside the
other task-MR hooks:

```
useUnlinkTaskMR(workspaceId: string): (associationId: string) => Promise<void>
```

This is the unlink closure that today lives privately inside `MRTopbarButton`
(`deleteTaskMR` -> `removeTaskMR`, error toast using the existing
`gitlab:failedToUnlinkMergeRequest` and `gitlab:theMergeRequestIsStillLinked`
keys), moved verbatim. Both `MRTopbarButton` and the chip SHALL call it, and
`MRTopbarButton`'s observable unlink behaviour SHALL be unchanged. The chip
SHALL NOT re-implement the closure: `apps/web/eslint.config.mjs` forbids
identical functions and 4+ duplicated strings, so a second copy is a lint
failure, not a style preference.

New testids, forming the chip's observable contract:

| testid | Element |
|---|---|
| `mr-status-chip` | the trigger button (both single and multi) |
| `mr-status-chip-drawer` | coarse-pointer drawer content |
| `mr-status-chip-drawer-close` | drawer close button |
| `mr-status-auto-fix-chip` | auto-fix round badge |
| `mr-status-auto-merge-chip` | auto-merge badge |
| `mr-status-glyph` | the status glyph, carrying `data-status` |

Trigger attributes, and which MR each one describes. This distinction is
load-bearing while the disclosure is open; see Selection and ordering.

| Attribute | Value | Which MR |
|---|---|---|
| `data-status` | the `MRChipStatus` | the **live** selected MR, always |
| `data-mr-count` | number of **open** linked MRs | n/a |
| `data-mr-iid` | `mr_iid` | the **acted-on** MR |
| `data-mr-state` | `state` | the **acted-on** MR |
| `data-mr-ready-to-merge` | `"true"` / `"false"` from the existing `isMRReadyToMerge` | the **acted-on** MR |
| `data-selection-frozen` | `"true"` while the disclosure is open, `"false"` otherwise | n/a |

The **acted-on MR** is the live selected MR while the disclosure is closed, and
the frozen MR while it is open. `data-selection-frozen` exists so that the two
regimes are distinguishable from the DOM rather than inferred; without it a test
cannot tell a frozen attribute from a stale one. All three per-MR attributes
switch regime together, so they always describe the same single MR as each
other and as the popover body.

`data-mr-ready-to-merge` is present on the single and the multi trigger alike,
always describing the acted-on MR. This is a deliberate divergence from
`PRStatusChip`, whose multi trigger omits the attribute: GitLab's chip always
has exactly one acted-on MR, because it has no multi-MR surface, so there is
always a well-defined MR for the attribute to describe.

Badge attributes. These sit on `mr-status-auto-fix-chip`, not on the trigger,
and they complete the observable contract — the tables above are the full
attribute inventory and nothing outside them is asserted by an AC:

| Attribute | Element | Value | Which MR |
|---|---|---|---|
| `data-auto-fix-exhausted` | `mr-status-auto-fix-chip` | `"true"` / `"false"`, from `autoFixRoundForState(...).exhausted` | the **badge-selected** MR |

The **badge-selected MR** is a third selection, distinct from both the live and
the frozen selection: it is the most attention-worthy auto-fix round across the
open MRs (exhausted beats non-exhausted, then higher `current`, then the same
`mr_iid` / `project_path` / `id` order — see Selection and ordering). On a
single-MR chip all three coincide. On a multi-MR chip they need not.

## State machine

`mrChipStatus(mr)` is a total function evaluated in this exact priority order,
first match wins. The colour column is what `getMRStatusColor` returns for that
MR today and MUST continue to return.

| # | Status | Condition | Colour | Glyph |
|---|---|---|---|---|
| 1 | `merged` | `state === "merged"` | `text-purple-500` | filled merge glyph |
| 2 | `closed` | `state === "closed"` or `state === "locked"` | `text-muted-foreground` | filled dot |
| 3 | `failed` | `pipeline_state === "failure"` | `text-red-500` | filled circle-X |
| 4 | `draft` | `draft === true` | `text-muted-foreground` | filled dot |
| 5 | `ready` | `approval_state === "approved"` and `pipeline_state === "success"` | `text-emerald-400` | filled circle-check |
| 6 | `awaiting_approval` | `approval_state === "pending"` | `text-sky-400` | clock |
| 7 | `running` | `pipeline_state === "pending"` | `text-yellow-500` | spinner, 3s per rotation |
| 8 | `neutral` | otherwise | `text-muted-foreground` | filled dot |

The ordering is deliberate and load-bearing. Stated exactly, because the
neighbouring rows are easy to get backwards:

- A failed pipeline outranks `draft` (row 3 before row 4), so a draft MR whose
  pipeline broke reads `failed`, not `draft`.
- `ready` requires **both** an approved review and a succeeded pipeline (row 5).
- A pending approval outranks a running pipeline (row 6 before row 7). Row 6
  carries **no pipeline condition at all**: an MR with
  `approval_state: "pending"` and `pipeline_state: "pending"` is
  `awaiting_approval`, not `running`.

This is exactly the order `getMRStatusColor` already evaluates, row for row.
The refactor SHALL NOT reorder it, and where this prose and the table above
could be read as disagreeing, the table governs.

### Rank, and why aggregation has no tiebreak of its own

`MR_CHIP_STATUS_RANK` orders the statuses by how much attention they deserve:

| Status | Rank |
|---|---|
| `failed` | 5 |
| `running` | 4 |
| `awaiting_approval` | 3 |
| `ready` | 2 |
| `merged` | 0 |
| `closed` | 0 |
| `draft` | 0 |
| `neutral` | 0 |

Four statuses share rank 0, so **rank alone does not identify a winner**. The
chip therefore never aggregates by rank directly. `aggregateMRChipStatus(mrs)`
is defined as `mrChipStatus(selectChipMR(mrs))` (`neutral` when `selectChipMR`
returns null), and `selectChipMR` breaks every tie by named field. Two
consequences, both intended:

1. There is no rank-tie ambiguity to resolve, because ties are resolved on MRs
   by `mr_iid` / `project_path` / `id`, not on statuses.
2. The trigger's `data-status` cannot disagree with its `data-mr-iid` **while
   `data-selection-frozen` is `"false"`**: the reported status is read off the
   identified MR. This qualifier is load-bearing and is not a hedge. While a
   disclosure is open the freeze rule deliberately holds `data-mr-iid` on the
   frozen MR while `data-status` keeps tracking the live selection, so the two
   may name different MRs for exactly that window. See "Freezing while the
   disclosure is open", which governs. A test asserting the invariant SHALL
   scope itself to `data-selection-frozen="false"`.

**`aggregateMRStatusColor` is NOT re-expressed and its body SHALL NOT be
touched.** This is a deliberate decision, not an omission:

- Its existing loop (`bestRank = -1`, strict `>` over colour ranks) is
  **first-in-input-order wins** on a tie, so `[merged, closed]` returns purple
  while `[closed, merged]` returns muted. That array-order dependence is
  observable today on the multi-MR kanban badge.
- The chip requires the opposite property: a selection that does not depend on
  array order. The two cannot share one implementation without either changing
  `aggregateMRStatusColor`'s output for all-terminal lists, or importing array
  ordering into the chip.
- Changing that behaviour is re-tuning the GitLab MR colour priority, which this
  spec already excludes (see Out of scope). Leaving the function byte-identical
  makes the mandated parity scenario a regression guard rather than a coin flip.

`MR_CHIP_STATUS_RANK` and the existing colour-keyed `STATUS_RANK` therefore
remain two separate tables, keyed on different things and serving different
callers. They are not permitted to drift: a test SHALL assert they are
**order-equivalent** under the status-to-colour map from the table above, i.e.
for any two statuses `a` and `b`,
`MR_CHIP_STATUS_RANK[a] < MR_CHIP_STATUS_RANK[b]` if and only if
`STATUS_RANK[colour(a)] < STATUS_RANK[colour(b)]`. The single-derivation
invariant this spec protects is the **priority table** (`mrChipStatus`, which
`getMRStatusColor` now reads from); it was never a claim that one rank table
serves both callers.

## Selection and ordering

The chip renders one MR inside its popover even when several are linked. That
MR is the **selected MR**, `selectChipMR(mrs)`, chosen deterministically:

1. Restrict to open MRs (the chip only renders when at least one exists). If
   none are open, return null.

   **"Open" means `state === "open"` exactly.** It is a positive equality test,
   never "not one of the terminal states". `TaskMR.state` is typed
   `open | closed | merged | locked | string`, so the two readings diverge for
   `locked` and for any unrecognised string the backend may emit: under the
   equality test a locked MR is not selectable and does not count toward
   `data-mr-count`, and an unknown state behaves the same way. This matches the
   test `aggregateMRStatusColor` and `isMRReadyToMerge` already apply, so all
   three agree on what "open" means. A consequence worth stating: rows 1 and 2
   of the status table (`merged`, `closed`) are therefore unreachable through
   `selectChipMR`, and remain live only for `getMRStatusColor`'s other callers
   and for the rank-parity test.
2. Keep those whose `mrChipStatus` has the maximum `MR_CHIP_STATUS_RANK`.
3. Tiebreak, in order, by **`mr_iid` ascending**, then **`project_path`
   ascending** (byte-wise, `<`), then **`id` ascending**. `id` is the
   association primary key, so the ordering is total and the selection is
   unique for any input.

Array position is explicitly NOT the tiebreak. The store's array order comes
from the backend response order and is not a stable, named property. (This rule
governs the chip only. It does not apply to `aggregateMRStatusColor`, which
keeps its existing array-order behaviour untouched; see State machine.)

### Freezing while the disclosure is open

**Selection is frozen while the popover or drawer is open.** The popover carries
destructive and irreversible actions (unlink, merge), and an action target that
swaps under the pointer mid-interaction is a hazard the aggregate status is not
worth.

What is captured is the **association `id` only, never a copy of the `TaskMR`
object.** The popover body renders the live store row for that id on every
render. So while frozen:

- Which MR the popover acts on does not change.
- That MR's own fields DO stay live: a pipeline that goes from pending to failed
  while the popover is open updates the body, and `isMRReadyToMerge` and the
  merge button are evaluated against current data, never a snapshot taken at
  open. Merging or unlinking on stale fields is the failure this rule exists to
  prevent, so a value snapshot is specifically forbidden.
- The trigger's `data-mr-iid`, `data-mr-state` and `data-mr-ready-to-merge`
  describe the frozen MR, and `data-selection-frozen` is `"true"`.
- The trigger's `data-status` and glyph continue to track the **live** selected
  MR, so a change on a different MR is still visible at a glance. This is the
  one window in which `data-status` and `data-mr-iid` may describe different
  MRs; it is bounded by `data-selection-frozen="true"` and is the deliberate
  cost of not swapping the action target.
- **The automation badges do NOT freeze.** `mr-status-auto-fix-chip` (its text
  and its `data-auto-fix-exhausted`) and `mr-status-auto-merge-chip` continue to
  track the live badge-selected MR, on the same side of the line as `data-status`
  and the glyph.
  The rule that decides this is what each surface is *for*, not which MR it
  happens to name. The freeze exists to stop an **action target** moving under
  the pointer: unlink and merge are destructive, so the MR they operate on is
  pinned. The badges are **informational** — nothing in the popover acts on the
  badge-selected MR, and no control's meaning changes when the badge changes. A
  frozen badge would instead hide exactly the event it exists to report: an MR
  exhausting its auto-fix rounds while the user has the popover open.
  This means a multi-MR chip may briefly show three different MRs at once —
  `data-mr-iid` (frozen, acted-on), `data-status` (live selection), and the
  badge (live badge-selection). That is accepted and is bounded by
  `data-selection-frozen="true"`. It is not a new hazard: none of the three is
  an action target except the frozen one, which is the whole point.

If the held `id` leaves the store, or its row stops being open, the popover or
drawer closes. On close the freeze is released and all attributes return to the
live selection in the same render.

DOM ordering inside both status rows SHALL be: `PRStatusChip`, then
`MRStatusChip`, then `AzureDevOpsTaskPullRequestChip`, then the existing
banners and right-hand controls. This matches the topbar's existing
`PRTopbarButton` then `MRTopbarButton` provider order, so a task linked to both
a PR and an MR presents the two providers in the same order in both places.
Both chips render; neither suppresses the other.

How each leg of that ordering is verified differs, and the difference is
deliberate. The `PRStatusChip` -> `MRStatusChip` leg has an AC, because both
elements carry testids. The `MRStatusChip` -> `AzureDevOpsTaskPullRequestChip`
leg has **no AC** and is a source-order requirement checked by reading the two
mount points.

The reason is a choice, not an impossibility, and an earlier draft overstated
it. It is true that the Azure chip exposes no `data-testid` and that adding one
is outside this card's permitted files. It does **not** follow that the leg is
unobservable: the chip renders a link whose `aria-label` is a hardcoded,
untranslated `` `Azure PR ${pullRequestId}: ...` ``, inside rows that do carry
testids, so a role-and-name locator could reach it without touching any Azure
file. This spec still declines to assert on it, for a different and narrower
reason: that label is an implementation detail of a component this card is
forbidden to modify, so an AC built on it could be broken by an unrelated Azure
change that this card's owner would be unable to fix. Coupling our ACs to
another team's untranslated internal string buys less than it costs. See
Scenarios.

The auto-fix round shown on a multi-MR chip is the most attention-worthy round
across the open MRs: an exhausted round beats a non-exhausted one, then a higher
`current` wins, and remaining ties resolve by the same
`mr_iid` / `project_path` / `id` order above.
