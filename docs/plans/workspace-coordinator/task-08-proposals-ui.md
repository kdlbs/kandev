---
id: "08-proposals-ui"
title: "Proposals decided: proposal UI"
status: pending
wave: 4
depends_on:
  - "06-copilot-wired"
  - "07-proposals-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-PROPOSALS-004
  - REQ-COORDINATOR-PROPOSALS-005
acceptance_criteria:
  - AC-COORDINATOR-PROPOSALS-004.4
  - AC-COORDINATOR-PROPOSALS-005.1
  - AC-COORDINATOR-PROPOSALS-005.2
  - AC-COORDINATOR-PROPOSALS-005.3
  - AC-COORDINATOR-PROPOSALS-005.4
  - AC-COORDINATOR-PROPOSALS-005.5
  - AC-COORDINATOR-PROPOSALS-005.6
  - AC-COORDINATOR-PROPOSALS-005.7
  - AC-COORDINATOR-PROPOSALS-005.8
  - AC-COORDINATOR-PROPOSALS-005.9
system_design:
  - ../../specs/coordinator/system-design/proposal-cards.md
  - ../../specs/coordinator/system-design/proposals.md
---

# Task 08: Proposals Decided, UI (WP-5b)

## Summary

Add the client proposal store and the Approve, Edit and Reject actions on both
surfaces: the Needs you item card (task 04) and the chat card in the copilot
transcript (task 06), on task 07's routes. Completes phase 1.

## In scope

