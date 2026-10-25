---
created: 2026-09-15
status: complete
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
legacy_specs: []
---

# Implementation Plan: Interrupt running FIFO turns with Send Now

## Overview

Allow a new Send Now request to replace an ordinary FIFO-delivered turn after
its prompt handoff completes. First implement the shared ownership transition
with deterministic backend regressions. Then verify the desktop/phone path,
real transport errors, and public guidance. Implementation is complete.

The user accepted this contract change on 2026-09-15. It amends
AC-UI-MESSAGE-QUEUE-SEND-NOW-001.8 and adds .9-.11. The existing
[replacement-turn ADR](../../decisions/2026-08-05-queue-send-now-replaces-turn.md)
remains authoritative for cancellation and workflow behavior.

## Evidence and reproduction

The reporter's Darwin build, commit `18d38c11a`, launched Cursor ACP. An earlier
queued input was dispatched at 10:32:15 +0100. A second input was queued at
10:35:06. Five Send Now attempts from 10:35:11 through 10:35:38 returned
`Another cancellation or Send Now operation is in progress`. Cursor continued
producing output. No dynamic-profile selection failure was observed.

The source retained ordinary FIFO reservations in `queuedDispatchAccepted`
until settlement because the live transition was gated to Send Now. The
accepted phase triggered `ErrSendNowConflict`. This implemented the former
whole-turn exclusion in criterion .8; the repair now applies the same guarded
accepted-to-live transition to ordinary FIFO.

Existing `TestSendQueuedNowConflictsAfterFIFOHandoffAccepted` passes and proves
the provider-independent rejection through a real service FIFO drain. It does
not replay the reporter's entire Cursor process. The bundle does not identify
which conflict predicate fired, so incident attribution is strongly supported
rather than a directly logged guard branch.

## Scope

### In scope

- Shared accepted-to-live transition for FIFO and Send Now reservations.
- Handoff, cancellation, successor, and late-completion regression coverage.
- Fixed and persisted dynamic-to-Cursor session attribution in service tests.
- Desktop and phone reproduction through the existing queue panel.
- Real `WebSocketRequestError` regression for Send Now conflict feedback.
- Clarify queue steering in the existing public how-to sections.

### Out of scope

- Provider adapters, dynamic routing policy, schema or protocol changes.
- Queue UI redesign, new copy, bulk header controls, or retry automation.
- Changes to explicit Cancel, queue editing, merging, or persistence policy.

## Technical approach

Follow the [paired design](../../specs/ui/system-design/message-queue-send-now.md).
Keep the accepted ownership record while promoting its phase at the existing
successful guarded prompt-claim boundary. Remove the Send Now-only eligibility
condition. Do not clear ownership to make the conflict disappear.

Rewrite the old whole-turn conflict test as two distinct cases: conflict before
handoff completion and successful interruption after claim ownership. Preserve
existing pending-FIFO supersession and live Send Now successor tests.

Current `queue-api.ts` already handles `WebSocketRequestError`. Verify this with
a Send Now-specific test; do not backport the reporter's old client code or
add redundant production mapping.

## ASCII UI preview

UI-01: Existing expanded queue while FIFO input A is running.
Shared composition; this package changes the action result, not layout.

```text
[Agent is working on A]
Queue   [Auto-run: ON] [Auto-merge: OFF] [Clear all]
  B: urgent correction       [Send Now] [row actions]
  C: later work              [Send Now] [row actions]
[Composer]

After Send Now on B:
[Cancel pending] -> [Agent is working on B]
Queue   [Auto-run: ON] [Auto-merge: OFF] [Clear all]
  C: later work              [Send Now] [row actions]
[Composer]
```

Desktop exposes row actions on hover/focus. Phone exposes touch actions
without hover, with at least 44px targets. Reuse the shipped
`mobile-message-queue-management.spec.ts` inline panel: one queue scroll owner,
visible composer, no horizontal overflow. A genuine handoff conflict shows the
existing localized conflict toast, preserves pending rows, and permits retry
after completion. Labels above are illustrative; existing translations and
control order remain authoritative. Maps to .1, .2, .9, .11.

## Tests

- .2/.7/.9: `TestSendQueuedNowCancelsLiveFIFOTurn`, table-driven for entry/all
  scope and fixed/dynamic attribution, in `queue_send_now_test.go`.
- .8: `TestSendQueuedNowConflictsAfterFIFOHandoffAccepted`, a barrier before successful
  guarded claim completion, in the same file.
- .10: `TestStreamCompletePreservesLiveFIFOSuccessorForStalePromptGeneration`
  plus the claim-to-provider-I/O half of `TestSendQueuedNowCancelsLiveFIFOTurn`
  in the same file; retain existing captured-turn, duplicate-cancel, workflow,
  and restoration cases.
