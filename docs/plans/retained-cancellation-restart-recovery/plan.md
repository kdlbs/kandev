---
created: 2026-11-25
status: done
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
legacy_specs: []
---

# Retained cancellation recovery after restart

## Overview

Teach the existing startup probe to recognize a retained cancellation.
Replay that event, then resume the saved ACP conversation without an initial
prompt.

Platform owns this fix because journal-to-SQL reconciliation determines
whether the delivery block can close. The existing
[restart requirements](../../specs/platform/requirements/durable-restart-reconciliation.md)
cover the behavior. The repair follows the
[outcome and admission decision](../../decisions/2026-10-12-total-session-recovery.md).

## Confirmed defect

In [issue #5506](https://github.com/kdlbs/kandev/issues/5506), agentctl retained
the cancelled submission and event 1772. SQLite had received and projected
through 1771. The stopped Worktree session had a matching saved Copilot
conversation and an open `unknown_prompt_outcome` block. It had no
`agent_delivery_recovery` snapshot.

The bounded startup worker selects the session. The legacy native resume check
rejects the retained terminal outcome and the unprojected tail. The next
branch checks for historical ownership without the block's submission ID.
The journal omits cancelled submissions from that inventory. The worker
gets no control identity for the cancelled submission and never calls replay.

A disposable Go test ran the startup pass at `c671d8080883` with SQLite
projected through 1 and an acknowledged journal prefix through 1.
`journal.CancelSubmission` retained a terminal `complete` event at 2.
`TestReproStoppedRestartProjectsRetainedCancellation` failed at the cursor
assertion. SQLite stayed at 1. A separate test drove the retained runtime
replayer with the same cancellation format. It settled `cancelled` and
preserved a single projection across repeated recovery.

## Scope

The case is a stopped, resumable standalone Worktree session with a saved
native conversation, an exact submission block, and no recovery snapshot.
Copilot and other ACP providers that can load the saved conversation use the
same path.

### Out of scope

Clarification copy, pruned journal recovery, and remote executors.

## Technical approach

1. Add a cancellation case to `legacy_native_restart.go`. Reuse the
   stored-submission lookup and its identity and inventory checks. This case
   requires the block's exact submission ID and a retained terminal
   cancellation from the verified stopped owner.
2. Pause the queue and call `RecoverAgentPromptStreamWithIdentity`. Its cursor
   and replay checks govern the retained events. The existing terminal
   settlement path records the cancellation and clears the matching block.
3. Verify that settlement closed the matching block. Use the existing resume
   path with the saved native ID required and no initial prompt. If startup
   stops after settlement, ordinary resume can use that committed result.

## Tests

`TestAutomaticRestartReplaysRetainedCancellation` drives startup with a
retained cancellation beyond the SQL cursor. It checks settlement, repeat
recovery, and use of the same native conversation. An identity mismatch
keeps the block open. The work order maps the applicable acceptance criteria.

## E2E tests

Use the existing ordinary restart recovery case in
`tests/session/automatic-restart-recovery.spec.ts` (`chromium`) and
`tests/session/mobile-automatic-restart-recovery.spec.ts` (`mobile-chrome`).
These exercise the composer and a new instruction after backend startup.
The Go regression owns the retained cancellation and absent snapshot setup.

## Work orders

- [x] [Task 01 Replay the retained cancellation before resume](task-01-replay-retained-cancellation.md)

## Verification results

- The temporary startup test reproduced the lag. The temporary cancellation
  replay test and the existing retained terminal replay test passed.
- Existing legacy resume, terminal block publication, and startup eligibility
  tests passed.
- The specification catalog, specification lint, and documentation coverage
  preflight passed.
- Task 01 implementation passed the focused Go regression, the requested
  orchestrator and lifecycle race tests, and the backend build.
- The code review follow-up passed the retained-cancellation regression with
  coverage for the production empty-stream block, independent open block,
  capacity refusal, received-but-unprojected receipt, and queue-claim
  reconciliation. The orchestrator race group, retained-terminal lifecycle
  race test, backend build, spec catalog validation, spec lint, `gofmt`, and
  `git diff --check` passed after these repairs.
- Frozen-lockfile pnpm install passed. The desktop Chromium and mobile Chrome
  restart recovery E2E cases each passed.
- The changed-file lint wrapper could not start during the review follow-up
  because another worktree had a `golangci-lint` process running. An earlier
  10-minute `golangci-lint` run completed with zero issues before these final
  review repairs.

## Risks

- The reported shutdown ran on macOS. The synthetic tests isolate the persisted
  state on Linux. The fix depends on an authentic saved process binding.