- `hooks/domains/coordinator/use-proposals.ts` with sticky settled status; a
  decision's response applies at once; `coordinator.updated` refreshes, and
  also backfills by id any locally cached proposal the refreshed pending list
  no longer contains, per
  [proposals design](../../specs/coordinator/system-design/proposal-cards.md#client-store).
  The chat transcript's `ProposalCard` fetches its own `proposal_id` by id on
  mount and on every `coordinator.updated`, independent of the pending list.
  One merge rule for every source (sticky settled, then later `updated_at`
  wins, settled wins a tie) and the read-failure and 404 rules, per
  [Client store](../../specs/coordinator/system-design/proposal-cards.md#client-store).
- Replace `app/coordinator/use-coordinator-inputs.ts`'s own `listProposals`
  read with `use-proposals.ts`, so Needs you, Queue and the toast count read
  one proposal cache (needs-you.md#inputs).
- Widen `classify()` in `lib/coordinator/attention.ts` from `pending` only to
  every open status (`pending`, `approving`, `failed`), so approving and failed
  cards stay on Needs you (`AC-COORDINATOR-NEEDS-YOU-001.1`); the Needs-you
  `ProposalCard` reads its full row from `use-proposals.ts` by item id, and
  `AttentionProposal` keeps its narrow shape, per
  [Client store](../../specs/coordinator/system-design/proposal-cards.md#client-store)
  and [Cards](../../specs/coordinator/system-design/proposal-cards.md#cards).
- `lib/coordinator/eligible-step.ts`: a TypeScript copy of the eligible-step
  walk for the Edit form's step list; the approve route stays authoritative.
- `ProposalCard` states (UI-03), Edit and Reject forms in place with focus
  handling, the chat card renderer for `propose_task_kandev` with navigation
  to the Needs-you form, decision toasts; six locales. One `failed` variant,
  keeping Approve, Edit and Reject with the "Could not create the task:
  <error>. Nothing was created." copy (`AC-COORDINATOR-PROPOSALS-005.3`).
  Per [Cards](../../specs/coordinator/system-design/proposal-cards.md#cards): the
  content-by-state table (including the no-error and no-reason copy and which
  surface shows "Proposed by"), `<card>`/`<step>` derivation and fallbacks,
  Edit form options, the per-card in-flight lock, the decision-outcome table
  (200 approved/rejected/failed, 400, 403, 404, 409, network), the
  `?proposal=<id>&form=edit|reject` navigation from the chat card, focus
  after a decision, the Edit form's option loading, read-failure and
  missing-current-value rules, and remote changes while a form is open.
- `docs/public/coordinator.md` through `/docs-maintainer`, including what the
  coordinator's agent can still do through its own tools; update
  `docs/public/feature-status.md` if the boundary changed.

## Out of scope

- Any backend change (task 07).
- Undo, other proposal classes, reply with a condition (later phases).
- The approved task's agent profile and executor (see the requirements' Out
  of scope; the approve path sets neither, so a later start relies on the
  workspace default agent profile). A follow-up decides the rule.

## ASCII UI preview

From [plan UI-03](plan.md#ui-03-proposal-card-states-needs-you-and-chat),
aligned with the content-by-state table of
[Cards](../../specs/coordinator/system-design/proposal-cards.md#cards), which
governs where they differ. Needs-you-only lines (description, "Proposed by",
policy) are left out.

```text
pending, can manage          approving                    failed
Pending Approval             Approval in progress.        Could not create the task:
<title>                      Edits are locked.            <error>. Nothing was created.
<workflow> · <step>          <title>                      <title>
[Approve] [Edit] [Reject]    <workflow> · <step>          <workflow> · <step>
                                                          [Approve] [Edit] [Reject]
approved (chat)              rejected (chat)
Approved: KAN-432            Rejected: <reason>
<title>                      <title>
<workflow> · <step>          <workflow> · <step>
```

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern):

- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](assets/p1-05-chat-create-task-proposal.png)
- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](assets/p1-01-needs-you.png)

Mockup scenario specs to port (in the workspace-coordinator analysis
mockup's `mockup/e2e/tests/`, outside this repository; see the plan's [Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)):

- `05-rule-on-a-proposal.spec.ts`: approve, edit and reject on both surfaces.
- `18-v21-copilot-anywhere.spec.ts`, "one write class": the proposal is the only write.

## Acceptance

- Ask to split a card: the proposal appears in the chat and on Needs you;
  approve in the chat; the item leaves; the task exists in its step with no
  agent started.
- Edit and Reject work in place with focus handling; a decision on either
  surface or in another browser updates the others after
  `coordinator.updated`, and a settled card never reverts.
- Phone layout at 390px passes; public docs describe the feature and its
  residual risk.

## Verification

```bash
cd apps/web && pnpm test -- hooks/domains/coordinator/use-proposals.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/proposals.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/mobile-proposals.spec.ts
node scripts/validate-public-docs.mjs
```

The `mobile-chrome` project matches on the `mobile-*.spec.ts` filename prefix
(`apps/web/e2e/playwright.config.ts`), not on project scope, so the 390px
assertions live in their own `mobile-proposals.spec.ts` file rather than a
rerun of `proposals.spec.ts` under a different project.

`use-proposals.test.ts` and `proposals.spec.ts` both cover the `failed`
variant: it renders Approve, Edit and Reject with the "Nothing was created"
copy. `use-proposals.test.ts` also
covers the reader case of `AC-COORDINATOR-PROPOSALS-005.1` ("for managers"):
`ProposalCard` given a `workspace.read`-scoped context renders the title,
workflow, step and "Proposed by" line with no Approve, Edit or Reject. The
reader assertion for this same card on Needs you is task 04's component test;
the copilot transcript surface needs no separate coverage of its own, because
`AC-COORDINATOR-COPILOT-004.1` already keeps the launcher, and so the popover
the chat card renders inside, unreachable to a reader.

Unit tests also pin the rules the design adds:

- `use-proposals.test.ts`: each of the five merge rules, including a late
  `pending` response after `approving` (kept) and an equal-`updated_at`
  settled row (taken); a failed pending read keeps the cache, sets `error`
  and skips backfill; a failed by-id read keeps the entry; a by-id 404
  evicts it; backfill skips settled entries.
- `use-coordinator-inputs.test.ts`: the proposals input comes from the store
  and the hook makes no `listProposals` call of its own.
- `attention.test.ts`: a `pending`, an `approving` and a `failed` proposal
  each classify as a proposal item; an `approved` and a `rejected` one do
  not; ordering and counts are otherwise unchanged.
- `eligible-step.test.ts`: the same cases as the Go `EligibleStep` table
  (start step, manual move off, `auto_start_agent` on enter, a feeder of an
  auto-start step directly and through a chain, unknown step).
- Component tests for `ProposalCard`: the content-by-state table (with null
  `error` and null `reject_reason`), "Proposed by" on Needs you only,
  `<card>` from `identifier` and the title fallback when `fetchTask` fails
  or the identifier is absent, `<step>` fallback to the id; the lock (a
  second click sends one request); every row of the decision-outcome table,
  including a 400 whose `field` is `step_id` (alert under Step, focus on
  Step) and a 400 from plain Approve opening the Edit form; empty title
  refused with no request; the chat card's tool-result parsing (error
  result or no `proposal_id` renders no card), loading placeholder and 404
  copy; the Needs-you card rendering `final_spec` and `error` from the store
  row, not the classified item.
- Edit form tests: fields disabled with "Loading options" while reads are in
  flight; a failed workflow or step read shows "Could not load options." with
  Try again and disables Approve with edits, and never the "No step" line; a
  failed repository read keeps Approve with edits enabled; a deleted current
  workflow or an ineligible current step opens that field empty with its note
  and Approve with edits disabled; for a deleted current workflow the
  snapshot read issued at opening is made once and its response, or its
  failure, is discarded (the step field shows neither steps nor "Could not
  load options."), whichever of it and the workflow list settles first; a
  stale snapshot response for a previously chosen workflow is discarded.
- Remote change tests: with a form open, a merge that keeps `pending` keeps
  the typed values; one that makes it `approving` closes the form and moves
  focus to the card heading; one that removes the item moves focus per the
  focus-after-decision rule and shows no toast.
- Needs-you page test: `?proposal=<id>&form=reject` opens that form with
  focus on Reason and clears the query; an id that is not an item shows the
  "no longer waiting" toast; focus after a decision goes to the next item's
  heading, else the previous, else the empty state's heading.

`proposals.spec.ts` also covers
the rest of REQ-COORDINATOR-PROPOSALS-005 directly: the `pending` card's
Approve/Edit/Reject set and copy (AC-005.1), the `approving` card's disabled,
action-less state (AC-005.2), Edit's in-place fields and empty-title refusal
plus Cancel restoring focus to Edit (AC-005.4), Reject's in-place reason field
plus Cancel restoring focus to Reject (AC-005.5), the chat card's Edit/Reject
navigating to and opening the same Needs-you forms (AC-005.6), the approve
toast and the reject toast "Rejected. Nothing was created", each followed by
the correct "Next: ..." count line (AC-005.7), and the chat card's settled
"Approved: <card>" / "Rejected: <reason>" text (AC-005.8).

## Likely files

- `apps/web/hooks/domains/coordinator/use-proposals.ts` and test
- `apps/web/app/coordinator/use-coordinator-inputs.ts` and its test
- `apps/web/lib/coordinator/attention.ts` and its test (open-status filter)
- `apps/web/lib/coordinator/eligible-step.ts` and test
- `apps/web/lib/coordinator/links.ts` (the `proposal`/`form` query)
- `apps/web/app/coordinator/proposal-card/`
- `apps/web/components/task/chat/` tool-call renderer for `propose_task_kandev`
- `apps/web/src/locales/*/`
- `apps/web/e2e/tests/coordinator/proposals.spec.ts`, `mobile-proposals.spec.ts`
- `docs/public/coordinator.md`

## Dependencies

- Task 06 (copilot transcript for the chat card; it depends on tasks 03, 04
  and 05).
- Task 07 (approve and reject routes).
- While G0 is open the branch starts from task 06's branch with task 07's
  reviewed branch merged in, and rebases onto main after each predecessor
  merges.

## Risks

- A settled status must never be replaced by an unsettled one from a late
  read; the store test pins the order.
