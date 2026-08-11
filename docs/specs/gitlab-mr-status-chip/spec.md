---
status: draft
created: 2026-08-11
owner: tbd
---

# GitLab MR Status Chip

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

## Link and unlink from the chip

`MRCIPopover` requires both `onLink` and `onUnlink`. What activating them does
is part of this contract, not an implementation detail.

**`onUnlink`** — a zero-argument closure over the acted-on MR's association `id`,
matching `MRCIPopover`'s declared `onUnlink: () => void`. It calls the extracted
`useUnlinkTaskMR(workspaceId)` described under API surface, passing the id it
closed over. (An earlier draft wrote this heading as `onUnlink(associationId)`,
which is the exact form the prop contract above says is wrong; the prop contract
governs.) Behaviour is the topbar's, unchanged: on success the row leaves the
store; on failure an error toast is shown and the row stays.

**`onLink()`** opens the existing `TaskMRLinkDialog`, which the chip mounts
itself as a sibling of its trigger. Its props come from the same places
`MRTopbarButton` sources them:

| Prop | Source |
|---|---|
| `open` | the chip's own dialog open state, which it owns per mounted instance |
| `onOpenChange` | sets that same state |
| `taskId` | the chip's own `taskId` |
| `workspaceId` | the active workspace |
| `taskRepositories` | `useTaskById(taskId)?.repositories`, defaulting to a module-level shared empty array |
| `repositories` | `state.repositories.itemsByWorkspaceId[workspaceId]`, defaulting to a module-level shared empty array |

Both defaults MUST be module-level constants, not a fresh `[]` per render, or
the dialog re-renders on every parent render. Both props are required and
non-nullable on `TaskMRLinkDialog`, and the workspace bucket can legitimately be
absent before repositories hydrate, so the fallback is reachable rather than
defensive. A dialog opened with an empty `repositories` list is the existing
component's own behaviour and this spec does not change it.

Ordering when the user activates "link another merge request":

1. The chip closes its own disclosure (popover or drawer) first.
2. Then `TaskMRLinkDialog` opens.

Closing first is required, not cosmetic: the drawer variant is itself a
focus-trapping dialog, and opening a second one inside it strands focus. The
dialog's own lifecycle is unchanged, so a successful link adds the association
to the store and the chip re-renders with the new MR in its input; if that MR
outranks the current selection the trigger updates, subject to the freeze rule
only while a disclosure is open, which by step 1 it is not.

Each mounted chip owns its own dialog instance and its own open state, like its
disclosure. Two chips mounted at once therefore have two dialog instances, both
closed until the user activates link on one of them; activating it opens only
that chip's dialog. The dialog is idempotent with respect to which chip opened
it, because both pass the same `taskId` and `workspaceId`.

**The dialog does not outlive its chip.** Because each chip mounts its own
dialog as a sibling of its trigger, a chip that unmounts takes its dialog with
it. So if the task's last open MR is unlinked from another surface, or goes
terminal, while this chip's link dialog is open, the dialog unmounts mid-edit
and any URL the user had typed is lost. That is the accepted behaviour, not an
oversight: the alternative is hoisting the dialog above the chip's own render
gate, which would leave a dialog on screen belonging to a chip that no longer
exists. The window is small (it needs a concurrent change on another surface)
and the user can reopen the dialog from `MRTopbarButton`, which is not gated on
an open MR existing.

## Failure modes

- **Task MR list never hydrated.** `useTaskMRs` returns the shared empty array
  and the chip renders nothing. There is no skeleton and no loading state: an
  absent chip and a not-yet-fetched chip are indistinguishable by design, which
  is the same contract `PRStatusChip` and `MRTaskIcon` already have.
- **Store holds a non-array value for the task** (possible during partial
  hydration). The chip guards with `Array.isArray` and renders nothing, matching
  `MRTaskIcon` and `PRStatusChip`.
