---
created: 2026-10-20
status: completed
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
legacy_specs: []
---

# Implementation plan: recover interrupted native startup

## Outcome

Allow native restoration to use its bounded startup budget. Settle failed restore
candidates so later recovery can proceed safely. Preserve the saved conversation,
workspace, and uncertain historical prompt without replaying that prompt.

This is a repair of existing requirements, not a new recovery contract.
The user authorized implementation of the single [work order](task-01-restore-startup.md)
after reviewing the diagnosis. Implementation and focused verification are recorded below.

## Evidence

Read-only investigation on 2026-10-20 identified two affected sessions:

| Task | Session | Observed state |
| --- | --- | --- |
| Preserve task drafts when switching tabs | `c8959e05-be1b-4390-9e9b-37328cdadf20` | CANCELLED after an API stop |
| Reproduce missing message comment icon | `d50a381e-a516-455d-a9e9-9adeeb2d1938` | WAITING_FOR_INPUT |

Both retain generation 1, a saved native conversation ID, an existing worktree,
an open `unresolved_durable_work` block, and a pending `silent_restart_restore`
attempt at `candidate_launching`. Saved identity does not prove the provider can
load its native state.

The running backend reports commit `76954ff53ea20b0263dd15c01c7149a531668341`.
The diagnostic manifest is complete for its requested backend source, with no
reported warnings. The retained log `backend-logs-2026-10-19-000001.log` records:

- Comment-icon restore: initialization at 18:43:03.954, ownership timeout at
  18:43:33.973, then native restore failure.
- Draft restore: initialization at 18:44:03.857, ownership timeout at
  18:44:33.876, then native restore failure.
- Both failures include `durable agent event ownership check: context deadline exceeded`
  and `load session request failed: agent stream not connected while waiting for response`.
- At 19:09, orphan reconciliation changed both sessions from STARTING to
  WAITING_FOR_INPUT. Their restore checkpoints remained pending.
- On October 20 at 10:08:39, draft workspace restoration failed with
  `session recovery required: session recovery is unresolved`.

Times above use the logs' UTC+01:00 offset. No live recovery, process stop,
database write, or prompt submission was performed during this investigation.

Further investigation read the provider's local `logs_2.sqlite` database in
read-only mode. The draft candidate's process log shows the following sequence
on October 19. Times in this table are UTC:

| Time | Provider event |
| --- | --- |
| 17:44:03.736 | Codex app-server startup |
| 17:44:08.742 | Model catalog request timed out |
| 17:44:13.854 | Plugin marketplace refresh failed: could not resolve `github.com` |
| 17:44:28.929 | Native `thread/resume` request started |
| 17:44:33.876 | Kandev cancelled the stream at its 30-second generation deadline |
| 17:44:33.964 | Model lookup inside `thread/resume` timed out |
| 17:44:38.974 | A second model lookup inside `thread/resume` timed out |
| 17:44:39.012 | Provider reached native session initialization |
| 17:44:44.031 | Subsequent `model/list` request timed out |

The provider continued processing after Kandev disconnected. Installed
`codex-acp` 2.2.2 awaits native resume and then fetches available models before
returning its ACP resume result. These network delays exceed the event deadline
without exhausting the enclosing restore budget. Earlier logs for the comment
candidate are incomplete, but retained logs show the same late model-list timeout.
DNS lookup for GitHub and ChatGPT succeeded during the October 20 investigation;
that does not repair the pending recovery checkpoints.

Neither affected candidate has an `executors_running` row, queried by session
and by candidate execution ID. Both checkpoints remain `candidate_launching`,
without persisted candidate-death evidence.

## Cause and limits

Before this fix, `awaitSilentRestoreGeneration` waited at most 30 seconds for the successor
generation. The orchestrator commits that generation only after native startup
succeeds. An early setup event therefore starts a deadline that can expire while
the native load request is still pending. Processor failure cancels the stream,
which releases that request with a disconnected-stream error.

The caller allows two minutes for a silent restore operation. The independent
30-second event deadline can terminate that operation early. The native startup
error path returns ownership of cleanup to its caller. The observed attempts
never advanced beyond `candidate_launching`, and later passes only inspect such
a candidate for readiness or proven process death.

The provider logs establish network failures and repeated model-catalog timeouts
during startup. They do not attribute every second of the initial 25-second delay
before `thread/resume`, or prove that every failed endpoint had the same DNS cause.
Control responses have independent stream processing; a response/event deadlock
is not established. The regression must model slow load and early setup events.

