---
id: coordinator-proposal-cards-design
title: Proposal cards design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-28
last_updated: 2026-09-28
requirements:
  - REQ-COORDINATOR-PROPOSALS-004
  - REQ-COORDINATOR-PROPOSALS-005
---

# Proposal cards System Design

## Purpose and boundaries

How the web client reads proposals and lets a manager decide them, on the
Needs you screen and in the copilot transcript. The routes, events and the
approve and reject semantics it calls are in [proposals.md](proposals.md);
this design adds no route and changes no backend behaviour.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-PROPOSALS-004` (`AC-COORDINATOR-PROPOSALS-004.4`) | [Client store](#client-store) |
| `REQ-COORDINATOR-PROPOSALS-005` | [Cards](#cards) |

## Client store

`hooks/domains/coordinator/use-proposals.ts` keeps proposals keyed by id per
coordinator. It loads `status=pending` on mount and refetches on
`coordinator.updated`. A merge never replaces a settled status (`approved`,
`rejected`) with an unsettled one, so a late list response cannot revive a
decided card.

It is the only proposal cache on the Needs you and Queue screens:
`app/coordinator/use-coordinator-inputs.ts` drops its own `listProposals`
read and takes its proposals input from this store (the cached entries whose
status is open: `pending`, `approving` or `failed`), with the same
`value`/`loadedAt`/`error` entry shape and the same `retryFailed` behaviour
it has today. The input's `error` is set while the latest `status=pending`
read failed.

**Open proposals are Needs-you items.** `classify()` in
`apps/web/lib/coordinator/attention.ts` makes every proposal in its input
whose status is `pending`, `approving` or `failed` a proposal item
(`AC-COORDINATOR-NEEDS-YOU-001.1`); an `approved` or `rejected` row, or any
other status, is not an item. Its current filter keeps `pending` only and is
widened by this design, so a proposal that is being approved, or whose
create failed, stays on Needs you and shows that state
(`AC-COORDINATOR-PROPOSALS-005.2`, `AC-COORDINATOR-PROPOSALS-005.3`). Item
ordering and counts are unchanged ([needs-you](needs-you.md#classification)).
`AttentionProposal` keeps its narrow shape (`id`, `status`, `task_id`,
`spec`, `created_at`), so `attention.ts` still has no dependency on the API
client.

**Merging one proposal.** Every source (the pending list, a by-id read, a
decision's 200 response, a 409 `proposal_conflict` body) merges one row at a
time by id, with these rules in order:

1. No cached entry: store the incoming row.
2. The cached row is settled and the incoming row is not: keep the cached
   row.
3. The incoming `updated_at` is later than the cached one: take the incoming
   row.
4. The incoming `updated_at` is earlier: keep the cached row.
5. Equal `updated_at`: take the incoming row only when it is settled and the
   cached one is not; otherwise keep the cached row.

Every proposal write stamps `updated_at`, so rule 3 orders two unsettled
responses (a late `pending` list response cannot overwrite a fresher
`approving` one), and rule 2 holds even against a clock that went
backwards.

A `status=pending` refetch alone cannot satisfy
`AC-COORDINATOR-PROPOSALS-004.4` for a proposal this client already knows
about: once it settles, the pending list simply stops containing it, so a
browser that had it cached as `pending`/`approving` would otherwise keep
showing that stale state forever instead of the settled one. On each
`coordinator.updated`, after merging the fresh `status=pending` response, the
hook also fetches by id (`GET .../proposals/:pid`) every locally cached id
that response no longer contains and whose cached status is not settled
(a settled entry cannot change again), and merges each result the same way.
This is bounded by how many ids the client has ever locally held for its
coordinator, not by the 25-open cap. A Needs-you item that settles and was
never cached beyond the list is not affected: it is correct for it to simply
stop being a Needs-you item.

**Read failures.** Reads never throw into the UI:

- A failed `status=pending` read keeps every cached entry as it is, sets the
  input's `error` (the screen's existing input-failure banner), and is
  retried by `retryFailed` or the next `coordinator.updated`. No by-id
  backfill runs for that event, because the hook cannot tell which ids the
  list dropped.
- A failed by-id read (network or 5xx) keeps that entry as it is; the next
  `coordinator.updated` tries it again. It does not set the input's `error`.
- A by-id read that returns 404 (the proposal was deleted with its
  coordinator or workspace) evicts that id from the cache.

Overlapping reads are not cancelled; the merge rules above decide which row
wins, whatever order the responses arrive in.

The chat transcript's `ProposalCard` (see [Cards](#cards)) is attached to a
specific `proposal_id` that can outlive the pending window entirely: a
reloaded second browser may never have fetched the pending list at the
moment a proposal it is displaying was still open. It therefore does not rely
on `use-proposals.ts`'s list-keyed cache: it fetches its own `proposal_id` by
id on mount and again on every `coordinator.updated` for its coordinator,
independent of whether that id is in the current pending list, which is what
lets it show "Approved: <card>" or "Rejected: <reason>"
(`AC-COORDINATOR-PROPOSALS-005.8`) after a reload with no other proposal
ever having been listed. It merges its own reads and its decision responses
with the same five rules. Until its first read succeeds it renders a
one-line "Loading proposal" placeholder; a failed read keeps the last row
it had (or the placeholder) and retries on the next `coordinator.updated`;
a 404 renders "This proposal no longer exists." with no actions.

## Cards

- One `ProposalCard` renders pending, approving, failed and settled states; it
  is used on Needs you and inside the copilot transcript.
- In the transcript the card is attached to the tool-call message of
  `propose_task_kandev` by the `proposal_id` in the tool's JSON result text.
  A tool call whose result is an error, or whose text has no string
  `proposal_id`, renders the ordinary tool-call message and no card.
- Readers get the card without actions.
- **Row source on Needs you.** A Needs-you proposal item carries only the
  narrow `AttentionProposal`. Its `ProposalCard` reads the full `Proposal`
  row (`final_spec`, `error`, `reject_reason`, `updated_at`, `task_id`) from
  `use-proposals.ts` by the item's id. The classification's proposals input
  is derived from the same store in the same render, so a classified item
  always has its row; the card never reads a proposal by id itself on Needs
  you. The chat card owns its own row (see [Client store](#client-store)).

**Content by state.** Every state shows, top to bottom: the status line of
the table below, the title, then "`<workflow>` · `<step>`" of the current
spec (`final_spec` when set, else `spec`), with
names from the workspace's workflow snapshots and the raw id as the fallback
when a name is unknown, as `proposal-details.tsx` does today.

| Status | Status line | Actions (managers) |
| --- | --- | --- |
| `pending` | "Pending Approval" | Approve, Edit, Reject |
| `approving` | "Approval in progress. Edits are locked." | none |
| `failed` | "Could not create the task: `<error>`. Nothing was created."; when `error` is null or empty, "Could not create the task. Nothing was created." | Approve, Edit, Reject |
| `approved` | "Approved: `<card>`" | none |
| `rejected` | "Rejected: `<reason>`"; when `reject_reason` is null, "Rejected" | none |

On Needs you the card also keeps the description, the "Proposed by
`<coordinator>`" line and the propose-only policy line that
`proposal-details.tsx` renders today (AC-COORDINATOR-NEEDS-YOU-002.8), for
managers and readers alike, in the order `proposal-details.tsx` uses today:
the description between the title and "`<workflow>` · `<step>`", then
"Proposed by", then the policy line, then the actions. The chat card omits
those three lines, because
the transcript around it already says who proposed it and why. Settled
states only occur on the chat card; a settled proposal is not a Needs-you
item.

**`<card>` and `<step>`.** Both come from the approved proposal:

- `<card>` is the task's `identifier` (for example `KAN-432`). The card
  reads it with `fetchTask(task_id)` once, when it first sees the proposal
  `approved`. When the identifier is absent, the read fails, or the task was
  deleted, `<card>` is the `final_spec` title.
- `<step>` (approve toast only) is the name of `final_spec.step_id` in
  `final_spec.workflow_id`, from the same workflow names as the card; when
  unknown, the step id.

**Edit form options.** The Edit form's options come from data the client
already reads; no route is added.

- Workflows: the workspace's workflows (`listWorkflows`).
- Steps: the chosen workflow's steps from its snapshot
  (`fetchWorkflowSnapshot`), filtered by a TypeScript copy of the eligible-step
  walk in `lib/coordinator/eligible-step.ts`, over the same fields the Go
  loader maps (`is_start_step`, `allow_manual_move`, `pull_from_step_id`, and
  an `on_enter` action of type `auto_start_agent`). The client filter is only
  a convenience: the approve route re-validates, and its 400 is shown as
  below. When the chosen workflow has no eligible step, the step field says
  "No step in this workflow can take a proposed task" and Approve with edits
  is disabled.
- Repository: "No repository" plus the workspace's repositories
  (`listRepositories`).
- The form opens with the current spec's values. Changing the workflow
  resets the step to that workflow's start step when it is eligible, else to
  empty; Approve with edits is disabled while the step is empty.
- **Loading.** Opening the form issues `listWorkflows`, `listRepositories`
  and `fetchWorkflowSnapshot` of the current workflow in parallel, once per
  opening. Choosing another workflow issues that workflow's snapshot read; a
  response for a workflow that is no longer selected is discarded. Title and
  description are editable at once. While a read is in flight its field shows
  "Loading options" and is disabled, and Approve with edits is disabled until
  the workflow list and the chosen workflow's steps have loaded. The step
  field shows the current workflow's snapshot outcome (steps or read failure)
  only once the workflow list has loaded and contains that workflow; until
  then it shows "Loading options". When the workflow list read fails, the
  current workflow stays selected and its snapshot outcome shows as usual.
- **Read failures.** A failed options read leaves its field disabled with an
  inline line "Could not load options." and a **Try again** button that
  re-issues only that read. While the workflow list or the step read has
  failed, Approve with edits is disabled; a failed step read never shows the
  "No step in this workflow can take a proposed task" line. A failed
  repository read does not disable Approve with edits: the repository field
  keeps the current value as its only option, and an untouched field is not
  sent. Cancel and the other fields stay usable in every case.
- **A current value missing from its options.** When the current workflow is
  not in the loaded workflow list (it was deleted), the workflow field opens
  empty with the note "The proposed workflow no longer exists. Choose a
  workflow.", and the step field is empty with no "Loading options", no
  "Could not load options." and no "No step" line. The current workflow's
  snapshot read, already issued at opening, is not cancelled: its response
  or failure, whenever it arrives, is discarded, and no further snapshot read
  is made until a workflow is chosen. When the current step is not among the chosen
  workflow's eligible steps (deleted, or no longer eligible), the step field
  opens empty with the note "The proposed step can no longer take this task.
  Choose a step." Both keep Approve with edits disabled until a value is
  chosen, which keeps the step list to eligible steps only
  (`AC-COORDINATOR-PROPOSALS-005.4`). When the current repository is not in
  the loaded repository list, the field keeps it selected, labelled "Unavailable
  repository", and an untouched field is not sent; the route's 400 decides
  whether it can still be used.
- A title that is empty after trimming is refused in place without a
  request: an inline `role="alert"` under Title says "Enter a title" and
  focus moves to Title (`AC-COORDINATOR-PROPOSALS-005.4`). Every other
  validation is the route's, shown as a 400 below.
- The approve body carries only fields whose value differs from the current
  spec, so an untouched form approves the spec unchanged.

**In-flight lock.** From the click of Approve, Approve with edits or Confirm
reject until that request settles, every action of that card (and of its
open form) is disabled and the pressed button shows a spinner; the button's
accessible name does not change, and one translated `role="status"` region on
the card announces "Working". A second click, Enter or tap while locked
sends nothing. The lock is per card: other cards stay usable.

**Decision outcomes.** The acting card reads the outcome from the request:

| Outcome | What the manager sees |
| --- | --- |
| 200, `approved` | The store takes the row; the approve toast of `AC-COORDINATOR-PROPOSALS-005.7`, then the "Next" line; the Needs-you item leaves the list. |
| 200, `rejected` | Same, with the reject toast. |
| 200, `failed` (the create failed) | The store takes the row; the card shows the failed state in place; no toast; the form, if open, closes. |
| 400 | The store is unchanged; the form (Edit or Reject) stays open with the typed values; the backend's `error` text renders in an inline `role="alert"` under the field its `field` names, or above the buttons when `field` is absent or not on the form, and focus moves to that field (or the alert). A 400 from plain Approve (no form open) opens the Edit form and shows it the same way. |
| 409 `proposal_conflict` | The store takes the embedded row at once; any form closes; a toast says "Someone else already decided this proposal." When the row is still open (for example `approving`), the card shows that state. |
| 403 | A toast says "You no longer have permission to decide proposals."; the form stays open; the store is unchanged. |
| 404 | The store evicts the id (the item leaves Needs you); a toast says "This proposal no longer exists." |
| Network error or 5xx | A toast says "Could not reach Kandev. Nothing was decided. Try again."; the form stays open with its values; the store is unchanged; the next `coordinator.updated` refreshes the row. |

**Forms and navigation.**

- Edit and Reject forms render in place on the Needs-you card; opening one
  moves focus to its first field (Title, or Reason). Only one form is open
  per card; opening the other closes the first.
- From the chat card, Edit and Reject navigate to
  `linkToCoordinatorNeedsYou(workspaceId, coordinatorId)` with the query
  `?proposal=<id>&form=edit|reject`, and close the copilot popover. The
  Needs you screen, once its inputs have loaded, scrolls the item
  `needs-you-item-<id>` into view, opens that form and moves focus to its
  first field, then removes both query params with a history replace. When
  the proposal is not a Needs-you item (it settled, was deleted, or the
  inputs failed), no form opens and a toast says "This proposal is no longer
  waiting for a decision." A `form` value other than `edit` or `reject` is
  ignored. A reader never sees the chat card's actions, so never navigates.

**Remote changes while a form is open.** A store merge from
`coordinator.updated` (a decision in another browser or on the other surface)
can change a card while its Edit or Reject form is open and no request of
its own is in flight:

- The status stays `pending` or `failed`: the form stays open with its typed
  values; only the card's content above the form updates.
- The status becomes `approving`: the form closes and its typed values are
  discarded, because `approving` has no actions
  (`AC-COORDINATOR-PROPOSALS-005.2`). When focus was inside the form, it
  moves to the card's heading. The card's `role="status"` region announces
  the new status line.
- The item leaves the list (settled or evicted): when focus was inside that
  item, it moves as in "Focus after a decision" below. No decision toast is
  shown; the toasts of `AC-COORDINATOR-PROPOSALS-005.7` belong to the
  manager whose request decided it.

When the card's own request is in flight, the lock holds and the request's
outcome row applies when it settles (typically the 409 row).

**Focus after a decision.** When a decided item leaves the Needs-you list,
focus moves to the heading of the next item in list order; with no next
item, to the previous item's heading; with no item left, to the empty
state's heading. Each item heading is focusable (`tabIndex={-1}`). On the
chat card, focus stays on the card, which now shows its settled line.

**Toast counts.** The decision toast reads the item count from the
classification of [needs-you](needs-you.md#classification) after the store
update.

## Security

- Decisions authorise at the backend by workspace scope; the client hides
  actions from readers but never relies on that.
- Spec strings, error text and reject reasons are untrusted and rendered as
  text.

## Related decisions

- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
