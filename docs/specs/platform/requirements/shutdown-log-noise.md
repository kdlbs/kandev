---
status: draft
system: platform
created: 2026-08-11
owners:
  - cfl12
---
# Quiet benign teardown log noise on shutdown Requirements

## Overview

Pressing Ctrl+C on the Kandev backend emits ERROR-level log lines with stack traces (and a go-plugin library ERROR) even though every one of them is a benign teardown race: an in-flight prompt or WS launch request losing its context or subprocess peer while the process is deliberately shutting down. Operators reading the logs cannot tell a real fault from expected shutdown, and the stack traces suggest a crash where none occurred.

## Requirements

### REQ-PLATFORM-SHUTDOWN-LOG-NOISE-001: Quiet benign teardown log noise on shutdown

**Intent:** Pressing Ctrl+C on the Kandev backend emits ERROR-level log lines with stack traces (and a go-plugin library ERROR) even though every one of them is a benign teardown race: an in-flight prompt or WS launch request losing its context or subprocess peer while the process is deliberately shutting down. Operators reading the logs cannot tell a real fault from expected shutdown, and the stack traces suggest a crash where none occurred.

#### Acceptance criteria

- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.1:** The backend SHALL log a benign teardown race at WARN (no stack trace), not ERROR, when it is caused by root-context cancellation or subprocess termination during graceful shutdown.
- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.2:** A genuine fault on a still-active session (for example a real startup deadline, an agent-reported error unrelated to disconnect) SHALL keep logging at ERROR with its stack trace. Severity classification must not swallow real failures.
- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.3:** The go-plugin subprocess exiting because Kandev killed it during shutdown SHALL NOT surface as a library ERROR (`plugin process exited ... signal: terminated`).
- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.4:** On platforms where the supervised root can be signaled without orphaning a wrapper's descendants, the launcher SHALL deliver the first graceful-shutdown signal only to that root. This gives the backend and its owned runtimes, including plugins, an opportunity to run their cleanup paths before forced descendant cleanup. For the Unix dev wrapper, the launcher SHALL target the backend PID written by the backend binary and validate that it remains in the wrapper's process group. If that target is unavailable, or root-only signaling is unsafe for a platform's wrapper process, the launcher SHALL retain its tree-wide graceful fallback so descendants are not orphaned. On platforms with process-group support, the complete shutdown MUST still terminate the supervised process tree, including descendants left after the root exits. Platforms without portable process-group support only guarantee cleanup through their available supervised-root operation.
- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.5:** No change to backend/plugin lifecycle control flow, task/session state transitions, returned errors, or WS responses. Launcher sequencing changes only at the graceful-signal versus forced-cleanup boundary, so the complete supervised process tree still terminates on platforms with process-group support.
- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.6:** **GIVEN** an initial prompt in flight and the agent subprocess is terminated during shutdown, **WHEN** `waitForPromptDone`/`dispatchInitialPrompt` observe `peer disconnected before response`, **THEN** the backend logs WARN (no stack trace) and does not log ERROR.
- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.7:** **GIVEN** a real agent-reported error that is not a transport death on an active session, **WHEN** the prompt completes with that error, **THEN** the backend still logs ERROR with a stack trace.
- **AC-PLATFORM-SHUTDOWN-LOG-NOISE-001.8:** **GIVEN** a `session.launch` with `intent=resume` whose MCP resolve failed with `context canceled` during shutdown, **WHEN** `wsLaunchSession` returns the error, **THEN** it is logged WARN, not ERROR.

## Migrated source detail

## Why
Pressing Ctrl+C on the Kandev backend emits ERROR-level log lines with stack
traces (and a go-plugin library ERROR) even though every one of them is a
benign teardown race: an in-flight prompt or WS launch request losing its
context or subprocess peer while the process is deliberately shutting down.
Operators reading the logs cannot tell a real fault from expected shutdown, and
the stack traces suggest a crash where none occurred.

## What
- The backend SHALL log a benign teardown race at WARN (no stack trace), not
  ERROR, when it is caused by root-context cancellation or subprocess
  termination during graceful shutdown.
- A genuine fault on a still-active session (for example a real startup
  deadline, an agent-reported error unrelated to disconnect) SHALL keep logging
  at ERROR with its stack trace. Severity classification must not swallow real
  failures.
- The go-plugin subprocess exiting because Kandev killed it during shutdown
  SHALL NOT surface as a library ERROR (`plugin process exited ... signal:
  terminated`).
- On platforms where the supervised root can be signaled without orphaning a
  wrapper's descendants, the launcher SHALL deliver the first
  graceful-shutdown signal only to that root. This gives the backend and its
  owned runtimes, including plugins, an opportunity to run their cleanup paths
  before forced descendant cleanup. For the Unix dev wrapper, the launcher
  SHALL target the backend PID written by the backend binary and validate that
  it remains in the wrapper's process group. If that target is unavailable, or
  root-only signaling is unsafe for a platform's wrapper process, the launcher
  SHALL retain its tree-wide graceful fallback so descendants are not orphaned.
  On platforms with process-group support, the complete shutdown MUST still
  terminate the supervised process tree, including descendants left after the
  root exits. Platforms without portable process-group support only guarantee
  cleanup through their available supervised-root operation.
- No change to backend/plugin lifecycle control flow, task/session state
  transitions, returned errors, or WS responses. Launcher sequencing changes
  only at the graceful-signal versus forced-cleanup boundary, so the complete
  supervised process tree still terminates on platforms with process-group
  support.

## Affected log sites and target behavior
Classification reuses existing helpers/sentinels wherever possible.