- **No active workspace, or `taskId` is null.** The chip renders nothing.
- **Popover feedback fetch fails.** `useMRFeedback` already resolves with
  partial data and an error string; the popover degrades exactly as it does from
  the topbar button. The chip's own trigger status is unaffected, because it is
  derived from `TaskMR` fields rather than from the feedback response.
- **Unlink fails.** The existing behaviour is preserved: an error toast is
  shown and the association stays in the store. A successful unlink removes the
  row; if that was the task's last open MR the chip unmounts on the next render.
- **Two surfaces unlink the same association.** The chip and the topbar read the
  same store row. Whichever request succeeds first removes the row; the other
  surface's request fails and toasts. The end state is identical either way, so
  the outcome does not depend on ordering. The chip does not retry.
- **An open MR transitions to merged/closed while its popover is open.** On the
  next store update the chip re-evaluates. If the transitioned MR is the frozen
  one, the popover or drawer closes; if another open MR remains the trigger
  re-selects it, and if none does the chip unmounts. No stale MR is left
  rendered either way.
- **The GitLab connection status has not resolved yet.** `useGitLabAvailable()`
  returns `false` until the mount-time `fetchGitLabStatus` resolves (and it
  actively clears any cached value first — see Sync and freshness). So `canLink`
  is `false` on the chip's first render and may become `true` a moment later
  **without a remount**. The chip SHALL treat this as ordinary prop change, not
  as a terminal "GitLab unavailable" state: it renders normally, and the
  popover's link control simply appears when the status resolves. The chip SHALL
  NOT cache the first value, gate its own render on it, or show an error. If the
  status request fails, `useGitLabStatus` stores `null`, `canLink` stays `false`,
  and the popover renders without the link control — the same as a workspace
  with GitLab genuinely not configured, which is the existing behaviour of every
  other consumer.
- **The automation-options fetch fails.** `useTaskMRAutomationOptions` stores the
  error and its lazy effect is gated on `error`, so it does not retry for the
  life of the mount. The chip treats this exactly as options-not-loaded: status
  glyph renders, no badges, no error text and no retry affordance on the chip
  itself. The topbar's `MRAutomationControls` remains the surface that reports
  and retries automation errors.

## Sync and freshness (explicit non-responsibility)

GitHub's `PRStatusChip` is load-bearing for refresh. It calls
`usePRFeedbackBackgroundSync`, and `pr-topbar-button.tsx:162` documents the
topbar button relying on the chip being mounted for that warming. GitLab's
refresh does not work that way and this chip SHALL NOT copy the responsibility:

- MR list hydration is a one-shot `useWorkspaceMRs(workspaceId)` fetch whose
  "already fetched" guard is a per-hook-instance ref, so each additional call
  site issues its own `GET /task-mrs` per workspace. There are already **four**
  production call sites, and the chip SHALL NOT become a fifth:

  | Call site | Mounted by |
  |---|---|
  | `components/gitlab/mr-topbar-button.tsx` | the task topbar |
  | `components/kanban-board.tsx` | the kanban board |
  | `hooks/domains/gitlab/use-mr-key-to-tasks.ts` | `app/gitlab/gitlab-page-client.tsx` |
  | `hooks/domains/workspace/use-external-vcs-file-link.ts` | `useExternalVcsFileLinkHydration` in `components/task/task-page-content.tsx`, gated on the task having a GitLab-provider repository |

  The chip SHALL NOT call `useWorkspaceMRs`.
- There is no shared MR feedback cache. `useMRFeedback` is a per-instance
  reducer with no store slice, so there is nothing for the chip to warm and
  nothing the topbar would inherit from it. The chip SHALL NOT add a warmer.

**Two** fetch triggers are genuinely new, and both are accepted rather than
hidden.

**New trigger 1: the automation options.** The chip calls
`useTaskMRAutomationOptions(taskId)` to render its badges, and that hook lazily
issues a `GET` when the store holds no options for the task. Before this feature
those options were fetched only when the topbar dropdown or popover mounted
`MRAutomationControls`; now they are fetched for any session view that has a
linked open MR.

