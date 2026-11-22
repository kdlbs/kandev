---
id: "01-retain-shell-bindings"
title: "Retain Claude shell bindings through turn cancellation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-010
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-002.1
  - AC-AGENTS-BACKGROUND-WORK-002.2
  - AC-AGENTS-BACKGROUND-WORK-005.1
  - AC-AGENTS-BACKGROUND-WORK-010.3
  - AC-AGENTS-BACKGROUND-WORK-010.5
  - AC-AGENTS-BACKGROUND-WORK-010.6
system_design:
  - ../../specs/agents/system-design/claude-background-shell-terminal-evidence.md
---

# Task 01. Retain Claude shell bindings through turn cancellation

## Summary

Preserve a verified detached shell's launch binding when Stop races an
already queued launch update. Reject late qualified shell events from a
replaced native session before they can mutate a binding reused by the new
session. When the bridge reports the shell's exit, use the same event that
handles an exit after ordinary prompt completion.

## Scope

1. Add `TestClaudeBackgroundTerminalExitAfterTurnCancel` in
   `claude_background_terminal_test.go`. Start a detached Bash tool and a
   confirmed sibling. Queue the first launch's completion behind a blocked
   notification worker, then call `Adapter.Cancel` through a mock ACP
   connection. Send `session/cancel` before waiting for the queued cleanup
   barrier. Bound the wait; if it expires, keep the cleanup queued behind the
   completion. The pending background Bash request must be cancelled without
   leaving running work. Send a matching null-code exit and verify it settles
   by launch ID.
2. When the qualified bridge completes a detached launch, fill its existing
   workload ID with the launch tool-call ID under the tool-state lock. Drain
   already queued ACP notifications before the cancellation sweep. Keep that
   `activeToolCalls` entry through cancellation on the same native session.
   Use the existing terminal conversion to settle it. Drop old shell entries
   when the adapter installs a different native session, and reject all tool
   starts and updates from the outgoing qualified Claude session before they
   can mutate current bindings, even when a frame omits shell metadata.

## Acceptance

1. A completed launch survives `session/cancel` on the qualified bridge.
   Pending Bash and foreground tools still receive cancellation. The
   qualified bridge's exit sets `finished_at` and retains a null exit code.
   The cancelled pending Bash event includes a typed `ended` workload
   observation, so projection does not infer `completed` from the normalized
   payload's ended marker.
2. One exit affects its own shell. Match the native session and launch ID.
   Reuse the existing tests for unqualified bridge versions. Cover native
   session replacement, a reused call ID, and late stale starts/results in
   the adapter regression.
3. The existing orchestrator checks pass. They verify the durable row, the
   original turn, and retirement of the final activity entry.

## Verification

