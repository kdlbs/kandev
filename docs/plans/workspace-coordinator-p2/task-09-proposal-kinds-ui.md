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
  - AC-COORDINATOR-PROPOSAL-KINDS-004.5
  - AC-COORDINATOR-PROPOSAL-KINDS-004.6
  - AC-COORDINATOR-PROPOSAL-KINDS-005.1
  - AC-COORDINATOR-PROPOSAL-KINDS-005.2
  - AC-COORDINATOR-PROPOSAL-KINDS-005.3
  - AC-COORDINATOR-PROPOSAL-KINDS-005.4
  - AC-COORDINATOR-PROPOSAL-KINDS-005.5
  - AC-COORDINATOR-PROPOSAL-KINDS-005.6
  - AC-COORDINATOR-PROPOSAL-KINDS-005.7
  - AC-COORDINATOR-STANDING-ORDERS-003.2
  - AC-COORDINATOR-STANDING-ORDERS-003.4
  - AC-COORDINATOR-STANDING-ORDERS-004.1
  - AC-COORDINATOR-STANDING-ORDERS-004.2
  - AC-COORDINATOR-STANDING-ORDERS-004.3
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
- Stall card Resume for managers by the client rule of the design (session
  exists, not `COMPLETED` or `CREATED`; no backend field), sending
  `buildResumeRequest` through `launchSession`, the seam the private
  `useManualResumeSession` uses, with in-flight, "Resuming" and inline-error
  states (`005.1`, `005.5`).
- Ready to merge row: Open the PR in a new tab (`pr_url` of the first
  `taskPRs` entry, `http(s)` only), "Or send it back with a note", Send it back
  with a trimmed 1 to 4000 code point note queued through `queueMessage`, only
  for managers and sessions that accept one (hidden otherwise), actions outside
  the row's task link, and the "always human" line (`005.2` to `005.7`).
- Card outcome copy for every failure code and approved variant, the missing
  target and unresolved step fallbacks, and unknown kinds hidden (`004.5`,
  `004.6`).
- A shared standing-orders read with retired orders for the Shaped by labels
  (`003.4`) and a page-level `RejectOfferHost` that owns the reject offer and
  its dialog (`004.3`).
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
  `e2e/tests/coordinator/proposal-kinds.spec.ts` (desktop) and `mobile-proposal-kinds.spec.ts` (phone; the `mobile-chrome` project matches `mobile-*` names only).

## Acceptance

- Each kind renders its card and approves through the phase-1 route.
- Stall Resume and Send it back never create a proposal or a log row.
- No control on any card, row or dialog merges a pull request.

## Verification

```bash
cd apps/web && pnpm test -- app/coordinator/components app/coordinator/proposal-card components/coordinators
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run e2e/tests/coordinator/proposal-kinds.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome e2e/tests/coordinator/mobile-proposal-kinds.spec.ts
```

E2E: the mock agent proposes a message; Edit, approve, and assert the task's
conversation holds the edited text; a move proposal approves and the task
changes step; a stalled task shows Resume and resumes; a PR-ready task sends
back a note; reject with a reason, choose Make it a standing order, save and
assert the order in Configure.

## Likely files

- `apps/web/app/coordinator/proposal-card/proposal-card.tsx` (there is no
  `proposal-details.tsx`), new `shaped-by.tsx`, `outcome-copy.ts` and
  `reject-offer-host.tsx` beside it, `app/coordinator/components/`
  `needs-you-item-card.tsx`, `queue-row.tsx`, `queue-group.tsx`, a new
  `send-it-back.tsx` there, and `hooks/domains/coordinator/use-standing-orders.ts`

## Dependencies

- Task 04 (kinds); task 05 (order add route for the offer).

## Risks

- Send it back and stall Resume call task and session routes directly; they
  must check the viewer is a manager on the client and rely on the server's
  existing authorisation.

## Round 4 decisions

Recorded here, not in the design, which is at its byte cap. Each is covered by
a unit test in `send-back-form.test.tsx`, `use-stall-resume.test.ts` or
`needs-you-item-actions.test.tsx`.

- **R4-01 Queue id scope.** The client queue id belongs to one Send it back
  form instance. It is reused only while the note equals the one last
  attempted, and it resets on success and on close. A deliberate re-send after
  either is a new send.
- **R4-02 Failure keeps the draft.** A failed send keeps the form open with the
  draft and a generic error. The form closes only with the row on success.
- **R4-03 Late responses.** Resume and Send it back fence late responses with a
  per-id sequence and tombstones (the pattern WP-5b's proposal store uses). A
  response for a superseded or pruned Resume is ignored. Send it back fences the
  session read by sequence and guards form state with the mounted flag; a send
  that succeeds after the form closed still toasts (the message was queued),
  and one that fails after it closed is silent.
- **R4-04 No raw transport text.** Errors show generic translated copy. A
  Resume timeout states that the outcome is unknown and does not silently
  re-enable. Send it back needs no such state: queueMessage reconciles an
  uncertain transport error by client_queue_id, and a retry of the same note
  reuses that id, so a retry cannot double-queue.
- **R4-05 Announce and focus.** AC 005.2 is conditioned on the phase-2 flag.
  The session read retries after a failure. Closing the form returns focus to
  the Send it back trigger, matching WP-5b's card and task 08's rows.

## E2E waiver

The stall Resume flow has no Playwright spec. A real `task.stalled` needs the
one-minute sweep to cross a 75 s threshold and land between one and two
thresholds (about 120 s per run, see `stall.spec.ts`), and a seeded
execution-less session cannot be resumed by a real agent process. The card's
Resume is covered by `use-stall-resume.test.ts` (lock, sequence and tombstones,
launch body dispositions, timeout outcome) and the card tests. Message Edit
then approve, Send it back, move approve, reject offer and the phone variants
of the message card and Ready to merge row are covered end to end.