**New trigger 2: the GitLab connection status, which also has a cross-surface
side effect.** The chip calls `useGitLabAvailable()` to source `canLink`. That
helper (`hooks/domains/gitlab/use-task-mr.ts`) wraps `useGitLabStatus()`, whose
effect (`hooks/domains/gitlab/use-gitlab-status.ts`) runs on **every mount** and
unconditionally does `setStatus(workspaceId, null)` and then
`void loadStatus(workspaceId)` -> `fetchGitLabStatus`. Two consequences, both
stated rather than discovered at Build time:

1. **It is a request, not a cache read.** Despite the hook's "store-cached" doc
   comment, the guard is per-hook-instance: a populated store does not
   short-circuit the effect. Every chip mount issues one `GET`, and two mounted
   chips issue two.
2. **It transiently blanks a value other surfaces read.** Because the effect
   clears the status to `null` before refetching, every other
   `useGitLabAvailable()` consumer — `hooks/use-nav-availability.ts`,
   `components/kanban-external-link-availability.ts`,
   `components/task/task-session-sidebar-task-linking.ts`, and
   `components/app-sidebar/sections/settings/workspaces-group.tsx` — reads
   `false` for the duration of the in-flight request. Mounting the chip
   therefore has a brief, observable effect on unrelated navigation UI.

This is **accepted, not fixed here.** The chip is not the first consumer to do
it (`MRTopbarButton` already calls `useGitLabAvailable()` on the same task
route), the window is one request long, and the recovery is automatic when the
status resolves. Removing the blanking means changing `useGitLabStatus` for
every caller, which is its own card (see Out of scope). The chip SHALL NOT work
around it with a local cache, a mount-order guard, or a debounce, and SHALL NOT
read the status slice directly to dodge the hook. What the chip owes here is
only that `canLink` is allowed to be `false` on the first render and become
`true` without a remount — see Failure modes.

The popover's MR feedback read is **not** a new trigger in this sense. It is
`MRCIPopover`'s own existing fetch, it stays gated on that component's `enabled`
prop, and the chip passes its disclosure-open state there (see the prop table
under API surface). So it fires when a user opens the chip's popover or drawer,
exactly as it already fires when a user opens the topbar's, and never on mount.

What that does and does not buy across two mounted chips, stated precisely
because the obvious argument is invalid:

- **Guaranteed.** A chip whose disclosure is closed passes `enabled: false` and
  issues no feedback read. A chip that is never opened never fetches feedback.
- **NOT guaranteed.** Two chips mounted for one task may each have an open
  disclosure at the same time, and then each issues its own feedback read. An
  earlier draft argued they "do not double it, because at most one disclosure is
  open per chip" — that is a **non-sequitur** and is retracted: per-*chip*
  exclusivity gives no cross-*chip* exclusivity, and one open disclosure on each
  of two chips is exactly two concurrent `enabled: true`.

**Simultaneous open across two chips IS permitted.** This is a decision, not an
omission. Each mounted chip owns its own disclosure state (see Accessibility)
and this spec adds **no cross-chip coordinator**, no shared "currently open
chip" context, and no global registry. The state is reachable in practice
because the fine-pointer chip opens on **focus** as well as hover: a keyboard
user can focus chip A open and then hover chip B, leaving both open.

Two doubled feedback reads is the accepted cost. The alternative — a global
mutual-exclusion coordinator — would be new shared state that neither mount
point nor `MRTopbarButton` has today, would have to decide which chip loses,
and would make each chip's behaviour depend on what else is mounted. That is a
worse contract than an occasional duplicate idempotent `GET`. `useMRFeedback` is
per-instance with no store slice, so two concurrent reads cannot corrupt each
other; they resolve independently into their own popovers.

The dedupe this buys, stated precisely rather than optimistically:

- **Guaranteed.** Once options for a task are in the store, any number of
  further chip mounts for that task issue **zero** requests. The hook's effect
  is gated on `options || loading || error`, and a populated store short-circuits
  it on the first render.