- .9/.10: `TestSendQueuedNowSerializesFIFOPredispatchAdmission` holds ordinary
  FIFO A after guarded claim completion and before provider admission, races
  Send Now B, and verifies that B waits for A's bounded admission, replaces it
  exactly once, preserves C, and survives late A completion without stale turn,
  ownership, or prompt-attempt mutation.
- Dispatch recovery: `TestPromptTaskReleasesDispatchGuardBeforeMissingExecutionRecovery`
  drives queued identity validation to `ErrExecutionNotFound` and verifies that
  fresh-launch recovery can reacquire the per-session guard without deadlock.
- Model switching: `TestPromptTaskKeepsQueuedDispatchGuardThroughModelSwitch`
  pauses provider model selection before prompt claim and verifies that the
  queued dispatch guard remains held across that I/O.
- .11: real-class conflict test in `queue-api.test.ts`.

Task 01 records the backend RED and GREEN results in its work order. Task 02
records the client, browser, and public-doc results after the backend change.

## E2E tests

- `message-queue.spec.ts`, chromium: ordinary FIFO starts A; wait for its
  distinct output, queue B/C, click B's Send Now, verify B once and C afterward.
- `mobile-message-queue-management.spec.ts`, mobile-chrome: the same scenario
  using tap, with visible touch controls and no horizontal overflow.
- Use `watchWs` before navigation/actions and API/agent-output evidence for
  lifecycle boundaries. Do not use elapsed sleeps to infer handoff completion.
- Mock provider scripts make the scenario deterministic. Real Cursor credentials
  are not required; service tests cover dynamic concrete-execution identity.

## Companion packages

The implemented [original package](../message-queue-send-now/plan.md) remains
historical delivery evidence. Its whole-turn exclusion is replaced by this
package and amended criterion .8. Existing queue-run and automation behavior is
unchanged. Do not relabel historical checks as proof of the new contract.

## Work orders

- [x] [Task 01: Make live FIFO turns interruptible](task-01-live-fifo.md)
- [x] [Task 02: Verify queue steering across clients](task-02-client-verification.md)

## Verification results

Diagnosis completed before implementation:

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run '^TestSendQueuedNowConflictsAfterFIFOHandoffAccepted$' -count=1)
```

PASS. This asserts the old rejection behavior, not the proposed fix.
Package validation passed:

- `python3 scripts/list-docs.py validate`: 272 decisions and 935 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Package Markdown links and acceptance IDs resolve to existing artifacts.
- `git diff --check`: passed.

Task 01 implementation results:

- `TestSendQueuedNowCancelsLiveFIFOTurn`: PASS for entry/all scope, fixed and
  dynamic-to-Cursor execution attribution.
- `TestSendQueuedNowConflictsAfterFIFOHandoffAccepted`: PASS with a prompt-claim
  barrier; the accepted handoff still fails closed.
- `TestStreamCompletePreservesLiveFIFOSuccessorForStalePromptGeneration`: PASS;
  a stale predecessor cannot settle the live FIFO successor.
- `TestSendQueuedNowSerializesFIFOPredispatchAdmission`: PASS; the ordinary
  FIFO cancellation guard remains held from live promotion through provider
  acceptance, then B replaces A exactly once, C remains queued, and a late A
  completion cannot mutate B's turn, ownership, or prompt-attempt record.
- `TestPromptTaskReleasesDispatchGuardBeforeMissingExecutionRecovery`: PASS;
  queued identity validation can enter missing-execution recovery only after
  releasing the transferred dispatch guard.
- `TestPromptTaskKeepsQueuedDispatchGuardThroughModelSwitch`: PASS; queued
  provider model selection remains inside the dispatch admission guard.
- `go test -tags fts5 -race ./internal/orchestrator -run
  'Test.*(SendNow|SendQueuedNow|QueuedDispatch|FIFOHandoff)' -count=1`: PASS.
- `git diff --check`: PASS.

Task 02 implementation results:

- `pnpm exec vitest run lib/api/domains/queue-api.test.ts`: 30 tests passed.
- Web typecheck, Prettier, and targeted ESLint passed.
- Chromium Send Now subset: 3 tests passed, including the FIFO interruption
  scenario.
- Mobile Chrome Send Now subset: 3 tests passed, including the FIFO
  interruption scenario and overflow check.
- Public-doc tests: 62 passed; public-doc validation covered 46 pages.
- Specification catalog validation covered 272 decisions and 935
  specifications; specification lint passed.
- `git diff --check`: passed.

## Risks

- Removing accepted ownership rather than changing phase can admit duplicate
  drains or let a late completion clear the replacement.
- The claim-to-provider-I/O window must remain serialized against cancellation.
- Existing tests intentionally assert the old contract and need a real handoff
  barrier; simply deleting their assertions would remove race protection.
- The reporter ran an older build. The generic-toast mapping is already repaired
  on this branch and needs a regression test, not another production fix.
