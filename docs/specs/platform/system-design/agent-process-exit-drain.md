---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001
created: 2026-08-26
owners:
  - kandev
---
# Agent process exit and stderr drain System Design

## Purpose and boundaries

The agentctl process manager owns the child process, its stderr pipe, exit
status, and safe recent-stderr diagnostics. This design changes only the order
in which process exit and stderr completion are observed. It does not change
agent protocol handling, stderr sanitization, or process-group cleanup.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001` | [Exit flow](#exit-flow) and [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `internal/agentctl/server/process.Manager` starts the managed command and
  owns its lifecycle.
- `Manager.readStderr` drains and sanitizes the current process generation's
  stderr stream, appending only the safe projection to the ring buffer.
- `Manager.waitForExit` waits for the child process and then coordinates the
  generation-specific `stderrDone` channel before publishing exit diagnostics.
- The adapter update channel receives the existing exit error event and recent
  stderr fields.

## Data and contracts

The process manager keeps `processStderrDrainTimeout` as the upper bound for a
reader that does not finish after process exit. `stderrDone` remains local to a
process generation so a delayed reader from an old command cannot close a
replacement command's completion channel.

No HTTP, WebSocket, database, or agent protocol payload changes are required.

## Exit flow

1. Process startup creates the pipes and starts `readStderr` and `waitForExit`
   concurrently.
2. `readStderr` continues draining while the child is alive, so a child that
   runs longer than the drain timeout does not create a timeout warning merely
   because it has not exited.
3. `waitForExit` calls `cmd.Wait()` to observe the child exit.
4. After `cmd.Wait()` returns, `waitForExit` waits on the same generation's
   `stderrDone` channel for the bounded drain interval.
5. Exit classification, recent-stderr event construction, process-group reap,
   and final stopped status follow the existing paths.

## Failure and recovery

If the reader finishes after process exit, final exit diagnostics include the
same sanitized recent stderr. If the reader remains blocked, the manager emits
`timed out waiting for agent stderr to drain` and continues within the existing
bound instead of hanging process cleanup. That warning then indicates a real
post-exit pipe-drain delay.

An intentional stop still uses the existing intentional-stop classification.
Non-zero process exits still retain their existing error event and exit code.

## Observability

The stderr-drain warning remains at warning level, but its occurrence is now
limited to the post-exit bounded wait. Existing process start, exit, and recent
stderr log fields remain unchanged.