- **Guaranteed.** A single chip mount issues at most one request.
- **NOT guaranteed, and not fixed by this card.** Two chips mounted for the same
  task in the **same React commit**, with no cached options, may each issue one
  request. The hook's guard reads render-captured values, so both effects run
  before either re-render observes `loading === true`. This is reachable:
  `PassthroughToolbar` has five mount sites and `ChatInputArea` three, several
  of them dockview-driven, so nothing structurally prevents two chips for one
  task existing at once.

The duplicate is a `GET` with no side effect, and the hook's own request-id
check (`isCurrentAndUnchangedExternally`) means the store converges to one
options object regardless of which response lands last, so the outcome does not
depend on ordering. Adding a shared in-flight guard to
`useTaskMRAutomationOptions` would fix it, but that hook is also used by
`MRAutomationControls` from the topbar, so the change belongs to a card that can
test both callers; it is named in Out of scope. The chip SHALL NOT work around
the gap with a module-level cache of its own.

The same render-captured pattern is present in `useTaskCIAutomationOptions`, so
`PRStatusChip` does not deliver cross-mount dedupe either. Parity with GitHub is
recorded here as an observation, not as evidence that the property holds.

**Hydration ownership, and why this spec pins no outcome for it.** An earlier
draft claimed `MRTopbarButton` was the only mount point for `useWorkspaceMRs`
and derived from that a specific consequence: that an archived task never
hydrates the MR store, so the chip renders nothing there. **Both halves were
wrong** and the claim is retracted. There are four call sites (table above), and
one of them — `useExternalVcsFileLinkHydration` at
`components/task/task-page-content.tsx` — runs on the `/tasks/:id` task page for
any task with a GitLab-provider repository, independently of the topbar and
independently of archived state.

So whether the MR store is hydrated for a given task depends on which surfaces
the session has visited and which route rendered the chip, not on a single
owner. This spec therefore states no archived-task rendering outcome and **no AC
depends on one.** What the chip guarantees is only the conditional already in
Failure modes: if the task's MR list is not hydrated, `useTaskMRs` returns the
shared empty array and the chip renders nothing, exactly as it does for a task
with no MRs. Tests SHALL seed the store rather than rely on any ambient
hydration path, which the E2E decision already requires.

Giving GitLab MR hydration a single provider-level owner is its own card (see
Out of scope). That card is what would make a hydration outcome specifiable;
until it lands, asserting one here would pin a route-history-dependent flake.

## Responsive behaviour

- Disclosure is chosen by pointer precision only, via `useTouchDrawer()`
  (`!isFinePointer`): coarse pointer renders the `Drawer` variant, fine pointer
  renders the hover `Popover` variant.
- The chip SHALL NOT gate any part of itself on a Tailwind width class
  (`sm:`, `md:`, ...). Pairing a width-gated class with the pointer-gated hook
  is what leaves 640-767px rendered by neither branch, and the chip has no
  width-dependent behaviour to justify the risk.
- The drawer variant carries a translated title, a screen-reader description,
  and a close button, and its body scrolls independently.
- **Hover-popover placement is pinned by value**, so two builders cannot diverge
  visually while passing every scenario: `align="end"`, `sideOffset={4}`, and a
  `w-80` content width — the values `MRTopbarButton` already uses. `side` is
  left to Radix's default collision handling, which is what keeps the content on
  screen for a chip sitting at the bottom of the viewport; the "bounding box `y`
  is greater than or equal to 0" scenario below is the observable consequence of
  that and SHALL NOT be satisfied by hard-coding a `side`.

## Accessibility, focus, and duplicate mounts

- The trigger is a `<button type="button">` with `cursor-pointer` and a
  translated `aria-label` naming the MR (or the MR count) and its status. The
  drawer variant additionally carries `aria-haspopup="dialog"` and
  `aria-expanded`.
- **The `aria-label` describes the acted-on MR**, so it follows the same regime
  as `data-mr-iid`: the live selected MR while the disclosure is closed, the
  frozen MR while it is open. It therefore always agrees with `data-mr-iid` and
  always names the MR whose controls the disclosure actually operates on. It
  does NOT follow `data-status`'s live tracking, because a label that renamed
  the MR under a screen-reader user mid-interaction would describe controls
  other than the ones in front of them.
