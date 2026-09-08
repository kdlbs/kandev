---
status: draft
system: platform
created: 2026-08-26
owners:
  - kandev
---
# Agent process exit and stderr drain Requirements

## Overview

The agent process manager reads child stderr while it supervises process exit.
The current order waits for stderr EOF before waiting for the child process. A
long-running agent therefore produces a false stderr-drain warning after one
second, even though the agent is still starting normally. This requirement keeps
exit diagnostics complete without making process shutdown unbounded.

## Terminology

- **stderr drain:** Completion of the generation-specific reader that consumes a
  managed agent process's stderr pipe.
- **exit publication:** The process-manager state and update event that expose a
  managed agent's final exit result.

## Requirements

### REQ-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001: Correct agent process exit drain sequencing

**Intent:** Make the stderr-drain warning describe a real post-exit cleanup delay,
while preserving safe recent-stderr diagnostics and a bounded shutdown path.

#### Acceptance criteria

- **AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.1:** When a managed agent remains running longer than the stderr-drain timeout, the process manager shall not emit a stderr-drain timeout warning solely because the process is still running.
- **AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.2:** When a managed agent exits, the process manager shall wait for its generation-specific stderr reader, up to the existing bounded timeout, before publishing the final exit result.
- **AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.3:** When the stderr reader remains blocked after the managed process exits, the process manager shall emit a warning and continue exit handling within the existing bounded timeout.
- **AC-PLATFORM-AGENT-PROCESS-EXIT-DRAIN-001.4:** The exit code, error classification, safe recent-stderr content, process-group cleanup, and intentional-stop behavior shall remain unchanged.

## Out of scope

- Changing the stderr timeout duration.
- Changing stderr sanitization or the recent-stderr buffer contents.
- Changing ACP capability filtering, agent authentication, shell grace periods,
  or process-group termination policy.
