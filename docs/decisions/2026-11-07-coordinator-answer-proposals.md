# ADR-2026-11-07-coordinator-answer-proposals: The coordinator drafts answers, a manager approves

**Status:** proposed
**Date:** 2026-11-07
**Area:** backend, frontend, workflow

## Context

[ADR-2026-09-29-coordinator-phase-3-autonomy](2026-09-29-coordinator-phase-3-autonomy.md)
let a manager answer a watched card's clarification or permission request on
the Needs you card (work order 07, "answer in place"). The answer is always
given by a person: the coordinator's `message` proposal cannot resolve a
pending clarification, and no coordinator action resolves a permission.

The owner decided on 2026-11-07 that the coordinator should answer the
questions its cards ask. The choice was "anything, you approve": it drafts an
answer to every question, maintainer-level ones included, and the owner
approves or edits each draft.

## Decision

1. Add a proposal kind and policy class `answer`. The coordinator proposes a
   complete draft for the task's current answerable clarification bundle or
   pending permission request, with cited sources, a confidence and a
   `needs_decision` flag. Nothing is delivered until a person with
   `workspace.manage` approves.
2. Approval delivers through the same resolution operations the in-place
   answer uses (the clarification resolver and the permission response
   operation). No second path, no new validation, no new race handling.
3. Staleness is a refusal, not an error: an approval whose target identity
   changed, or that loses to an in-place answer, ends `refused (stale)` and
   delivers nothing. A superseded pending proposal is refused automatically
   by the same-transaction supersede on a newer proposal, or by the next
   approval attempt; a periodic check is deferred.
4. `answer` is never in `raisableClasses`. It is `requires_approval` by default
   (when the flag is effective) and can be set to `denied`. It becomes
   automatic only through a later decision.
5. The coordinator cannot choose a permission option that grants a lasting
   permission. A manager can still choose one in place.
6. The proposal status vocabulary does not change. A refused answer proposal
   (`stale`, `task_archived`) is a `rejected` proposal with no deciding person
   and its refusal code recorded as the error, plus a `refused` activity row.
   It must not be `failed`: a `failed` proposal counts as open (unique
   open-target index, the open cap, the pending list) and can be re-approved,
   which would block the superseding insert and let a stale answer be retried.
   `failed` stays for delivery errors that a manager may retry.
7. Pending answer proposals stay approvable after the class is set to `denied`;
   approval consults only the flag. A task keeps at most one open answer
   proposal, a new one for a different target is refused while the open one is
   still current.
8. An absent `answer` policy entry stays absent in storage and resolves to
   `requires_approval` only in the effective policy; the flag forces `denied`
   there.
9. Ship behind a new `features.coordinatorAnswer` flag, `false` in every shipped
   profile, `true` in e2e.

## Consequences

- The phase 3 position that the coordinator never answers for a person
  (the relay requirements' out-of-scope item, D5) is amended: a coordinator
  drafts, a person still decides. Delivery still requires a person. The phase 3
  rule that a failed proposal keeps Approve, Reject and Try again is amended
  for refused answer proposals, which offer no action.
- Sources that are not URLs are shown as text, not links, because no stable
  deep link to a file in a task workspace exists today.
- A task with a long-waiting question now has an open proposal beside its
  Needs you item; counts follow the existing derivation of each.
- Absent `answer` entries in stored policies read as `requires_approval` while
  the flag is effective, resolved only in the effective policy so a settings
  save or a class lowering never writes `denied` over absence. This is a default
  for a class that cannot act alone, not a grant of autonomy.
- Two additive exports in `internal/clarification` expose the resolver's
  existing validation; its behaviour does not change.

## Alternatives considered

- **Answer through `message`.** Rejected: a queued message cannot resolve a
  pending clarification or permission, and would race the in-place answer.
- **Add a `refused` proposal status.** Rejected: a new status touches every
  consumer of the record; `rejected` with a reserved error code plus the activity
  `refused` row carries the distinction.
- **Store refusals as `failed` with text `stale`.** Rejected: see decision 6.
- **Auto-refuse by a periodic sweep or event subscription.** Deferred: the
  on-propose supersede and the approve-time check are enough for the first
  release; a stale proposal stays pending until one of them runs and the card
  shows a live not-current notice.
- **Let `needs_decision` omit the draft.** Rejected: a partial draft would need
  a second approval shape; a best guess marked as such keeps one.