Run the first command for the expected red result. Run the complete block
after implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^TestClaudeBackgroundTerminalExitAfterTurnCancel$' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestClaudeBackgroundTerminalExitAfterTurnCancel|TestClaudeBackgroundTerminalBindingEndsOnNativeSessionReplacement|TestCancelPendingClaudeShellEmitsEndedWorkObservation)$' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestClaudeBackgroundTerminal|TestCancelActiveToolCalls|TestPromptEndSweepsPreserveHandoff)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^(TestClaudeBackgroundTerminal|TestCancelledPendingShellProjectsEndedState)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/{adapter_copilot_native.go,adapter_prompt.go,adapter_tools.go,adapter_updates.go,claude_background_terminal_test.go,dialect_claude_terminal.go} apps/backend/internal/orchestrator/claude_background_terminal_projection_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_copilot_native.go`
  contains the shared `installNativeBindingBarrier` for session changes.
- `adapter_prompt.go` owns the ACP cancel sweep. `adapter_tools.go` owns
  launch completion and terminal conversion. `dialect_claude_terminal.go`
  owns bridge qualification and retained shell binding checks.
- `claude_background_terminal_test.go` in the ACP package covers regression
  and bridge compatibility.

## Dependencies

PR #5405 is already on `main`. This is the only work order.

## Risks

Cancellation can race terminal delivery. Make the retain and retire decisions
under the same lock. A failed session replacement keeps the old native owner
and must leave its binding usable.

## Parallelism

`sequential`

## Inputs

- [Claude shell terminal evidence](../../specs/agents/system-design/claude-background-shell-terminal-evidence.md#exit-delivery-after-turn-cancellation)
- [Managed bridge qualification](../claude-background-job-settlement/task-01-qualify-managed-shell-exits.md)
- [Foreground and background ownership](../../decisions/0049-fine-grained-foreground-idle-busy-signal.md)

## Results

The permanent adapter regression failed before implementation at the expected
boundary: after `session/cancel`, the matching exit had no workload observation.
It passes with the exact launch binding preserved through Stop.

| Lifecycle scenario | Criteria | Invariant and evidence |
| --- | --- | --- |
| Queued launch, Stop, pending calls, and live sibling | 010.3, 010.6 | The regression holds the notification worker, queues a completed launch, and verifies `session/cancel` is sent before Stop waits for its bounded sweep barrier. Releasing the worker processes the launch before cleanup. The confirmed launch settles by exact ID with a null exit code. The confirmed sibling remains tracked. Pending background Bash and foreground calls are cancelled; the pending workload carries an explicit `ended` observation and is inactive. The ACP race command passed. |
| Duplicate exit and durable projection | 002.1, 002.2, 010.5, 010.6 | The adapter terminal tests cover duplicate evidence and preserve a live sibling. The orchestrator tests verify the durable `ended` row, original turn, and exact activity retirement. Both prescribed commands passed. |
| Native-session replacement and reused call ID | 002.2 | The adapter regression clears the old pending shell, binds the same tool ID in a new native session, rejects an unclassified late old-session start before it can overwrite that binding, rejects the old launch result and exit, then accepts the new owner's exit. The ACP race command passed. |
| Restart and historical-row recovery | 002.3 | Not changed: already-consumed frames cannot be replayed by this repair. The existing verified owner-exit recovery remains responsible for historical rows; no recovery or persistence code changed. |
| Pause, resume, and live compaction timing | 010.3 | The bridge exposes no shell pause/resume action on this path. Same-connection Stop and native-session replacement are covered. Live compaction timing remains unverified. |
| API reads and browser reload with terminal history | 005.1, 010.5 | Existing desktop and phone evidence covers count and History after reload: [browser results](../claude-background-job-settlement/task-01-qualify-managed-shell-exits.md#results). This work order does not change those read paths. |

After a verified owner exit, the existing recovery path handles open rows.
This work order tests delivery on the original connection.

### Results

- `go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestClaudeBackgroundTerminal|TestCancelActiveToolCalls|TestPromptEndSweepsPreserveHandoff)' -count=1` passed from `apps/backend`.
- `go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestClaudeBackgroundTerminal' -count=1` passed from `apps/backend`.
- `go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestClaudeBackgroundTerminalExitAfterTurnCancel|TestClaudeBackgroundTerminalBindingEndsOnNativeSessionReplacement)$' -count=1` passed from `apps/backend` after adding the queue-order and stale-session regressions.
- The stale-session regression rejects an old-session start without shell metadata after the tool-call ID is reused, then verifies the new session's matching exit still settles.
- `bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/{adapter_copilot_native.go,adapter_prompt.go,adapter_tools.go,claude_background_terminal_test.go,dialect_claude_terminal.go}` reported 0 issues.
- `python3 scripts/list-docs.py validate` validated 460 decisions and 2081 specifications; `python3 scripts/lint-spec-files.py --all` passed.
- `make build` passed from `apps/backend`.
- `git diff --check` passed after the final documentation update.

PR fixup regression coverage also verifies that cancellation attaches an
explicit `RunStateEnded` observation to a pending detached Bash tool update,
which keeps the orchestrator from projecting the cancelled request as running.

Post-review verification on the updated adapter:

- `go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestClaudeBackgroundTerminalExitAfterTurnCancel|TestClaudeBackgroundTerminalBindingEndsOnNativeSessionReplacement|TestCancelPendingClaudeShellEmitsEndedWorkObservation)$' -count=1` passed.
- `go test -race -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run '^(TestClaudeBackgroundTerminal|TestCancelActiveToolCalls|TestPromptEndSweepsPreserveHandoff)' -count=1` passed.
- `go test -trimpath -tags fts5 ./internal/orchestrator -run '^(TestClaudeBackgroundTerminal|TestCancelledPendingShellProjectsEndedState)$' -count=1` passed.
- `make build` passed from `apps/backend` after the final adapter edit. macOS signing tools were unavailable; the Go builds succeeded and the targets were left unsigned.
- `bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/{adapter_copilot_native.go,adapter_prompt.go,adapter_tools.go,adapter_updates.go,claude_background_terminal_test.go,dialect_claude_terminal.go} apps/backend/internal/orchestrator/claude_background_terminal_projection_test.go` reported 0 issues.
- `python3 scripts/list-docs.py validate` validated 460 decisions and 2081 specifications. `python3 scripts/lint-spec-files.py --all` passed.
- `gofmt -l` reported no changed Go files. `git diff --check` passed.