- The fine-pointer hover lifecycle SHALL be the existing
  `useHoverPopover({openDelayMs, closeDelayMs, disabled})`
  (`apps/web/hooks/domains/github/use-hover-popover.ts` — note it takes a single
  options object, not three positional arguments), wired exactly as
  `MRTopbarButton` wires it. That wiring is, in full and with nothing omitted —
  **thirteen** handlers, not the four-plus-two a reader might assume:

  | Element | Event | Handler |
  |---|---|---|
  | trigger | `onMouseOver` | `onTriggerEnter` |
  | trigger | `onMouseEnter` | `onTriggerEnter` |
  | trigger | `onMouseMove` | `onTriggerEnter` |
  | trigger | `onPointerOver` | `onTriggerEnter` |
  | trigger | `onPointerEnter` | `onTriggerEnter` |
  | trigger | `onPointerMove` | `onTriggerEnter` |
  | trigger | `onFocus` | `onTriggerEnter` |
  | trigger | `onMouseLeave` | `onTriggerLeave` |
  | trigger | `onPointerLeave` | `onTriggerLeave` |
  | trigger | `onBlur` | `onTriggerLeave` |
  | content | `onMouseEnter` | `onContentEnter` |
  | content | `onMouseMove` | `onContentEnter` |
  | content | `onMouseLeave` | `onContentLeave` |

  **The redundancy is deliberate and SHALL be reproduced, not tidied.** An
  earlier draft of this spec listed only six of these rows while also declaring
  the list complete; that was wrong, and a builder who implements six rows ships
  a chip that is measurably flakier than the topbar rather than identical to it.
  Two of the omitted rows are individually load-bearing:

  - **`onMouseMove` on the content.** `use-hover-popover.ts`'s own doc comment
    states the reason: "The content also treats mouse-move as 'enter' so a flaky
    or missed portal mouseenter can't strand a pending close." This is precisely
    the mechanism the "pointer crosses the gap" scenario below depends on.
    Without it that scenario does not fail cleanly — it flakes.
  - **`onMouseMove` / `onPointerMove` on the trigger.** These re-assert hover for
    a pointer that is resting on the trigger without generating a fresh
    `mouseenter`, which is the state the closed-trigger click scenario describes.

  The remaining aliases (`onMouseOver` / `onPointerOver` / `onPointerEnter`, and
  `onPointerLeave`) are defensive duplicates covering browsers and input devices
  that dispatch the pointer family but not the mouse family, or vice versa.
  `onTriggerEnter` and `onTriggerLeave` are idempotent — they set a boolean ref
  and schedule or cancel a timer — so firing them several times for one physical
  gesture is harmless by design, and that is what makes the duplication safe.

  Because reproducing thirteen rows by hand is exactly the kind of thing that
  drifts, the chip SHOULD prefer **exporting and reusing** the existing private
  `useMRPopoverInteractions` wrapper (and, if it is convenient, the trigger's
  handler-spreading shape) from `mr-topbar-button.tsx` over re-typing the set.
  Reuse is the preferred closure of this requirement; hand-copying is permitted
  only if every row above is present.

  The content handlers are what let the pointer cross the `sideOffset` gap
  without closing. `disabled` is `useTouchDrawer()`. `openDelayMs` and
  `closeDelayMs` SHALL both be **150**, the values `MRTopbarButton` uses. Those
  constants live in a private `useMRPopoverInteractions` wrapper inside
  `mr-topbar-button.tsx`, so "wired as the topbar wires it" does not by itself
  pin them; they are pinned here by value. The chip MAY either export that
  wrapper for reuse or wire `useHoverPopover` directly with the same values.
