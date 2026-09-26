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
  [proposals design](../../specs/coordinator/system-design/proposals.md#client-store).
  The chat transcript's `ProposalCard` fetches its own `proposal_id` by id on
  mount and on every `coordinator.updated`, independent of the pending list.
- `ProposalCard` states (UI-03), Edit and Reject forms in place with focus
  handling, the chat card renderer for `propose_task_kandev` with navigation
  to the Needs-you form, decision toasts; six locales.
- `docs/public/coordinator.md` through `/docs-maintainer`, including what the
  coordinator's agent can still do through its own tools; update
  `docs/public/feature-status.md` if the boundary changed.

## Out of scope

- Any backend change (task 07).
- Undo, other proposal classes, reply with a condition (later phases).

## ASCII UI preview

From [plan UI-03](plan.md#ui-03-proposal-card-states-needs-you-and-chat):

```text
pending, can manage       approving                 failed
! create_task             ! create_task             ! Pending Approval
  Pending Approval          Approval in progress      Could not create the task:
  <title, workflow, step>   Edits are locked.         <error>. Nothing was created.
  [Approve] [Edit] [Reject]                           [Approve] [Edit] [Reject]
approved (chat)           rejected (chat)
v Approved: KAN-432       x Rejected: <reason>
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

## Likely files

- `apps/web/hooks/domains/coordinator/use-proposals.ts` and test
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
