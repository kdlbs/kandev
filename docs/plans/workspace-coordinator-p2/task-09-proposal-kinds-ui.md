---
id: "09-proposal-kinds-ui"
title: "Proposal cards, stall Resume and Ready to merge"
status: pending
wave: 4
depends_on:
  - "04-proposal-kinds-backend"
  - "05-standing-orders-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-PROPOSAL-KINDS-004
  - REQ-COORDINATOR-PROPOSAL-KINDS-005
  - REQ-COORDINATOR-STANDING-ORDERS-003
  - REQ-COORDINATOR-STANDING-ORDERS-004
acceptance_criteria:
  - AC-COORDINATOR-PROPOSAL-KINDS-004.1
  - AC-COORDINATOR-PROPOSAL-KINDS-004.2
  - AC-COORDINATOR-PROPOSAL-KINDS-004.3
  - AC-COORDINATOR-PROPOSAL-KINDS-004.4
  - AC-COORDINATOR-PROPOSAL-KINDS-005.1
  - AC-COORDINATOR-PROPOSAL-KINDS-005.2
  - AC-COORDINATOR-PROPOSAL-KINDS-005.3
  - AC-COORDINATOR-PROPOSAL-KINDS-005.4
  - AC-COORDINATOR-STANDING-ORDERS-003.2
  - AC-COORDINATOR-STANDING-ORDERS-004.1
  - AC-COORDINATOR-STANDING-ORDERS-004.2
system_design:
  - ../../specs/coordinator/system-design/proposal-kinds.md
  - ../../specs/coordinator/system-design/standing-orders.md
---

# Task 09: Proposal Cards, Stall Resume and Ready to Merge (WP-8)

## Summary

Render resume, message and move proposals in Needs you, give stall cards a
direct Resume, give Ready to merge rows Open the PR and Send it back, show
Shaped by labels, and offer to turn a reject reason into a standing order.

## In scope

- Card per kind in `app/coordinator/proposal-card/proposal-card.tsx` (the phase-1 card; there is no `proposal-details.tsx`): titles, task link,
  rationale, "Policy: <action> requires approval", Shaped by labels (active
  or "a retired standing order", text on hover or tap), "Approving this
  starts an agent", Edit only for message text, the failed text for
  `outcome_unknown` (`004.1` to `004.4`, `STANDING-ORDERS-003.2`).
- The approve 409 `policy_denied` text on a card.
- Stall card Resume for managers when the session is resumable, using the
  existing session resume call (`useManualResumeSession`) (`005.1`).
- Ready to merge row: Open the PR in a new tab, "Or send it back with a
  note", Send it back with a 1 to 4000 character note delivered through the
  existing message route, only for managers and sessions that accept one,
  and the "always human" line (`005.2` to `005.4`).
- Reject with a reason shows the 10-second offer; Make it a standing order
  opens the add dialog with the reason and `source_proposal_id` (`004.1`,
  `004.2`).
- Six locales; phone layout of each card.

## Out of scope

- Server behaviour of approval (task 04).
- Merging from Kandev (never).

## ASCII UI preview

From [UI-07](plan.md#ui-07-proposal-kinds-stall-resume-ready-to-merge-needs-you-and-queue)
and [UI-04](plan.md#ui-04-standing-orders-section-and-the-reject-offer):

```text
Message KAN-409                       Policy: Message a task requires approval
  > Please rebase on main before continuing.   Shaped by: Standing order 2
  [Approve] [Edit] [Reject]
Move KAN-411 from Build to Review     Policy: Move a task requires approval
  Approving this starts an agent.
  [Approve] [Reject]
Stall card:     KAN-420 stopped 3 h ago ...          [Resume]  Ask about this
Ready to merge (Queue)   Merging a pull request is always human.
  KAN-402 Add SSO   PR #88 ready                     [Open the PR]
    Or send it back with a note                      [Send it back]
Toast: "Rejected. Keep the reason as a standing order?"  [Make it a standing order]
```

## Mockup screenshots and scenarios

- [`assets/p2-01-queue-what-it-did.png`](assets/p2-01-queue-what-it-did.png)
  (Ready to merge).
- Scenario `05-rule-on-a-proposal` (reject with a reason) maps to
  `e2e/tests/coordinator/proposal-kinds.spec.ts`.

## Acceptance

- Each kind renders its card and approves through the phase-1 route.
- Stall Resume and Send it back never create a proposal or a log row.
- No control on any card, row or dialog merges a pull request.

## Verification

```bash
cd apps/web && pnpm test -- app/coordinator/components
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run e2e/tests/coordinator/proposal-kinds.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/coordinator/proposal-kinds.spec.ts
```

E2E: the mock agent proposes a message; Edit, approve, and assert the task's
conversation holds the edited text; a move proposal approves and the task
changes step; a stalled task shows Resume and resumes; a PR-ready task sends
back a note; reject with a reason, choose Make it a standing order, save and
assert the order in Configure.

## Likely files

- `apps/web/app/coordinator/components/proposal-details.tsx`,
  `needs-you-item-card.tsx`, `queue-row.tsx`, `queue-group.tsx`,
  `shaped-by.tsx`, `send-it-back.tsx`, `reject-offer.tsx`

## Dependencies

- Task 04 (kinds); task 05 (order add route for the offer).

## Risks

- Send it back and stall Resume call task and session routes directly; they
  must check the viewer is a manager on the client and rely on the server's
  existing authorisation.