- **The popover DOES close on blur**, because `onBlur` is wired to
  `onTriggerLeave` exactly as the topbar wires it. It also closes on Escape, on
  outside interaction (Radix `onOpenChange`), and when the pointer leaves both
  the trigger and the content. It opens on keyboard focus of the trigger and
  SHALL NOT steal focus when it opens.

  This corrects a claim an earlier draft of this spec made. For the record,
  because the wrong version is the kind of thing that gets re-derived: it is
  true that `useHoverPopover` itself declares no blur handler, but
  `MRTopbarButton` wires `onBlur` to the hook's `onTriggerLeave` at the
  component level. So wiring blur is what matches the topbar and omitting it is
  the divergence. Nor does wiring it re-introduce the portal race:
  `onTriggerLeave` does not close anything directly, it calls `scheduleClose`,
  which re-checks `overTrigger` and `overContent` at the moment the timer fires
  and commits the close only when the pointer is over neither region. That
  re-check is precisely the mechanism the hook exists to provide.

  Closing on blur does not resurrect the keyboard-operability problem, because
  this spec does not claim the popover's controls are Tab-reachable — see the
  next bullet, which states the opposite plainly. Focus-to-open makes the
  content *readable* by a keyboard user; tabbing away closes it again; the
  controls inside it were never Tab-reachable from the trigger in the first
  place, so nothing is lost by the close.
- **Scope of the keyboard claim.** Focus-to-open makes the popover's *content
  readable* by a keyboard user. It does NOT make the popover's controls
  (link, unlink, merge) Tab-reachable: Radix portals the content to the end of
  the document and provides no tab bridge from the trigger, so Tab moves past
  the chip rather than into it. This spec does not claim otherwise, and does not
  fix it. No capability is lost: link and unlink are keyboard-operable today
  through `MRTopbarButton`'s click-driven `DropdownMenu`, and on a coarse
  pointer the chip's own drawer is a focus-trapping dialog in which every
  control is operable. Making `MRCIPopover`'s controls keyboard-reachable from a
  hover popover is a pre-existing gap shared with `PRStatusChip` and the topbar,
  and is named in Out of scope.
- **Clicking the fine-pointer trigger is a no-op in both states.** While the
  popover is open, it stays open and nothing navigates; Radix treats the trigger
  as outside the content, so this requires an explicit outside-pointer-down
  guard. While the popover is closed, the click also does nothing on its own:
  the fine-pointer chip opens on hover and focus only, and there is no
  click-to-open path. A user who clicks a closed chip faster than the 150ms
  open delay therefore sees the popover open on the hover timer as normal,
  because the pointer is still over the trigger; the click neither accelerates
  nor suppresses it. This is a deliberate divergence from `MRTopbarButton`,
  whose trigger navigates to the MR detail panel on click — the chip has no
  detail-panel action at all (see Out of scope), so it has nothing to do with a
  click.
- **Closing the drawer returns focus to the trigger when the trigger is still
  mounted.** It often will not be: unlinking the task's last open MR from inside
  the drawer, or that MR going terminal, unmounts the chip and its trigger in
  the same update that closes the drawer. In that case the chip makes no focus
  claim and focus falls to `document.body` per browser default. This is a named,
  accepted consequence rather than a specified behaviour, it is shared with
  `PRStatusChip`, and improving it is listed in Out of scope. No AC asserts a
  focus target for the unmounted case.
- The two mount points are alternative surfaces, but nothing guarantees only one
  is mounted at a time. Two `mr-status-chip` elements may therefore coexist in
  the DOM. Both are correct and identical; E2E selectors SHALL scope to
  `chat-status-bar` or `passthrough-status-row` rather than matching the testid
  globally, and each mounted chip owns its own disclosure state.
- On a workspace switch the chip does not show the previous workspace's MRs.
  **The mechanism is workspace-scoped selection, not a reset of the outgoing
  bucket** — an earlier draft of this spec said the opposite and it was wrong.
  What actually happens: `useWorkspaceMRs` calls `resetTaskMRs(workspaceId)`
  with the **incoming** workspace id (the all-buckets `resetTaskMRs()` runs only
  when the workspace goes null), so the outgoing workspace's bucket is left in
  the store untouched. The chip reads nothing from it because `useTaskMRs`
  selects `byWorkspaceId[activeWorkspaceId]`, and `activeWorkspaceId` is already
  the incoming one. The chip therefore unmounts until the incoming workspace's
  MRs land.
  The correction matters for what it forbids: the chip SHALL NOT add any
  outgoing-workspace cleanup of its own, and no AC asserts that the previous
  workspace's bucket was cleared.

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