The failure return chain is also incomplete: the executor returns native startup
failure to the continuation owner; the continuation-owned resume path returns it;
`startSilentRestoreCandidate` then returns it without settling the launched
checkpoint. Later `silentRestoreCandidateReady` returns not-ready without an error
when its execution row is absent. That leaves the attempt pending. The retained
logs do not identify which operation removed the candidate rows. Missing rows
must not be treated as proof that candidate processes stopped.

## Technical approach

1. Give the pending successor-generation wait the restore operation's lifecycle.
   Preserve the existing overall deadline, cancellation, bounded buffering,
   committed-generation checks, and exact execution identity. Do not replace
   the fixed timeout with an unbounded wait or permit projection before commit.
2. On definitive startup failure, settle the exact owned candidate through the
   existing cleanup and checkpoint machinery. Preserve evidence across a crash.
   Persist exact candidate-death proof before discarding execution inventory.
   Reuse a verified ready candidate. Rotate only after exact death proof.
   Keep active, foreign, or unverifiable candidates blocked with a specific cause.
3. Exercise slow success, failure, cancellation, restart, and a healthy sibling.
   Verify ordinary new-prompt admission only after recovery commits.

Existing requirements and designs remain authoritative:
[requirements](../../specs/platform/requirements/durable-restart-reconciliation.md),
[restart design](../../specs/platform/system-design/durable-restart-reconciliation.md),
[stream design](../../specs/platform/system-design/durable-agent-stream-processing.md),
and [ADR on recovery convergence](../../decisions/2026-10-12-total-session-recovery.md).
No new storage schema, recovery action, or rendered UI is proposed.

| Path | Required behavior | Evidence |
| --- | --- | --- |
| Codex ACP native restore over durable v1 | Early setup and slow native load can complete within the operation budget | Coupled lifecycle/stream regression and browser restart scenario |
| Other eligible native restore over durable v1 | Preserve exact identity and deadline rules | Provider-neutral lifecycle fixtures |
| Foreign generation, unavailable native state, or unknown owner | Remain blocked without prompt replay | Negative lifecycle and orchestrator tests |
| Legacy transport and explicit fresh start | Preserve their existing admission behavior | Existing focused regression suites |

The implementation base was refreshed to main `e29a5b36448892d12b9eac9cb75d837007fb55f4`.
It includes the successor-generation handling from PR #4523. The fix preserves
that handling for fresh starts.

## Tests and traceability

- AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1, .2, .3, .5:
  coupled startup/stream tests and failed-candidate restart tests.
- AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4:
  desktop and phone recovery, reload, and a distinct follow-up instruction.
- AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.5:
  timeout, cancellation, conflicting ownership, and bounded failure tests.

Extend the existing desktop and phone agent-survival restart specs. Use a
disposable mock native-load delay after an early setup event. Do not connect
tests to the affected live tasks. The work order owns exact commands.

## Work orders

- [x] [Task 01: Restore startup and failed-candidate convergence](task-01-restore-startup.md)

## Risks and exclusions

Changing timeout ownership without cancellation propagation can leak workers.
Cleaning up by session ID alone can stop a newer execution. Advancing a
generation before native readiness can admit events without authority.

This package excludes direct database repair, deploying or restarting the live
instance, replacing conversations, and automatically replaying interrupted work.
Earlier delivery packages retain their historical results. Their results do not
cover this combined slow-load and successor-generation scenario.

## Implementation results

The pending-generation wait follows the launch owner's cancellation and keeps a
two-minute ceiling. A final bounded SQL read handles the race where the owner
commits and returns while a generation read is in flight. Receipt and projection
still require committed authority.

A failed silent restore owns candidate cleanup, including cancellation. Cleanup
checks the execution ID and inventory timestamp under the session operation
slot. It confirms runtime termination, checkpoints `candidate_dead`, then removes
inventory. Failed checkpoint writes retain inventory. A concurrent workspace
operation cannot substitute its result for cleanup. A verified ready candidate
remains available for commit retry.

The original live tasks remain outside this implementation. Their missing
execution rows do not provide termination proof. The existing regression that
keeps such candidates blocked remains unchanged.

## Verification results

Final results are recorded in the [work order](task-01-restore-startup.md#results).
The regression tests reproduced the 30-second disconnect, stranded checkpoint,
cancellation cleanup, ready-candidate cleanup, concurrent workspace admission,
and cancellation during the commit read before their respective fixes.

The backend diagnostic bundle remains in the ignored `.kandev/diagnostics/`
directory. Browser checks use disposable instances and mock provider data.
No live recovery, prompt submission, database repair, or deployment was performed.