1. `internal/agent/runtime/lifecycle/session.go` "prompt completed with error"
   (~L919) and "initial prompt failed" (~L835), currently ERROR. During
   shutdown the error is the ACP transport JSON `-32603 ... peer disconnected
   before response`. Log WARN when `isTransportDeadErr(err)` /
   `isCancelReleaseError` matches or `sm.stopCh` is already closed; otherwise
   keep ERROR.
2. `internal/orchestrator/handlers/handlers.go` "failed to launch session"
   (~L91), currently ERROR. Log WARN when the launch failed for a benign
   teardown reason: `intent=resume` yields a stringified
   `... session failed: ... context canceled` (the sentinel is lost at
   `task_operations.go:2229` `%s`), and `intent=restore_workspace` yields an
   error that wraps `lifecycle.ErrSessionTerminal` (`manager_launch.go:1397`
   `%w`). The classifier must handle BOTH the wrapped-sentinel case
   (`errors.Is`) and the stringified-cancel case.
3. `internal/plugins/runtime/manager.go` (`hcplugin.NewClient`, L394). go-plugin
   is constructed with no `Logger`, so its default hclog logger prints the
   process exit at ERROR when `client.Kill()` runs during `StopAll`. Supply an
   hclog logger routed through Kandev's logger and downgrade the deliberate
   kill so an expected `signal: terminated` is not an ERROR. The launcher must
   signal the supervised backend root first; otherwise a process-group
   SIGTERM reaches the plugin before this intentional-stop marker is set.
4. `internal/launcher/process.go` graceful child shutdown. On platforms with a
   safe root-only signal, the first signal SHALL target only the supervised
   root process. The Unix dev wrapper SHALL use the backend PID handoff for
   this target. If the root does not exit within the existing grace period, or
   a second interrupt arrives, the launcher SHALL use process-group force
   cleanup to reap the complete tree, including descendants after root exit.
   Platforms with an unsafe wrapper process SHALL use the existing tree-wide
   graceful fallback.

## Log sites reviewed and deliberately left unchanged
- `internal/agent/runtime/lifecycle/manager_events.go:544` "agent updates
  stream disconnected" (WARN, no stack) already correct.
- `internal/agent/runtime/agentctl/launcher/launcher.go:465` relays agentctl
  child **stderr** at WARN by design ("logged at WARN for visibility"); the
  `connection closed component=acp-conn` line is the child's own INFO output.
  Left as-is to preserve visibility.
- `internal/agent/hostutility/manager.go:244` "failed to delete host utility
  instance" (WARN). A `404 not found` during teardown means the instance is
  already gone; optionally downgrade only the not-found case to DEBUG. Non-404
  failures stay WARN.

## Failure modes
- Classification is conservative: if an error is not recognized as a teardown
  race it stays ERROR. A false negative (real error logged as WARN) must be
  impossible for the deadline/real-fault cases, which is why `context.Canceled`
  is treated as benign but `context.DeadlineExceeded` on an active session is
  not (matching the existing `handleAgentProcessStartFailure` convention).

## Scenarios
- **GIVEN** an initial prompt in flight and the agent subprocess is terminated
  during shutdown, **WHEN** `waitForPromptDone`/`dispatchInitialPrompt` observe
  `peer disconnected before response`, **THEN** the backend logs WARN (no stack
  trace) and does not log ERROR.
- **GIVEN** a real agent-reported error that is not a transport death on an
  active session, **WHEN** the prompt completes with that error, **THEN** the
  backend still logs ERROR with a stack trace.
- **GIVEN** a `session.launch` with `intent=resume` whose MCP resolve failed
  with `context canceled` during shutdown, **WHEN** `wsLaunchSession` returns
  the error, **THEN** it is logged WARN, not ERROR.
- **GIVEN** a `session.launch` with `intent=restore_workspace` on a session that
  is FAILED/terminal, **WHEN** `wsLaunchSession` returns an error wrapping
  `ErrSessionTerminal`, **THEN** it is logged WARN, not ERROR.
- **GIVEN** a `session.launch` that fails for a non-teardown reason (e.g. unknown
  task, validation), **WHEN** `wsLaunchSession` returns the error, **THEN** it
  is still logged ERROR.
- **GIVEN** the plugin manager `StopAll` kills a running plugin during shutdown,
  **WHEN** the subprocess exits with `signal: terminated`, **THEN** no ERROR log
  line for that expected exit is emitted.
- **GIVEN** a supervised backend has plugin subprocesses in the same process
  group, **WHEN** the launcher receives the first Ctrl+C, **THEN** it signals
  the backend root first on platforms with safe root-only signaling, using the
  backend PID handoff when the managed process is a Unix dev wrapper. The
  backend can invoke plugin cleanup, and the plugin subprocesses are not
  directly terminated by the launcher's initial graceful signal. Platforms
  with an unsafe wrapper use the existing tree-wide graceful fallback.
- **GIVEN** the supervised root ignores the graceful signal or a second Ctrl+C
  arrives, **WHEN** the launcher's grace/force path runs, **THEN** the existing
  process-group force cleanup terminates the root and all descendants, even if
  the root exits before a descendant does.

## Out of scope
- Changing the existing grace duration or second-signal force-kill policy.
- Changing which processes the final forced cleanup terminates; on platforms
  with process-group support, the complete supervised process tree remains
  covered by process-group force cleanup. Unsupported platforms retain their
  existing root-only limitation.
- Changing WS responses or task/session state on launch failure.
- Suppressing agentctl child stderr relay during shutdown.
- Any non-shutdown logging severity review.
