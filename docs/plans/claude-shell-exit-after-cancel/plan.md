---
created: 2026-11-22
status: complete
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
system_design:
  - ../../specs/agents/system-design/claude-background-shell-terminal-evidence.md
legacy_specs: []
---

# Fix plan for Claude shell exits after turn cancellation

## Incident and reproduction

Kandev v0.102.0 installs Claude ACP 0.107.0. Its adapter only accepts the
detached shell's null-code exit from 0.97.0. PR #5405 qualifies 0.107.0 and
0.109.0 on `main`. A temporary replay of
`testdata/claude-0.107.0-detached-bash.jsonl` with two launches failed on
v0.102.0 and passed on `main` at `c68b3340d72`. Both runs exercised exit
delivery before and after prompt-end cleanup.
[Issue #5421](https://github.com/kdlbs/kandev/issues/5421),
[PR #5405](https://github.com/kdlbs/kandev/pull/5405).

A separate replay on the same `main` commit delivered an exit after chat
Stop. `Adapter.Cancel` sent `session/cancel` to a mock ACP peer and removed
both completed launch bindings. The peer received the notification.
The later matching exit produced a tool update without a workload
observation. The running row and its accounting entry depend on that
observation to settle. The qualified bridge retains the terminal on the
native session and processes task endings on its event stream.
[Bridge cancel handler](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.109.0/src/v1/acp-agent.ts#L7323-L7328),
[Bridge terminal handling](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.109.0/src/v1/background-work.ts).

## Scope

Keep completed detached Bash launches eligible for their own terminal
evidence after turn cancellation on the same Claude ACP connection.
Agents owns the launch and exit identity. The orchestrator and Background
Work panel consume the existing observations.

The repair uses the existing [workload requirements](../../specs/agents/requirements/background-work.md#req-agents-background-work-010-claude-background-workload-state)
and [terminal evidence design](../../specs/agents/system-design/claude-background-shell-terminal-evidence.md#exit-delivery-after-turn-cancellation).
Historical rows follow verified owner-exit recovery. Live compaction timing
remains unverified.

## Technical approach

Send `session/cancel` before waiting for notification processing. Then enqueue
a FIFO cleanup barrier behind ACP notifications already accepted by the
adapter. This preserves queue order when a completed detached launch is
waiting behind the notification worker. Bound the caller's wait; if the wait
expires, the cleanup remains queued and runs when the worker reaches the
barrier. When the qualified bridge confirms a detached launch, fill the
existing `BackgroundWorkPayload.WorkID` with the tool-call ID. The workload
already uses that ID through the orchestrator's fallback. The confirmed ID
lets the cancel sweep keep only those Claude shell entries in
`activeToolCalls`. A pending Bash request still follows the ordinary cancel
sweep; attach an explicit `ended` observation so projection cannot treat its
cancelled tool event as running.

Use the current native session and launch ID to match exits. The native
session binding boundary drops old shell entries on replacement. Qualified
Claude tool starts and updates from the outgoing native session are rejected
before payload inspection or mutation, including unclassified frames without
shell metadata. This prevents an old start from replacing a new binding that
reuses the same tool-call ID. The existing exit handler ends the matching job.
The existing projection clears its activity and records `ended` with no
exit code when the bridge supplies a null code.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| 010.3, 010.6 | `TestClaudeBackgroundTerminalExitAfterTurnCancel` queues a completed launch behind a blocked notification worker before `Adapter.Cancel`. It verifies `session/cancel` is sent before a bounded barrier wait, exact-ID settlement after cancellation, a live sibling, and cancellation of pending background Bash and foreground tools. The cancelled Bash workload has an explicit `ended` observation. |
| 002.2 | `TestClaudeBackgroundTerminalBindingEndsOnNativeSessionReplacement` reuses the tool-call ID in the new native session, rejects an unclassified late old-session start plus a late launch result and exit, then accepts the new session's matching exit. |
| 002.1, 002.2, 010.5, 010.6 | Reuse `TestClaudeBackgroundTerminalExitRetiresExactAccounting` and `TestClaudeBackgroundTerminalProjection`. They cover exact retirement, duplicate handling, stored state, and the original turn association. |

## E2E tests

The existing desktop and phone scenarios cover the Background Jobs count
and History after reload for AC-AGENTS-BACKGROUND-WORK-005.1 and 010.5.
The work order exercises the changed cancellation path through a mock ACP
connection. [Existing browser evidence](../claude-background-job-settlement/task-01-qualify-managed-shell-exits.md#results).

## Work orders

- [x] [Task 01. Retain Claude shell bindings through turn cancellation](task-01-retain-shell-bindings.md)

## Verification results

- The temporary 0.107.0 replay failed on v0.102.0 and passed on `c68b3340d72`.
- The temporary `TestIssue5421QualifiedExitAfterChatCancel` failed on
  `c68b3340d72` because the matching exit carried no workload observation.
- The permanent `TestClaudeBackgroundTerminalExitAfterTurnCancel` reproduced
  the missing observation before the code change, then passed with the fix. The
  regression now places the completed launch in the notification queue before
  Stop and verifies a pending background Bash request cannot leave live work.
- `TestClaudeBackgroundTerminalBindingEndsOnNativeSessionReplacement` verifies
  stale starts and launch results cannot restore an old shell binding after
  replacement, including when the new session reuses the call ID.
- The ACP race checks and orchestrator terminal-projection checks passed.
  They cover a live sibling, duplicate exits, original-turn association,
  exact activity retirement, and the reused-ID session replacement case.
- `bash scripts/lint-go-changed ...` reported 0 issues. The specification
  catalog validated 460 decisions and 2081 specifications, and all spec files
  passed lint.
- `make build` passed from `apps/backend`. The plan and work order record the
  remaining compaction timing limitation.
- Review fixup verified that a pending detached Bash cancellation projects as
  `ended`, and that an outgoing Claude session's unclassified start cannot
  overwrite a reused current binding.

Post-review verification on the updated adapter:

- The focused ACP regression command, the ACP race command, and the focused
  orchestrator projection command recorded in the work order all passed.
- `make build` passed from `apps/backend` after the final adapter edit. The
  platform Go builds succeeded; macOS signing tools were unavailable, so those
  artifacts were left unsigned.
- The full changed-Go command in the work order reported 0 issues.
- The docs catalog validated 460 decisions and 2081 specifications;
  specification lint, `gofmt -l`, and `git diff --check` passed.

## Risks

- Raw input can label a Bash call as background before its launch completes.
  Verify completed launch status before retaining its binding across Stop.
- A replaced native session can reuse a tool ID. Scope exit handling to the
  original owner and release its bindings at replacement.
- A process probe can miss reparented jobs. The shell's exact exit frame
  provides the evidence for this repair.
  [Orphaned workload design](../../specs/disambiguate-waiting/system-design/orphaned-background-workloads.md).