## Constraints

- New files SHALL be at most 600 lines and new components at most 200 lines.
  The GitHub original is 634 lines in one file and is not a shape to copy;
  the trigger, the glyph, the disclosure variants and the status derivation are
  separate units.
- New copy SHALL use a `mrChip*` key prefix in
  `apps/web/src/locales/en/gitlab.json`, following the same surface-scoped
  prefix convention as the existing `mrPopover*` and `mrAutomation*` keys.
  Copy that already exists under `mrPopover*` SHALL be reused rather than
  duplicated. `data-testid` values and `data-status` values are identifiers and
  SHALL NOT be translated.
- Every new file SHALL be appended to `i18nGuardFiles` in
  `apps/web/eslint.i18n.options.mjs` in the same change.
- E2E coverage is required. The page object SHALL mirror the shape of
  `apps/web/e2e/pages/session-page.ts` `tapPRStatusChip`, exposing
  `mrStatusChip()`, `mrStatusChipDrawer()`, `mrStatusChipDrawerClose()`,
  `mrStatusChipPopoverInner()` and `tapMRStatusChip()`.

  Mirroring the shape means mirroring its **scoping**, not just its arity.
  `prStatusChip()` is not a bare `getByTestId`; it resolves
  `activeChat()` -> `chat-status-bar` -> `pr-status-chip`, which is what keeps it
  from tripping Playwright's strict-locator check when more than one status row
  is mounted. `mrStatusChip()` SHALL do the same for `chat-status-bar`. Because
  this feature also has a scenario against the passthrough row, the page object
  SHALL additionally expose **`mrStatusChipInPassthrough()`**, scoped to
  `passthrough-status-row`. Without it the mandated zero-argument set can reach
  only one of the two surfaces this spec requires coverage of. No accessor may
  resolve `mr-status-chip` globally or paper over duplicates with `.first()`.
- Unit coverage SHALL include the `getMRStatusColor` and
  `aggregateMRStatusColor` parity tables described above, updating
  `mr-task-icon.test.ts` and `mr-status-colour-parity.test.tsx`. The
  `aggregateMRStatusColor` table SHALL include at least one all-terminal pair in
  both input orders, so the preserved array-order tie is pinned rather than
  assumed.
- **File boundary.** The rule below governs **production React source only**. It
  does not and cannot restrict the supporting files this same section mandates:
  the locale catalog `apps/web/src/locales/en/gitlab.json`, the guard list
  `apps/web/eslint.i18n.options.mjs`, the page object
  `apps/web/e2e/pages/session-page.ts`, new specs under `apps/web/e2e/`, and the
  unit tests named above are all required edits and all sit outside
  `components/gitlab/`. An earlier draft stated the rule without that carve-out
  and so contradicted its own i18n and E2E requirements.

  Within production React source: the one permitted change outside
  `components/gitlab/` and the two mount points is extracting `useUnlinkTaskMR`
  into `apps/web/hooks/domains/gitlab/use-task-mr.ts` and pointing
  `MRTopbarButton` at it. `MRTopbarButton`'s rendered output and observable
  behaviour SHALL be unchanged, and `components/github/` SHALL NOT be touched at
  all. `components/azure-devops/` SHALL NOT be touched either — which is why the
  Azure leg of the DOM ordering has no AC.

## Related

- [gitlab-integration](../gitlab-integration/spec.md): the umbrella GitLab
  feature this chip sits inside.
- `apps/web/CLAUDE.md`, "GitHub PR status UI": the GitHub-side invariant this
  feature's single-derivation requirement mirrors for GitLab.
