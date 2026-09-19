---
id: "02-agent-stderr-exit"
title: "Agent stderr exit sequencing"
status: done
wave: 2
depends_on:
  - "01-startup-version"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001
acceptance_criteria:
  - AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.1
  - AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.2
  - AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.3
  - AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.4
system_design:
  - ../../specs/platform/system-design/agent-process-exit-drain.md
---

# Task 02: Agent stderr exit sequencing

## Summary

Correct the process-manager exit order so a long-running agent does not produce
a false stderr-drain timeout warning. Keep the bounded post-exit drain and all
existing sanitized exit diagnostics.

## In scope

- Replace the manager's `Cmd.StderrPipe` with an explicitly owned stderr pipe.
- Change `Manager.waitForExit` to wait for process exit before stderr drain.
- Preserve generation-specific completion channels and the bounded timeout.
- Add regression coverage for running, successful, intentional-stop, and
  non-zero-exit paths.

## Out of scope

- Changing the drain timeout value.
- Changing stderr sanitization, redaction, or ring-buffer limits.
- Changing process-group cleanup or ACP adapter behavior.

## Acceptance

- No stderr-drain timeout is emitted while a managed process is still running.
- Exit publication waits for the generation-specific stderr reader after
  `cmd.Wait`, but remains bounded when the reader is held open.
- Exit code, error event, safe recent stderr, intentional-stop classification,
  and process-group cleanup remain unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/process -run 'TestWaitForExit.*|TestReadStderr.*Generation' -count=1
cd apps/backend && go test ./internal/agentctl/server/process -count=1
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_stderr_exit_test.go`

## Dependencies

Task 01 is a prior plan wave. The code paths are independent, but the plan
keeps lifecycle changes sequential for review.

## Risks

- The manager must close its parent stderr writer after start and close its
  reader if the bounded post-exit drain expires.

## Parallelism

`sequential`

## Inputs

- `docs/specs/platform/requirements/agent-process-exit-drain.md`.
- `docs/specs/platform/system-design/agent-process-exit-drain.md`.
- Existing generation-channel and sanitized-stderr tests.

## Results

Changed the manager to use an explicitly owned stderr pipe, then wait for the
child process before waiting on the generation-specific stderr reader. Added
regression coverage proving a live process does not emit the drain timeout
warning, exit diagnostics preserve stderr after `Cmd.Wait`, and the reader
drain remains bounded.

Verification:

- `go test ./internal/agentctl/server/process -run 'TestWaitForExit.*|TestReadStderr.*Generation' -count=1` passed.
- `go test ./internal/agentctl/server/process -count=1` passed with 707 tests.
- Race-enabled stderr exit tests passed, including owned-pipe preservation.
